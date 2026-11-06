package tvcompat

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultTVHeight    = 1080
	tvWidth            = 1920
	tvLevel            = 41
	h264ProfileHigh    = "High"
	tvLevel40          = 40
	h264Codec          = "h264"
	levelScale         = 10
	tvSampleRate       = "48000"
	durationTolerance  = 2.0
	maxTVFrameRate     = 30.0
	originalBackupMode = 0o700
)

type mediaStream struct {
	CodecType         string `json:"codec_type"`
	CodecName         string `json:"codec_name"`
	Profile           string `json:"profile"`
	Level             int    `json:"level"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	PixFmt            string `json:"pix_fmt"`
	FieldOrder        string `json:"field_order"`
	SampleAspectRatio string `json:"sample_aspect_ratio"`
	RFrameRate        string `json:"r_frame_rate"`
	AvgFrameRate      string `json:"avg_frame_rate"`
	SampleRate        string `json:"sample_rate"`
	Channels          int    `json:"channels"`
}

type mediaInfo struct {
	Streams []mediaStream `json:"streams"`
	Format  struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
	} `json:"format"`
}

func (m *mediaInfo) videoAudio() (video, audio mediaStream) {
	for i := range m.Streams {
		stream := m.Streams[i]
		if stream.CodecType == "video" && video.CodecName == "" {
			video = stream
		}
		if stream.CodecType == "audio" && audio.CodecName == "" {
			audio = stream
		}
	}
	return video, audio
}

func probeMedia(ctx context.Context, path string) (*mediaInfo, error) {
	// #nosec G204 -- executable and options are fixed; path is an individual argument.
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_streams", "-show_format", "-of", "json", path).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}
	var info mediaInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("decode ffprobe: %w", err)
	}
	return &info, nil
}

func heightLimit(requested int) int {
	if requested > 0 && requested < defaultTVHeight {
		return requested - requested%2
	}
	return defaultTVHeight
}

func frameRate(value string) float64 {
	parts := strings.SplitN(value, "/", 2)
	n, _ := strconv.ParseFloat(parts[0], 64)
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	if len(parts) == 1 {
		return n
	}
	d, _ := strconv.ParseFloat(parts[1], 64)
	if d <= 0 || math.IsNaN(d) || math.IsInf(d, 0) {
		return 0
	}
	return n / d
}

// Supported video is kept intact; Main is the encoder fallback, not a reason
// to transcode an already supported High or Baseline stream.
func validateProfile(info *mediaInfo, height, targetLevel int) error {
	if !strings.Contains(info.Format.FormatName, "mp4") {
		return fmt.Errorf("TV output is not MP4")
	}
	return validateStreams(info, height, targetLevel)
}

func validateStreams(info *mediaInfo, height, targetLevel int) error {
	if len(info.Streams) != 2 {
		return fmt.Errorf("TV output must contain exactly one video and audio stream")
	}
	video, audio := info.videoAudio()
	if err := validateVideo(&video, height, targetLevel); err != nil {
		return err
	}
	if !supportedAudio(&audio) {
		return fmt.Errorf("TV audio must be AAC-LC stereo at 44100 or 48000 Hz")
	}
	return nil
}

func supportedAudio(audio *mediaStream) bool {
	return audio.CodecName == "aac" && audio.Profile == "LC" && audio.Channels == 2 &&
		(audio.SampleRate == "44100" || audio.SampleRate == tvSampleRate)
}

func validateVideo(video *mediaStream, height, targetLevel int) error {
	if video.CodecName != h264Codec || !supportedH264Profile(video.Profile) || video.Level <= 0 || video.Level > targetLevel ||
		video.PixFmt != "yuv420p" || video.FieldOrder != "progressive" || sampleAspectRatioMismatch(video.SampleAspectRatio) {
		return fmt.Errorf("TV video is outside the supported H.264 profile")
	}
	return validateVideoGeometry(video, height)
}

func supportedH264Profile(profile string) bool {
	return profile == "Main" || profile == h264ProfileHigh || profile == "Baseline" || profile == "Constrained Baseline"
}

func validateVideoGeometry(video *mediaStream, height int) error {
	if video.Width <= 0 || video.Width > tvWidth || video.Height <= 0 || video.Height > height ||
		video.Width%2 != 0 || video.Height%2 != 0 {
		return fmt.Errorf("TV video dimensions exceed the configured even-sized 1080p profile")
	}
	fps := frameRate(video.AvgFrameRate)
	// Unknown average FPS (including MPEG-TS 0/0) is resolved by mandatory PTS sampling.
	if fps > maxTVFrameRate {
		return fmt.Errorf("TV video average frame rate exceeds 30")
	}
	return nil
}

type preparationPlan struct{ videoEncode, audioEncode, remux bool }

func (plan preparationPlan) required() bool {
	return plan.videoEncode || plan.audioEncode || plan.remux
}

func planPreparation(info *mediaInfo, fastStart bool, height, targetLevel int) (preparationPlan, error) {
	video, audio := info.videoAudio()
	if !knownVideoMetadata(&video) {
		return preparationPlan{}, inconclusiveTiming()
	}
	if video.CodecName == h264Codec && (video.Profile == "" || video.Level <= 0) {
		return preparationPlan{}, inconclusiveTiming()
	}
	if audio.CodecName == "aac" && (audio.Profile == "" || audio.Channels <= 0 || audio.SampleRate == "") {
		return preparationPlan{}, inconclusiveTiming()
	}
	if audio.CodecName == "" {
		return preparationPlan{}, fmt.Errorf("video has no audio stream; original preserved")
	}
	return preparationPlan{
		videoEncode: validateVideo(&video, height, targetLevel) != nil,
		audioEncode: !supportedAudio(&audio),
		remux:       !fastStart || len(info.Streams) != 2,
	}, nil
}

func knownVideoMetadata(video *mediaStream) bool {
	// Missing SAR and average FPS are not evidence of a stream mismatch. Every
	// copy/no-op path still verifies bounded packet timing before accepting media.
	return video.CodecName != "" && video.Width > 0 && video.Height > 0 && video.PixFmt != "" &&
		video.FieldOrder != "" && video.FieldOrder != "unknown"
}

func inconclusiveTiming() error {
	return fmt.Errorf("TV verification inconclusive; original preserved")
}

func conversionArgs(input, output string, height, targetLevel int, plan preparationPlan) []string {
	return append(streamPreparationArgs(input, height, targetLevel, plan), "-movflags", "+faststart", "-f", "mp4", output)
}

func streamPreparationArgs(input string, height, targetLevel int, plan preparationPlan) []string {
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-filter_threads", "1", "-i", input,
		"-map", "0:v:0", "-map", "0:a:0", "-sn", "-dn", "-map_metadata", "-1", "-map_chapters", "-1"}
	if plan.videoEncode {
		args = append(args, videoEncodingArgs(height, targetLevel)...)
	} else {
		args = append(args, "-c:v", "copy")
	}
	if plan.audioEncode {
		args = append(args, "-c:a", "aac", "-profile:a", "aac_low", "-ac", "2", "-ar", tvSampleRate, "-b:a", "160k")
	} else {
		args = append(args, "-c:a", "copy")
	}
	return args
}
func videoEncodingArgs(height, targetLevel int) []string {
	filter := fmt.Sprintf("yadif=deint=interlaced,scale=w='min(%d,iw)':h='min(%d,ih)':"+
		"force_original_aspect_ratio=decrease:force_divisible_by=2,setsar=1", tvWidth, height)
	return []string{
		"-vf", filter, "-c:v", "libx264", "-preset", "veryfast", "-crf", "22", "-threads", "2",
		"-profile:v", "main", "-level:v", fmt.Sprintf("%d.%d", targetLevel/levelScale, targetLevel%levelScale),
		"-pix_fmt", "yuv420p", "-refs", "3", "-bf", "2", "-g", "60",
		"-x264-params", "open-gop=0", "-maxrate", "4M", "-bufsize", "8M", "-r", "30000/1001", "-fps_mode", "cfr",
	}
}

// prepareFile chooses stream copy or individual stream encoding from measured
// properties. Packet samples are a bounded confidence check, not a full-file proof.
func prepareFile(ctx context.Context, path string, requestedHeight, targetLevel int) error {
	if targetLevel != tvLevel40 && targetLevel != tvLevel {
		return fmt.Errorf("TV H.264 output level must be 4.0 or 4.1; original preserved")
	}
	height := heightLimit(requestedHeight)
	if height < 2 {
		return fmt.Errorf("TV height must be at least 2 pixels")
	}
	original, sourceDuration, err := inspectSource(ctx, path)
	if err != nil {
		return err
	}
	info, err := probeMedia(ctx, path)
	if err != nil {
		return err
	}
	plan, err := planPreparation(info, hasFastStart(path), height, targetLevel)
	if err != nil {
		return err
	}
	if !plan.videoEncode {
		timing, timingErr := samplePacketTiming(ctx, path, sourceDuration)
		if timingErr != nil {
			return timingErr
		}
		plan.videoEncode = timing == timingAboveLimit
	}
	if !plan.required() {
		if sourceErr := checkSourceUnchanged(path, original); sourceErr != nil {
			return sourceErr
		}
		return ctx.Err()
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".tvcompat-*.mp4")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if closeErr := temp.Close(); closeErr != nil {
		return closeErr
	}
	// #nosec G204 -- executable/options are fixed, and file paths are individual arguments.
	cmd := exec.CommandContext(ctx, "ffmpeg", conversionArgs(path, tempPath, height, targetLevel, plan)...)
	// Avoid retaining unbounded ffmpeg output or exposing source URLs in diagnostics.
	if encodeErr := cmd.Run(); encodeErr != nil {
		return fmt.Errorf("ffmpeg TV encode: %w", encodeErr)
	}
	if validateErr := validateOutput(ctx, tempPath, height, targetLevel, sourceDuration); validateErr != nil {
		return validateErr
	}
	return publishPrepared(ctx, path, tempPath, original)
}

func inspectSource(ctx context.Context, path string) (fileInfo os.FileInfo, duration float64, err error) {
	if strings.ToLower(filepath.Ext(path)) != ".mp4" {
		return nil, 0, fmt.Errorf("automatic TV preparation currently supports .mp4 only; source preserved")
	}
	original, err := os.Lstat(path)
	if err != nil {
		return nil, 0, err
	}
	if !original.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("TV input must be a regular file")
	}
	before, err := probeMedia(ctx, path)
	if err != nil {
		return nil, 0, err
	}
	duration, err = strconv.ParseFloat(before.Format.Duration, 64)
	if err != nil || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return nil, 0, fmt.Errorf("source duration could not be established")
	}
	return original, duration, nil
}

func validateOutput(ctx context.Context, path string, height, targetLevel int, sourceDuration float64) error {
	after, err := probeMedia(ctx, path)
	if err != nil {
		return err
	}
	if profileErr := validateProfile(after, height, targetLevel); profileErr != nil {
		return profileErr
	}
	duration, err := strconv.ParseFloat(after.Format.Duration, 64)
	if err != nil || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) ||
		math.Abs(duration-sourceDuration) > durationTolerance {
		return fmt.Errorf("TV output duration does not match the original")
	}
	if !hasFastStart(path) {
		return fmt.Errorf("TV output lacks MP4 faststart")
	}
	timing, timingErr := samplePacketTiming(ctx, path, duration)
	if timingErr != nil {
		return timingErr
	}
	if timing != timingSupported {
		return fmt.Errorf("TV output frame pacing exceeds 30fps")
	}
	return ctx.Err()
}

func publishPrepared(ctx context.Context, path, tempPath string, original os.FileInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if sourceErr := checkSourceUnchanged(path, original); sourceErr != nil {
		return sourceErr
	}
	if chmodErr := os.Chmod(tempPath, original.Mode().Perm()); chmodErr != nil {
		return chmodErr
	}
	if syncErr := syncPath(tempPath); syncErr != nil {
		return syncErr
	}
	if backupErr := retainOriginal(path); backupErr != nil {
		return fmt.Errorf("retain original: %w", backupErr)
	}
	if cancelErr := ctx.Err(); cancelErr != nil {
		return cancelErr
	}
	if renameErr := os.Rename(tempPath, path); renameErr != nil {
		return renameErr
	}
	return syncPath(filepath.Dir(path))
}

func checkSourceUnchanged(path string, original os.FileInfo) error {
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(original, current) || original.Size() != current.Size() || !original.ModTime().Equal(current.ModTime()) {
		return fmt.Errorf("TV source changed during preparation; original not replaced")
	}
	return nil
}

func syncPath(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func retainOriginal(path string) error {
	if err := syncPath(path); err != nil {
		return err
	}
	root := filepath.Join(filepath.Dir(path), ".tvcompat-originals")
	if err := os.MkdirAll(root, originalBackupMode); err != nil {
		return err
	}
	// Each attempt gets a unique directory. No previous original is overwritten.
	dir, err := os.MkdirTemp(root, "original-")
	if err != nil {
		return err
	}
	if err := os.Link(path, filepath.Join(dir, filepath.Base(path))); err != nil {
		return err
	}
	if err := syncPath(dir); err != nil {
		return err
	}
	if err := syncPath(root); err != nil {
		return err
	}
	return syncPath(filepath.Dir(root))
}

// hasFastStart inspects only top-level MP4 atom headers, never the media payload.
func hasFastStart(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return false
	}
	const headerSize = 8
	for offset := int64(0); offset <= stat.Size()-headerSize; {
		var header [headerSize]byte
		if _, err := file.ReadAt(header[:], offset); err != nil {
			return false
		}
		size := int64(binary.BigEndian.Uint32(header[:4]))
		atom := string(header[4:])
		if atom == "mdat" {
			return false
		}
		if atom == "moov" {
			return true
		}
		if size == 1 {
			var extended [headerSize]byte
			if _, err := file.ReadAt(extended[:], offset+headerSize); err != nil {
				return false
			}
			value := binary.BigEndian.Uint64(extended[:])
			if value > math.MaxInt64 {
				return false
			}
			if int64(value) > stat.Size()-offset {
				return false
			}
			size = int64(value) // bounded by MaxInt64 and the remaining file size above
		}
		if size < headerSize || size > stat.Size()-offset {
			return false
		}
		offset += size
	}
	return false
}

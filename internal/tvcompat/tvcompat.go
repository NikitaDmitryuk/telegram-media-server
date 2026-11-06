package tvcompat

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/database"
)

// Video extensions that may be processed for TV compatibility.
var videoExtensions = map[string]struct{}{
	".ts": {}, ".mkv": {}, ".mp4": {}, ".avi": {}, ".mov": {}, ".webm": {}, ".m4v": {},
}

// Extensions that almost always contain a non-H.264 codec (VP9 / AV1).
var likelyNonH264Extensions = map[string]struct{}{
	".webm": {},
}

// IsVideoFilePath returns true if the file path has a known video extension.
func IsVideoFilePath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := videoExtensions[ext]
	return ok
}

// CompatFromTorrentFileNames returns a preliminary TV compatibility from torrent file paths.
// Since torrent metadata does not contain codec information, we can only use file extensions:
//   - .webm → red (almost always VP9/AV1);
//   - other video extensions → yellow (unknown, will be resolved by ffprobe later);
//   - no video files → "".
func CompatFromTorrentFileNames(filePaths []string) string {
	for _, p := range filePaths {
		ext := strings.ToLower(filepath.Ext(p))
		if _, ok := videoExtensions[ext]; !ok {
			continue
		}
		if _, bad := likelyNonH264Extensions[ext]; bad {
			return TvCompatRed // .webm is almost always VP9/AV1
		}
		return TvCompatYellow // has video files but unknown codec
	}
	return ""
}

// CompatFromVcodec returns a preliminary TV compatibility from a codec string
// (e.g. from yt-dlp JSON: "avc1.64001f", "vp9", "hevc").
// Returns yellow for H.264/AVC, red for VP9/AV1/HEVC, "" if unknown or empty.
func CompatFromVcodec(vcodec string) string {
	v := strings.ToLower(strings.TrimSpace(vcodec))
	if v == "" || v == "none" {
		return ""
	}
	// Non-H.264 codecs → red (needs full re-encoding for old TVs).
	if strings.Contains(v, "vp9") || strings.Contains(v, "av01") || strings.Contains(v, "av1") {
		return TvCompatRed
	}
	if strings.Contains(v, "h265") || strings.Contains(v, "hevc") {
		return TvCompatRed
	}
	// H.264 metadata alone cannot establish the complete TV profile.
	if strings.Contains(v, "h264") || strings.Contains(v, "avc") {
		return TvCompatYellow
	}
	return ""
}

// TvCompatGreen means the complete TV output profile was verified.
// TvCompatYellow means unknown or requiring preparation.
// TvCompatRed means confirmed incompatible codec (HEVC, VP9, AV1, etc.) — full re-encoding needed.
const (
	TvCompatGreen  = "green"
	TvCompatYellow = "yellow"
	TvCompatRed    = "red"
)

// ProbeTvCompatibility returns green only for the prepared TV profile. H.264
// needing preparation is yellow, so RejectIncompatible does not reject it.
func ProbeTvCompatibility(ctx context.Context, movieID uint, moviePath string, db tmsdb.Database, requestedLevel int) string {
	if requestedLevel <= 0 || requestedLevel > tvLevel {
		requestedLevel = tvLevel
	}
	files, err := db.GetFilesByMovieID(ctx, movieID)
	if err != nil {
		return ""
	}
	result := ""
	for i := range files {
		if !IsVideoFilePath(files[i].FilePath) {
			continue
		}
		path := filepath.Join(moviePath, files[i].FilePath)
		compat := probeFileCompatibility(ctx, path, requestedLevel)
		if compat != TvCompatGreen {
			return compat
		}
		result = TvCompatGreen
	}
	return result
}

func probeFileCompatibility(ctx context.Context, path string, requestedLevel int) string {
	media, err := probeMedia(ctx, path)
	if err != nil {
		return ""
	}
	video, _ := media.videoAudio()
	if video.CodecName == "" {
		return ""
	}
	if video.CodecName != h264Codec {
		return TvCompatRed
	}
	if strings.EqualFold(filepath.Ext(path), ".ts") {
		if validateExistingTransport(ctx, path, defaultTVHeight, requestedLevel) == nil {
			return TvCompatGreen
		}
		return TvCompatYellow
	}
	transport, transportErr := needsTransportStream(path)
	if transportErr != nil || transport {
		return TvCompatYellow
	}
	if !strings.EqualFold(filepath.Ext(path), ".mp4") ||
		validateProfile(media, defaultTVHeight, requestedLevel) != nil || !hasFastStart(path) {
		return TvCompatYellow
	}
	duration, err := strconv.ParseFloat(media.Format.Duration, 64)
	if err != nil || duration <= 0 {
		return TvCompatYellow
	}
	timing, err := samplePacketTiming(ctx, path, duration)
	if err != nil || timing != timingSupported {
		return TvCompatYellow
	}
	return TvCompatGreen
}

// RunTvCompatibility prepares every video and returns errors to the worker.
// Original MP4s are retained in a hidden sibling backup directory.
func RunTvCompatibility(ctx context.Context, movieID uint, moviePath string, db tmsdb.Database,
	videoConfig *tmsconfig.VideoConfig) error {
	if videoConfig == nil || !videoConfig.CompatibilityMode {
		return nil
	}
	level, err := configuredTVLevel(videoConfig.TvH264Level)
	if err != nil {
		return err
	}
	files, err := db.GetFilesByMovieID(ctx, movieID)
	if err != nil {
		return fmt.Errorf("list TV input files: %w", err)
	}
	count := 0
	for i := range files {
		rel := files[i].FilePath
		if !IsVideoFilePath(rel) {
			continue
		}
		if filepath.IsAbs(rel) || !filepath.IsLocal(rel) {
			return fmt.Errorf("invalid TV input path")
		}
		if err := prepareMovieFile(ctx, movieID, &files[i], moviePath, db, videoConfig.MaxHeight, level); err != nil {
			return fmt.Errorf("prepare TV video: %w", err)
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("no video files available for TV preparation")
	}
	return nil
}

// ParseH264Level converts "4.0" -> 40, "4.1" -> 41. Returns 0 on parse error.
func ParseH264Level(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	parts := strings.SplitN(s, ".", 2)
	maj, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || maj < 0 || maj > 9 {
		return 0
	}
	minor := 0
	if len(parts) == 2 {
		minor, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
		if minor < 0 || minor > 9 {
			minor = 0
		}
	}
	return maj*10 + minor
}

func configuredTVLevel(setting string) (int, error) {
	switch strings.TrimSpace(setting) {
	case "", "4.1":
		return tvLevel, nil
	case "4", "4.0":
		return tvLevel40, nil
	default:
		return 0, fmt.Errorf("VIDEO_TV_H264_LEVEL supports only 4.0 or 4.1 for the 1080p TV profile; original preserved")
	}
}

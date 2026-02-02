package tvcompat

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

// Video extensions that may be processed for TV compatibility.
var videoExtensions = map[string]struct{}{
	".mkv": {}, ".mp4": {}, ".avi": {}, ".mov": {}, ".webm": {}, ".m4v": {},
}

// Extensions that typically indicate non-H.264 (VP9, AV1) — show red until probe.
var likelyNonH264Extensions = map[string]struct{}{
	".webm": {},
}

// Extensions that are commonly H.264 in practice — show green optimistically until probe.
var likelyH264Extensions = map[string]struct{}{
	".mkv": {}, ".mp4": {}, ".mov": {}, ".m4v": {},
}

// IsVideoFilePath returns true if the file path has a known video extension.
func IsVideoFilePath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := videoExtensions[ext]
	return ok
}

// CompatFromTorrentFileNames returns a preliminary TV compatibility from the first video file name in the list.
// Used to show the circle immediately when a torrent is added (from torrent meta). Returns "" if no video file found.
// For .mkv/.mp4/.mov/.m4v returns green (optimistic); .avi returns yellow; .webm returns red.
func CompatFromTorrentFileNames(filePaths []string) string {
	for _, p := range filePaths {
		ext := strings.ToLower(filepath.Ext(p))
		if _, ok := videoExtensions[ext]; !ok {
			continue
		}
		if _, bad := likelyNonH264Extensions[ext]; bad {
			return TvCompatRed
		}
		if _, good := likelyH264Extensions[ext]; good {
			return TvCompatGreen
		}
		// .avi — likely H.264 but level unknown
		return TvCompatYellow
	}
	return ""
}

// CompatFromVcodec returns a preliminary TV compatibility from a codec string (e.g. from yt-dlp JSON: "avc1.64001f", "vp9").
// Returns "" if unknown or empty.
func CompatFromVcodec(vcodec string) string {
	v := strings.ToLower(strings.TrimSpace(vcodec))
	if v == "" || v == "none" {
		return ""
	}
	if strings.Contains(v, "vp9") || strings.Contains(v, "av01") || strings.Contains(v, "av1") {
		return TvCompatRed
	}
	if strings.Contains(v, "h264") || strings.Contains(v, "avc") {
		return TvCompatYellow
	}
	return ""
}

// TvCompatGreen means video will play on old TV without conversion.
// TvCompatYellow means light conversion (remux) may be needed.
// TvCompatRed means heavy re-encoding would be needed (not implemented).
const (
	TvCompatGreen  = "green"
	TvCompatYellow = "yellow"
	TvCompatRed    = "red"
)

// ProbeTvCompatibility probes the first video file of the movie and returns
// green (H.264 level <= target), yellow (H.264 level > target or unknown), or red (not H.264 / probe failed).
func ProbeTvCompatibility(
	ctx context.Context,
	movieID uint,
	moviePath string,
	db tmsdb.Database,
	targetLevel int,
) string {
	if targetLevel <= 0 {
		targetLevel = 41
	}
	files, err := db.GetFilesByMovieID(ctx, movieID)
	if err != nil {
		logutils.Log.WithError(err).WithField("movie_id", movieID).Debug("TV compatibility probe: failed to get files")
		return TvCompatRed
	}
	for i := range files {
		rel := files[i].FilePath
		ext := strings.ToLower(filepath.Ext(rel))
		if _, ok := videoExtensions[ext]; !ok {
			continue
		}
		absPath := filepath.Join(moviePath, rel)
		if _, err := os.Stat(absPath); err != nil {
			continue
		}
		codec, level := probeCodecAndLevel(ctx, absPath)
		if codec == "" {
			return TvCompatRed
		}
		if codec != "h264" {
			return TvCompatRed
		}
		if level <= 0 {
			return TvCompatYellow // unknown level, light remux may help
		}
		if level <= targetLevel {
			return TvCompatGreen
		}
		return TvCompatYellow
	}
	return TvCompatRed
}

// probeCodecAndLevel returns codec name and level (e.g. 41 for 4.1). level is 0 if unknown or error.
func probeCodecAndLevel(ctx context.Context, absPath string) (codec string, level int) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name,level",
		"-of", "csv=p=0",
		absPath,
	)
	out, err := cmd.Output()
	if err != nil {
		return "", 0
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return "", 0
	}
	parts := strings.SplitN(line, ",", 2)
	codec = strings.TrimSpace(strings.ToLower(parts[0]))
	if len(parts) == 2 {
		levelStr := strings.TrimSpace(parts[1])
		if levelStr != "" {
			level, _ = strconv.Atoi(levelStr)
		}
	}
	return codec, level
}

// RunTvCompatibility runs a light remux on main video files for the given movie:
// copies video/audio streams and sets H.264 level metadata (e.g. 4.1 for old LG Smart TVs)
// without re-encoding. Only H.264 streams with level above the target are processed.
// Call this from the conversion worker when CompatibilityMode is on.
func RunTvCompatibility(
	ctx context.Context,
	movieID uint,
	moviePath string,
	db tmsdb.Database,
	videoConfig *tmsconfig.VideoConfig,
) {
	if videoConfig == nil || !videoConfig.CompatibilityMode {
		return
	}

	files, err := db.GetFilesByMovieID(ctx, movieID)
	if err != nil {
		logutils.Log.WithError(err).WithField("movie_id", movieID).Error("TV compatibility: failed to get files")
		return
	}

	targetLevel := ParseH264Level(videoConfig.TvH264Level)
	if targetLevel <= 0 {
		logutils.Log.WithField("level", videoConfig.TvH264Level).Warn("TV compatibility: invalid H.264 level, using 41")
		targetLevel = 41
	}

	for i := range files {
		rel := files[i].FilePath
		ext := strings.ToLower(filepath.Ext(rel))
		if _, ok := videoExtensions[ext]; !ok {
			continue
		}
		absPath := filepath.Join(moviePath, rel)
		if _, err := os.Stat(absPath); err != nil {
			logutils.Log.WithError(err).WithField("path", absPath).Debug("TV compatibility: skip missing file")
			continue
		}
		needs, err := needsTvRemux(ctx, absPath, targetLevel)
		if err != nil {
			logutils.Log.WithError(err).WithField("path", absPath).Debug("TV compatibility: skip (probe failed)")
			continue
		}
		if !needs {
			continue
		}
		if err := remuxForTv(ctx, absPath, targetLevel); err != nil {
			logutils.Log.WithError(err).WithField("path", absPath).Warn("TV compatibility: remux failed")
		} else {
			logutils.Log.WithField("path", absPath).Info("TV compatibility: remux done")
		}
	}
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

// needsTvRemux runs ffprobe and returns true if the file is H.264 and level is above target or missing.
func needsTvRemux(ctx context.Context, absPath string, targetLevel int) (bool, error) {
	// ffprobe -v error -select_streams v:0 -show_entries stream=codec_name,level -of csv=p=0
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name,level",
		"-of", "csv=p=0",
		absPath,
	)
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return false, nil
	}
	parts := strings.SplitN(line, ",", 2)
	codec := strings.TrimSpace(strings.ToLower(parts[0]))
	if codec != "h264" {
		return false, nil
	}
	levelStr := ""
	if len(parts) == 2 {
		levelStr = strings.TrimSpace(parts[1])
	}
	if levelStr == "" {
		return true, nil // unknown level, apply cap
	}
	level, err := strconv.Atoi(levelStr)
	if err != nil {
		return true, nil
	}
	return level > targetLevel, nil
}

// remuxForTv runs ffmpeg with -c:v copy -bsf:v h264_metadata=level=X.Y -c:a copy, then replaces original.
func remuxForTv(ctx context.Context, absPath string, targetLevel int) error {
	levelStr := formatH264Level(targetLevel)
	dir := filepath.Dir(absPath)
	tmpPath := filepath.Join(dir, ".tvcompat_"+filepath.Base(absPath)+".tmp")
	defer os.Remove(tmpPath) // best-effort cleanup

	// ffmpeg -i input -c:v copy -bsf:v h264_metadata=level=4.1 -c:a copy -y output
	// #nosec G204 -- absPath/tmpPath from filepath, levelStr from formatH264Level(level)
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", absPath,
		"-c:v", "copy",
		"-bsf:v", "h264_metadata=level="+levelStr,
		"-c:a", "copy",
		"-y",
		tmpPath,
	)
	if err := cmd.Run(); err != nil {
		return err
	}
	return os.Rename(tmpPath, absPath)
}

func formatH264Level(level int) string {
	maj := level / 10
	minor := level % 10
	if minor == 0 {
		return strconv.Itoa(maj)
	}
	return strconv.Itoa(maj) + "." + strconv.Itoa(minor)
}

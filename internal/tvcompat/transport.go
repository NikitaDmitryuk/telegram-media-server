package tvcompat

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
)

type preparedPathStore interface {
	ReplaceMainMovieFilePath(ctx context.Context, movieID, fileID uint, expectedPath, preparedPath string) error
}

func prepareMovieFile(ctx context.Context, movieID uint, file *database.MovieFile, root string,
	db database.Database, height, level int) error {
	path := filepath.Join(root, file.FilePath)
	if strings.EqualFold(filepath.Ext(path), ".ts") {
		return validateExistingTransport(ctx, path, heightLimit(height), level)
	}
	if strings.EqualFold(filepath.Ext(path), ".mp4") {
		transport, err := needsTransportStream(path)
		if err != nil {
			return fmt.Errorf("inspect MP4 transport workaround: %w", err)
		}
		if transport {
			return prepareMP4AsTransport(ctx, movieID, file, root, db, height, level)
		}
	}
	return prepareFile(ctx, path, height, level)
}

// prepareMP4AsTransport selects MPEG-TS, copying compatible streams and encoding
// only streams needing repair. Publication precedes DB CAS, so referenced paths exist.
// Errors after publication intentionally retain both files, including ambiguous
// DB errors. A crash can leave an extra file; it must never lose the original.
func prepareMP4AsTransport(ctx context.Context, movieID uint, file *database.MovieFile, root string,
	db database.Database, requestedHeight, level int) error {
	store, ok := db.(preparedPathStore)
	if !ok {
		return fmt.Errorf("database cannot safely register a prepared TS path; original preserved")
	}
	source := filepath.Join(root, file.FilePath)
	original, duration, err := inspectSource(ctx, source)
	if err != nil {
		return err
	}
	height := heightLimit(requestedHeight)
	plan, planErr := planTransportInput(ctx, source, height, level, duration)
	if planErr != nil {
		return planErr
	}
	temp, err := os.CreateTemp(filepath.Dir(source), ".tvcompat-*.ts")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Close(); err != nil {
		return err
	}
	// #nosec G204 -- fixed executable and options; paths are individual arguments.
	command := exec.CommandContext(ctx, "ffmpeg", transportArgs(source, tempPath, height, level, plan)...)
	if err := command.Run(); err != nil {
		return fmt.Errorf("TV MPEG-TS preparation: %w", err)
	}
	if err := validateTransportOutput(ctx, tempPath, height, level, duration); err != nil {
		return err
	}
	return publishTransport(ctx, source, tempPath, root, movieID, file, store, original)
}

func transportArgs(source, output string, height, level int, plan preparationPlan) []string {
	return append(streamPreparationArgs(source, height, level, plan), "-mpegts_flags", "+resend_headers", "-f", "mpegts", output)
}

func planTransportInput(ctx context.Context, path string, height, level int, duration float64) (preparationPlan, error) {
	info, err := probeMedia(ctx, path)
	if err != nil {
		return preparationPlan{}, err
	}
	plan, err := planPreparation(info, false, height, level)
	if err != nil {
		return preparationPlan{}, err
	}
	if !plan.videoEncode {
		timing, timingErr := samplePacketTiming(ctx, path, duration)
		if timingErr != nil {
			return preparationPlan{}, timingErr
		}
		plan.videoEncode = timing == timingAboveLimit
	}
	return plan, nil
}

func validateTransportOutput(ctx context.Context, path string, height, level int, sourceDuration float64) error {
	info, err := probeMedia(ctx, path)
	if err != nil {
		return err
	}
	if info.Format.FormatName != "mpegts" {
		return fmt.Errorf("TV transport output is not MPEG-TS")
	}
	if profileErr := validateStreams(info, height, level); profileErr != nil {
		return profileErr
	}
	duration, err := strconv.ParseFloat(info.Format.Duration, 64)
	if err != nil || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) ||
		math.Abs(duration-sourceDuration) > durationTolerance {
		return fmt.Errorf("TV transport duration does not match original")
	}
	return validateSampledTiming(ctx, path, duration)
}

func validateExistingTransport(ctx context.Context, path string, height, level int) error {
	info, err := probeMedia(ctx, path)
	if err != nil {
		return err
	}
	duration, err := strconv.ParseFloat(info.Format.Duration, 64)
	if err != nil || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return inconclusiveTiming()
	}
	return validateTransportOutput(ctx, path, height, level, duration)
}

func validateSampledTiming(ctx context.Context, path string, duration float64) error {
	timing, err := samplePacketTiming(ctx, path, duration)
	if err != nil {
		return err
	}
	if timing != timingSupported {
		return fmt.Errorf("TV sampled packet pacing exceeds 30fps; original preserved")
	}
	return ctx.Err()
}

func publishTransport(ctx context.Context, source, tempPath, root string, movieID uint, file *database.MovieFile,
	store preparedPathStore, original os.FileInfo) error {
	if err := checkSourceUnchanged(source, original); err != nil {
		return err
	}
	if err := os.Chmod(tempPath, original.Mode().Perm()); err != nil {
		return err
	}
	if err := syncPath(tempPath); err != nil {
		return err
	}
	if err := retainOriginal(source); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	prepared, err := linkTransportOutput(source, tempPath)
	if err != nil {
		return err
	}
	if syncErr := syncPath(filepath.Dir(source)); syncErr != nil {
		return syncErr
	}
	relative, err := filepath.Rel(root, prepared)
	if err != nil || !filepath.IsLocal(relative) {
		return fmt.Errorf("prepared TS path is outside the media root")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// On an ambiguous write failure retain both paths. Never remove an output that
	// might already be referenced by a committed DB update.
	if err := store.ReplaceMainMovieFilePath(ctx, movieID, file.ID, file.FilePath, relative); err != nil {
		return fmt.Errorf("register prepared TS path; both files retained: %w", err)
	}
	if err := checkSourceUnchanged(source, original); err != nil {
		return err
	}
	if err := os.Remove(source); err != nil {
		return err
	}
	return syncPath(filepath.Dir(source))
}

func linkTransportOutput(source, tempPath string) (string, error) {
	stem := strings.TrimSuffix(source, filepath.Ext(source))
	candidate := stem + ".ts"
	if err := os.Link(tempPath, candidate); err == nil {
		return candidate, nil
	} else if !os.IsExist(err) {
		return "", err
	}
	// The temporary random suffix avoids overwriting any existing .ts material.
	suffix := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(tempPath), ".tvcompat-"), ".ts")
	candidate = stem + "-tv-" + suffix + ".ts"
	if err := os.Link(tempPath, candidate); err != nil {
		return "", err
	}
	return candidate, nil
}

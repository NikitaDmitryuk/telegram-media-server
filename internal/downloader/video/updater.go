package ytdlp

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

const updateTimeout = 3 * time.Minute

// pipInstallMessage is the message yt-dlp prints when installed via pip; self-update is not supported.
const pipInstallMessage = "You installed yt-dlp with pip"

// RunUpdate runs yt-dlp -U to self-update. Does not block application startup on failure.
// When yt-dlp is installed via pip (e.g. in Docker), it refuses to self-update; we log that at DEBUG.
func RunUpdate(ctx context.Context) {
	updateCtx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	cmd := exec.CommandContext(updateCtx, "yt-dlp", "-U")
	output, err := cmd.CombinedOutput()
	out := string(output)
	if err != nil {
		if updateCtx.Err() != nil {
			logutils.Log.WithError(err).Warn("yt-dlp update timed out or was canceled")
			return
		}
		if strings.Contains(out, pipInstallMessage) || strings.Contains(out, "PyPi") {
			logutils.Log.Debug("yt-dlp installed via pip, self-update skipped (use pip install -U yt-dlp to update)")
			return
		}
		logutils.Log.WithError(err).WithField("output", out).Warn("yt-dlp update failed")
		return
	}
	logutils.Log.WithField("output", out).Info("yt-dlp updated successfully")
}

// StartPeriodicUpdater runs RunUpdate every interval until ctx is canceled.
// If interval is zero or negative, it returns immediately without starting.
func StartPeriodicUpdater(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logutils.Log.WithField("interval", interval).Info("Starting periodic yt-dlp updates")

	for {
		select {
		case <-ctx.Done():
			logutils.Log.Info("Stopping periodic yt-dlp updater")
			return
		case <-ticker.C:
			RunUpdate(ctx)
		}
	}
}

package ytdlp

import (
	"context"
	"os/exec"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

const updateTimeout = 3 * time.Minute

// RunUpdate runs yt-dlp -U to self-update. Does not block application startup on failure.
func RunUpdate(ctx context.Context) {
	updateCtx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	cmd := exec.CommandContext(updateCtx, "yt-dlp", "-U")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if updateCtx.Err() != nil {
			logutils.Log.WithError(err).Warn("yt-dlp update timed out or was canceled")
		} else {
			logutils.Log.WithError(err).WithField("output", string(output)).Warn("yt-dlp update failed")
		}
		return
	}
	logutils.Log.WithField("output", string(output)).Info("yt-dlp updated successfully")
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

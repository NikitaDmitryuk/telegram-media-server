package downloader

import (
	"context"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

func StartPeriodicUpdater(ctx context.Context, interval time.Duration, u Updater) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logutils.Log.WithField("interval", interval).Info("Starting periodic external tool updater")

	for {
		select {
		case <-ctx.Done():
			logutils.Log.Info("Stopping periodic external tool updater")
			return
		case <-ticker.C:
			u.RunUpdate(ctx)
		}
	}
}

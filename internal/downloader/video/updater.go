package ytdlp

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

const updateTimeout = 3 * time.Minute

func RunUpdate(ctx context.Context, binaryPath string) {
	if binaryPath == "" {
		binaryPath = defaultYtdlpBinary
	}
	updateCtx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	cmd := exec.CommandContext(updateCtx, binaryPath, "-U")
	output, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(output))

	if err != nil {
		if updateCtx.Err() != nil {
			logutils.Log.WithError(err).Warn("yt-dlp update timed out or was canceled")
			return
		}
		msg := "yt-dlp update failed: " + err.Error()
		if out != "" {
			msg += "; output: " + out
		}
		logutils.Log.WithError(err).WithFields(map[string]any{
			"output": string(output),
			"binary": binaryPath,
		}).Warn(msg)
		return
	}

	logutils.Log.WithFields(map[string]any{
		"binary": binaryPath,
		"output": out,
	}).Info("yt-dlp update check completed successfully")
}

type ytdlpUpdater struct{ binaryPath string }

func (u *ytdlpUpdater) RunUpdate(ctx context.Context) { RunUpdate(ctx, u.binaryPath) }

func NewUpdater(binaryPath string) downloader.Updater {
	if binaryPath == "" {
		binaryPath = defaultYtdlpBinary
	}
	return &ytdlpUpdater{binaryPath: binaryPath}
}

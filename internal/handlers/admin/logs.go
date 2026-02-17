package admin

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const logsCommandTimeout = 30 * time.Second

// RunCommand executes an external command and returns its output.
// Overridden in tests to avoid calling real system commands.
var RunCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// LogsHandler sends the last day's journalctl logs as a .txt file.
func LogsHandler(a *app.App, update *tgbotapi.Update) {
	chatID := update.Message.Chat.ID

	ctx, cancel := context.WithTimeout(context.Background(), logsCommandTimeout)
	defer cancel()

	out, err := RunCommand(ctx, "journalctl",
		"-u", "telegram-media-server",
		"--since", "1 day ago",
		"--no-pager",
	)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to execute journalctl")
		a.Bot.SendMessage(chatID,
			fmt.Sprintf("%s: %v", lang.Translate("error.commands.logs_error", nil), err), nil)
		return
	}

	if len(out) == 0 {
		a.Bot.SendMessage(chatID, lang.Translate("general.commands.logs_empty", nil), nil)
		return
	}

	if err := a.Bot.SendDocument(chatID, "logs.txt", out); err != nil {
		logutils.Log.WithError(err).Error("Failed to send logs document")
		a.Bot.SendMessage(chatID,
			fmt.Sprintf("%s: %v", lang.Translate("error.commands.logs_error", nil), err), nil)
	}
}

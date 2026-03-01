package admin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	logsCommandTimeout = 30 * time.Second
	maxLogErrorMsgLen  = 500 // cap journalctl stderr in user-facing message
)

// RunCommand executes an external command and returns combined stdout+stderr.
// Overridden in tests to avoid calling real system commands.
var RunCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// LogsHandler sends the last day's journalctl logs as a .txt file.
func LogsHandler(a *app.App, update *tgbotapi.Update) {
	chatID := update.Message.Chat.ID

	if os.Getenv("RUNNING_IN_DOCKER") == "true" {
		a.Bot.SendMessage(chatID,
			lang.Translate("error.commands.logs_docker", map[string]any{
				"Hint": "docker logs telegram-media-server",
			}), nil)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), logsCommandTimeout)
	defer cancel()

	out, err := RunCommand(ctx, "journalctl",
		"-u", "telegram-media-server",
		"--since", "1 day ago",
		"--no-pager",
	)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to execute journalctl")
		var errMsg string
		if len(out) > 0 {
			errMsg = strings.TrimSpace(string(out))
			if len(errMsg) > maxLogErrorMsgLen {
				errMsg = errMsg[:maxLogErrorMsgLen] + "..."
			}
		} else {
			errMsg = err.Error()
		}
		msg := fmt.Sprintf("%s: %s", lang.Translate("error.commands.logs_error", nil), errMsg)
		if strings.Contains(strings.ToLower(errMsg), "permission") || strings.Contains(strings.ToLower(errMsg), "access") {
			msg += "\n\n" + lang.Translate("error.commands.logs_journal_hint", nil)
		}
		a.Bot.SendMessage(chatID, msg, nil)
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

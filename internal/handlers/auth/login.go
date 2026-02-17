package auth

import (
	"context"
	"strings"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/ui"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func LoginHandler(a *app.App, update *tgbotapi.Update) {
	textFields := strings.Fields(update.Message.Text)

	if len(textFields) != 2 {
		logutils.Log.Warn("Invalid login command format")
		a.Bot.SendMessage(update.Message.Chat.ID, lang.Translate("error.commands.invalid_format", nil), nil)
		return
	}

	password := textFields[1]
	chatID := update.Message.Chat.ID
	userName := update.Message.From.UserName

	success, err := a.DB.Login(context.Background(), password, chatID, userName, a.Config)
	if err != nil {
		logutils.Log.WithError(err).Error("Login failed due to an error")
		a.Bot.SendMessage(chatID, lang.Translate("error.authentication.login", nil), nil)
		return
	}

	if success {
		logutils.Log.WithField("username", userName).Info("User logged in successfully")
		a.Bot.SendMessage(chatID, lang.Translate("general.status_messages.login_success", nil), ui.GetMainMenuKeyboard())
		ui.SendMainMenu(a.Bot, chatID, lang.Translate("general.commands.start", nil))
	} else {
		logutils.Log.WithField("username", userName).Warn("Login failed due to incorrect or expired password")
		a.Bot.SendMessage(chatID, lang.Translate("error.authentication.wrong_password", nil), nil)
	}
}

package auth

import (
	"context"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/ui"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func LoginHandler(bot *tmsbot.Bot, update *tgbotapi.Update) {
	textFields := strings.Fields(update.Message.Text)

	if len(textFields) != 2 {
		logutils.Log.Warn("Invalid login command format")
		bot.SendErrorMessage(update.Message.Chat.ID, lang.Translate("error.commands.invalid_format", nil))
		return
	}

	password := textFields[1]
	chatID := update.Message.Chat.ID
	userName := update.Message.From.UserName

	success, err := database.GlobalDB.Login(context.Background(), password, chatID, userName)
	if err != nil {
		logutils.Log.WithError(err).Error("Login failed due to an error")
		bot.SendErrorMessage(chatID, lang.Translate("error.authentication.login", nil))
		return
	}

	if success {
		logutils.Log.WithField("username", userName).Info("User logged in successfully")
		bot.SendSuccessMessage(chatID, lang.Translate("general.status_messages.login_success", nil))
		ui.SendMainMenu(bot, chatID, lang.Translate("general.commands.start", nil))
	} else {
		logutils.Log.WithField("username", userName).Warn("Login failed due to incorrect or expired password")
		bot.SendErrorMessage(chatID, lang.Translate("error.authentication.wrong_password", nil))
	}
}

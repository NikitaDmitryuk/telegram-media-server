package auth

import (
	"context"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func GenerateTempPasswordHandler(bot *tmsbot.Bot, update *tgbotapi.Update) {
	args := strings.Fields(update.Message.Text)
	if len(args) != 2 {
		logutils.Log.Warn("Invalid /temp command format")
		bot.SendErrorMessage(update.Message.Chat.ID, lang.Translate("error.commands.invalid_format", nil))
		return
	}

	durationStr := args[1]
	duration, err := utils.ValidateDurationString(durationStr)
	if err != nil {
		logutils.Log.WithError(err).Warn("Invalid duration string for /temp command")
		bot.SendErrorMessage(update.Message.Chat.ID, lang.Translate("error.validation.invalid_duration", nil))
		return
	}

	password, err := database.GlobalDB.GenerateTemporaryPassword(context.Background(), duration)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to generate temporary password")
		bot.SendErrorMessage(update.Message.Chat.ID, lang.Translate("error.security.temp_password_error", nil))
		return
	}

	bot.SendSuccessMessage(update.Message.Chat.ID, password)

	logutils.Log.Infof("Temporary password generated for chat ID %d: %s", update.Message.Chat.ID, password)
}

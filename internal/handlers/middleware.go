package handlers

import (
	"context"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

func LoggingMiddleware(update tgbotapi.Update) {
	if update.Message != nil {
		logrus.WithFields(logrus.Fields{
			"username": update.Message.From.UserName,
			"text":     update.Message.Text,
		}).Info("Received a new message")
	}
}

func AuthMiddleware(bot *bot.Bot, update tgbotapi.Update) bool {
	if update.Message == nil {
		return false
	}

	userExists, err := database.GlobalDB.CheckUser(context.Background(), update.Message.From.ID)
	if err != nil {
		logrus.WithError(err).Error("Failed to check if user exists")
		bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.CheckUserErrorMsgID))
		return false
	}

	if !userExists {
		bot.SendSuccessMessage(update.Message.Chat.ID, lang.GetMessage(lang.UnknownUserMsgID))
		return false
	}

	return true
}

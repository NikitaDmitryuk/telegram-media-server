package handlers

import (
	"context"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
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

func AuthMiddleware(bot *bot.Bot, update tgbotapi.Update) (bool, database.UserRole) {
	var chatID int64
	var userID int64

	if update.Message != nil {
		chatID = update.Message.Chat.ID
		userID = update.Message.From.ID
	} else if update.CallbackQuery != nil {
		chatID = update.CallbackQuery.Message.Chat.ID
		userID = update.CallbackQuery.From.ID
	} else {
		logrus.Error("Unable to determine chat ID or user ID")
		return false, ""
	}

	isAllowed, userRole, err := database.GlobalDB.IsUserAccessAllowed(context.Background(), userID)
	if err != nil {
		logrus.WithError(err).Error("Failed to check user access")
		return false, ""
	}

	if !isAllowed {
		logrus.Warnf("Access denied for user with chat ID %d", chatID)
		return false, userRole
	}

	logrus.Debugf("Access granted for user with chat ID %d, role: %s", chatID, userRole)
	return true, userRole
}

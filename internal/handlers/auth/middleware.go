package auth

import (
	"context"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func LoggingMiddleware(update *tgbotapi.Update) {
	if update.Message != nil {
		logutils.Log.WithFields(map[string]any{
			"username": update.Message.From.UserName,
			"text":     update.Message.Text,
		}).Info("Received a new message")
	}
}

func AuthMiddleware(update *tgbotapi.Update) (bool, database.UserRole) {
	var chatID int64
	var userID int64

	if update.Message != nil {
		chatID = update.Message.Chat.ID
		userID = update.Message.From.ID
	} else if update.CallbackQuery != nil {
		chatID = update.CallbackQuery.Message.Chat.ID
		userID = update.CallbackQuery.From.ID
	} else {
		logutils.Log.Error("Unable to determine chat ID or user ID")
		return false, ""
	}

	isAllowed, userRole, err := database.GlobalDB.IsUserAccessAllowed(context.Background(), userID)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to check user access")
		return false, ""
	}

	if !isAllowed {
		logutils.Log.Warnf("Access denied for user with chat ID %d", chatID)
		return false, userRole
	}

	logutils.Log.Debugf("Access granted for user with chat ID %d, role: %s", chatID, userRole)
	return true, userRole
}

func CheckAccess(update *tgbotapi.Update) bool {
	allowed, _ := AuthMiddleware(update)
	return allowed
}

func CheckAccessWithRole(update *tgbotapi.Update, allowedRoles []database.UserRole) bool {
	allowed, role := AuthMiddleware(update)
	if !allowed {
		return false
	}

	for _, allowedRole := range allowedRoles {
		if role == allowedRole {
			return true
		}
	}

	return false
}

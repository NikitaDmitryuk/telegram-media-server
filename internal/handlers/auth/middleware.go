package auth

import (
	"context"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/models"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func LoggingMiddleware(update *tgbotapi.Update) {
	if update == nil {
		logutils.Log.Error("Update is nil in LoggingMiddleware")
		return
	}

	if update.Message != nil {
		logutils.Log.WithFields(map[string]any{
			"username": update.Message.From.UserName,
			"text":     update.Message.Text,
		}).Info("Received a new message")
	}
}

func AuthMiddleware(update *tgbotapi.Update, db database.Database) (bool, models.UserRole) {
	if update == nil {
		logutils.Log.Error("Update is nil")
		return false, ""
	}

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

	isAllowed, userRole, err := db.IsUserAccessAllowed(context.Background(), userID)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to check user access")
		return false, ""
	}

	if !isAllowed {
		logutils.Log.Infof("Access denied for user with chat ID %d", chatID)
		return false, userRole
	}

	logutils.Log.Debugf("Access granted for user with chat ID %d, role: %s", chatID, userRole)
	return true, userRole
}

func CheckAccess(update *tgbotapi.Update, db database.Database) bool {
	allowed, _ := AuthMiddleware(update, db)
	return allowed
}

func CheckAccessWithRole(update *tgbotapi.Update, allowedRoles []models.UserRole, db database.Database) bool {
	allowed, role := AuthMiddleware(update, db)
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

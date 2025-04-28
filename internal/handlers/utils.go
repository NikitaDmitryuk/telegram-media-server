package handlers

import (
	"strconv"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	filemanager "github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

func DeleteMovieByID(bot *tmsbot.Bot, chatID int64, movieID string) {
	id, err := strconv.Atoi(movieID)
	if err != nil {
		logrus.WithError(err).Errorf("Invalid movie ID: %s", movieID)
		bot.SendErrorMessage(chatID, lang.Translate("error.validation.invalid_ids", map[string]interface{}{
			"IDs": movieID,
		}))
		return
	}

	err = filemanager.DeleteMovie(id)
	if err != nil {
		logrus.WithError(err).Errorf("Failed to delete movie with ID %d", id)
		bot.SendErrorMessage(chatID, lang.Translate("error.database.delete_movie_error", map[string]interface{}{
			"ID": id,
		}))
		return
	}

	bot.SendSuccessMessage(chatID, lang.Translate("general.status_messages.deleted_movie", map[string]interface{}{
		"ID": id,
	}))
}

func checkAccess(bot *tmsbot.Bot, update tgbotapi.Update) bool {
	allowed, _ := AuthMiddleware(bot, update)
	return allowed
}

func checkAccessWithRole(bot *tmsbot.Bot, update tgbotapi.Update, allowedRoles []database.UserRole) bool {
	allowed, role := AuthMiddleware(bot, update)
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

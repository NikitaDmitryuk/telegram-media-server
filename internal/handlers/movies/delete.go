package movies

import (
	"context"
	"strconv"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func DeleteMoviesHandler(bot *tmsbot.Bot, update *tgbotapi.Update) {
	args := strings.Fields(update.Message.Text)
	if len(args) < 2 {
		bot.SendErrorMessage(update.Message.Chat.ID, lang.Translate("error.commands.invalid_format", nil))
		return
	}

	chatID := update.Message.Chat.ID

	if args[1] == "all" {
		deleteAllMovies(bot, chatID)
	} else {
		deleteMoviesByID(bot, chatID, args[1:])
	}
}

func deleteAllMovies(bot *tmsbot.Bot, chatID int64) {
	movies, err := database.GlobalDB.GetMovieList(context.Background())
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to retrieve movie list for deletion")
		bot.SendErrorMessage(chatID, lang.Translate("error.movies.fetch_error", nil))
		return
	}

	for _, movie := range movies {
		if err := filemanager.DeleteMovie(movie.ID); err != nil {
			logutils.Log.WithError(err).Errorf("Failed to delete movie with ID %d", movie.ID)
		}
	}

	bot.SendSuccessMessage(chatID, lang.Translate("general.status_messages.all_movies_deleted", nil))
}

func deleteMoviesByID(bot *tmsbot.Bot, chatID int64, ids []string) {
	for _, idStr := range ids {
		id64, err := strconv.ParseUint(idStr, 10, 32)
		id := uint(id64)
		if err != nil {
			logutils.Log.WithError(err).Warnf("Invalid movie ID: %s", idStr)
			bot.SendErrorMessage(chatID, lang.Translate("error.validation.invalid_ids", map[string]any{
				"IDs": idStr,
			}))
			continue
		}

		if err := filemanager.DeleteMovie(id); err != nil {
			logutils.Log.WithError(err).Errorf("Failed to delete movie with ID %d", id)
			bot.SendErrorMessage(chatID, lang.Translate("error.database.delete_movie_error", map[string]any{
				"ID": id,
			}))
		} else {
			bot.SendSuccessMessage(chatID, lang.Translate("general.status_messages.deleted_movie", map[string]any{
				"ID": id,
			}))
		}
	}
}

func DeleteMovieByID(bot *tmsbot.Bot, chatID int64, movieID string) {
	id64, err := strconv.ParseUint(movieID, 10, 32)
	id := uint(id64)
	if err != nil {
		logutils.Log.WithError(err).Errorf("Invalid movie ID: %s", movieID)
		bot.SendErrorMessage(chatID, lang.Translate("error.validation.invalid_ids", map[string]any{
			"IDs": movieID,
		}))
		return
	}

	if err := filemanager.DeleteMovie(id); err != nil {
		logutils.Log.WithError(err).Errorf("Failed to delete movie with ID %d", id)
		bot.SendErrorMessage(chatID, lang.Translate("error.database.delete_movie_error", map[string]any{
			"ID": id,
		}))
		return
	}

	bot.SendSuccessMessage(chatID, lang.Translate("general.status_messages.deleted_movie", map[string]any{
		"ID": id,
	}))
}

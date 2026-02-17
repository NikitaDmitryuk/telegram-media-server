package movies

import (
	"context"
	"strconv"
	"strings"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/ui"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func DeleteMoviesHandler(
	a *app.App,
	update *tgbotapi.Update,
) {
	args := strings.Fields(update.Message.Text)
	if len(args) < 2 {
		a.Bot.SendMessage(update.Message.Chat.ID, lang.Translate("error.commands.invalid_format", nil), ui.GetMainMenuKeyboard())
		return
	}

	chatID := update.Message.Chat.ID

	if args[1] == "all" {
		deleteAllMovies(a, chatID)
	} else {
		deleteMoviesByID(a, chatID, args[1:])
	}
}

func deleteAllMovies(a *app.App, chatID int64) {
	movies, err := a.DB.GetMovieList(context.Background())
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to retrieve movie list for deletion")
		a.Bot.SendMessage(chatID, lang.Translate("error.movies.fetch_error", nil), ui.GetMainMenuKeyboard())
		return
	}

	a.Bot.SendMessage(chatID, lang.Translate("general.status_messages.deleting_all_movies", map[string]any{
		"Count": len(movies),
	}), ui.GetMainMenuKeyboard())

	go func() {
		for i := range movies {
			m := &movies[i]
			if err := filemanager.DeleteMovie(m.ID, a.Config.MoviePath, a.DB, a.DownloadManager); err != nil {
				logutils.Log.WithError(err).Errorf("Failed to delete movie with ID %d", m.ID)
			}
		}

		a.Bot.SendMessage(chatID, lang.Translate("general.status_messages.all_movies_deleted", nil), ui.GetMainMenuKeyboard())
	}()
}

func deleteMoviesByID(
	a *app.App,
	chatID int64,
	ids []string,
) {
	for _, idStr := range ids {
		id64, err := strconv.ParseUint(idStr, 10, 32)
		id := uint(id64)
		if err != nil {
			logutils.Log.WithError(err).Warnf("Invalid movie ID: %s", idStr)
			a.Bot.SendMessage(chatID, lang.Translate("error.validation.invalid_ids", map[string]any{
				"IDs": idStr,
			}), ui.GetMainMenuKeyboard())
			continue
		}

		a.Bot.SendMessage(chatID, lang.Translate("general.status_messages.deleting_movie", map[string]any{
			"ID": id,
		}), ui.GetMainMenuKeyboard())

		go func(movieID uint) {
			if err := filemanager.DeleteMovie(movieID, a.Config.MoviePath, a.DB, a.DownloadManager); err != nil {
				logutils.Log.WithError(err).Errorf("Failed to delete movie with ID %d", movieID)
				a.Bot.SendMessage(chatID, lang.Translate("error.database.delete_movie_error", map[string]any{
					"ID": movieID,
				}), ui.GetMainMenuKeyboard())
			} else {
				a.Bot.SendMessage(chatID, lang.Translate("general.status_messages.deleted_movie", map[string]any{
					"ID": movieID,
				}), ui.GetMainMenuKeyboard())
			}
		}(id)
	}
}

func DeleteMovieByID(
	a *app.App,
	chatID int64,
	movieID string,
) {
	if after, ok := strings.CutPrefix(movieID, "delete_movie:"); ok {
		movieID = after
	}

	id64, err := strconv.ParseUint(movieID, 10, 32)
	id := uint(id64)
	if err != nil {
		logutils.Log.WithError(err).Errorf("Invalid movie ID: %s", movieID)
		a.Bot.SendMessage(chatID, lang.Translate("error.validation.invalid_ids", map[string]any{
			"IDs": movieID,
		}), ui.GetMainMenuKeyboard())
		return
	}

	if err := filemanager.DeleteMovie(id, a.Config.MoviePath, a.DB, a.DownloadManager); err != nil {
		logutils.Log.WithError(err).Errorf("Failed to delete movie with ID %d", id)
		a.Bot.SendMessage(chatID, lang.Translate("error.database.delete_movie_error", map[string]any{
			"ID": id,
		}), ui.GetMainMenuKeyboard())
		return
	}

	a.Bot.SendMessage(chatID, lang.Translate("general.status_messages.deleted_movie", map[string]any{
		"ID": id,
	}), ui.GetMainMenuKeyboard())
}

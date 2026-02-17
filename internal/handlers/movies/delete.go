package movies

import (
	"context"
	"strconv"
	"strings"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
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
	if a.DeleteQueue == nil {
		a.Bot.SendMessage(chatID, lang.Translate("error.movies.fetch_error", nil), ui.GetMainMenuKeyboard())
		return
	}
	movies, err := a.DB.GetMovieList(context.Background())
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to retrieve movie list for deletion")
		a.Bot.SendMessage(chatID, lang.Translate("error.movies.fetch_error", nil), ui.GetMainMenuKeyboard())
		return
	}
	for i := range movies {
		a.DeleteQueue.Enqueue(movies[i].ID)
	}
	a.Bot.SendMessage(chatID, lang.Translate("general.status_messages.all_movies_deleted", nil), ui.GetMainMenuKeyboard())
}

func deleteMoviesByID(
	a *app.App,
	chatID int64,
	ids []string,
) {
	if a.DeleteQueue == nil {
		a.Bot.SendMessage(chatID, lang.Translate("error.movies.fetch_error", nil), ui.GetMainMenuKeyboard())
		return
	}
	var validIDs []uint
	for _, idStr := range ids {
		id64, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			logutils.Log.WithError(err).Warnf("Invalid movie ID: %s", idStr)
			a.Bot.SendMessage(chatID, lang.Translate("error.validation.invalid_ids", map[string]any{
				"IDs": idStr,
			}), ui.GetMainMenuKeyboard())
			continue
		}
		validIDs = append(validIDs, uint(id64))
	}
	for _, id := range validIDs {
		a.DeleteQueue.Enqueue(id)
	}
	if len(validIDs) == 0 {
		return
	}
	if len(validIDs) == 1 {
		a.Bot.SendMessage(chatID, lang.Translate("general.status_messages.deleted_movie", map[string]any{
			"ID": validIDs[0],
		}), ui.GetMainMenuKeyboard())
	} else {
		a.Bot.SendMessage(chatID, lang.Translate("general.status_messages.all_movies_deleted", nil), ui.GetMainMenuKeyboard())
	}
}

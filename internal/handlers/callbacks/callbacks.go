package callbacks

import (
	"context"
	"strconv"
	"strings"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	movies "github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/movies"
	tmssession "github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/session"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func HandleCallbackQuery(
	a *app.App,
	update *tgbotapi.Update,
) {
	callbackData := update.CallbackQuery.Data
	chatID := update.CallbackQuery.Message.Chat.ID

	allowed, role, err := a.DB.IsUserAccessAllowed(context.Background(), update.CallbackQuery.From.ID)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to check user access")
		a.Bot.SendMessage(chatID, lang.Translate("error.authentication.access_check_failed", nil), nil)
		return
	}
	if !allowed {
		a.Bot.SendMessage(chatID, lang.Translate("error.authentication.access_denied", nil), nil)
		return
	}

	switch {
	case strings.HasPrefix(callbackData, "delete_movie:"):
		handleDeleteMovieCallback(a, update, chatID, role, callbackData)

	case callbackData == "cancel_delete_menu":
		_ = a.Bot.DeleteMessage(chatID, update.CallbackQuery.Message.MessageID)

	case callbackData == "list_movies":
		movies.ListMoviesHandler(a, update)

	case strings.HasPrefix(callbackData, "torrent_search_download:"):
		tmssession.HandleTorrentSearchCallback(a, update)
		return
	case callbackData == "torrent_search_back":
		tmssession.HandleTorrentSearchCallback(a, update)
		return
	case callbackData == "torrent_search_cancel":
		tmssession.HandleTorrentSearchCallback(a, update)
		return
	case callbackData == "torrent_search_more":
		tmssession.HandleTorrentSearchCallback(a, update)
		return

	default:
		logutils.Log.Warnf("Unknown callback data: %s", callbackData)
		a.Bot.SendMessage(chatID, lang.Translate("error.commands.unknown_command", nil), nil)
	}

	a.Bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
}

func handleDeleteMovieCallback(
	a *app.App,
	update *tgbotapi.Update,
	chatID int64,
	role database.UserRole,
	callbackData string,
) {
	if role != database.AdminRole && role != database.RegularRole {
		a.Bot.SendMessage(chatID, lang.Translate("error.authentication.access_denied", nil), nil)
		return
	}

	a.Bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))

	movieIDStr := strings.TrimPrefix(callbackData, "delete_movie:")
	movieID, err := strconv.ParseUint(movieIDStr, 10, 32)
	if err != nil {
		logutils.Log.WithError(err).Errorf("Invalid movie ID: %s", movieIDStr)
		return
	}

	logutils.Log.WithFields(map[string]any{
		"callback_data": callbackData,
		"chat_id":       chatID,
		"action":        "delete_callback_started",
	}).Info("Starting delete from callback")

	go func() {
		movies.DeleteMovieByID(a, chatID, callbackData)

		logutils.Log.WithFields(map[string]any{
			"chat_id": chatID,
			"action":  "delete_callback_completed",
		}).Info("Delete completed, updating menu")

		movieList, err := a.DB.GetMovieList(context.Background())
		if err != nil {
			logutils.Log.WithError(err).Error("Failed to get movie list for menu update")
			return
		}

		var remainingMovies []database.Movie
		for i := range movieList {
			if movieList[i].ID != uint(movieID) {
				remainingMovies = append(remainingMovies, movieList[i])
			}
		}

		updateDeleteMenuWithMovies(a, chatID, update.CallbackQuery.Message.MessageID, remainingMovies)
	}()
}

func updateDeleteMenuWithMovies(a *app.App, chatID int64, messageID int, movieList []database.Movie) {
	logutils.Log.WithFields(map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"action":     "update_menu_started",
	}).Info("Starting menu update")

	logutils.Log.WithFields(map[string]any{
		"chat_id":      chatID,
		"movies_count": len(movieList),
		"action":       "menu_update_movies_found",
	}).Info("Found movies for menu update")

	if len(movieList) == 0 {
		logutils.Log.WithFields(map[string]any{
			"chat_id":    chatID,
			"message_id": messageID,
			"action":     "menu_update_delete_message",
		}).Info("No movies left, deleting menu message")

		_ = a.Bot.DeleteMessage(chatID, messageID)
		return
	}

	logutils.Log.WithFields(map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"action":     "menu_update_edit_message",
	}).Info("Updating menu with remaining movies")

	newMarkup := movies.CreateDeleteMovieMenuMarkup(movieList)
	if err := a.Bot.EditMessageTextAndMarkup(
		chatID,
		messageID,
		lang.Translate("general.user_prompts.delete_prompt", nil),
		newMarkup,
	); err != nil {
		if strings.Contains(err.Error(), "message is not modified") {
			logutils.Log.WithFields(map[string]any{
				"chat_id":    chatID,
				"message_id": messageID,
				"action":     "menu_update_no_changes",
			}).Info("Menu content unchanged, no update needed")
		} else {
			logutils.Log.WithError(err).Error("Failed to send edit message")
		}
	} else {
		logutils.Log.WithFields(map[string]any{
			"chat_id":    chatID,
			"message_id": messageID,
			"action":     "menu_update_success",
		}).Info("Menu updated successfully")
	}
}

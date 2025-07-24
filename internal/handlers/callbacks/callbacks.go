package callbacks

import (
	"context"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	movies "github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/movies"
	tmssession "github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/session"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func HandleCallbackQuery(bot *tmsbot.Bot, update *tgbotapi.Update) {
	callbackData := update.CallbackQuery.Data
	chatID := update.CallbackQuery.Message.Chat.ID

	allowed, role, err := database.GlobalDB.IsUserAccessAllowed(context.Background(), update.CallbackQuery.From.ID)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to check user access")
		bot.SendMessage(chatID, lang.Translate("error.authentication.access_check_failed", nil), nil)
		return
	}
	if !allowed {
		bot.SendMessage(chatID, lang.Translate("error.authentication.access_denied", nil), nil)
		return
	}

	switch {
	case strings.HasPrefix(callbackData, "delete_movie:"):
		handleDeleteMovieCallback(bot, update, chatID, role, callbackData)

	case callbackData == "cancel_delete_menu":
		deleteMsg := tgbotapi.NewDeleteMessage(chatID, update.CallbackQuery.Message.MessageID)
		if _, err := bot.Api.Request(deleteMsg); err != nil {
			logutils.Log.WithError(err).Error("Failed to delete message")
		}

	case callbackData == "list_movies":
		movies.ListMoviesHandler(bot, update)

	case strings.HasPrefix(callbackData, "torrent_search_download:"):
		tmssession.HandleTorrentSearchCallback(bot, update)
		return
	case callbackData == "torrent_search_cancel":
		tmssession.HandleTorrentSearchCallback(bot, update)
		return
	case callbackData == "torrent_search_more":
		tmssession.HandleTorrentSearchCallback(bot, update)
		return

	default:
		logutils.Log.Warnf("Unknown callback data: %s", callbackData)
		bot.SendMessage(chatID, lang.Translate("error.commands.unknown_command", nil), nil)
	}

	bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
}

func handleDeleteMovieCallback(bot *tmsbot.Bot, update *tgbotapi.Update, chatID int64, role database.UserRole, callbackData string) {
	if role != database.AdminRole && role != database.RegularRole {
		bot.SendMessage(chatID, lang.Translate("error.authentication.access_denied", nil), nil)
		return
	}

	movieID := strings.TrimPrefix(callbackData, "delete_movie:")
	movies.DeleteMovieByID(bot, chatID, movieID)

	movieList, err := database.GlobalDB.GetMovieList(context.Background())
	if err != nil {
		bot.SendMessage(chatID, lang.Translate("error.movies.fetch_error", nil), nil)
		return
	}

	if len(movieList) == 0 {
		deleteMsg := tgbotapi.NewDeleteMessage(chatID, update.CallbackQuery.Message.MessageID)
		if _, err := bot.Api.Request(deleteMsg); err != nil {
			logutils.Log.WithError(err).Error("Failed to delete message")
		}
		return
	}

	newMarkup := movies.CreateDeleteMovieMenuMarkup(movieList)
	editMsg := tgbotapi.NewEditMessageTextAndMarkup(
		chatID,
		update.CallbackQuery.Message.MessageID,
		lang.Translate("general.user_prompts.delete_prompt", nil),
		newMarkup,
	)
	if _, err := bot.Api.Send(editMsg); err != nil {
		logutils.Log.WithError(err).Error("Failed to send edit message")
	}
}

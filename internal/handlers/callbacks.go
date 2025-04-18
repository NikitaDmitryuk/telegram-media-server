package handlers

import (
	"context"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

func HandleCallbackQuery(bot *tmsbot.Bot, update tgbotapi.Update) {
	callbackData := update.CallbackQuery.Data
	chatID := update.CallbackQuery.Message.Chat.ID

	allowed, role := AuthMiddleware(bot, update)
	if !allowed {
		bot.SendErrorMessage(chatID, lang.GetMessage(lang.AccessDeniedMsgID))
		return
	}

	switch {
	case strings.HasPrefix(callbackData, "delete_movie:"):
		if role == database.AdminRole || role == database.RegularRole {
			movieID := strings.TrimPrefix(callbackData, "delete_movie:")
			DeleteMovieByID(bot, chatID, movieID)
		} else {
			bot.SendErrorMessage(chatID, lang.GetMessage(lang.AccessDeniedMsgID))
		}
	case callbackData == "delete_movie_menu":
		if role == database.AdminRole || role == database.RegularRole {
			movies, err := database.GlobalDB.GetMovieList(context.Background())
			if err != nil {
				bot.SendErrorMessage(chatID, lang.GetMessage(lang.GetMovieListErrorMsgID))
				return
			}
			SendDeleteMovieMenu(bot, chatID, movies)
		} else {
			bot.SendErrorMessage(chatID, lang.GetMessage(lang.AccessDeniedMsgID))
		}
	case callbackData == "list_movies":
		ListMoviesHandler(bot, update)
	default:
		logrus.Warnf("Unknown callback data: %s", callbackData)
		bot.SendErrorMessage(chatID, lang.GetMessage(lang.UnknownCommandMsgID))
	}

	bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
}

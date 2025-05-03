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
		bot.SendErrorMessage(chatID, lang.Translate("error.authentication.access_denied", nil))
		return
	}

	switch {
	case strings.HasPrefix(callbackData, "delete_movie:"):
		if role == database.AdminRole || role == database.RegularRole {
			movieID := strings.TrimPrefix(callbackData, "delete_movie:")
			DeleteMovieByID(bot, chatID, movieID)

			movies, err := database.GlobalDB.GetMovieList(context.Background())
			if err != nil {
				bot.SendErrorMessage(chatID, lang.Translate("error.movies.fetch_error", nil))
				return
			}

			if len(movies) == 0 {
				deleteMsg := tgbotapi.NewDeleteMessage(chatID, update.CallbackQuery.Message.MessageID)
				if _, err := bot.Api.Request(deleteMsg); err != nil {
					logrus.WithError(err).Error("Failed to delete message")
				}
				return
			}

			newMarkup := CreateDeleteMovieMenuMarkup(movies)
			editMsg := tgbotapi.NewEditMessageTextAndMarkup(
				chatID,
				update.CallbackQuery.Message.MessageID,
				lang.Translate("general.user_prompts.delete_prompt", nil),
				newMarkup,
			)
			if _, err := bot.Api.Send(editMsg); err != nil {
				logrus.WithError(err).Error("Failed to send edit message")
				return
			}
		} else {
			bot.SendErrorMessage(chatID, lang.Translate("error.authentication.access_denied", nil))
		}

	case callbackData == "delete_movie_menu":
		if role == database.AdminRole || role == database.RegularRole {
			movies, err := database.GlobalDB.GetMovieList(context.Background())
			if err != nil {
				bot.SendErrorMessage(chatID, lang.Translate("error.movies.fetch_error", nil))
				bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
				return
			}

			if len(movies) == 0 {
				bot.SendSuccessMessage(chatID, lang.Translate("general.user_prompts.no_movies_to_delete", nil))
				bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
				return
			}

			SendDeleteMovieMenu(bot, chatID, movies)
			bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
		} else {
			bot.SendErrorMessage(chatID, lang.Translate("error.authentication.access_denied", nil))
			bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
		}

	case callbackData == "cancel_delete_menu":
		if role == database.AdminRole || role == database.RegularRole {
			deleteMsg := tgbotapi.NewDeleteMessage(chatID, update.CallbackQuery.Message.MessageID)
			if _, err := bot.Api.Request(deleteMsg); err != nil {
				logrus.WithError(err).Error("Failed to delete message")
			}

		} else {
			bot.SendErrorMessage(chatID, lang.Translate("error.authentication.access_denied", nil))
		}

	case callbackData == "list_movies":
		ListMoviesHandler(bot, update)

	default:
		logrus.Warnf("Unknown callback data: %s", callbackData)
		bot.SendErrorMessage(chatID, lang.Translate("error.commands.unknown_command", nil))
	}

	bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
}

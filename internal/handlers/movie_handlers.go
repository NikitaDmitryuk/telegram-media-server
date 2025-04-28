package handlers

import (
	"context"
	"strconv"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

func ListMoviesHandler(bot *tmsbot.Bot, update tgbotapi.Update) {
	var chatID int64
	if update.Message != nil {
		chatID = update.Message.Chat.ID
	} else if update.CallbackQuery != nil {
		chatID = update.CallbackQuery.Message.Chat.ID
	} else {
		logrus.Error("Unable to determine chat ID")
		return
	}

	movies, err := tmsdb.GlobalDB.GetMovieList(context.Background())
	if err != nil {
		logrus.WithError(err).Error("Failed to retrieve movie list")
		bot.SendErrorMessage(chatID, lang.Translate("error.movies.fetch_error", nil))
		return
	}

	var msg string
	for _, movie := range movies {
		msg += lang.Translate("general.downloaded_list", map[string]interface{}{
			"ID":       movie.ID,
			"Name":     movie.Name,
			"Progress": movie.DownloadedPercentage,
		})
	}

	if msg == "" {
		msg = lang.Translate("general.status_messages.empty_list", nil)
	}

	SendMainMenu(bot, chatID, msg)
}

func DeleteMoviesHandler(bot *tmsbot.Bot, update tgbotapi.Update) {
	args := strings.Fields(strings.TrimPrefix(update.Message.Text, "/"))
	if len(args) < 2 {
		bot.SendErrorMessage(update.Message.Chat.ID, lang.Translate("error.commands.invalid_format", nil))
		return
	}

	if args[1] == "all" {
		movies, err := tmsdb.GlobalDB.GetMovieList(context.Background())
		if err != nil {
			logrus.WithError(err).Error("Failed to retrieve movie list for deletion")
			bot.SendErrorMessage(update.Message.Chat.ID, lang.Translate("error.movies.fetch_error", nil))
			return
		}
		for _, movie := range movies {
			DeleteMovieByID(bot, update.Message.Chat.ID, strconv.Itoa(int(movie.ID)))
		}
		bot.SendSuccessMessage(update.Message.Chat.ID, lang.Translate("general.status_messages.all_movies_deleted", nil))
	} else {
		for _, arg := range args[1:] {
			DeleteMovieByID(bot, update.Message.Chat.ID, arg)
		}
	}
}

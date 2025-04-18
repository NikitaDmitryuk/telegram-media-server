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
		bot.SendErrorMessage(chatID, lang.GetMessage(lang.GetMovieListErrorMsgID))
		return
	}

	var msg string
	for _, movie := range movies {
		msg += lang.GetMessage(lang.MovieDownloadingMsgID, movie.ID, movie.Name, movie.DownloadedPercentage) + "\n"
	}

	if msg == "" {
		msg = lang.GetMessage(lang.NoMoviesMsgID)
	}

	SendMainMenu(bot, chatID, msg)
}

func DeleteMoviesHandler(bot *tmsbot.Bot, update tgbotapi.Update) {
	args := strings.Fields(strings.TrimPrefix(update.Message.Text, "/"))
	if len(args) < 2 {
		bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.InvalidCommandFormatMsgID))
		return
	}

	if args[1] == "all" {
		movies, err := tmsdb.GlobalDB.GetMovieList(context.Background())
		if err != nil {
			bot.SendErrorMessage(update.Message.Chat.ID, err.Error())
			return
		}
		for _, movie := range movies {
			DeleteMovieByID(bot, update.Message.Chat.ID, strconv.Itoa(int(movie.ID)))
		}
		bot.SendSuccessMessage(update.Message.Chat.ID, lang.GetMessage(lang.AllMoviesDeletedMsgID))
	} else {
		for _, arg := range args[1:] {
			DeleteMovieByID(bot, update.Message.Chat.ID, arg)
		}
	}
}

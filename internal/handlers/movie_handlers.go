package handlers

import (
	"context"
	"strconv"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	filemanager "github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

func ListMoviesHandler(bot *tmsbot.Bot, update tgbotapi.Update) {
	movies, err := tmsdb.GlobalDB.GetMovieList(context.Background())
	if err != nil {
		logrus.WithError(err).Error("Failed to retrieve movie list")
		bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.GetMovieListErrorMsgID))
		return
	}

	var msg string
	for _, movie := range movies {
		msg += lang.GetMessage(lang.MovieDownloadingMsgID, movie.ID, movie.Name, movie.DownloadedPercentage)
	}

	if msg == "" {
		msg = lang.GetMessage(lang.NoMoviesMsgID)
	}

	bot.SendSuccessMessage(update.Message.Chat.ID, msg)
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
			err := filemanager.DeleteMovie(int(movie.ID))
			if err != nil {
				bot.SendErrorMessage(update.Message.Chat.ID, err.Error())
				return
			}
		}
		bot.SendSuccessMessage(update.Message.Chat.ID, lang.GetMessage(lang.AllMoviesDeletedMsgID))
	} else {
		var deletedIDs []string
		for _, arg := range args[1:] {
			id, err := strconv.Atoi(arg)
			if err != nil {
				continue
			}

			err = filemanager.DeleteMovie(id)
			if err != nil {
				logrus.WithError(err).Errorf("Failed to delete movie with ID %d", id)
				continue
			}

			deletedIDs = append(deletedIDs, strconv.Itoa(id))
		}

		if len(deletedIDs) > 0 {
			bot.SendSuccessMessage(update.Message.Chat.ID, lang.GetMessage(lang.DeletedMoviesMsgID, strings.Join(deletedIDs, ", ")))
		} else {
			bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.NoValidIDsMsgID))
		}
	}
}

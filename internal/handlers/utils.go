package handlers

import (
	"strconv"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	filemanager "github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/sirupsen/logrus"
)

func DeleteMovieByID(bot *tmsbot.Bot, chatID int64, movieID string) {
	id, err := strconv.Atoi(movieID)
	if err != nil {
		logrus.WithError(err).Errorf("Invalid movie ID: %s", movieID)
		bot.SendErrorMessage(chatID, lang.GetMessage(lang.InvalidIDsMsgID, movieID))
		return
	}

	err = filemanager.DeleteMovie(id)
	if err != nil {
		logrus.WithError(err).Errorf("Failed to delete movie with ID %d", id)
		bot.SendErrorMessage(chatID, lang.GetMessage(lang.FailedToDeleteMovieMsgID, strconv.Itoa(id)))
		return
	}

	bot.SendSuccessMessage(chatID, lang.GetMessage(lang.DeletedMoviesMsgID, strconv.Itoa(id)))
}

package api

import (
	"log"
	"strconv"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func HandleUpdates(update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

	if userExists, err := dbCheckUser(update.Message.From.ID); err != nil {
		tmsbot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.CheckUserErrorMsgID))
	} else if !userExists {
		handleUnknownUser(update)
	} else {
		handleKnownUser(update)
	}
}

func handleUnknownUser(update tgbotapi.Update) {
	if update.Message.IsCommand() {
		switch update.Message.Command() {
		case "login":
			loginHandler(update)
		default:
			tmsbot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.UnknownUserMsgID))
		}
	} else {
		tmsbot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.UnknownUserMsgID))
	}
}

func handleKnownUser(update tgbotapi.Update) {
	message := update.Message
	chatID := message.Chat.ID

	if message.IsCommand() {
		switch message.Command() {
		case "start":
			tmsbot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.StartCommandMsgID))
		case "ls":
			listHandler(update)
		case "rm":
			deleteHandler(update)
		case "stop":
			stopHandler(update)
		default:
			tmsbot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.UnknownCommandMsgID))
		}
		return
	}

	if isValidLink(message.Text) {
		go downloadVideo(update)
		tmsbot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.VideoDownloadingMsgID))
		return
	}

	if doc := message.Document; doc != nil && strings.HasSuffix(doc.FileName, ".torrent") {
		fileName := doc.FileName

		if err := downloadFile(doc.FileID, fileName); err != nil {
			tmsbot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.DownloadDocumentErrorMsgID))
			return
		}

		exists, err := dbMovieExistsTorrent(fileName)
		if err != nil {
			tmsbot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.TorrentFileExistsErrorMsgID))
			return
		}
		if exists {
			tmsbot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.FileAlreadyExistsMsgID))
			return
		}

		if err := downloadTorrent(fileName, update); err != nil {
			tmsbot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.TorrentFileDownloadErrorMsgID))
			return
		}

		tmsbot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.VideoDownloadingMsgID))
		return
	}

	tmsbot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.UnknownCommandMsgID))
}

func loginHandler(update tgbotapi.Update) {
	textFields := strings.Fields(update.Message.Text)

	if len(textFields) != 2 {
		tmsbot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.InvalidCommandFormatMsgID))
	} else {
		success, err := dbLogin(textFields[1], update.Message.Chat.ID, update.Message.From.UserName)
		if err != nil {
			tmsbot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.LoginErrorMsgID))
		} else if success {
			tmsbot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.LoginSuccessMsgID))
		} else {
			tmsbot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.WrongPasswordMsgID))
		}
	}
}

func listHandler(update tgbotapi.Update) {
	movies, err := dbGetMovieList()
	if err != nil {
		tmsbot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.GetMovieListErrorMsgID))
		return
	}

	var msg string
	for _, movie := range movies {
		if movie.Downloaded {
			msg += tmslang.GetMessage(tmslang.MovieDownloadedMsgID, movie.ID, movie.Name)
		} else {
			msg += tmslang.GetMessage(tmslang.MovieDownloadingMsgID, movie.ID, movie.Name, movie.DownloadedPercentage)
		}
	}

	if len(msg) == 0 {
		msg = tmslang.GetMessage(tmslang.NoMoviesMsgID)
	}

	tmsbot.SendSuccessMessage(update.Message.Chat.ID, msg)
}

func deleteHandler(update tgbotapi.Update) {
	commandID := strings.Fields(update.Message.Text)

	if len(commandID) != 2 {
		tmsbot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.InvalidCommandFormatMsgID))
		return
	}

	if commandID[1] == "all" {
		movies, _ := dbGetMovieList()
		for _, movie := range movies {
			err := deleteMovie(movie.ID)
			if err != nil {
				sendErrorMessage(update.Message.Chat.ID, err.Error())
				return
			}
		}
		tmsbot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.AllMoviesDeletedMsgID))
	} else {
		id, err := strconv.Atoi(commandID[1])
		if err != nil {
			tmsbot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.InvalidIDMsgID))
			return
		}

		if exists, err := dbMovieExistsId(id); err != nil {
			tmsbot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.MovieCheckErrorMsgID))
		} else if exists {
			err := deleteMovie(id)
			if err != nil {
				tmsbot.SendErrorMessage(update.Message.Chat.ID, err.Error())
			} else {
				tmsbot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.MovieDeletedMsgID))
			}
		} else {
			tmsbot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.InvalidIDMsgID))
		}
	}
}

func stopHandler(update tgbotapi.Update) {
	go stopTorrentDownload()
	tmsbot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.TorrentDownloadsStoppedMsgID))
}

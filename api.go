package main

import (
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func orchestrator(update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

	if userExists, err := dbCheckUser(update.Message.From.ID); err != nil {
		sendErrorMessage(update.Message.Chat.ID, GetMessage(CheckUserErrorMsgID))
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
			sendSuccessMessage(update.Message.Chat.ID, GetMessage(UnknownUserMsgID))
		}
	} else {
		sendSuccessMessage(update.Message.Chat.ID, GetMessage(UnknownUserMsgID))
	}
}

func handleKnownUser(update tgbotapi.Update) {
	message := update.Message
	chatID := message.Chat.ID

	if message.IsCommand() {
		switch message.Command() {
		case "start":
			sendSuccessMessage(chatID, GetMessage(StartCommandMsgID))
		case "ls":
			listHandler(update)
		case "rm":
			deleteHandler(update)
		case "stop":
			stopHandler(update)
		default:
			sendErrorMessage(chatID, GetMessage(UnknownCommandMsgID))
		}
		return
	}

	if isValidLink(message.Text) {
		go downloadVideo(update)
		sendSuccessMessage(chatID, GetMessage(VideoDownloadingMsgID))
		return
	}

	if doc := message.Document; doc != nil && strings.HasSuffix(doc.FileName, ".torrent") {
		fileName := doc.FileName

		if err := downloadFile(doc.FileID, fileName); err != nil {
			sendErrorMessage(chatID, GetMessage(DownloadDocumentErrorMsgID))
			return
		}

		exists, err := dbMovieExistsTorrent(fileName)
		if err != nil {
			sendErrorMessage(chatID, GetMessage(TorrentFileExistsErrorMsgID))
			return
		}
		if exists {
			sendErrorMessage(chatID, GetMessage(FileAlreadyExistsMsgID))
			return
		}

		if err := downloadTorrent(fileName, update); err != nil {
			sendErrorMessage(chatID, GetMessage(TorrentFileDownloadErrorMsgID))
			return
		}

		sendSuccessMessage(chatID, GetMessage(VideoDownloadingMsgID))
		return
	}

	sendErrorMessage(chatID, GetMessage(UnknownCommandMsgID))
}

func loginHandler(update tgbotapi.Update) {
	textFields := strings.Fields(update.Message.Text)

	if len(textFields) != 2 {
		sendErrorMessage(update.Message.Chat.ID, GetMessage(InvalidCommandFormatMsgID))
	} else {
		success, err := dbLogin(textFields[1], update.Message.Chat.ID, update.Message.From.UserName)
		if err != nil {
			sendErrorMessage(update.Message.Chat.ID, GetMessage(LoginErrorMsgID))
		} else if success {
			sendSuccessMessage(update.Message.Chat.ID, GetMessage(LoginSuccessMsgID))
		} else {
			sendErrorMessage(update.Message.Chat.ID, GetMessage(WrongPasswordMsgID))
		}
	}
}

func listHandler(update tgbotapi.Update) {
	movies, err := dbGetMovieList()
	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, GetMessage(GetMovieListErrorMsgID))
		return
	}

	var msg string
	for _, movie := range movies {
		if movie.Downloaded {
			msg += GetMessage(MovieDownloadedMsgID, movie.ID, movie.Name)
		} else {
			msg += GetMessage(MovieDownloadingMsgID, movie.ID, movie.Name, movie.DownloadedPercentage)
		}
	}

	if len(msg) == 0 {
		msg = GetMessage(NoMoviesMsgID)
	}

	sendSuccessMessage(update.Message.Chat.ID, msg)
}

func deleteHandler(update tgbotapi.Update) {
	commandID := strings.Fields(update.Message.Text)

	if len(commandID) != 2 {
		sendErrorMessage(update.Message.Chat.ID, GetMessage(InvalidCommandFormatMsgID))
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
		sendSuccessMessage(update.Message.Chat.ID, GetMessage(AllMoviesDeletedMsgID))
	} else {
		id, err := strconv.Atoi(commandID[1])
		if err != nil {
			sendErrorMessage(update.Message.Chat.ID, GetMessage(InvalidIDMsgID))
			return
		}

		if exists, err := dbMovieExistsId(id); err != nil {
			sendErrorMessage(update.Message.Chat.ID, GetMessage(MovieCheckErrorMsgID))
		} else if exists {
			err := deleteMovie(id)
			if err != nil {
				sendErrorMessage(update.Message.Chat.ID, err.Error())
			} else {
				sendSuccessMessage(update.Message.Chat.ID, GetMessage(MovieDeletedMsgID))
			}
		} else {
			sendErrorMessage(update.Message.Chat.ID, GetMessage(InvalidIDMsgID))
		}
	}
}

func stopHandler(update tgbotapi.Update) {
	go stopTorrentDownload()
	sendSuccessMessage(update.Message.Chat.ID, GetMessage(TorrentDownloadsStoppedMsgID))
}

func sendErrorMessage(chatID int64, message string) {
	log.Print(message)
	msg := tgbotapi.NewMessage(chatID, message)
	GlobalBot.Send(msg)
}

func sendSuccessMessage(chatID int64, message string) {
	msg := tgbotapi.NewMessage(chatID, message)
	GlobalBot.Send(msg)
}

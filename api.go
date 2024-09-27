package main

import (
	"fmt"
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
		sendErrorMessage(update.Message.Chat.ID, messages[lang].Error.CheckUserError)
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
			sendSuccessMessage(update.Message.Chat.ID, messages[lang].Info.UnknownUser)
		}
	} else {
		sendSuccessMessage(update.Message.Chat.ID, messages[lang].Info.UnknownUser)
	}
}

func handleKnownUser(update tgbotapi.Update) {

	if update.Message.IsCommand() {
		switch update.Message.Command() {
		case "start":
			sendSuccessMessage(update.Message.Chat.ID, messages[lang].Info.StartCommand)
		case "ls":
			listHandler(update)
		case "rm":
			deleteHandler(update)
		case "stop":
			stopHandler(update)
		default:
			sendErrorMessage(update.Message.Chat.ID, messages[lang].Error.UnknownCommand)
		}
	} else {
		if isValidLink(update.Message.Text) {
			go downloadVideo(update)
			sendSuccessMessage(update.Message.Chat.ID, messages[lang].Info.VideoDownloading)
		} else {
			sendErrorMessage(update.Message.Chat.ID, messages[lang].Error.UnknownCommand)
		}
	}
}

func loginHandler(update tgbotapi.Update) {
	textFields := strings.Fields(update.Message.Text)

	if len(textFields) != 2 {
		sendErrorMessage(update.Message.Chat.ID, messages[lang].Error.InvalidCommandFormat)
	} else {
		if success, err := dbLogin(textFields[1], update.Message.Chat.ID, update.Message.From.UserName); err != nil {
			sendErrorMessage(update.Message.Chat.ID, messages[lang].Error.LoginError)
		} else if success {
			sendSuccessMessage(update.Message.Chat.ID, messages[lang].Info.LoginSuccess)
		} else {
			sendErrorMessage(update.Message.Chat.ID, messages[lang].Error.WrongPassword)
		}
	}
}

func listHandler(update tgbotapi.Update) {
	movies, err := dbGetMovieList()
	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, messages[lang].Error.GetMovieListError)
		return
	}

	var msg string
	for _, movie := range movies {
		if movie.Downloaded {
			msg += fmt.Sprintf(messages[lang].Info.MovieDownloaded, movie.ID, movie.Name)
		} else {
			msg += fmt.Sprintf(messages[lang].Info.MovieDownloading, movie.ID, movie.Name, movie.DownloadedPercentage)
		}
	}

	if len(msg) == 0 {
		msg = messages[lang].Info.NoMovies
	}

	sendSuccessMessage(update.Message.Chat.ID, msg)
}

func deleteHandler(update tgbotapi.Update) {
	commandID := strings.Split(update.Message.Text, " ")

	if len(commandID) != 2 {
		sendErrorMessage(update.Message.Chat.ID, messages[lang].Error.InvalidCommandFormat)
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
		sendSuccessMessage(update.Message.Chat.ID, messages[lang].Info.AllMoviesDeleted)
	} else {
		id, err := strconv.Atoi(commandID[1])
		if err != nil {
			sendErrorMessage(update.Message.Chat.ID, messages[lang].Error.InvalidID)
			return
		}

		if exists, err := dbMovieExistsId(id); err != nil {
			sendErrorMessage(update.Message.Chat.ID, messages[lang].Error.MovieCheckError)
		} else if exists {
			err := deleteMovie(id)
			if err != nil {
				sendErrorMessage(update.Message.Chat.ID, err.Error())
			} else {
				sendSuccessMessage(update.Message.Chat.ID, messages[lang].Info.MovieDeleted)
			}
		} else {
			sendErrorMessage(update.Message.Chat.ID, messages[lang].Error.InvalidID)
		}
	}
}

func stopHandler(update tgbotapi.Update) {
	go stopTorrentDownload()
	sendSuccessMessage(update.Message.Chat.ID, messages[lang].Info.TorrentDownloadsStopped)
}

func sendErrorMessage(chatID int64, message string) {
	log.Printf(message)
	msg := tgbotapi.NewMessage(chatID, message)
	GlobalBot.Send(msg)
}

func sendSuccessMessage(chatID int64, message string) {
	msg := tgbotapi.NewMessage(chatID, message)
	GlobalBot.Send(msg)
}

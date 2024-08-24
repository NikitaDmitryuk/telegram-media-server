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
	var msg tgbotapi.MessageConfig

	if !checkUser(update.Message.From.ID) {
		msg = handleUnknownUser(update)
	} else {
		msg = handleKnownUser(update)
	}

	GlobalBot.Send(msg)
}

func handleUnknownUser(update tgbotapi.Update) tgbotapi.MessageConfig {
	if update.Message.IsCommand() {
		switch update.Message.Command() {
		case "start":
			return unknownUserHandler(update)
		case "login":
			return loginHandler(update)
		default:
			return unknownCommandHandler(update)
		}
	}
	return tgbotapi.MessageConfig{}
}

func handleKnownUser(update tgbotapi.Update) tgbotapi.MessageConfig {
	var msg string
	if update.Message.IsCommand() {
		switch update.Message.Command() {
		case "start":
			return startHandler(update)
		case "list":
			return listHandler(update)
		case "delete":
			return deleteHandler(update)
		default:
			return unknownCommandHandler(update)
		}
	}
	if update.Message.Document != nil || update.Message.Video != nil {
		var fileID string
		var fileName string
		switch {
		case update.Message.Video != nil:
			fileID = update.Message.Video.FileID
			fileName = update.Message.Video.FileName
		case update.Message.Document != nil:
			fileID = update.Message.Document.FileID
			fileName = update.Message.Document.FileName
		default:
			msg = "Unknown file type"
			log.Printf("Unknown file type")
			break
		}
		log.Printf("Received file with ID: %s\n", fileID)
		switch {
		case strings.HasSuffix(fileName, ".torrent"):
			err := downloadFile(fileID, fileName)
			if err != nil {
				log.Printf("Error: %v\n", err)
			} else {
				log.Println("File downloaded successfully")
				if !movieExistsTorrent(fileName) {
					downloadTorrent(fileName, update)
					msg = "Start download!"
				} else {
					msg = "File alredy exists"
				}
			}
		case strings.HasSuffix(fileName, ".mp4"):
			err := downloadFile(fileID, fileName)
			if err != nil {
				log.Printf("Error: %v\n", err)
			} else {
				log.Println("File downloaded successfully")
				addMovie(fileName, fileName, "")
			}
		}
	}
	return tgbotapi.NewMessage(update.Message.Chat.ID, msg)
}

func startHandler(update tgbotapi.Update) tgbotapi.MessageConfig {
	return tgbotapi.NewMessage(update.Message.Chat.ID, "/list - for get files\n/delete <ID> - remove movie\n")
}

func listHandler(update tgbotapi.Update) tgbotapi.MessageConfig {
	var msg string
	for _, movie := range getMovieList() {
		msg += fmt.Sprintf("ID: %d\nName: %s\nDownloaded: %t\nDownloaded percentage: %d\n\n", movie.ID, movie.Name, movie.Downloaded, movie.DownloadedPercentage)
	}
	if len(msg) == 0 {
		msg = "List empty"
	}
	return tgbotapi.NewMessage(update.Message.Chat.ID, msg)
}

func loginHandler(update tgbotapi.Update) tgbotapi.MessageConfig {
	textFields := strings.Fields(update.Message.Text)
	var msgText string

	if len(textFields) != 2 {
		msgText = "Invalid Login"
	} else {
		if login(textFields[1], update.Message.Chat.ID, update.Message.From.UserName) {
			msgText = "Login success"
		} else {
			msgText = "Invalid Login"
		}
	}

	return tgbotapi.NewMessage(update.Message.Chat.ID, msgText)
}

func unknownCommandHandler(update tgbotapi.Update) tgbotapi.MessageConfig {
	return tgbotapi.NewMessage(update.Message.Chat.ID, "Unknown command")
}

func unknownUserHandler(update tgbotapi.Update) tgbotapi.MessageConfig {
	return tgbotapi.NewMessage(update.Message.Chat.ID, "/login [PASSWORD]")
}

func deleteHandler(update tgbotapi.Update) tgbotapi.MessageConfig {
	command_id := strings.Split(update.Message.Text, " ")
	var msg string
	if len(command_id) != 2 {
		return startHandler(update)
	} else {
		id, err := strconv.Atoi(command_id[1])
		if err != nil {
			msg = "Invalid ID"
		}
		if movieExistsId(id) {
			msg = deleteMovie(id)
		} else {
			msg = "Invalid file ID"
		}
	}
	return tgbotapi.NewMessage(update.Message.Chat.ID, msg)
}

package main

import (
	"log"
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
	if update.Message.IsCommand() {
		switch update.Message.Command() {
		case "start":
			return startHandler(update)
		default:
			return unknownCommandHandler(update)
		}
	}
	if update.Message.Document != nil {
		fileID := update.Message.Document.FileID
		log.Printf("Received file with ID: %s\n", fileID)

		// Now you can download the file using the downloadFile function
		err := downloadFile(fileID, update.Message.Document.FileName)
		if err != nil {
			log.Printf("Error: %v\n", err)
		} else {
			log.Println("File downloaded successfully")
		}
		if strings.HasSuffix(update.Message.Document.FileName, ".torrent") {
			downloadTorrent(update.Message.Document.FileName, update)
		}
	}
	return tgbotapi.MessageConfig{}
}

func startHandler(update tgbotapi.Update) tgbotapi.MessageConfig {
	return tgbotapi.NewMessage(update.Message.Chat.ID, "/upload [URL]")
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

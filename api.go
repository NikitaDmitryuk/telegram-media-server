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

	if userExists, err := checkUser(update.Message.From.ID); err != nil {
		msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Произошла ошибка при проверке пользователя. Пожалуйста, попробуйте позже.")
		log.Printf("Error checking user: %v", err)
	} else if !userExists {
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
		case "rm":
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
			msg = "Неизвестный тип файла"
			log.Printf("Unknown file type")
			break
		}
		log.Printf("Получен файл с ID: %s\n", fileID)
		switch {
		case strings.HasSuffix(fileName, ".torrent"):
			err := downloadFile(fileID, fileName)
			if err != nil {
				log.Printf("Ошибка: %v\n", err)
				msg = "Произошла ошибка при загрузке файла. Пожалуйста, попробуйте снова."
			} else {
				log.Println("Файл успешно загружен")
				if exists, err := movieExistsTorrent(fileName); err != nil {
					log.Printf("Ошибка при проверке существования фильма: %v", err)
					msg = "Произошла ошибка при проверке существования файла. Пожалуйста, попробуйте снова."
				} else if !exists {
					downloadTorrent(fileName, update)
					msg = "Загрузка началась!"
				} else {
					msg = "Файл уже существует"
				}
			}
		case strings.HasSuffix(fileName, ".mp4"):
			err := downloadFile(fileID, fileName)
			if err != nil {
				log.Printf("Ошибка: %v\n", err)
				msg = "Произошла ошибка при загрузке файла. Пожалуйста, попробуйте снова."
			} else {
				log.Println("Файл успешно загружен")
				addMovie(fileName, fileName, "")
				setLoaded(fileName)
				updateDownloadedPercentage(fileName, 100)
				msg = "Файл успешно добавлен"
			}
		}
	}
	return tgbotapi.NewMessage(update.Message.Chat.ID, msg)
}

func startHandler(update tgbotapi.Update) tgbotapi.MessageConfig {
	return tgbotapi.NewMessage(update.Message.Chat.ID, "/list - получить список файлов\n/rm <ID> - удалить фильм\n")
}

func listHandler(update tgbotapi.Update) tgbotapi.MessageConfig {
	movies, err := getMovieList()
	if err != nil {
		log.Printf("Ошибка при получении списка фильмов: %v", err)
		return tgbotapi.NewMessage(update.Message.Chat.ID, "Произошла ошибка при получении списка фильмов. Пожалуйста, попробуйте позже.")
	}

	var msg string
	for _, movie := range movies {
		msg += fmt.Sprintf("ID: %d\nНазвание: %s\nЗагружено: %t\nПроцент загрузки: %d%%\n\n", movie.ID, movie.Name, movie.Downloaded, movie.DownloadedPercentage)
	}
	if len(msg) == 0 {
		msg = "Список пуст"
	}
	return tgbotapi.NewMessage(update.Message.Chat.ID, msg)
}

func loginHandler(update tgbotapi.Update) tgbotapi.MessageConfig {
	textFields := strings.Fields(update.Message.Text)
	var msgText string

	if len(textFields) != 2 {
		msgText = "Неверный формат команды. Используйте: /login [PASSWORD]"
	} else {
		if success, err := login(textFields[1], update.Message.Chat.ID, update.Message.From.UserName); err != nil {
			log.Printf("Ошибка при входе: %v", err)
			msgText = "Произошла ошибка при входе. Пожалуйста, попробуйте снова."
		} else if success {
			msgText = "Вход выполнен успешно"
		} else {
			msgText = "Неверный пароль"
		}
	}

	return tgbotapi.NewMessage(update.Message.Chat.ID, msgText)
}

func unknownCommandHandler(update tgbotapi.Update) tgbotapi.MessageConfig {
	return tgbotapi.NewMessage(update.Message.Chat.ID, "Неизвестная команда. Используйте /start для получения списка доступных команд.")
}

func unknownUserHandler(update tgbotapi.Update) tgbotapi.MessageConfig {
	return tgbotapi.NewMessage(update.Message.Chat.ID, "Выполните вход с помощью команды /login [PASSWORD]")
}

func deleteHandler(update tgbotapi.Update) tgbotapi.MessageConfig {
	commandID := strings.Split(update.Message.Text, " ")
	var msg string
	if len(commandID) != 2 {
		return startHandler(update)
	} else {
		id, err := strconv.Atoi(commandID[1])
		if err != nil {
			msg = "Неверный ID"
		} else if exists, err := movieExistsId(id); err != nil {
			log.Printf("Ошибка при проверке существования фильма: %v", err)
			msg = "Произошла ошибка при проверке существования файла. Пожалуйста, попробуйте снова."
		} else if exists {
			msg = deleteMovie(id)
		} else {
			msg = "Неверный ID файла"
		}
	}
	return tgbotapi.NewMessage(update.Message.Chat.ID, msg)
}

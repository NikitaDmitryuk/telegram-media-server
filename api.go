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
	if isYouTubeVideoLink(update.Message.Text) {
		go downloadYouTubeVideo(update)
		msg = "Видео скачивается с youtube"
	}
	if update.Message.IsCommand() {
		switch update.Message.Command() {
		case "start":
			return startHandler(update)
		case "ls":
			return listHandler(update)
		case "rm":
			return deleteHandler(update)
		case "stop":
			return stopHandler(update)
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
				msg = "Произошла ошибка при загрузке файла (загрузка файлов больше 50МБ недоступна)"
			} else {
				log.Println("Файл успешно загружен")
				if exists, err := movieExistsTorrent(fileName); err != nil {
					log.Printf("Ошибка при проверке существования фильма: %v", err)
					msg = "Произошла ошибка при проверке существования файла. Пожалуйста, попробуйте снова."
				} else if !exists {
					err := downloadTorrent(fileName, update)
					if err == nil {
						msg = "Загрузка началась!"
					} else {
						msg = "Ошибка при загрузке торрет файла (возможно не хватает места на диске)"
					}
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
				id := addMovie(fileName, fileName, "")
				setLoaded(id)
				updateDownloadedPercentage(id, 100)
				msg = "Файл успешно добавлен"
			}
		}
	}
	return tgbotapi.NewMessage(update.Message.Chat.ID, msg)
}

func startHandler(update tgbotapi.Update) tgbotapi.MessageConfig {
	return tgbotapi.NewMessage(update.Message.Chat.ID, "/ls - получить список файлов\n/rm <ID> - удалить фильм\n/stop - остановить все загрузки торрентов")
}

func stopHandler(update tgbotapi.Update) tgbotapi.MessageConfig {
	go stopTorrentDownload()
	return tgbotapi.NewMessage(update.Message.Chat.ID, "Все загрузки торрентов остановлены!")
}

func listHandler(update tgbotapi.Update) tgbotapi.MessageConfig {
	movies, err := getMovieList()
	if err != nil {
		log.Printf("Ошибка при получении списка фильмов: %v", err)
		return tgbotapi.NewMessage(update.Message.Chat.ID, "Произошла ошибка при получении списка фильмов. Пожалуйста, попробуйте позже.")
	}

	var msg string
	for _, movie := range movies {
		if movie.Downloaded {
			msg += fmt.Sprintf("ID: %d\nНазвание: %s\nЗагружено: Да\n\n", movie.ID, movie.Name)
		} else {
			msg += fmt.Sprintf("ID: %d\nНазвание: %s\nПроцент загрузки: %d%%\n\n", movie.ID, movie.Name, movie.DownloadedPercentage)
		}
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
		if commandID[1] == "all" {
			movies, _ := getMovieList()
			msg = "Все видео удалены"
			for _, movie := range movies {
				err := deleteMovie(movie.ID)
				if err != nil {
					msg = err.Error()
				}
			}
		} else {
			id, err := strconv.Atoi(commandID[1])
			if err != nil {
				msg = "Неверный ID"
			} else if exists, err := movieExistsId(id); err != nil {
				log.Printf("Ошибка при проверке существования фильма: %v", err)
				msg = "Произошла ошибка при проверке существования файла. Пожалуйста, попробуйте снова."
			} else if exists {
				err := deleteMovie(id)
				if err != nil {
					msg = err.Error()
				} else {
					msg = "Фильм успешно удален"
				}

			} else {
				msg = "Неверный ID файла"
			}

		}
	}
	return tgbotapi.NewMessage(update.Message.Chat.ID, msg)
}

func sendErrorMessage(chatID int64, message string) {
	text := "Ошибка: " + message
	log.Printf(text)
	msg := tgbotapi.NewMessage(chatID, text)
	GlobalBot.Send(msg)
}

func sendSuccessMessage(chatID int64, message string) {
	msg := tgbotapi.NewMessage(chatID, message)
	GlobalBot.Send(msg)
}

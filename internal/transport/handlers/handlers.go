package handlers

import (
	"context"
	"fmt"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// FullDownloadPercentage процент полной загрузки
	FullDownloadPercentage = 100
)

// SimpleHandler - простой обработчик для базовых команд
func HandleCommand(bot domain.BotInterface, update *tgbotapi.Update, db database.Database, _ *domain.Config) {
	if update.Message == nil {
		return
	}

	command := update.Message.Command()
	chatID := update.Message.Chat.ID

	switch command {
	case "start":
		// Создаем меню с кнопками
		keyboard := createMainMenuKeyboard()
		bot.SendMessage(chatID, lang.Translate("general.commands.start", nil), keyboard)
	case "ls":
		movies, err := db.GetMovieList(context.Background())
		if err != nil {
			bot.SendMessage(chatID, lang.Translate("error.movies.fetch_error", nil), nil)
			return
		}

		if len(movies) == 0 {
			bot.SendMessage(chatID, lang.Translate("general.status_messages.empty_list", nil), nil)
			return
		}

		message := lang.Translate("general.movies.list_header", nil)
		for i, movie := range movies {
			status := lang.Translate("general.movies.status_pending", nil)
			if movie.DownloadedPercentage == FullDownloadPercentage {
				status = lang.Translate("general.movies.status_completed", nil)
			} else if movie.DownloadedPercentage > 0 {
				status = lang.Translate("general.movies.status_downloading", nil)
			}
			message += fmt.Sprintf("%d. %s %s (%d%%)\n", i+1, status, movie.Name, movie.DownloadedPercentage)
		}

		bot.SendMessage(chatID, message, nil)
	default:
		bot.SendMessage(chatID, lang.Translate("error.commands.unknown_command", nil), nil)
	}
}

// createMainMenuKeyboard создает основное меню с кнопками
func createMainMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(lang.Translate("general.interface.list_movies", nil)),
			tgbotapi.NewKeyboardButton(lang.Translate("general.interface.delete_movie", nil)),
			tgbotapi.NewKeyboardButton(lang.Translate("general.interface.search_torrents", nil)),
		),
	)
}

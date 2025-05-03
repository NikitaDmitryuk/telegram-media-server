package handlers

import (
	"strconv"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func SendMainMenu(bot *tmsbot.Bot, chatID int64, message string) {
	buttons := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(lang.Translate("general.interface.list_movies", nil)),
			tgbotapi.NewKeyboardButton(lang.Translate("general.interface.delete_movie", nil)),
		),
	)

	buttons.OneTimeKeyboard = false
	buttons.ResizeKeyboard = true

	msg := tgbotapi.NewMessage(chatID, message)
	msg.ReplyMarkup = buttons
	bot.Send(msg)
}

func CreateDeleteMovieMenuMarkup(movies []tmsdb.Movie) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, movie := range movies {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(movie.Name, "delete_movie:"+strconv.Itoa(int(movie.ID))),
		))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(lang.Translate("general.interface.cancel", nil), "cancel_delete_menu"),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func SendDeleteMovieMenu(bot *tmsbot.Bot, chatID int64, movies []tmsdb.Movie) {
	if len(movies) == 0 {
		bot.SendSuccessMessage(chatID, lang.Translate("general.user_prompts.no_movies_to_delete", nil))
		return
	}

	buttons := CreateDeleteMovieMenuMarkup(movies)
	msg := tgbotapi.NewMessage(chatID, lang.Translate("general.user_prompts.delete_prompt", nil))
	msg.ReplyMarkup = buttons
	bot.Send(msg)
}

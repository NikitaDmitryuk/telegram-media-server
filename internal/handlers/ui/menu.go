package ui

import (
	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func SendMainMenu(bot *tmsbot.Bot, chatID int64, message string) {
	buttons := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(tmslang.Translate("general.interface.list_movies", nil)),
			tgbotapi.NewKeyboardButton(tmslang.Translate("general.interface.delete_movie", nil)),
			tgbotapi.NewKeyboardButton(tmslang.Translate("general.interface.search_torrents", nil)),
		),
	)

	buttons.OneTimeKeyboard = false
	buttons.ResizeKeyboard = true

	msg := tgbotapi.NewMessage(chatID, message)
	msg.ReplyMarkup = buttons
	bot.Send(msg)
}

func SendMainMenuNoText(bot *tmsbot.Bot, chatID int64) {
	buttons := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(tmslang.Translate("general.interface.list_movies", nil)),
			tgbotapi.NewKeyboardButton(tmslang.Translate("general.interface.delete_movie", nil)),
			tgbotapi.NewKeyboardButton(tmslang.Translate("general.interface.search_torrents", nil)),
		),
	)
	buttons.OneTimeKeyboard = false
	buttons.ResizeKeyboard = true

	msg := tgbotapi.NewMessage(chatID, tmslang.Translate("general.interface.main_menu", nil))
	msg.ReplyMarkup = buttons
	bot.Send(msg)
}

package ui

import (
	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func SendMainMenu(bot *tmsbot.Bot, chatID int64, message string) {
	bot.SendMessage(chatID, message, GetMainMenuKeyboard())
}

func SendMainMenuNoText(bot *tmsbot.Bot, chatID int64) {
	bot.SendMessage(chatID, tmslang.Translate("general.interface.main_menu", nil), GetMainMenuKeyboard())
}

func GetMainMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(tmslang.Translate("general.interface.list_movies", nil)),
			tgbotapi.NewKeyboardButton(tmslang.Translate("general.interface.delete_movie", nil)),
			tgbotapi.NewKeyboardButton(tmslang.Translate("general.interface.search_torrents", nil)),
		),
	)
}

func GetEmptyKeyboard() tgbotapi.ReplyKeyboardRemove {
	return tgbotapi.NewRemoveKeyboard(true)
}

func GetTorrentSearchKeyboard(hasMore bool) tgbotapi.ReplyKeyboardMarkup {
	var menuBtns []tgbotapi.KeyboardButton
	if hasMore {
		menuBtns = append(menuBtns, tgbotapi.NewKeyboardButton(tmslang.Translate("general.torrent_search.more", nil)))
	}
	menuBtns = append(menuBtns, tgbotapi.NewKeyboardButton(tmslang.Translate("general.torrent_search.cancel", nil)))
	menu := tgbotapi.NewReplyKeyboard(menuBtns)
	menu.OneTimeKeyboard = true
	menu.ResizeKeyboard = true
	return menu
}

package handlers

import (
	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func Router(bot *tmsbot.Bot, update tgbotapi.Update) {
	if update.CallbackQuery != nil {
		HandleCallbackQuery(bot, update)
		return
	}

	if update.Message == nil {
		return
	}

	LoggingMiddleware(update)

	if update.Message.IsCommand() {
		handleCommand(bot, update)
		return
	}

	if !checkAccess(bot, update) {
		bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.UnknownUserMsgID))
		return
	}

	handleMessage(bot, update)
}

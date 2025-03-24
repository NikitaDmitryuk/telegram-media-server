package handlers

import (
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tmsutils "github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

func Router(bot *tmsbot.Bot, update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	LoggingMiddleware(update)

	if update.Message.IsCommand() {
		switch update.Message.Command() {
		case "login":
			LoginHandler(bot, update)
		default:
			if !AuthMiddleware(bot, update) {
				return
			}
			switch update.Message.Command() {
			case "start":
				StartHandler(bot, update)
			case "ls":
				ListMoviesHandler(bot, update)
			case "rm":
				DeleteMoviesHandler(bot, update)
			default:
				logrus.Warnf("Unknown command: %s", update.Message.Command())
				bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.UnknownCommandMsgID))
			}
		}
		return
	}

	if !AuthMiddleware(bot, update) {
		return
	}

	if tmsutils.IsValidLink(update.Message.Text) {
		HandleDownloadLink(bot, update)
	} else if doc := update.Message.Document; doc != nil && strings.HasSuffix(doc.FileName, ".torrent") {
		HandleTorrentFile(bot, update)
	} else {
		logrus.Warn("Invalid input received")
		bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.UnknownCommandMsgID))
	}
}

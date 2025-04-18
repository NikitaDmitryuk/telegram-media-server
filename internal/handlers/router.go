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
	if update.CallbackQuery != nil {
		HandleCallbackQuery(bot, update)
		return
	}

	if update.Message == nil {
		return
	}

	LoggingMiddleware(update)

	if update.Message.IsCommand() {
		command := strings.ToLower(update.Message.Command())
		switch command {
		case "login":
			LoginHandler(bot, update)
		case "start":
			if !AuthMiddleware(bot, update) {
				return
			}
			StartHandler(bot, update)
		case "ls", "rm":
			if !AuthMiddleware(bot, update) {
				return
			}
			handleBasicCommands(bot, update, command)
		default:
			logrus.Warnf("Unknown command: %s", command)
			bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.UnknownCommandMsgID))
		}
		return
	}

	if !AuthMiddleware(bot, update) {
		return
	}

	if tmsutils.IsValidLink(update.Message.Text) {
		HandleDownloadLink(bot, update)
		return
	} else if doc := update.Message.Document; doc != nil {
		if strings.HasSuffix(doc.FileName, ".torrent") {
			HandleTorrentFile(bot, update)
		} else {
			logrus.Warnf("Unsupported document type: %s", doc.FileName)
			bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.UnsupportedFileTypeMsgID))
		}
		return
	}

	args := strings.Fields(strings.ToLower(update.Message.Text))
	if len(args) == 0 {
		logrus.Warn("Empty message received")
		bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.UnknownCommandMsgID))
		return
	}

	command := args[0]
	switch command {
	case "ls", "rm":
		handleBasicCommands(bot, update, command)
	default:
		logrus.Warnf("Unknown command or message: %s", update.Message.Text)
		bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.UnknownCommandMsgID))
	}
}

func handleBasicCommands(bot *tmsbot.Bot, update tgbotapi.Update, command string) {
	switch command {
	case "ls":
		ListMoviesHandler(bot, update)
	case "rm":
		DeleteMoviesHandler(bot, update)
	}
}

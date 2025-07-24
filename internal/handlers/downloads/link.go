package downloads

import (
	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	video "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/video"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func HandleDownloadLink(bot *tmsbot.Bot, update *tgbotapi.Update) {
	message := update.Message
	chatID := message.Chat.ID

	logutils.Log.WithField("link", message.Text).Info("Starting download for a valid link")
	downloaderInstance := video.NewYTDLPDownloader(bot, message.Text)

	if downloaderInstance == nil {
		logutils.Log.Warn("Failed to initialize downloader for the provided link")
		bot.SendMessage(chatID, lang.Translate("error.file_management.unsupported_type", nil), tgbotapi.NewRemoveKeyboard(true))
		return
	}

	HandleDownload(bot, chatID, downloaderInstance)
}

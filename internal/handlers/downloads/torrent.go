package downloads

import (
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	torrent "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/torrent"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func HandleTorrentFile(bot *tmsbot.Bot, update *tgbotapi.Update) {
	message := update.Message
	chatID := message.Chat.ID
	doc := message.Document

	if !strings.HasSuffix(doc.FileName, ".torrent") {
		logutils.Log.Warnf("Unsupported file type: %s", doc.FileName)
		bot.SendErrorMessage(chatID, lang.Translate("error.file_management.unsupported_type", nil))
		return
	}

	logutils.Log.WithField("file_name", doc.FileName).Info("Received a torrent file")

	if err := bot.DownloadFile(doc.FileID, doc.FileName); err != nil {
		logutils.Log.WithError(err).Error("Failed to download torrent file")
		bot.SendErrorMessage(chatID, lang.Translate("error.downloads.document_download_error", nil))
		return
	}

	downloaderInstance := torrent.NewAria2Downloader(bot, doc.FileName)

	if downloaderInstance == nil {
		logutils.Log.Warn("Failed to initialize torrent downloader")
		bot.SendErrorMessage(chatID, lang.Translate("error.file_management.unsupported_type", nil))
		return
	}

	handleDownload(bot, chatID, downloaderInstance)
}

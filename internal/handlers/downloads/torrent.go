package downloads

import (
	"strings"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	tmsfactory "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/factory"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func HandleTorrentFile(
	a *app.App,
	update *tgbotapi.Update,
) {
	message := update.Message
	chatID := message.Chat.ID
	doc := message.Document

	if !strings.HasSuffix(doc.FileName, ".torrent") {
		logutils.Log.Warnf("Unsupported file type: %s", doc.FileName)
		a.Bot.SendMessage(chatID, lang.Translate("error.file_management.unsupported_type", nil), tgbotapi.NewRemoveKeyboard(false))
		return
	}

	logutils.Log.WithField("file_name", doc.FileName).Info("Received a torrent file")

	if err := a.Bot.DownloadFile(doc.FileID, doc.FileName); err != nil {
		logutils.Log.WithError(err).Error("Failed to download torrent file")
		a.Bot.SendMessage(chatID, lang.Translate("error.downloads.document_download_error", nil), tgbotapi.NewRemoveKeyboard(false))
		return
	}

	downloaderInstance := tmsfactory.NewTorrentDownloader(doc.FileName, a.Config.MoviePath, a.Config)

	if downloaderInstance == nil {
		logutils.Log.Warn("Failed to initialize torrent downloader")
		a.Bot.SendMessage(chatID, lang.Translate("error.file_management.unsupported_type", nil), tgbotapi.NewRemoveKeyboard(false))
		return
	}

	HandleDownload(a, chatID, downloaderInstance)
}

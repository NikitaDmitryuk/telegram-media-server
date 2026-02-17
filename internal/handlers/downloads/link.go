package downloads

import (
	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	tmsfactory "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/factory"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func HandleDownloadLink(
	a *app.App,
	update *tgbotapi.Update,
) {
	message := update.Message
	chatID := message.Chat.ID

	logutils.Log.WithField("link", message.Text).Info("Starting download for a valid link")
	downloaderInstance := tmsfactory.NewVideoDownloader(message.Text, a.Config)

	HandleDownload(a, chatID, downloaderInstance)
}

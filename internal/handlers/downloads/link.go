package downloads

import (
	"context"

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
	ctx := context.Background()
	downloaderInstance, err := tmsfactory.CreateDownloaderFromURL(ctx, message.Text, a.Config.MoviePath, a.Config)
	if err != nil {
		logutils.Log.WithError(err).WithField("link", message.Text).Debug("HandleDownloadLink: CreateDownloaderFromURL failed")
		sendDownloadStartError(a, chatID, err, nil)
		return
	}
	HandleDownload(a, chatID, downloaderInstance)
}

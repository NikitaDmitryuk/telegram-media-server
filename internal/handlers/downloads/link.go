package downloads

import (
	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	video "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/video"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func HandleDownloadLink(
	bot *tmsbot.Bot,
	update *tgbotapi.Update,
	config *tmsconfig.Config,
	db tmsdb.Database,
	downloadManager *tmsdmanager.DownloadManager,
) {
	message := update.Message
	chatID := message.Chat.ID

	logutils.Log.WithField("link", message.Text).Info("Starting download for a valid link")
	downloaderInstance := video.NewYTDLPDownloader(bot, message.Text, config)

	HandleDownload(bot, chatID, downloaderInstance, config, db, downloadManager)
}

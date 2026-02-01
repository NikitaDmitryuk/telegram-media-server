package downloads

import (
	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

var notificationBot *tmsbot.Bot

func InitNotificationHandler(bot *tmsbot.Bot, downloadManager *tmsdmanager.DownloadManager) {
	notificationBot = bot
	go handleQueueNotifications(downloadManager)
}

func handleQueueNotifications(downloadManager *tmsdmanager.DownloadManager) {
	notificationChan := downloadManager.GetNotificationChan()

	for notification := range notificationChan {
		switch notification.Type {
		case "queued":
			sendQueuedNotification(&notification)
		case "started":
			sendStartedNotification(&notification)
		case "first_episode_ready":
			sendFirstEpisodeReadyNotification(&notification)
		default:
			logutils.Log.WithField("type", notification.Type).Warn("Unknown notification type")
		}
	}
}

func sendFirstEpisodeReadyNotification(notification *tmsdmanager.QueueNotification) {
	message := lang.Translate("general.first_episode_ready", map[string]any{
		"Title": notification.Title,
	})
	notificationBot.SendMessage(notification.ChatID, message, nil)
	logutils.Log.WithFields(map[string]any{
		"chat_id":  notification.ChatID,
		"movie_id": notification.MovieID,
		"title":    notification.Title,
	}).Info("Sent first episode ready notification")
}

func sendQueuedNotification(notification *tmsdmanager.QueueNotification) {
	message := lang.Translate("general.download_queued", map[string]any{
		"Title":         notification.Title,
		"Position":      notification.Position,
		"MaxConcurrent": notification.MaxConcurrent,
	})

	notificationBot.SendMessage(notification.ChatID, message, nil)

	logutils.Log.WithFields(map[string]any{
		"chat_id":  notification.ChatID,
		"movie_id": notification.MovieID,
		"title":    notification.Title,
		"position": notification.Position,
	}).Info("Sent queued notification")
}

func sendStartedNotification(notification *tmsdmanager.QueueNotification) {
	message := lang.Translate("general.download_started_from_queue", map[string]any{
		"Title": notification.Title,
	})

	notificationBot.SendMessage(notification.ChatID, message, nil)

	logutils.Log.WithFields(map[string]any{
		"chat_id":  notification.ChatID,
		"movie_id": notification.MovieID,
		"title":    notification.Title,
	}).Info("Sent started notification")
}

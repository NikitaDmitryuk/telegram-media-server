package downloads

import (
	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

func InitNotificationHandler(a *app.App) {
	go handleQueueNotifications(a.Bot, a.DownloadManager)
}

func handleQueueNotifications(bot tmsbot.Service, dm tmsdmanager.Service) {
	notificationChan := dm.GetNotificationChan()

	for notification := range notificationChan {
		switch notification.Type {
		case "queued":
			sendQueuedNotification(bot, &notification)
		case "started":
			sendStartedNotification(bot, &notification)
		case "first_episode_ready":
			sendFirstEpisodeReadyNotification(bot, &notification)
		case "video_not_supported":
			sendVideoNotSupportedNotification(bot, &notification)
		default:
			logutils.Log.WithField("type", notification.Type).Warn("Unknown notification type")
		}
	}
}

func sendFirstEpisodeReadyNotification(bot tmsbot.Service, notification *tmsdmanager.QueueNotification) {
	message := lang.Translate("general.first_episode_ready", map[string]any{
		"Title": notification.Title,
	})
	bot.SendMessage(notification.ChatID, message, nil)
	logutils.Log.WithFields(map[string]any{
		"chat_id":  notification.ChatID,
		"movie_id": notification.MovieID,
		"title":    notification.Title,
	}).Info("Sent first episode ready notification")
}

func sendVideoNotSupportedNotification(bot tmsbot.Service, notification *tmsdmanager.QueueNotification) {
	message := lang.Translate("general.video_not_supported", map[string]any{
		"Title": notification.Title,
	})
	bot.SendMessage(notification.ChatID, message, nil)
	logutils.Log.WithFields(map[string]any{
		"chat_id":  notification.ChatID,
		"movie_id": notification.MovieID,
		"title":    notification.Title,
	}).Info("Sent video not supported notification")
}

func sendQueuedNotification(bot tmsbot.Service, notification *tmsdmanager.QueueNotification) {
	message := lang.Translate("general.download_queued", map[string]any{
		"Title":         notification.Title,
		"Position":      notification.Position,
		"MaxConcurrent": notification.MaxConcurrent,
	})

	bot.SendMessage(notification.ChatID, message, nil)

	logutils.Log.WithFields(map[string]any{
		"chat_id":  notification.ChatID,
		"movie_id": notification.MovieID,
		"title":    notification.Title,
		"position": notification.Position,
	}).Info("Sent queued notification")
}

func sendStartedNotification(bot tmsbot.Service, notification *tmsdmanager.QueueNotification) {
	message := lang.Translate("general.download_started_from_queue", map[string]any{
		"Title": notification.Title,
	})

	bot.SendMessage(notification.ChatID, message, nil)

	logutils.Log.WithFields(map[string]any{
		"chat_id":  notification.ChatID,
		"movie_id": notification.MovieID,
		"title":    notification.Title,
	}).Info("Sent started notification")
}

package manager

import (
	"fmt"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

func (dm *DownloadManager) processQueue() {
	ticker := time.NewTicker(QueueProcessingDelay)
	defer ticker.Stop()

	for range ticker.C {
		dm.queueMutex.Lock()
		if len(dm.queue) == 0 {
			dm.queueMutex.Unlock()
			continue
		}

		select {
		case dm.semaphore <- struct{}{}:
			queued := dm.queue[0]
			dm.queue = dm.queue[1:]
			dm.queueMutex.Unlock()

			dm.startQueuedDownload(queued)
		default:
			dm.queueMutex.Unlock()
		}
	}
}

func (dm *DownloadManager) startQueuedDownload(queued queuedDownload) {
	logutils.Log.WithFields(map[string]any{
		"movie_id": queued.movieID,
		"title":    queued.title,
	}).Info("Starting queued download")

	dm.notificationChan <- QueueNotification{
		Type:          "started",
		ChatID:        queued.chatID,
		MovieID:       queued.movieID,
		Title:         queued.title,
		MaxConcurrent: dm.downloadSettings.MaxConcurrentDownloads,
	}

	movieID, progressChan, outerErrChan, err := dm.startDownloadImmediately(
		queued.movieID,
		queued.downloader,
		queued.title,
	)
	if err != nil {
		logutils.Log.WithError(err).WithFields(map[string]any{
			"movie_id": queued.movieID,
			"title":    queued.title,
		}).Error("Failed to start queued download")

		select {
		case outerErrChan <- err:
		default:
		}
		<-dm.semaphore
	} else {
		logutils.Log.WithFields(map[string]any{
			"movie_id": movieID,
			"title":    queued.title,
		}).Info("Successfully started queued download")

		_ = progressChan
	}
}

func (dm *DownloadManager) addToQueue(
	movieID uint,
	dl downloader.Downloader,
	movieTitle string,
	chatID int64,
) (movieIDOut uint, progressChan chan float64, outerErrChan chan error) {
	progressChan = make(chan float64, 1)
	outerErrChan = make(chan error, 1)

	dm.queueMutex.Lock()
	dm.queue = append(dm.queue, queuedDownload{
		downloader: dl,
		movieID:    movieID,
		title:      movieTitle,
		addedAt:    time.Now(),
		chatID:     chatID,
	})
	queuePosition := len(dm.queue)
	dm.queueMutex.Unlock()

	logutils.Log.WithFields(map[string]any{
		"movie_id": movieID,
		"title":    movieTitle,
		"position": queuePosition,
	}).Info("Added download to queue")

	estimatedWaitTime := dm.calculateEstimatedWaitTime(queuePosition)

	dm.notificationChan <- QueueNotification{
		Type:          "queued",
		ChatID:        chatID,
		MovieID:       movieID,
		Title:         movieTitle,
		Position:      queuePosition,
		WaitTime:      estimatedWaitTime,
		MaxConcurrent: dm.downloadSettings.MaxConcurrentDownloads,
	}

	go func() {
		defer close(progressChan)
		defer close(outerErrChan)

		updateTicker := time.NewTicker(QueueProgressUpdateInterval)
		defer updateTicker.Stop()

		for {
			select {
			case <-updateTicker.C:
				dm.queueMutex.Lock()
				currentPosition := -1
				for i, item := range dm.queue {
					if item.movieID == movieID {
						currentPosition = i + 1
						break
					}
				}
				dm.queueMutex.Unlock()

				if currentPosition == -1 {
					return
				}

			case <-time.After(dm.downloadSettings.DownloadTimeout):
				dm.queueMutex.Lock()
				for i, item := range dm.queue {
					if item.movieID == movieID {
						dm.queue = append(dm.queue[:i], dm.queue[i+1:]...)
						break
					}
				}
				dm.queueMutex.Unlock()

				outerErrChan <- fmt.Errorf("download timeout in queue")
				return
			}
		}
	}()

	return movieID, progressChan, outerErrChan
}

func (dm *DownloadManager) calculateEstimatedWaitTime(position int) string {
	if position <= 0 {
		return "Starting soon"
	}

	avgDownloadTime := 30 * time.Minute
	estimatedMinutes := time.Duration(position) * avgDownloadTime / time.Duration(dm.downloadSettings.MaxConcurrentDownloads)

	if estimatedMinutes < time.Minute {
		return "Less than 1 minute"
	} else if estimatedMinutes < time.Hour {
		return fmt.Sprintf("~%d minutes", int(estimatedMinutes.Minutes()))
	}

	hours := int(estimatedMinutes.Hours())
	minutes := int(estimatedMinutes.Minutes()) % 60
	if minutes == 0 {
		return fmt.Sprintf("~%d hours", hours)
	}
	return fmt.Sprintf("~%d hours %d minutes", hours, minutes)
}

func (dm *DownloadManager) GetQueueCount() int {
	dm.queueMutex.Lock()
	defer dm.queueMutex.Unlock()
	return len(dm.queue)
}

func (dm *DownloadManager) GetQueueItems() []map[string]any {
	dm.queueMutex.Lock()
	defer dm.queueMutex.Unlock()

	items := make([]map[string]any, len(dm.queue))
	for i, item := range dm.queue {
		items[i] = map[string]any{
			"movie_id": item.movieID,
			"title":    item.title,
			"position": i + 1,
			"added_at": item.addedAt,
		}
	}
	return items
}

func (dm *DownloadManager) RemoveFromQueue(movieID uint) bool {
	dm.queueMutex.Lock()
	defer dm.queueMutex.Unlock()

	for i, item := range dm.queue {
		if item.movieID == movieID {
			dm.queue = append(dm.queue[:i], dm.queue[i+1:]...)
			return true
		}
	}
	return false
}

func (dm *DownloadManager) GetTotalDownloads() int {
	return dm.GetDownloadCount() + dm.GetQueueCount()
}

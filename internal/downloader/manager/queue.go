package manager

import (
	"fmt"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/notifier"
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

			dm.startQueuedDownload(&queued)
		default:
			dm.queueMutex.Unlock()
		}
	}
}

func (dm *DownloadManager) startQueuedDownload(queued *queuedDownload) {
	logutils.Log.WithFields(map[string]any{
		"movie_id": queued.movieID,
		"title":    queued.title,
	}).Info("Starting queued download")

	queued.queueNotifier.OnStarted(queued.movieID, queued.title)

	_, innerProgressChan, innerErrChan, err := dm.startDownloadImmediately(
		queued.movieID,
		queued.downloader,
		queued.title,
		queued.queueNotifier,
	)
	if err != nil {
		logutils.Log.WithError(err).WithFields(map[string]any{
			"movie_id": queued.movieID,
			"title":    queued.title,
		}).Error("Failed to start queued download")

		// Forward error to the original caller's channel
		select {
		case queued.errChan <- err:
		default:
		}
		close(queued.progressChan)
		close(queued.errChan)
		<-dm.semaphore
		return
	}

	logutils.Log.WithFields(map[string]any{
		"movie_id": queued.movieID,
		"title":    queued.title,
	}).Info("Successfully started queued download")

	// Forward progress and errors from inner channels to the original caller's channels
	go func() {
		defer close(queued.progressChan)
		defer close(queued.errChan)

		// Use select to handle both channels concurrently
		// This prevents deadlock if errChan receives value before progressChan closes
		progressDone := false
		errDone := false

		for !progressDone || !errDone {
			select {
			case progress, ok := <-innerProgressChan:
				if !ok {
					progressDone = true
					continue
				}
				select {
				case queued.progressChan <- progress:
				default:
					// Drop progress if channel is full (non-blocking)
				}
			case innerErr, ok := <-innerErrChan:
				if !ok {
					errDone = true
					continue
				}
				queued.errChan <- innerErr
				errDone = true
			}
		}
	}()
}

func (dm *DownloadManager) addToQueue(
	movieID uint,
	dl downloader.Downloader,
	movieTitle string,
	queueNotifier notifier.QueueNotifier,
) (movieIDOut uint, progressChan chan float64, outerErrChan chan error) {
	// Create channels that will be used by the caller to track progress
	// These channels will be forwarded to when the download actually starts
	progressChan = make(chan float64, ProgressChannelBuffSize)
	outerErrChan = make(chan error, 1)

	dm.queueMutex.Lock()
	dm.queue = append(dm.queue, queuedDownload{
		downloader:    dl,
		movieID:       movieID,
		title:         movieTitle,
		addedAt:       time.Now(),
		queueNotifier: queueNotifier,
		progressChan:  progressChan,
		errChan:       outerErrChan,
	})
	queuePosition := len(dm.queue)
	dm.queueMutex.Unlock()

	logutils.Log.WithFields(map[string]any{
		"movie_id": movieID,
		"title":    movieTitle,
		"position": queuePosition,
	}).Info("Added download to queue")

	queueNotifier.OnQueued(movieID, movieTitle, queuePosition, dm.downloadSettings.MaxConcurrentDownloads)

	// Start a goroutine to monitor queue timeout only
	// Note: We do NOT close channels here - they will be closed by startQueuedDownload
	// when the download actually starts, or by the timeout handler
	go func() {
		if dm.downloadSettings.DownloadTimeout <= 0 {
			return
		}

		timeoutTimer := time.NewTimer(dm.downloadSettings.DownloadTimeout)
		defer timeoutTimer.Stop()

		<-timeoutTimer.C

		// Check if still in queue (not started yet)
		dm.queueMutex.Lock()
		stillInQueue := false
		for i, item := range dm.queue {
			if item.movieID == movieID {
				dm.queue = append(dm.queue[:i], dm.queue[i+1:]...)
				stillInQueue = true
				break
			}
		}
		dm.queueMutex.Unlock()

		if stillInQueue {
			logutils.Log.WithField("movie_id", movieID).Warn("Download timed out while in queue")
			select {
			case outerErrChan <- fmt.Errorf("download timeout in queue"):
			default:
			}
			close(progressChan)
			close(outerErrChan)
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

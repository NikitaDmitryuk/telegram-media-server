package manager

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
)

const (
	QueueProgressUpdateInterval = 5 * time.Second
	NotificationChannelSize     = 100
	QueueProcessingDelay        = 100 * time.Millisecond
)

var GlobalDownloadManager *DownloadManager

func InitDownloadManager() {
	GlobalDownloadManager = NewDownloadManager()
}

type DownloadManager struct {
	mu               sync.RWMutex
	jobs             map[uint]downloadJob
	queue            []queuedDownload
	semaphore        chan struct{}
	downloadSettings config.DownloadConfig
	queueMutex       sync.Mutex
	notificationChan chan QueueNotification
}

type downloadJob struct {
	downloader   downloader.Downloader
	startTime    time.Time
	progressChan chan float64
	errChan      chan error
	ctx          context.Context
	cancel       context.CancelFunc
}

type queuedDownload struct {
	downloader downloader.Downloader
	movieID    uint
	title      string
	addedAt    time.Time
	chatID     int64
}

type QueueNotification struct {
	Type          string
	ChatID        int64
	MovieID       uint
	Title         string
	Position      int
	WaitTime      string
	MaxConcurrent int
}

func NewDownloadManager() *DownloadManager {
	settings := config.GlobalConfig.GetDownloadSettings()
	dm := &DownloadManager{
		jobs:             make(map[uint]downloadJob),
		queue:            make([]queuedDownload, 0),
		semaphore:        make(chan struct{}, settings.MaxConcurrentDownloads),
		downloadSettings: settings,
		notificationChan: make(chan QueueNotification, NotificationChannelSize),
	}

	go dm.processQueue()
	return dm
}

func (dm *DownloadManager) processQueue() {
	for {
		time.Sleep(1 * time.Second)

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

			logutils.Log.WithFields(map[string]any{
				"movie_id": queued.movieID,
				"title":    queued.title,
				"action":   "process_queue_starting",
			}).Info("Starting download from queue")

			go dm.startQueuedDownload(queued)

			time.Sleep(QueueProcessingDelay)
		default:
			dm.queueMutex.Unlock()
		}
	}
}

func (dm *DownloadManager) startQueuedDownload(queued queuedDownload) {
	logutils.Log.WithFields(map[string]any{
		"movie_id":   queued.movieID,
		"title":      queued.title,
		"action":     "download_start_from_queue",
		"queue_time": time.Since(queued.addedAt).String(),
	}).Info("Starting download from queue")

	select {
	case dm.notificationChan <- QueueNotification{
		Type:     "started",
		ChatID:   queued.chatID,
		MovieID:  queued.movieID,
		Title:    queued.title,
		WaitTime: time.Since(queued.addedAt).String(),
	}:
	default:
		logutils.Log.Warn("Notification channel is full, dropping notification")
	}

	dm.mu.Lock()
	defer dm.mu.Unlock()

	var ctx context.Context
	var cancel context.CancelFunc

	if dm.downloadSettings.DownloadTimeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), dm.downloadSettings.DownloadTimeout)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}

	progressChan, errChan, err := queued.downloader.StartDownload(ctx)
	if err != nil {
		logutils.Log.WithError(err).Errorf("Failed to start queued download for movieID %d", queued.movieID)
		cancel()
		<-dm.semaphore
		return
	}

	job := downloadJob{
		downloader:   queued.downloader,
		startTime:    time.Now(),
		progressChan: progressChan,
		errChan:      errChan,
		ctx:          ctx,
		cancel:       cancel,
	}

	dm.jobs[queued.movieID] = job

	outerErrChan := make(chan error, 1)
	go dm.monitorDownload(queued.movieID, &job, outerErrChan)

	logutils.Log.WithFields(map[string]any{
		"movie_id": queued.movieID,
		"title":    queued.title,
		"action":   "start_queued_download_complete",
	}).Info("Started queued download, function returning")
}

func (dm *DownloadManager) StartDownload(
	dl downloader.Downloader,
	chatID int64,
) (
	movieID uint,
	progressChan chan float64,
	outerErrChan chan error,
	err error,
) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	movieTitle, err := dl.GetTitle()
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get video title")
		return 0, nil, nil, utils.WrapError(err, "failed to get video title", nil)
	}

	mainFiles, tempFiles, err := dl.GetFiles()
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get files")
		return 0, nil, nil, utils.WrapError(err, "failed to get files", nil)
	}

	movieID, err = database.GlobalDB.AddMovie(context.Background(), movieTitle, mainFiles, tempFiles)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to add movie to database")
		return 0, nil, nil, utils.WrapError(err, "failed to add movie to database", nil)
	}

	select {
	case dm.semaphore <- struct{}{}:
		return dm.startDownloadImmediately(movieID, dl, movieTitle)
	default:
		movieIDOut, progressChan, outerErrChan := dm.addToQueue(movieID, dl, movieTitle, chatID)
		return movieIDOut, progressChan, outerErrChan, nil
	}
}

func (dm *DownloadManager) startDownloadImmediately(
	movieID uint,
	dl downloader.Downloader,
	movieTitle string,
) (movieIDOut uint, progressChan chan float64, outerErrChan chan error, err error) {
	var ctx context.Context
	var cancel context.CancelFunc

	if dm.downloadSettings.DownloadTimeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), dm.downloadSettings.DownloadTimeout)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}

	progressChan, errChan, err := dl.StartDownload(ctx)
	if err != nil {
		logutils.Log.WithError(err).Errorf("Failed to start download for movieID %d", movieID)
		cancel()
		<-dm.semaphore
		return movieID, nil, nil, utils.WrapError(err, "failed to start download", map[string]any{
			"movie_id": movieID,
		})
	}

	job := downloadJob{
		downloader:   dl,
		startTime:    time.Now(),
		progressChan: progressChan,
		errChan:      errChan,
		ctx:          ctx,
		cancel:       cancel,
	}

	dm.jobs[movieID] = job

	logutils.Log.WithFields(map[string]any{
		"movie_id": movieID,
		"title":    movieTitle,
		"action":   "download_start",
	}).Info("Download started immediately")

	outerErrChan = make(chan error, 1)
	go dm.monitorDownload(movieID, &job, outerErrChan)

	return movieID, progressChan, outerErrChan, nil
}

func (dm *DownloadManager) addToQueue(
	movieID uint,
	dl downloader.Downloader,
	movieTitle string,
	chatID int64,
) (movieIDOut uint, progressChan chan float64, outerErrChan chan error) {
	queued := queuedDownload{
		downloader: dl,
		movieID:    movieID,
		title:      movieTitle,
		addedAt:    time.Now(),
		chatID:     chatID,
	}

	dm.queueMutex.Lock()
	dm.queue = append(dm.queue, queued)
	queuePosition := len(dm.queue)
	dm.queueMutex.Unlock()

	logutils.Log.WithFields(map[string]any{
		"movie_id":       movieID,
		"title":          movieTitle,
		"action":         "download_queued",
		"queue_position": queuePosition,
		"max_concurrent": dm.downloadSettings.MaxConcurrentDownloads,
	}).Info("Download added to queue")

	select {
	case dm.notificationChan <- QueueNotification{
		Type:          "queued",
		ChatID:        chatID,
		MovieID:       movieID,
		Title:         movieTitle,
		Position:      queuePosition,
		MaxConcurrent: dm.downloadSettings.MaxConcurrentDownloads,
	}:
	default:
		logutils.Log.Warn("Notification channel is full, dropping notification")
	}

	progressChan = make(chan float64, 1)
	outerErrChan = make(chan error, 1)

	go func() {
		defer close(progressChan)
		defer close(outerErrChan)

		for {
			time.Sleep(QueueProgressUpdateInterval)

			dm.queueMutex.Lock()
			found := false
			for _, q := range dm.queue {
				if q.movieID == movieID {
					progressChan <- 0.0
					found = true
					break
				}
			}
			dm.queueMutex.Unlock()

			if !found {
				dm.mu.RLock()
				_, isActive := dm.jobs[movieID]
				dm.mu.RUnlock()

				if isActive {
					progressChan <- 0.0
				} else {
					break
				}
			}
		}
	}()

	return movieID, progressChan, outerErrChan
}

func (dm *DownloadManager) monitorDownload(
	movieID uint,
	job *downloadJob,
	outerErrChan chan error,
) {
	defer func() {
		<-dm.semaphore
		close(outerErrChan)
	}()

	lastUpdate := time.Now()
	updateInterval := dm.downloadSettings.ProgressUpdateInterval

	for {
		select {
		case progress, ok := <-job.progressChan:
			if !ok {
				goto downloadComplete
			}

			if time.Since(lastUpdate) >= updateInterval {
				err := database.GlobalDB.UpdateDownloadedPercentage(context.Background(), movieID, int(math.Floor(progress)))
				if err != nil {
					logutils.Log.WithError(err).Warnf("Failed to update downloaded percentage for movieID %d", movieID)
				} else {
					logutils.Log.WithFields(map[string]any{
						"movie_id": movieID,
						"progress": progress,
						"action":   "download_progress",
					}).Debug("Download progress update")
				}
				lastUpdate = time.Now()
			}

		case <-job.ctx.Done():
			if job.ctx.Err() == context.DeadlineExceeded {
				logutils.Log.WithField("movie_id", movieID).Error("Download timeout exceeded")
				outerErrChan <- utils.WrapError(utils.ErrDownloadFailed, "download timeout exceeded", map[string]any{
					"movie_id": movieID,
					"timeout":  dm.downloadSettings.DownloadTimeout,
				})
			} else {
				logutils.Log.WithField("movie_id", movieID).Info("Download canceled")
				outerErrChan <- nil
			}
			goto cleanup

		case err := <-job.errChan:
			downloadDuration := time.Since(job.startTime)

			if err != nil {
				logutils.Log.WithError(err).WithFields(map[string]any{
					"movie_id": movieID,
					"title":    "unknown_title",
					"action":   "download_error",
				}).Error("Download failed")
				outerErrChan <- utils.WrapError(err, "download failed", map[string]any{
					"movie_id": movieID,
					"duration": downloadDuration,
				})
			} else if job.downloader.StoppedManually() {
				logutils.Log.WithField("movie_id", movieID).Info("Download was manually stopped")
				outerErrChan <- nil
			} else {
				logutils.Log.WithFields(map[string]any{
					"movie_id": movieID,
					"title":    "unknown_title",
					"duration": downloadDuration.String(),
					"action":   "download_complete",
				}).Info("Download completed successfully")

				err = database.GlobalDB.SetLoaded(context.Background(), movieID)
				if err != nil {
					logutils.Log.WithError(err).Warnf("Failed to set movie as loaded for movieID %d", movieID)
				}
				outerErrChan <- nil
			}
			goto cleanup
		}
	}

downloadComplete:
	select {
	case err := <-job.errChan:
		downloadDuration := time.Since(job.startTime)
		if err != nil {
			logutils.Log.WithError(err).WithFields(map[string]any{
				"movie_id": movieID,
				"title":    "unknown_title",
				"action":   "download_error",
			}).Error("Download failed")
			outerErrChan <- err
		} else {
			logutils.Log.WithFields(map[string]any{
				"movie_id": movieID,
				"title":    "unknown_title",
				"duration": downloadDuration.String(),
				"action":   "download_complete",
			}).Info("Download completed successfully")
			outerErrChan <- nil
		}
	default:
		downloadDuration := time.Since(job.startTime)
		logutils.Log.WithFields(map[string]any{
			"movie_id": movieID,
			"title":    "unknown_title",
			"duration": downloadDuration.String(),
			"action":   "download_complete",
		}).Info("Download completed successfully")
		outerErrChan <- nil
	}

cleanup:
	dm.mu.Lock()
	delete(dm.jobs, movieID)
	dm.mu.Unlock()
}

func (dm *DownloadManager) StopDownload(movieID uint) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if job, exists := dm.jobs[movieID]; exists {
		job.cancel()
		if err := job.downloader.StopDownload(); err != nil {
			logutils.Log.WithError(err).Errorf("Failed to stop download for movieID %d", movieID)
			return utils.WrapError(err, "failed to stop download", map[string]any{
				"movie_id": movieID,
			})
		}

		logutils.Log.WithField("movie_id", movieID).Info("Download stopped")
		delete(dm.jobs, movieID)
		return nil
	}

	if dm.RemoveFromQueue(movieID) {
		return nil
	}

	return utils.WrapError(utils.ErrDownloadFailed, "download not found", map[string]any{
		"movie_id": movieID,
	})
}

func (dm *DownloadManager) StopAllDownloads() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	for movieID, job := range dm.jobs {
		job.cancel()
		if err := job.downloader.StopDownload(); err != nil {
			logutils.Log.WithError(err).Errorf("Failed to stop download for movieID %d", movieID)
		} else {
			logutils.Log.WithField("movie_id", movieID).Info("Download stopped")
		}
		delete(dm.jobs, movieID)
	}
}

func (dm *DownloadManager) GetDownloadStatus(movieID uint) (isActive bool, progress float64, err error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	if _, exists := dm.jobs[movieID]; exists {
		movie, err := database.GlobalDB.GetMovieByID(context.Background(), movieID)
		if err != nil {
			return true, 0, err
		}
		return true, float64(movie.DownloadedPercentage), nil
	}

	return false, 0, nil
}

func (dm *DownloadManager) GetActiveDownloads() []uint {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	activeDownloads := make([]uint, 0, len(dm.jobs))
	for movieID := range dm.jobs {
		activeDownloads = append(activeDownloads, movieID)
	}
	return activeDownloads
}

func (dm *DownloadManager) GetDownloadCount() int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return len(dm.jobs)
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
	for i, queued := range dm.queue {
		items[i] = map[string]any{
			"movie_id":     queued.movieID,
			"title":        queued.title,
			"position":     i + 1,
			"added_at":     queued.addedAt,
			"waiting_time": time.Since(queued.addedAt).String(),
		}
	}
	return items
}

func (dm *DownloadManager) RemoveFromQueue(movieID uint) bool {
	dm.queueMutex.Lock()
	defer dm.queueMutex.Unlock()

	for i, queued := range dm.queue {
		if queued.movieID == movieID {
			dm.queue = append(dm.queue[:i], dm.queue[i+1:]...)
			logutils.Log.WithField("movie_id", movieID).Info("Download removed from queue")
			return true
		}
	}
	return false
}

func (dm *DownloadManager) GetTotalDownloads() int {
	return dm.GetDownloadCount() + dm.GetQueueCount()
}

func (dm *DownloadManager) GetNotificationChan() <-chan QueueNotification {
	return dm.notificationChan
}

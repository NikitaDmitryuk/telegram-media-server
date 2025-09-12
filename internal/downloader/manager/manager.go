package manager

import (
	"context"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
)

var GlobalDownloadManager *DownloadManager

func InitDownloadManager(cfg *config.Config) {
	GlobalDownloadManager = NewDownloadManager(cfg, database.GlobalDB)
}

func NewDownloadManager(cfg *config.Config, db database.Database) *DownloadManager {
	dm := &DownloadManager{
		jobs:             make(map[uint]downloadJob),
		queue:            make([]queuedDownload, 0),
		semaphore:        make(chan struct{}, cfg.GetDownloadSettings().MaxConcurrentDownloads),
		downloadSettings: cfg.GetDownloadSettings(),
		notificationChan: make(chan QueueNotification, NotificationChannelSize),
		db:               db,
	}

	go dm.processQueue()

	return dm
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
	movieTitle, err := dl.GetTitle()
	if err != nil {
		return 0, nil, nil, utils.WrapError(err, "Failed to get download title", nil)
	}

	movieFiles, tempFiles, err := dl.GetFiles()
	if err != nil {
		return 0, nil, nil, utils.WrapError(err, "Failed to get download files", nil)
	}

	fileSize, err := dl.GetFileSize()
	if err != nil {
		logutils.Log.WithError(err).Warn("Failed to get file size, using 0")
		fileSize = 0
	}

	movieID, err = dm.db.AddMovie(context.Background(), movieTitle, fileSize, movieFiles, tempFiles)
	if err != nil {
		return 0, nil, nil, utils.WrapError(err, "Failed to add movie to database", map[string]any{
			"title": movieTitle,
		})
	}

	logutils.Log.WithFields(map[string]any{
		"movie_id":  movieID,
		"title":     movieTitle,
		"file_size": fileSize,
	}).Info("Starting download")

	select {
	case dm.semaphore <- struct{}{}:
		return dm.startDownloadImmediately(movieID, dl, movieTitle)
	default:
		queuedMovieID, progressChan, outerErrChan := dm.addToQueue(movieID, dl, movieTitle, chatID)
		return queuedMovieID, progressChan, outerErrChan, nil
	}
}

func (dm *DownloadManager) startDownloadImmediately(
	movieID uint,
	dl downloader.Downloader,
	movieTitle string,
) (movieIDOut uint, progressChan chan float64, outerErrChan chan error, err error) {
	ctx, cancel := context.WithCancel(context.Background())

	progressChan, errChan, err := dl.StartDownload(ctx)
	if err != nil {
		cancel()
		<-dm.semaphore
		return 0, nil, nil, utils.WrapError(err, "Failed to start download", map[string]any{
			"movie_id": movieID,
			"title":    movieTitle,
		})
	}

	outerErrChan = make(chan error, 1)

	job := downloadJob{
		downloader:   dl,
		startTime:    dm.getCurrentTime(),
		progressChan: progressChan,
		errChan:      errChan,
		ctx:          ctx,
		cancel:       cancel,
	}

	dm.mu.Lock()
	dm.jobs[movieID] = job
	dm.mu.Unlock()

	go dm.monitorDownload(movieID, &job, outerErrChan)

	return movieID, progressChan, outerErrChan, nil
}

func (dm *DownloadManager) StopDownload(movieID uint) error {
	dm.mu.Lock()
	job, exists := dm.jobs[movieID]
	dm.mu.Unlock()

	if exists {
		logutils.Log.WithField("movie_id", movieID).Info("Stopping active download")
		if err := job.downloader.StopDownload(); err != nil {
			logutils.Log.WithError(err).WithField("movie_id", movieID).Warn("Issue stopping downloader (may have stopped anyway)")
		}
		job.cancel()
		return nil
	}

	if dm.RemoveFromQueue(movieID) {
		logutils.Log.WithField("movie_id", movieID).Info("Removed download from queue")
		return nil
	}

	// It's normal for completed downloads to not be found in active downloads or queue
	logutils.Log.WithField("movie_id", movieID).Debug("Download not found in active downloads or queue (likely already completed)")
	return nil
}

func (dm *DownloadManager) StopAllDownloads() {
	dm.mu.Lock()
	jobs := make(map[uint]downloadJob)
	for k, v := range dm.jobs {
		jobs[k] = v
	}
	dm.mu.Unlock()

	for movieID, job := range jobs {
		logutils.Log.WithField("movie_id", movieID).Info("Stopping download")
		if err := job.downloader.StopDownload(); err != nil {
			logutils.Log.WithError(err).WithField("movie_id", movieID).Warn("Issue stopping downloader (may have stopped anyway)")
		}
		job.cancel()
	}
}

func (*DownloadManager) getCurrentTime() time.Time {
	return time.Now()
}

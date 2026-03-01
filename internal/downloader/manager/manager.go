package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/qbittorrent"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/notifier"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/tvcompat"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
)

func NewDownloadManager(cfg *config.Config, db database.Database) *DownloadManager {
	dm := &DownloadManager{
		jobs:             make(map[uint]*downloadJob),
		queue:            make([]queuedDownload, 0),
		semaphore:        make(chan struct{}, cfg.GetDownloadSettings().MaxConcurrentDownloads),
		downloadSettings: cfg.GetDownloadSettings(),
		db:               db,
		cfg:              cfg,
		conversionQueue:  make(chan conversionJob, ConversionQueueSize),
	}

	go dm.processQueue()
	if cfg.VideoSettings.CompatibilityMode {
		go dm.runConversionWorker()
	}

	return dm
}

func (dm *DownloadManager) ResumeDownload(
	movieID uint,
	dl downloader.Downloader,
	title string,
	totalEpisodes int,
	queueNotifier notifier.QueueNotifier,
) (chan error, error) {
	select {
	case dm.semaphore <- struct{}{}:
		return dm.resumeDownloadImmediately(movieID, dl, title, totalEpisodes, queueNotifier)
	default:
		return nil, utils.WrapError(fmt.Errorf("no slot for resumed download"), "resume failed", map[string]any{
			"movie_id": movieID,
		})
	}
}

func (dm *DownloadManager) resumeDownloadImmediately(
	movieID uint,
	dl downloader.Downloader,
	title string,
	totalEpisodes int,
	queueNotifier notifier.QueueNotifier,
) (chan error, error) {
	ctx, cancel := context.WithCancel(context.Background())

	progressChan, errChan, episodesChan, err := dl.StartDownload(ctx)
	if err != nil {
		cancel()
		<-dm.semaphore
		return nil, utils.WrapError(err, "Failed to resume download", map[string]any{
			"movie_id": movieID,
			"title":    title,
		})
	}

	outerErrChan := make(chan error, 1)

	job := &downloadJob{
		downloader:    dl,
		startTime:     dm.getCurrentTime(),
		progressChan:  progressChan,
		errChan:       errChan,
		episodesChan:  episodesChan,
		ctx:           ctx,
		cancel:        cancel,
		queueNotifier: queueNotifier,
		title:         title,
		totalEpisodes: totalEpisodes,
	}

	dm.mu.Lock()
	dm.jobs[movieID] = job
	dm.mu.Unlock()

	go dm.monitorDownload(movieID, job, outerErrChan)

	return outerErrChan, nil
}

func (dm *DownloadManager) StartDownload(
	dl downloader.Downloader,
	queueNotifier notifier.QueueNotifier,
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

	totalEpisodes := dl.TotalEpisodes()
	movieID, err = dm.db.AddMovie(context.Background(), movieTitle, fileSize, movieFiles, tempFiles, totalEpisodes)
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
		return dm.startDownloadImmediately(movieID, dl, movieTitle, queueNotifier)
	default:
		queuedMovieID, progressChan, outerErrChan := dm.addToQueue(movieID, dl, movieTitle, queueNotifier)
		return queuedMovieID, progressChan, outerErrChan, nil
	}
}

func (dm *DownloadManager) startDownloadImmediately(
	movieID uint,
	dl downloader.Downloader,
	movieTitle string,
	queueNotifier notifier.QueueNotifier,
) (movieIDOut uint, progressChan chan float64, outerErrChan chan error, err error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Set hash callback BEFORE starting the download goroutine to avoid a data race
	// where run() checks d.onHashKnown before the main goroutine sets it.
	if setter, ok := dl.(downloader.OnHashKnownSetter); ok {
		setter.SetOnHashKnown(func(hash string) {
			if dbErr := dm.db.SetQBittorrentHash(context.Background(), movieID, hash); dbErr != nil {
				logutils.Log.WithError(dbErr).WithField("movie_id", movieID).Warn("Failed to persist qBittorrent hash")
			}
		})
	}

	progressChan, errChan, episodesChan, err := dl.StartDownload(ctx)
	if err != nil {
		cancel()
		<-dm.semaphore
		dm.removeMovieRollback(context.Background(), movieID)
		return 0, nil, nil, utils.WrapError(err, "Failed to start download", map[string]any{
			"movie_id": movieID,
			"title":    movieTitle,
		})
	}

	// Show TV compatibility circle immediately from metadata (torrent file names or yt-dlp format info).
	if dm.cfg.VideoSettings.CompatibilityMode {
		if early, ok := dl.(downloader.EarlyCompatDownloader); ok {
			if compat, earlyErr := early.GetEarlyTvCompatibility(ctx); earlyErr == nil && compat != "" {
				_ = dm.db.SetTvCompatibility(ctx, movieID, compat)
			}
		}
	}
	// Fallback: also persist hash via channel (idempotent second persist).
	if hd, ok := dl.(downloader.QBittorrentHashDownloader); ok {
		if ch := hd.QBittorrentHashChan(); ch != nil {
			go func() {
				if hash, ok := <-ch; ok && hash != "" {
					_ = dm.db.SetQBittorrentHash(context.Background(), movieID, hash)
				}
			}()
		}
	}

	outerErrChan = make(chan error, 1)

	job := &downloadJob{
		downloader:    dl,
		startTime:     dm.getCurrentTime(),
		progressChan:  progressChan,
		errChan:       errChan,
		episodesChan:  episodesChan,
		ctx:           ctx,
		cancel:        cancel,
		queueNotifier: queueNotifier,
		title:         movieTitle,
		totalEpisodes: dl.TotalEpisodes(),
	}

	dm.mu.Lock()
	dm.jobs[movieID] = job
	dm.mu.Unlock()

	go dm.monitorDownload(movieID, job, outerErrChan)

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

	return dm.stopDownloadNotFound(movieID)
}

func (dm *DownloadManager) StopDownloadSilent(movieID uint) error {
	dm.mu.Lock()
	job, exists := dm.jobs[movieID]
	dm.mu.Unlock()

	if exists {
		job.silentStop = true
		logutils.Log.WithField("movie_id", movieID).Info("Stopping active download")
		if err := job.downloader.StopDownload(); err != nil {
			logutils.Log.WithError(err).WithField("movie_id", movieID).Warn("Issue stopping downloader (may have stopped anyway)")
		}
		job.cancel()
		return nil
	}

	return dm.stopDownloadNotFound(movieID)
}

func (dm *DownloadManager) stopDownloadNotFound(movieID uint) error {
	if dm.RemoveFromQueue(movieID) {
		logutils.Log.WithField("movie_id", movieID).Info("Removed download from queue")
		return nil
	}

	// It's normal for completed downloads to not be found in active downloads or queue
	logutils.Log.WithField("movie_id", movieID).Debug("Download not found in active downloads or queue (likely already completed)")
	return nil
}

func (dm *DownloadManager) RemoveQBittorrentTorrent(ctx context.Context, movieID uint) error {
	if dm.cfg.QBittorrentURL == "" {
		return nil
	}
	movie, err := dm.db.GetMovieByID(ctx, movieID)
	if err != nil {
		logutils.Log.WithError(err).WithField("movie_id", movieID).Warn("RemoveQBittorrentTorrent: failed to get movie from DB")
		return err
	}
	if movie.QBittorrentHash == "" {
		logutils.Log.WithField("movie_id", movieID).Debug("RemoveQBittorrentTorrent: no qBittorrent hash stored, skipping")
		return nil
	}
	client, err := qbittorrent.NewClient(dm.cfg.QBittorrentURL, dm.cfg.QBittorrentUsername, dm.cfg.QBittorrentPassword)
	if err != nil {
		logutils.Log.WithError(err).WithField("movie_id", movieID).Warn("Failed to create qBittorrent client for removal")
		return err
	}
	if err := client.Login(ctx); err != nil {
		logutils.Log.WithError(err).WithField("movie_id", movieID).Warn("qBittorrent login failed for removal")
		return err
	}
	if err := client.DeleteTorrent(ctx, movie.QBittorrentHash, false); err != nil {
		logutils.Log.WithError(err).
			WithField("movie_id", movieID).
			WithField("hash", movie.QBittorrentHash).
			Warn("Failed to delete torrent from qBittorrent")
		return err
	}
	logutils.Log.WithField("movie_id", movieID).Info("Removed torrent from qBittorrent Web UI")
	return nil
}

func (dm *DownloadManager) StopAllDownloads() {
	dm.mu.Lock()
	jobs := make(map[uint]*downloadJob)
	for k := range dm.jobs {
		jobs[k] = dm.jobs[k]
	}
	dm.mu.Unlock()

	for movieID := range jobs {
		job := jobs[movieID]
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

// removeMovieRollback removes the movie and its file records from DB and deletes associated files from disk.
// Used when StartDownload fails so the movie does not stay in the list.
func (dm *DownloadManager) removeMovieRollback(ctx context.Context, movieID uint) {
	moviePath := dm.cfg.MoviePath
	for _, isTemp := range []bool{true, false} {
		var files []database.MovieFile
		var err error
		if isTemp {
			files, err = dm.db.GetTempFilesByMovieID(ctx, movieID)
		} else {
			files, err = dm.db.GetFilesByMovieID(ctx, movieID)
		}
		if err != nil {
			logutils.Log.WithError(err).WithField("movie_id", movieID).Error("Failed to get files for rollback")
			continue
		}
		for _, f := range files {
			path := filepath.Join(moviePath, f.FilePath)
			if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
				logutils.Log.WithError(removeErr).WithField("path", path).Warn("Failed to remove file during rollback")
			}
		}
		if isTemp {
			_ = dm.db.RemoveTempFilesByMovieID(ctx, movieID)
		} else {
			_ = dm.db.RemoveFilesByMovieID(ctx, movieID)
		}
	}
	if removeErr := dm.db.RemoveMovie(ctx, movieID); removeErr != nil {
		logutils.Log.WithError(removeErr).WithField("movie_id", movieID).Error("Failed to remove movie during rollback")
	} else {
		logutils.Log.WithField("movie_id", movieID).Info("Movie removed after start download failure")
	}
}

// enqueueConversionIfNeeded runs after download completes: probes TV compatibility, sets tv_compatibility,
// and either marks as skipped (red) or enqueues for light conversion (green/yellow).
// Returns needWait, done channel, and compatRed (true if video is red / not playable on TV).
func (dm *DownloadManager) enqueueConversionIfNeeded(
	ctx context.Context,
	movieID uint,
	title string,
) (needWait bool, done <-chan struct{}, compatRed bool) {
	if !dm.cfg.VideoSettings.CompatibilityMode {
		return false, nil, false
	}
	targetLevel := tvcompat.ParseH264Level(dm.cfg.VideoSettings.TvH264Level)
	if targetLevel <= 0 {
		targetLevel = 41
	}
	compat := tvcompat.ProbeTvCompatibility(ctx, movieID, dm.cfg.MoviePath, dm.db, targetLevel)
	if compat == "" {
		// Probe could not determine compatibility (ffprobe missing, etc.)
		// Keep the early estimate and skip conversion (we can't convert without ffprobe anyway).
		logutils.Log.WithField("movie_id", movieID).Debug("TV compatibility probe returned unknown, keeping early estimate")
		return false, nil, false
	}
	_ = dm.db.SetTvCompatibility(ctx, movieID, compat)
	if compat == tvcompat.TvCompatRed {
		_ = dm.db.UpdateConversionStatus(ctx, movieID, "skipped")
		return false, nil, true
	}
	_ = dm.db.UpdateConversionStatus(ctx, movieID, "pending")
	_ = dm.db.UpdateConversionPercentage(ctx, movieID, 0)
	needWait, ch := dm.EnqueueConversion(movieID, title)
	return needWait, ch, false
}

// EnqueueConversion adds movieID to the conversion queue (no-op if compatibility mode is off).
// Returns (enqueued, done). If enqueued, caller must <-done before signaling "download completed" to the user.
func (dm *DownloadManager) EnqueueConversion(movieID uint, title string) (enqueued bool, done <-chan struct{}) {
	if !dm.cfg.VideoSettings.CompatibilityMode {
		return false, nil
	}
	jobDone := make(chan struct{}, 1)
	job := conversionJob{MovieID: movieID, Title: title, Done: jobDone}
	select {
	case dm.conversionQueue <- job:
		logutils.Log.WithField("movie_id", movieID).Info("Movie enqueued for TV compatibility conversion")
		return true, jobDone
	default:
		logutils.Log.WithField("movie_id", movieID).Warn("Conversion queue full, movie not enqueued")
		return false, nil
	}
}

func (dm *DownloadManager) runConversionWorker() {
	ctx := context.Background()
	vs := &dm.cfg.VideoSettings
	for job := range dm.conversionQueue {
		movieID := job.MovieID
		if err := dm.db.UpdateConversionStatus(ctx, movieID, "in_progress"); err != nil {
			logutils.Log.WithError(err).WithField("movie_id", movieID).Warn("Failed to set conversion status")
		}
		movie, _ := dm.db.GetMovieByID(ctx, movieID)
		if movie.TvCompatibility != tvcompat.TvCompatGreen {
			tvcompat.RunTvCompatibility(ctx, movieID, dm.cfg.MoviePath, dm.db, vs)
		}
		const completeConversionPct = 100
		if err := dm.db.UpdateConversionPercentage(ctx, movieID, completeConversionPct); err != nil {
			logutils.Log.WithError(err).WithField("movie_id", movieID).Warn("Failed to set conversion percentage")
		}
		if err := dm.db.UpdateConversionStatus(ctx, movieID, "done"); err != nil {
			logutils.Log.WithError(err).WithField("movie_id", movieID).Warn("Failed to set conversion status done")
		}
		logutils.Log.WithField("movie_id", movieID).Info("TV compatibility conversion completed")
		close(job.Done)
	}
}

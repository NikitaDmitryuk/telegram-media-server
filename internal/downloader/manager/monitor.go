package manager

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/tvcompat"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
)

//nolint:gocyclo // Complex monitoring logic is acceptable here
func (dm *DownloadManager) monitorDownload(
	movieID uint,
	job *downloadJob,
	outerErrChan chan error,
) {
	defer func() {
		dm.mu.Lock()
		delete(dm.jobs, movieID)
		dm.mu.Unlock()
		<-dm.semaphore
	}()

	var (
		lastProgress         float64
		progressStagnantTime time.Time
		downloadStartTime    = time.Now()
		maxStagnantDuration  = 30 * time.Minute
	)

	updateTicker := time.NewTicker(dm.downloadSettings.ProgressUpdateInterval)
	defer updateTicker.Stop()

	var timeoutTimer *time.Timer
	var timeoutChan <-chan time.Time
	if dm.downloadSettings.DownloadTimeout > 0 {
		timeoutTimer = time.NewTimer(dm.downloadSettings.DownloadTimeout)
		timeoutChan = timeoutTimer.C
		defer timeoutTimer.Stop()
	}

	for {
		select {
		case completed, ok := <-job.episodesChan:
			if !ok {
				job.episodesChan = nil
				continue
			}
			// Episode completion means the download is progressing; reset stagnation timer.
			progressStagnantTime = time.Time{}
			if updateErr := dm.db.UpdateEpisodesProgress(context.Background(), movieID, completed); updateErr != nil {
				logutils.Log.WithError(updateErr).WithField("movie_id", movieID).Error("Failed to update episodes progress")
			}
			if completed == 1 {
				// "First episode ready" only for series (multiple files); skip for single-file video/yt-dlp.
				if job.totalEpisodes > 1 {
					dm.notificationChan <- QueueNotification{
						Type:    "first_episode_ready",
						ChatID:  job.chatID,
						MovieID: movieID,
						Title:   job.title,
					}
				}
				// Probe TV compatibility as soon as first file is ready so user sees green/yellow/red immediately.
				if dm.cfg.VideoSettings.CompatibilityMode {
					ctx := context.Background()
					targetLevel := tvcompat.ParseH264Level(dm.cfg.VideoSettings.TvH264Level)
					if targetLevel <= 0 {
						targetLevel = 41
					}
					compat := tvcompat.ProbeTvCompatibility(ctx, movieID, dm.cfg.MoviePath, dm.db, targetLevel)
					// Only update DB when the probe actually determined compatibility.
					// Empty result means the probe failed (ffprobe missing, file not ready, etc.)
					// — keep the early estimate (e.g. green from file extension).
					if compat != "" {
						_ = dm.db.SetTvCompatibility(ctx, movieID, compat)
						if compat == tvcompat.TvCompatRed && dm.cfg.VideoSettings.RejectIncompatible {
							job.rejectedIncompatible = true
							job.cancel()
						}
					}
				}
			}

		case progress, ok := <-job.progressChan:
			if !ok {
				// If the download was manually stopped, progressChan may close before
				// errChan delivers ErrStoppedByUser. Don't treat this as successful completion:
				// skip SetLoaded / conversion and propagate the correct signal.
				if job.downloader.StoppedManually() {
					if job.silentStop {
						logutils.Log.WithField("movie_id", movieID).Info("Download stopped by deletion (progressChan closed first)")
						outerErrChan <- downloader.ErrStoppedByDeletion
					} else {
						logutils.Log.WithField("movie_id", movieID).Info("Download stopped by user (progressChan closed first)")
						outerErrChan <- nil
					}
					return
				}

				logutils.Log.WithFields(map[string]any{
					"movie_id": movieID,
					"duration": time.Since(downloadStartTime),
				}).Info("Download completed successfully")

				needWait, done, compatRed := dm.enqueueConversionIfNeeded(context.Background(), movieID, job.chatID, job.title)
				if compatRed && dm.cfg.VideoSettings.RejectIncompatible {
					dm.notificationChan <- QueueNotification{
						Type:    "video_not_supported",
						ChatID:  job.chatID,
						MovieID: movieID,
						Title:   job.title,
					}
					outerErrChan <- nil
					return
				}
				if needWait && done != nil {
					<-done
				}
				if err := dm.db.SetLoaded(context.Background(), movieID); err != nil {
					logutils.Log.WithError(err).WithField("movie_id", movieID).Error("Failed to mark movie as loaded")
					outerErrChan <- utils.WrapError(err, "Failed to mark movie as loaded", map[string]any{
						"movie_id": movieID,
					})
				} else {
					outerErrChan <- nil
				}
				return
			}

			currentTime := time.Now()
			progressDiff := math.Abs(progress - lastProgress)

			const significantProgressChange = 0.1
			if progressDiff > significantProgressChange {
				lastProgress = progress
				progressStagnantTime = time.Time{}
			} else if progressStagnantTime.IsZero() && progress > 0 {
				progressStagnantTime = currentTime
			}

			const maxProgress = 100
			if progress > maxProgress {
				progress = maxProgress
			}

			if updateErr := dm.db.UpdateDownloadedPercentage(context.Background(), movieID, int(progress)); updateErr != nil {
				logutils.Log.WithError(updateErr).WithField("movie_id", movieID).Error("Failed to update progress in database")
			}

			if !progressStagnantTime.IsZero() && currentTime.Sub(progressStagnantTime) > maxStagnantDuration {
				err := fmt.Errorf("download appears to be stagnant (no progress for %v)", maxStagnantDuration)
				logutils.Log.WithError(err).WithField("movie_id", movieID).Warn("Download stagnant")
				outerErrChan <- err
				return
			}

		case err, ok := <-job.errChan:
			if !ok {
				logutils.Log.WithField("movie_id", movieID).Info("Download completed (error channel closed without error)")
				outerErrChan <- nil
				return
			}

			if errors.Is(err, downloader.ErrStoppedByUser) {
				logutils.Log.WithField("movie_id", movieID).Info("Download stopped by user")
				if job.silentStop {
					outerErrChan <- downloader.ErrStoppedByDeletion
				} else {
					outerErrChan <- nil
				}
				return
			}
			if err != nil {
				logutils.Log.WithError(err).WithField("movie_id", movieID).Error("Download failed")
				outerErrChan <- utils.WrapError(err, "Download failed", map[string]any{
					"movie_id": movieID,
				})
			} else {
				logutils.Log.WithField("movie_id", movieID).Info("Download completed successfully")

				needWait, done, compatRed := dm.enqueueConversionIfNeeded(context.Background(), movieID, job.chatID, job.title)
				if compatRed && dm.cfg.VideoSettings.RejectIncompatible {
					dm.notificationChan <- QueueNotification{
						Type:    "video_not_supported",
						ChatID:  job.chatID,
						MovieID: movieID,
						Title:   job.title,
					}
					outerErrChan <- nil
					return
				}
				if needWait && done != nil {
					<-done
				}
				if err := dm.db.SetLoaded(context.Background(), movieID); err != nil {
					logutils.Log.WithError(err).WithField("movie_id", movieID).Error("Failed to mark movie as loaded")
					outerErrChan <- utils.WrapError(err, "Failed to mark movie as loaded", map[string]any{
						"movie_id": movieID,
					})
				} else {
					outerErrChan <- nil
				}
			}
			return

		case <-updateTicker.C:
			// Periodic logging for debugging
			dm.mu.RLock()
			if _, exists := dm.jobs[movieID]; exists {
				elapsed := time.Since(downloadStartTime)
				logutils.Log.WithFields(map[string]any{
					"movie_id":      movieID,
					"elapsed":       elapsed,
					"last_progress": lastProgress,
				}).Debug("Download status update")
			}
			dm.mu.RUnlock()

		case <-timeoutChan:
			if timeoutChan != nil {
				err := fmt.Errorf("download timeout after %v", dm.downloadSettings.DownloadTimeout)
				logutils.Log.WithError(err).WithField("movie_id", movieID).Error("Download timed out")
				outerErrChan <- err
				return
			}

		case <-job.ctx.Done():
			if job.rejectedIncompatible {
				dm.notificationChan <- QueueNotification{
					Type:    "video_not_supported",
					ChatID:  job.chatID,
					MovieID: movieID,
					Title:   job.title,
				}
				outerErrChan <- nil
			} else {
				logutils.Log.WithField("movie_id", movieID).Info("Download canceled")
				outerErrChan <- job.ctx.Err()
			}
			return
		}
	}
}

func (dm *DownloadManager) GetDownloadStatus(movieID uint) (isActive bool, progress float64, err error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	job, exists := dm.jobs[movieID]
	if !exists {
		return false, 0, nil
	}

	movie, err := dm.db.GetMovieByID(context.Background(), movieID)
	if err != nil {
		return false, 0, err
	}

	_ = job
	return true, float64(movie.DownloadedPercentage), nil
}

func (dm *DownloadManager) GetActiveDownloads() []uint {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	downloads := make([]uint, 0, len(dm.jobs))
	for movieID := range dm.jobs {
		downloads = append(downloads, movieID)
	}
	return downloads
}

func (dm *DownloadManager) GetDownloadCount() int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return len(dm.jobs)
}

func (dm *DownloadManager) GetNotificationChan() <-chan QueueNotification {
	return dm.notificationChan
}

package manager

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/utils"
)

//nolint:gocyclo // Complex monitoring logic is acceptable here
func (dm *DownloadManager) monitorDownload(
	movieID uint,
	job *downloadJob,
	outerProgressChan chan float64,
	outerErrChan chan error,
) {
	defer func() {
		dm.mu.Lock()
		delete(dm.jobs, movieID)
		dm.mu.Unlock()
		<-dm.semaphore

		// Закрываем внешние каналы
		close(outerProgressChan)
		close(outerErrChan)
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
		case progress, ok := <-job.progressChan:
			if !ok {
				logger.Log.WithFields(map[string]any{
					"movie_id": movieID,
					"duration": time.Since(downloadStartTime),
				}).Info("Download manager: progress channel closed")
				// Не завершаем здесь - ждем errChan
				continue
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

			// Обновляем базу данных
			if updateErr := dm.db.UpdateDownloadedPercentage(context.Background(), movieID, int(progress)); updateErr != nil {
				logger.Log.WithError(updateErr).WithField("movie_id", movieID).Error("Failed to update progress in database")
			}

			// Передаем прогресс дальше в download service
			select {
			case outerProgressChan <- progress:
				logger.Log.WithFields(map[string]any{
					"movie_id": movieID,
					"progress": progress,
				}).Debug("Progress forwarded to download service")
			default:
				logger.Log.WithField("movie_id", movieID).Warn("Failed to forward progress - channel full")
			}

			if !progressStagnantTime.IsZero() && currentTime.Sub(progressStagnantTime) > maxStagnantDuration {
				err := fmt.Errorf("download appears to be stagnant (no progress for %v)", maxStagnantDuration)
				logger.Log.WithError(err).WithField("movie_id", movieID).Warn("Download stagnant")
				select {
				case outerErrChan <- err:
				default:
				}
				return
			}

		case err, ok := <-job.errChan:
			if !ok {
				logger.Log.WithField("movie_id", movieID).Info("Download manager: error channel closed without error")
				select {
				case outerErrChan <- nil:
					logger.Log.WithField("movie_id", movieID).Info("Completion signal forwarded to download service")
				default:
				}
				return
			}

			if err != nil {
				logger.Log.WithError(err).WithField("movie_id", movieID).Error("Download failed")
				select {
				case outerErrChan <- utils.WrapError(err, "Download failed", map[string]any{
					"movie_id": movieID,
				}):
				default:
				}
			} else {
				logger.Log.WithField("movie_id", movieID).Info("Download completed successfully")

				if err := dm.db.SetLoaded(context.Background(), movieID); err != nil {
					logger.Log.WithError(err).WithField("movie_id", movieID).Error("Failed to mark movie as loaded")
					select {
					case outerErrChan <- utils.WrapError(err, "Failed to mark movie as loaded", map[string]any{
						"movie_id": movieID,
					}):
					default:
					}
				} else {
					select {
					case outerErrChan <- nil:
						logger.Log.WithField("movie_id", movieID).Info("Success signal forwarded to download service")
					default:
					}
				}
			}
			return

		case <-updateTicker.C:
			// Periodic logging for debugging
			dm.mu.RLock()
			if job, exists := dm.jobs[movieID]; exists {
				elapsed := time.Since(downloadStartTime)
				logger.Log.WithFields(map[string]any{
					"movie_id":      movieID,
					"elapsed":       elapsed,
					"last_progress": lastProgress,
				}).Debug("Download status update")
				_ = job
			}
			dm.mu.RUnlock()

		case <-timeoutChan:
			if timeoutChan != nil {
				err := fmt.Errorf("download timeout after %v", dm.downloadSettings.DownloadTimeout)
				logger.Log.WithError(err).WithField("movie_id", movieID).Error("Download timed out")
				select {
				case outerErrChan <- err:
				default:
				}
				return
			}

		case <-job.ctx.Done():
			logger.Log.WithField("movie_id", movieID).Info("Download canceled")
			select {
			case outerErrChan <- job.ctx.Err():
			default:
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

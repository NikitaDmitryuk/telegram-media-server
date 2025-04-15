package manager

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
)

var GlobalDownloadManager *DownloadManager

func init() {
	GlobalDownloadManager = NewDownloadManager()
}

type DownloadManager struct {
	mu   sync.RWMutex
	jobs map[int]downloader.Downloader
}

func NewDownloadManager() *DownloadManager {
	return &DownloadManager{
		jobs: make(map[int]downloader.Downloader),
	}
}

func (dm *DownloadManager) StartDownload(dl downloader.Downloader) (int, chan float64, chan error, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	movieTitle, err := dl.GetTitle()
	if err != nil {
		logrus.WithError(err).Error("Failed to get video title")
		return 0, nil, nil, err
	}

	mainFiles, tempFiles, err := dl.GetFiles()
	if err != nil {
		logrus.WithError(err).Error("Failed to get files")
		return 0, nil, nil, err
	}

	movieID, err := database.GlobalDB.AddMovie(context.Background(), movieTitle, mainFiles, tempFiles)
	if err != nil {
		logrus.WithError(err).Error("Failed to add movie to database")
		return 0, nil, nil, err
	}

	dm.jobs[movieID] = dl

	progressChan, errChan, err := dl.StartDownload(context.Background())
	if err != nil {
		logrus.WithError(err).Errorf("Failed to start download for movieID %d", movieID)
		delete(dm.jobs, movieID)
		return movieID, nil, nil, err
	}

	outerErrChan := make(chan error, 1)

	go func(movieID int, progressChan chan float64, innerErrChan chan error, outerErrChan chan error) {
		lastUpdate := time.Now()

		for progress := range progressChan {
			if time.Since(lastUpdate) >= 5*time.Second {
				err := database.GlobalDB.UpdateDownloadedPercentage(context.Background(), movieID, int(math.Floor(progress)))
				if err != nil {
					logrus.WithError(err).Warnf("Failed to update downloaded percentage for movieID %d", movieID)
				} else {
					logrus.Debugf("MovieID %d: %.2f%% downloaded", movieID, progress)
				}
				lastUpdate = time.Now()
			}
		}

		err := <-innerErrChan

		dm.mu.Lock()
		defer dm.mu.Unlock()

		if err != nil {
			logrus.WithError(err).Errorf("Download failed for movieID %d", movieID)
		} else if dl.StoppedManually() {
			logrus.Infof("Download for movieID %d was manually stopped", movieID)
		} else {
			logrus.Infof("Download completed successfully for movieID %d", movieID)
			err = database.GlobalDB.SetLoaded(context.Background(), movieID)
			if err != nil {
				logrus.WithError(err).Warnf("Failed to set movie as loaded for movieID %d", movieID)
			}

			if tempFilesErr := DeleteTempFilesByMovieID(movieID); tempFilesErr != nil {
				logrus.WithError(tempFilesErr).Warnf("Failed to delete temp files for movieID %d", movieID)
			} else {
				logrus.Infof("Temp files deleted successfully for movieID %d", movieID)
			}
		}

		delete(dm.jobs, movieID)

		outerErrChan <- err
		close(outerErrChan)
	}(movieID, progressChan, errChan, outerErrChan)

	return movieID, progressChan, outerErrChan, nil
}

func (dm *DownloadManager) StopDownload(movieID int) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dl, exists := dm.jobs[movieID]; exists {
		if err := dl.StopDownload(); err != nil {
			logrus.WithError(err).Errorf("Failed to stop download for movieID %d", movieID)
		} else {
			logrus.Infof("Download stopped for movieID %d", movieID)
		}
		delete(dm.jobs, movieID)
	}
}

func (dm *DownloadManager) StopAllDownloads() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	for movieID, dl := range dm.jobs {
		if err := dl.StopDownload(); err != nil {
			logrus.WithError(err).Errorf("Failed to stop download for movieID %d", movieID)
		} else {
			logrus.Infof("Download stopped for movieID %d", movieID)
		}
		delete(dm.jobs, movieID)
	}
}

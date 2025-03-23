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

func (dm *DownloadManager) StartDownload(dl downloader.Downloader) (chan float64, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	movieTitle, err := dl.GetTitle()
	if err != nil {
		logrus.WithError(err).Error("Failed to get video title")
		return nil, err
	}

	mainFile, tempFiles, err := dl.GetFiles()
	if err != nil {
		logrus.WithError(err).Error("Failed to get files")
		return nil, err
	}

	movieID, err := database.GlobalDB.AddMovie(context.Background(), movieTitle, mainFile, tempFiles)
	if err != nil {
		logrus.WithError(err).Error("Failed to add movie to database")
		return nil, err
	}

	dm.jobs[movieID] = dl

	progressChan, err := dl.StartDownload(context.Background())
	if err != nil {
		logrus.WithError(err).Errorf("Failed to start download for movieID %d", movieID)
		return nil, err
	}

	go func(movieID int, progressChan chan float64) {
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

		dm.mu.RLock()
		task, exists := dm.jobs[movieID]
		dm.mu.RUnlock()
		if exists {
			if task.StoppedManually() {
				logrus.Infof("Download for movieID %d was manually stopped", movieID)
			} else {
				err = database.GlobalDB.SetLoaded(context.Background(), movieID)
				if err != nil {
					logrus.WithError(err).Warnf("Failed to set movie as loaded for movieID %d", movieID)
				}
			}
		}

		dm.mu.Lock()
		delete(dm.jobs, movieID)
		dm.mu.Unlock()
	}(movieID, progressChan)

	return progressChan, nil
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

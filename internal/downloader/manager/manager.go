package manager

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

var GlobalDownloadManager *DownloadManager

func InitDownloadManager() {
	GlobalDownloadManager = NewDownloadManager()
}

type DownloadManager struct {
	mu   sync.RWMutex
	jobs map[uint]downloader.Downloader
}

func NewDownloadManager() *DownloadManager {
	return &DownloadManager{
		jobs: make(map[uint]downloader.Downloader),
	}
}

func (dm *DownloadManager) StartDownload(
	dl downloader.Downloader,
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
		return 0, nil, nil, err
	}

	mainFiles, tempFiles, err := dl.GetFiles()
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get files")
		return 0, nil, nil, err
	}

	movieID, err = database.GlobalDB.AddMovie(context.Background(), movieTitle, mainFiles, tempFiles)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to add movie to database")
		return 0, nil, nil, err
	}

	dm.jobs[movieID] = dl

	progressChan, errChan, err := dl.StartDownload(context.Background())
	if err != nil {
		logutils.Log.WithError(err).Errorf("Failed to start download for movieID %d", movieID)
		delete(dm.jobs, movieID)
		return movieID, nil, nil, err
	}

	outerErrChan = make(chan error, 1)

	go dm.monitorDownload(movieID, dl, progressChan, errChan, outerErrChan)

	return movieID, progressChan, outerErrChan, nil
}

func (dm *DownloadManager) monitorDownload(
	movieID uint,
	dl downloader.Downloader,
	progressChan chan float64,
	innerErrChan chan error,
	outerErrChan chan error,
) {
	lastUpdate := time.Now()

	for progress := range progressChan {
		if time.Since(lastUpdate) >= 5*time.Second {
			err := database.GlobalDB.UpdateDownloadedPercentage(context.Background(), movieID, int(math.Floor(progress)))
			if err != nil {
				logutils.Log.WithError(err).Warnf("Failed to update downloaded percentage for movieID %d", movieID)
			} else {
				logutils.Log.Debugf("MovieID %d: %.2f%% downloaded", movieID, progress)
			}
			lastUpdate = time.Now()
		}
	}

	err := <-innerErrChan

	dm.mu.Lock()
	defer dm.mu.Unlock()

	if err != nil {
		logutils.Log.WithError(err).Errorf("Download failed for movieID %d", movieID)
	} else if dl.StoppedManually() {
		logutils.Log.Infof("Download for movieID %d was manually stopped", movieID)
	} else {
		logutils.Log.Infof("Download completed successfully for movieID %d", movieID)
		err = database.GlobalDB.SetLoaded(context.Background(), movieID)
		if err != nil {
			logutils.Log.WithError(err).Warnf("Failed to set movie as loaded for movieID %d", movieID)
		}
	}

	delete(dm.jobs, movieID)

	outerErrChan <- err
	close(outerErrChan)
}

func (dm *DownloadManager) StopDownload(movieID uint) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dl, exists := dm.jobs[movieID]; exists {
		if err := dl.StopDownload(); err != nil {
			logutils.Log.WithError(err).Errorf("Failed to stop download for movieID %d", movieID)
		} else {
			logutils.Log.Infof("Download stopped for movieID %d", movieID)
		}
		delete(dm.jobs, movieID)
	}
}

func (dm *DownloadManager) StopAllDownloads() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	for movieID, dl := range dm.jobs {
		if err := dl.StopDownload(); err != nil {
			logutils.Log.WithError(err).Errorf("Failed to stop download for movieID %d", movieID)
		} else {
			logutils.Log.Infof("Download stopped for movieID %d", movieID)
		}
		delete(dm.jobs, movieID)
	}
}

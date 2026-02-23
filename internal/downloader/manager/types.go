package manager

import (
	"context"
	"sync"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/notifier"
)

const (
	QueueProgressUpdateInterval = 5 * time.Second
	QueueProcessingDelay        = 100 * time.Millisecond
)

const (
	ConversionQueueSize     = 100
	ProgressChannelBuffSize = 100
)

// Service defines the external interface for the download manager.
// Consumers outside the manager package should depend on this interface.
type Service interface {
	StartDownload(dl downloader.Downloader, queueNotifier notifier.QueueNotifier) (uint, chan float64, chan error, error)
	StopDownload(movieID uint) error
	// StopDownloadSilent stops the download without triggering "download stopped" user notification (e.g. when stopping from deletion queue).
	StopDownloadSilent(movieID uint) error
	StopAllDownloads()
	GetActiveDownloads() []uint
	GetQueueItems() []map[string]any
}

// conversionJob is sent to the conversion worker; Done is closed when conversion (or skip) is finished.
type conversionJob struct {
	MovieID uint
	Title   string
	Done    chan struct{}
}

type DownloadManager struct {
	mu               sync.RWMutex
	jobs             map[uint]*downloadJob
	queue            []queuedDownload
	semaphore        chan struct{}
	downloadSettings config.DownloadConfig
	queueMutex       sync.Mutex
	db               database.Database
	cfg              *config.Config
	conversionQueue  chan conversionJob
}

type downloadJob struct {
	downloader           downloader.Downloader
	startTime            time.Time
	progressChan         chan float64
	errChan              chan error
	episodesChan         <-chan int
	ctx                  context.Context
	cancel               context.CancelFunc
	queueNotifier        notifier.QueueNotifier
	title                string
	totalEpisodes        int  // > 1 for series (multi-file); only then we send "first_episode_ready"
	rejectedIncompatible bool // set when we cancel due to red + RejectIncompatible
	silentStop           bool // if true, monitor sends ErrStoppedByDeletion so handler does not notify user
}

type queuedDownload struct {
	downloader    downloader.Downloader
	movieID       uint
	title         string
	addedAt       time.Time
	queueNotifier notifier.QueueNotifier
	progressChan  chan float64 // Channel to forward progress to the caller
	errChan       chan error   // Channel to forward errors to the caller
}

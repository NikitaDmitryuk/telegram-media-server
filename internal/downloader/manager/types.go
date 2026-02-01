package manager

import (
	"context"
	"sync"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
)

const (
	QueueProgressUpdateInterval = 5 * time.Second
	NotificationChannelSize     = 100
	QueueProcessingDelay        = 100 * time.Millisecond
)

const ConversionQueueSize = 100

// conversionJob is sent to the conversion worker; Done is closed when conversion (or skip) is finished.
type conversionJob struct {
	MovieID uint
	ChatID  int64
	Title   string
	Done    chan struct{}
}

type DownloadManager struct {
	mu               sync.RWMutex
	jobs             map[uint]downloadJob
	queue            []queuedDownload
	semaphore        chan struct{}
	downloadSettings config.DownloadConfig
	queueMutex       sync.Mutex
	notificationChan chan QueueNotification
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
	chatID               int64
	title                string
	totalEpisodes        int  // > 1 for series (multi-file); only then we send "first_episode_ready"
	rejectedIncompatible bool // set when we cancel due to red + RejectIncompatible
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

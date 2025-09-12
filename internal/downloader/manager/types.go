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

type DownloadManager struct {
	mu               sync.RWMutex
	jobs             map[uint]downloadJob
	queue            []queuedDownload
	semaphore        chan struct{}
	downloadSettings config.DownloadConfig
	queueMutex       sync.Mutex
	notificationChan chan QueueNotification
	db               database.Database
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

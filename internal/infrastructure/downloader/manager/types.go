package manager

import (
	"context"
	"sync"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/database"
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
	downloadSettings domain.DownloadConfig
	queueMutex       sync.Mutex
	notificationChan chan QueueNotification
	db               database.Database
}

type downloadJob struct {
	downloader   domain.Downloader
	startTime    time.Time
	progressChan chan float64
	errChan      chan error
	ctx          context.Context
	cancel       context.CancelFunc
}

type queuedDownload struct {
	downloader domain.Downloader
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

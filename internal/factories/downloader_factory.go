package factories

import (
	"context"
	"path/filepath"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/errors"
	aria2 "github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/downloader/torrent"
	ytdlp "github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/downloader/video"
)

// DefaultDownloaderFactory реализует фабрику загрузчиков
type DefaultDownloaderFactory struct {
	bot                 domain.BotInterface
	notificationService domain.NotificationService
	urlValidator        domain.URLValidator
	fileSystem          domain.FileSystemInterface
	processExecutor     domain.ProcessExecutor
	timeProvider        domain.TimeProvider
}

// NewDownloaderFactory создает новую фабрику загрузчиков
func NewDownloaderFactory(
	bot domain.BotInterface,
	notificationService domain.NotificationService,
	urlValidator domain.URLValidator,
	fileSystem domain.FileSystemInterface,
	processExecutor domain.ProcessExecutor,
	timeProvider domain.TimeProvider,
) domain.DownloaderFactory {
	return &DefaultDownloaderFactory{
		bot:                 bot,
		notificationService: notificationService,
		urlValidator:        urlValidator,
		fileSystem:          fileSystem,
		processExecutor:     processExecutor,
		timeProvider:        timeProvider,
	}
}

// CreateVideoDownloader создает загрузчик для видео
func (f *DefaultDownloaderFactory) CreateVideoDownloader(
	_ context.Context,
	url string,
	config *domain.Config,
) (domain.Downloader, error) {
	// Валидируем URL
	if !f.urlValidator.IsValidVideoURL(url) {
		return nil, errors.NewDomainError(
			errors.ErrorTypeValidation,
			"invalid_video_url",
			"provided URL is not a valid video URL",
		).WithDetails(map[string]any{
			"url": url,
		})
	}

	// Создаем загрузчик с полными зависимостями
	videoDownloader := ytdlp.NewYTDLPDownloader(f.bot, url, config)

	return videoDownloader, nil
}

// CreateTorrentDownloader создает загрузчик для торрентов
func (f *DefaultDownloaderFactory) CreateTorrentDownloader(
	_ context.Context,
	fileName, moviePath string,
	config *domain.Config,
) (domain.Downloader, error) {
	// Проверяем существование файла торрента
	if !f.fileSystem.Exists(fileName) {
		return nil, errors.NewDomainError(
			errors.ErrorTypeValidation,
			"torrent_file_not_found",
			"torrent file not found",
		).WithDetails(map[string]any{
			"file_name":  fileName,
			"movie_path": moviePath,
		})
	}

	// Создаем загрузчик с полными зависимостями
	// Извлекаем только имя файла из полного пути
	torrentFileName := filepath.Base(fileName)
	torrentDownloader := aria2.NewAria2Downloader(f.bot, torrentFileName, moviePath, config)

	return torrentDownloader, nil
}

// MockDownloaderFactory для тестирования
type MockDownloaderFactory struct {
	videoDownloader   domain.Downloader
	torrentDownloader domain.Downloader
	shouldError       bool
	errorToReturn     error
}

// NewMockDownloaderFactory создает mock фабрику для тестов
func NewMockDownloaderFactory() *MockDownloaderFactory {
	return &MockDownloaderFactory{}
}

// SetVideoDownloader устанавливает mock загрузчик видео
func (m *MockDownloaderFactory) SetVideoDownloader(d domain.Downloader) {
	m.videoDownloader = d
}

// SetTorrentDownloader устанавливает mock загрузчик торрентов
func (m *MockDownloaderFactory) SetTorrentDownloader(d domain.Downloader) {
	m.torrentDownloader = d
}

// SetError заставляет фабрику возвращать ошибку
func (m *MockDownloaderFactory) SetError(err error) {
	m.shouldError = true
	m.errorToReturn = err
}

// CreateVideoDownloader mock реализация
func (m *MockDownloaderFactory) CreateVideoDownloader(
	_ context.Context,
	_ string,
	_ *domain.Config,
) (domain.Downloader, error) {
	if m.shouldError {
		return nil, m.errorToReturn
	}
	return m.videoDownloader, nil
}

// CreateTorrentDownloader mock реализация
func (m *MockDownloaderFactory) CreateTorrentDownloader(
	_ context.Context,
	_, _ string,
	_ *domain.Config,
) (domain.Downloader, error) {
	if m.shouldError {
		return nil, m.errorToReturn
	}
	return m.torrentDownloader, nil
}

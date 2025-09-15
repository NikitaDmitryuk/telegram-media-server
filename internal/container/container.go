package container

import (
	"fmt"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/services"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/factories"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/filesystem"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/notification"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/validation"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/process"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/timeutil"
)

// Container реализует паттерн Dependency Injection
type Container struct {
	config          *domain.Config
	database        database.Database
	bot             domain.BotInterface
	downloadManager domain.DownloadManagerInterface
	prowlarr        domain.ProwlarrInterface
	httpClient      domain.HTTPClientInterface

	// Инфраструктурные компоненты
	fileSystem          domain.FileSystemInterface
	processExecutor     domain.ProcessExecutor
	timeProvider        domain.TimeProvider
	downloaderFactory   domain.DownloaderFactory
	notificationService domain.NotificationService
	urlValidator        domain.URLValidator

	// Сервисы
	authService     domain.AuthServiceInterface
	downloadService domain.DownloadServiceInterface
	movieService    domain.MovieServiceInterface
}

// NewContainer создает новый DI контейнер
func NewContainer(cfg *domain.Config) *Container {
	return &Container{
		config: cfg,
	}
}

// SetDatabase устанавливает базу данных
func (c *Container) SetDatabase(db database.Database) {
	c.database = db
}

// SetBot устанавливает бот
func (c *Container) SetBot(bot domain.BotInterface) {
	c.bot = bot
}

// SetDownloadManager устанавливает менеджер загрузок
func (c *Container) SetDownloadManager(dm domain.DownloadManagerInterface) {
	c.downloadManager = dm
}

// SetProwlarr устанавливает Prowlarr клиент
func (c *Container) SetProwlarr(p domain.ProwlarrInterface) {
	c.prowlarr = p
}

// SetHTTPClient устанавливает HTTP клиент
func (c *Container) SetHTTPClient(client domain.HTTPClientInterface) {
	c.httpClient = client
}

// GetConfig возвращает конфигурацию
func (c *Container) GetConfig() *domain.Config {
	return c.config
}

// GetDatabase возвращает базу данных
func (c *Container) GetDatabase() database.Database {
	if c.database == nil {
		panic("database not initialized in container")
	}
	return c.database
}

// GetBot возвращает бот
func (c *Container) GetBot() domain.BotInterface {
	if c.bot == nil {
		panic("bot not initialized in container")
	}
	return c.bot
}

// GetDownloadManager возвращает менеджер загрузок
func (c *Container) GetDownloadManager() domain.DownloadManagerInterface {
	if c.downloadManager == nil {
		panic("download manager not initialized in container")
	}
	return c.downloadManager
}

// GetProwlarr возвращает Prowlarr клиент
func (c *Container) GetProwlarr() domain.ProwlarrInterface {
	return c.prowlarr // может быть nil если не настроен
}

// GetHTTPClient возвращает HTTP клиент
func (c *Container) GetHTTPClient() domain.HTTPClientInterface {
	if c.httpClient == nil {
		panic("HTTP client not initialized in container")
	}
	return c.httpClient
}

// GetAuthService возвращает сервис авторизации (lazy initialization)
func (c *Container) GetAuthService() domain.AuthServiceInterface {
	if c.authService == nil {
		c.authService = services.NewAuthService(c.GetDatabase(), c.GetConfig())
	}
	return c.authService
}

// GetDownloadService возвращает сервис загрузок (lazy initialization)
func (c *Container) GetDownloadService() domain.DownloadServiceInterface {
	if c.downloadService == nil {
		c.downloadService = services.NewDownloadService(
			c.GetDownloadManager(),
			c.GetDownloaderFactory(),
			c.GetNotificationService(),
			c.GetDatabase(),
			c.GetConfig(),
			c.GetBot(),
		)
	}
	return c.downloadService
}

// GetMovieService возвращает сервис фильмов (lazy initialization)
func (c *Container) GetMovieService() domain.MovieServiceInterface {
	if c.movieService == nil {
		c.movieService = services.NewMovieService(
			c.GetDatabase(),
			c.GetDownloadManager(),
			c.GetConfig(),
		)
	}
	return c.movieService
}

// GetFileSystem возвращает файловую систему
func (c *Container) GetFileSystem() domain.FileSystemInterface {
	if c.fileSystem == nil {
		c.fileSystem = filesystem.NewOSFileSystem()
	}
	return c.fileSystem
}

// SetFileSystem устанавливает файловую систему
func (c *Container) SetFileSystem(fs domain.FileSystemInterface) {
	c.fileSystem = fs
}

// GetProcessExecutor возвращает исполнитель процессов
func (c *Container) GetProcessExecutor() domain.ProcessExecutor {
	if c.processExecutor == nil {
		c.processExecutor = process.NewOSProcessExecutor()
	}
	return c.processExecutor
}

// SetProcessExecutor устанавливает исполнитель процессов
func (c *Container) SetProcessExecutor(executor domain.ProcessExecutor) {
	c.processExecutor = executor
}

// GetDownloaderFactory возвращает фабрику загрузчиков
func (c *Container) GetDownloaderFactory() domain.DownloaderFactory {
	if c.downloaderFactory == nil {
		c.downloaderFactory = factories.NewDownloaderFactory(
			c.GetBot(),
			c.GetNotificationService(),
			c.GetURLValidator(),
			c.GetFileSystem(),
			c.GetProcessExecutor(),
			c.GetTimeProvider(),
		)
	}
	return c.downloaderFactory
}

// SetDownloaderFactory устанавливает фабрику загрузчиков
func (c *Container) SetDownloaderFactory(factory domain.DownloaderFactory) {
	c.downloaderFactory = factory
}

// GetNotificationService возвращает сервис уведомлений
func (c *Container) GetNotificationService() domain.NotificationService {
	if c.notificationService == nil {
		c.notificationService = notification.NewTelegramNotificationService(c.GetBot())
	}
	return c.notificationService
}

// SetNotificationService устанавливает сервис уведомлений
func (c *Container) SetNotificationService(service domain.NotificationService) {
	c.notificationService = service
}

// GetTimeProvider возвращает провайдер времени
func (c *Container) GetTimeProvider() domain.TimeProvider {
	if c.timeProvider == nil {
		c.timeProvider = timeutil.NewSystemTimeProvider()
	}
	return c.timeProvider
}

// SetTimeProvider устанавливает провайдер времени
func (c *Container) SetTimeProvider(provider domain.TimeProvider) {
	c.timeProvider = provider
}

// GetURLValidator возвращает URL валидатор
func (c *Container) GetURLValidator() domain.URLValidator {
	if c.urlValidator == nil {
		c.urlValidator = validation.NewDefaultURLValidator()
	}
	return c.urlValidator
}

// SetURLValidator устанавливает URL валидатор
func (c *Container) SetURLValidator(validator domain.URLValidator) {
	c.urlValidator = validator
}

// Validate проверяет, что все необходимые зависимости установлены
func (c *Container) Validate() error {
	if c.config == nil {
		return fmt.Errorf("config is required")
	}
	if c.database == nil {
		return fmt.Errorf("database is required")
	}
	if c.bot == nil {
		return fmt.Errorf("bot is required")
	}
	if c.downloadManager == nil {
		return fmt.Errorf("download manager is required")
	}
	if c.httpClient == nil {
		return fmt.Errorf("HTTP client is required")
	}
	return nil
}

// Global container instance для обратной совместимости
var GlobalContainer *Container

// InitGlobalContainer инициализирует глобальный контейнер
func InitGlobalContainer(cfg *domain.Config) {
	GlobalContainer = NewContainer(cfg)
}

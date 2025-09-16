package app

import (
	"os"
	"strconv"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/container"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/database"
	tmsdownloadmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/downloader/manager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/http"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/notification"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/prowlarr"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/metrics"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/ratelimit"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/transport/telegram/bot"
)

const (
	// Default rate limiting settings
	defaultRateLimitRequests = 10
	defaultRateLimitWindow   = 60
)

// initializeComponents инициализирует все компоненты приложения
func initializeComponents(appContainer *container.Container, domainConfig *domain.Config) error {
	// Инициализируем инфраструктуру
	if err := initializeInfrastructure(appContainer, domainConfig); err != nil {
		return err
	}

	// Инициализируем бот и сервисы
	if err := initializeBotAndServices(appContainer, domainConfig); err != nil {
		return err
	}

	// Инициализируем уведомления
	initializeNotifications(appContainer)

	// Валидируем контейнер
	if err := appContainer.Validate(); err != nil {
		logger.Log.WithError(err).Fatal("Container validation failed")
	}

	// Инициализируем метрики и rate limiting
	initializeMetricsAndRateLimit()

	return nil
}

// initializeInfrastructure инициализирует базовую инфраструктуру
func initializeInfrastructure(appContainer *container.Container, domainConfig *domain.Config) error {
	// Инициализируем базу данных
	if err := database.InitDatabase(domainConfig); err != nil {
		logger.Log.WithError(err).Fatal("Failed to initialize the database")
		return err
	}
	appContainer.SetDatabase(database.GlobalDB)

	// Инициализируем локализацию
	if err := lang.InitLocalizer(domainConfig); err != nil {
		logger.Log.WithError(err).Fatal("Failed to initialize localizer")
		return err
	}

	// Инициализируем HTTP клиент
	httpClient := http.NewHTTPClient()
	appContainer.SetHTTPClient(httpClient)

	return nil
}

// initializeBotAndServices инициализирует бот и связанные сервисы
func initializeBotAndServices(appContainer *container.Container, domainConfig *domain.Config) error {
	// Инициализируем менеджер загрузок
	downloadManager := tmsdownloadmanager.NewDownloadManager(domainConfig, database.GlobalDB)
	appContainer.SetDownloadManager(downloadManager)
	logger.Log.Info("Download manager initialized")

	// Инициализируем бот
	botInstance, err := bot.NewBot(domainConfig.BotToken)
	if err != nil {
		logger.Log.WithError(err).Fatal("Bot initialization failed")
		return err
	}
	// Устанавливаем конфигурацию в бот
	botInstance.Config = domainConfig
	appContainer.SetBot(botInstance)

	// Инициализируем Prowlarr (если настроен)
	if domainConfig.ProwlarrURL != "" && domainConfig.ProwlarrAPIKey != "" {
		prowlarrClient := prowlarr.NewProwlarr(appContainer.GetHTTPClient(), domainConfig.ProwlarrURL, domainConfig.ProwlarrAPIKey)
		appContainer.SetProwlarr(prowlarrClient)
		logger.Log.Info("Prowlarr client initialized")
	}

	return nil
}

// initializeNotifications инициализирует сервис уведомлений
func initializeNotifications(appContainer *container.Container) {
	notificationService := notification.NewTelegramNotificationService(appContainer.GetBot())
	appContainer.SetNotificationService(notificationService)
	logger.Log.Info("Notification service initialized")
}

// initializeMetricsAndRateLimit инициализирует метрики и rate limiting
func initializeMetricsAndRateLimit() {
	// Инициализируем метрики
	enableMetrics := getEnvBool("ENABLE_METRICS", false)
	if enableMetrics {
		_ = metrics.NewInMemoryMetrics()
		logger.Log.Info("Metrics enabled")
	} else {
		_ = metrics.NewNoOpMetrics()
		logger.Log.Info("Metrics disabled")
	}

	// Инициализируем rate limiter
	enableRateLimit := getEnvBool("ENABLE_RATE_LIMIT", true)
	if enableRateLimit {
		rateLimitRequests := getEnvInt("RATE_LIMIT_REQUESTS", defaultRateLimitRequests)
		rateLimitWindow := getEnvInt("RATE_LIMIT_WINDOW", defaultRateLimitWindow)
		_ = ratelimit.NewTokenBucketLimiter(
			rateLimitRequests,
			time.Duration(rateLimitWindow)*time.Second,
		)
		logger.Log.Info("Rate limiting enabled")
	} else {
		_ = ratelimit.NewNoOpRateLimiter()
		logger.Log.Info("Rate limiting disabled")
	}
}

// Utility functions

// getEnvBool получает булево значение из переменной окружения
func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	result, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return result
}

// getEnvInt получает целочисленное значение из переменной окружения
func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	result, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return result
}

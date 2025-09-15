package app

import (
	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
)

// initializeConfig инициализирует конфигурацию и логгер
func initializeConfig() (*domain.Config, error) {
	pkgConfig, err := config.NewConfig()
	if err != nil {
		return nil, err
	}

	// Инициализируем логгер
	logger.InitLogger(pkgConfig.LogLevel)

	// Конвертируем в domain.Config
	domainConfig := convertConfig(pkgConfig)

	return domainConfig, nil
}

// convertConfig конвертирует pkg.Config в domain.Config
func convertConfig(cfg *config.Config) *domain.Config {
	return &domain.Config{
		BotToken:       cfg.BotToken,
		AdminPassword:  cfg.AdminPassword,
		MoviePath:      cfg.MoviePath,
		ProwlarrURL:    cfg.ProwlarrURL,
		ProwlarrAPIKey: cfg.ProwlarrAPIKey,
		LogLevel:       cfg.LogLevel,
	}
}

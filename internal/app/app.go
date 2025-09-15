package app

import (
	"context"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/container"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
)

// Application представляет главное приложение
type Application struct {
	container *container.Container
	config    *domain.Config
}

// New создает новое приложение
func New() (*Application, error) {
	// Инициализируем конфигурацию
	config, err := initializeConfig()
	if err != nil {
		return nil, err
	}

	// Создаем контейнер зависимостей
	appContainer := container.NewContainer(config)

	// Инициализируем все компоненты
	if err := initializeComponents(appContainer, config); err != nil {
		return nil, err
	}

	return &Application{
		container: appContainer,
		config:    config,
	}, nil
}

// Run запускает приложение
func (app *Application) Run(ctx context.Context) error {
	logger.Log.Info("Starting Telegram Media Server")

	// Запускаем сервер
	return runServer(ctx, app.container)
}

// Shutdown gracefully останавливает приложение
func (app *Application) Shutdown(ctx context.Context) error {
	logger.Log.Info("Shutting down Telegram Media Server")
	return shutdownServer(ctx, app.container)
}

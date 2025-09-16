package app

import (
	"context"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/container"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/shutdown"
	router "github.com/NikitaDmitryuk/telegram-media-server/internal/transport/telegram"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// Default shutdown timeout
	defaultShutdownTimeout = 30
)

// runServer запускает сервер и обрабатывает обновления
func runServer(ctx context.Context, appContainer *container.Container) error {
	// Инициализируем роутер
	appRouter := router.NewRouter(appContainer)

	// Инициализируем shutdown manager
	shutdownManager := shutdown.NewManager(defaultShutdownTimeout * time.Second)
	shutdownManager.Register(shutdown.NewDownloadManagerShutdown(appContainer.GetDownloadManager()))
	shutdownManager.Register(shutdown.NewDatabaseShutdown(database.GlobalDB))

	serverCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go processUpdates(serverCtx, appRouter, appContainer.GetBot())

	logger.Log.Info("Telegram Media Server started successfully")

	// Ждем сигнал завершения и выполняем graceful shutdown
	if err := shutdownManager.WaitForShutdown(); err != nil {
		logger.Log.WithError(err).Error("Graceful shutdown completed with errors")
		return err
	}

	logger.Log.Info("Telegram Media Server shutdown complete")
	return nil
}

// shutdownServer выполняет graceful shutdown
func shutdownServer(_ context.Context, appContainer *container.Container) error {
	// Инициализируем shutdown manager
	shutdownManager := shutdown.NewManager(defaultShutdownTimeout * time.Second)
	shutdownManager.Register(shutdown.NewDownloadManagerShutdown(appContainer.GetDownloadManager()))
	shutdownManager.Register(shutdown.NewDatabaseShutdown(database.GlobalDB))

	// Выполняем graceful shutdown
	return shutdownManager.Shutdown()
}

// processUpdates обрабатывает обновления от Telegram
func processUpdates(
	ctx context.Context,
	appRouter *router.Router,
	botInstance domain.BotInterface,
) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := botInstance.GetUpdatesChan(u)

	for {
		select {
		case update := <-updates:
			appRouter.HandleUpdate(ctx, &update)
		case <-ctx.Done():
			logger.Log.Info("Stopping update processing")
			return
		}
	}
}

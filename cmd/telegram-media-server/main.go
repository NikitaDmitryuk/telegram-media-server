package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	// Создаем приложение
	application, err := app.New()
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	// Логируем информацию о запуске
	logger.Log.WithFields(map[string]any{
		"version":    Version,
		"build_time": BuildTime,
	}).Info("Starting Telegram Media Server")

	// Создаем контекст для graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Настраиваем обработку сигналов
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)

	// Запускаем приложение в горутине
	go func() {
		if err := application.Run(ctx); err != nil {
			logger.Log.WithError(err).Error("Application run failed")
			cancel()
		}
	}()

	// Ждем сигнал остановки
	<-stopChan
	logger.Log.Info("Shutting down...")

	// Выполняем graceful shutdown
	if err := application.Shutdown(ctx); err != nil {
		logger.Log.WithError(err).Error("Error during shutdown")
	}

	logger.Log.Info("Server stopped")
}

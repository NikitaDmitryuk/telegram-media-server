package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	tmsdownloadmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/common"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/downloads"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	config, err := tmsconfig.NewConfig()
	if err != nil {
		logutils.Log.WithError(err).Fatal("Failed to initialize configuration")
	}

	logutils.InitLogger(config.LogLevel)
	logutils.Log.WithFields(map[string]any{
		"version":    Version,
		"build_time": BuildTime,
	}).Info("Starting Telegram Media Server")

	if dbErr := database.InitDatabase(config); dbErr != nil {
		logutils.Log.WithError(dbErr).Fatal("Failed to initialize the database")
	}
	db := database.GlobalDB

	if langErr := lang.InitLocalizer(config); langErr != nil {
		logutils.Log.WithError(langErr).Fatal("Failed to initialize localizer")
	}

	downloadManager := tmsdownloadmanager.NewDownloadManager(config, db)
	logutils.Log.Info("Download manager initialized")

	botInstance, err := tmsbot.InitBot(config)
	if err != nil {
		logutils.Log.WithError(err).Fatal("Bot initialization failed")
	}

	downloads.InitNotificationHandler(botInstance, downloadManager)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go processUpdates(ctx, botInstance, db, config, downloadManager)

	logutils.Log.Info("Telegram Media Server started successfully")

	<-sigChan
	logutils.Log.Info("Received shutdown signal, starting graceful shutdown...")

	cancel()

	downloadManager.StopAllDownloads()
	logutils.Log.Info("All downloads stopped")

	logutils.Log.Info("Telegram Media Server shutdown complete")
}

func processUpdates(
	ctx context.Context,
	bot *tmsbot.Bot,
	db database.Database,
	config *tmsconfig.Config,
	downloadManager *tmsdownloadmanager.DownloadManager,
) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.Api.GetUpdatesChan(u)

	for {
		select {
		case update := <-updates:
			common.Router(bot, &update, db, config, downloadManager)
		case <-ctx.Done():
			logutils.Log.Info("Stopping update processing")
			return
		}
	}
}

package main

import (
	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	tmsdownloadmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/common"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	if err := tmsconfig.InitConfig(); err != nil {
		logutils.Log.Fatal("Failed to initialize configuration")
	}

	logutils.InitLogger(tmsconfig.GlobalConfig.LogLevel)

	if err := database.InitDatabase(tmsconfig.GlobalConfig); err != nil {
		logutils.Log.Fatal("Failed to initialize the database")
	}

	if err := lang.InitLocalizer(); err != nil {
		logutils.Log.Fatal("Failed to initialize localizer")
	}

	tmsdownloadmanager.InitDownloadManager()

	botInstance, err := tmsbot.InitBot(tmsconfig.GlobalConfig)
	if err != nil {
		logutils.Log.WithError(err).Fatal("Bot initialization failed")
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := botInstance.Api.GetUpdatesChan(u)

	for update := range updates {
		common.Router(botInstance, &update)
	}
}

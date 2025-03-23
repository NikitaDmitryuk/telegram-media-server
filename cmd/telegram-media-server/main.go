package main

import (
	"github.com/sirupsen/logrus"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers"
	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {

	if err := tmslang.SetupLang(tmsconfig.GlobalConfig); err != nil {
		logrus.WithError(err).Error("Failed to load messages")
	}

	botInstance, err := tmsbot.InitBot(tmsconfig.GlobalConfig)

	if err != nil {
		logrus.WithError(err).Error("Bot initialization failed")
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := botInstance.Api.GetUpdatesChan(u)

	for update := range updates {
		handlers.Router(botInstance, update)
	}
}

package main

import (
	"github.com/sirupsen/logrus"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {

	botInstance, err := tmsbot.InitBot(tmsconfig.GlobalConfig)

	if err != nil {
		logrus.WithError(err).Fatal("Bot initialization failed")
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := botInstance.Api.GetUpdatesChan(u)

	for update := range updates {
		handlers.Router(botInstance, update)
	}
}

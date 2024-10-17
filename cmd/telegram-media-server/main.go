package main

import (
	"log"
	"sync"

	tmsapi "github.com/NikitaDmitryuk/telegram-media-server/internal/api"
	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/db"
	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {

	config, err := tmsconfig.NewConfig()

	if err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	if err := tmslang.SetupLang(config); err != nil {
		log.Fatalf("Failed to load messages: %v", err)
	}

	if err := tmsdb.DBInit(config); err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}

	updates, err := tmsbot.InitBot(config)

	if err != nil {
		log.Fatalf("Bot initialization failed: %v", err)
	}

	var wg sync.WaitGroup

	for update := range updates {
		wg.Add(1)
		go func(update tgbotapi.Update) {
			defer wg.Done()
			tmsapi.HandleUpdates(update)
		}(update)
	}

	wg.Wait()
}

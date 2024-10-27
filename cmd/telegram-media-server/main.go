package main

import (
	"log"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/db"
	tmsapi "github.com/NikitaDmitryuk/telegram-media-server/internal/handlers"
	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
)

func main() {

	config, err := tmsconfig.NewConfig()

	if err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	if err := tmslang.SetupLang(config); err != nil {
		log.Fatalf("Failed to load messages: %v", err)
	}

	db, err := tmsdb.DBInit(config)

	if err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}

	bot, err := tmsbot.InitBot(config, db)

	if err != nil {
		log.Fatalf("Bot initialization failed: %v", err)
	}

	tmsapi.HandleUpdates(bot)

}

package main

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"sync"
)

var GlobalConfig *Config
var GlobalBot *tgbotapi.BotAPI
var lang string

func init() {
	GlobalConfig = NewConfig()
	if err := GlobalConfig.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}
	if err := dbInit(); err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}
	if err := initBot(); err != nil {
		log.Fatalf("Bot initialization failed: %v", err)
	}
	lang = GlobalConfig.Lang
	if err := LoadMessagesFromFile(GlobalConfig.MessageFilePath); err != nil {
		log.Fatalf("Не удалось загрузить сообщения: %v", err)
	}
}

func initBot() error {
	bot, err := tgbotapi.NewBotAPI(GlobalConfig.BotToken)
	if err != nil {
		return fmt.Errorf("error creating bot: %v", err)
	}
	bot.Debug = true
	GlobalBot = bot
	log.Printf("Authorized on account %s", bot.Self.UserName)
	return nil
}

func main() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := GlobalBot.GetUpdatesChan(u)

	var wg sync.WaitGroup

	for update := range updates {
		wg.Add(1)
		go func(update tgbotapi.Update) {
			defer wg.Done()
			orchestrator(update)
		}(update)
	}

	wg.Wait()
}

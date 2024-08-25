package main

import (
	"log"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var GlobalConfig *Config
var GlobalBot *tgbotapi.BotAPI

func init() {
	GlobalConfig = NewConfig()
	initDB()
	initBot()
}

func initBot() {
	bot, err := tgbotapi.NewBotAPI(GlobalConfig.BotToken)
	if err != nil {
		log.Fatalf("Error creating bot: %v", err)
	}
	bot.Debug = true
	GlobalBot = bot
	log.Printf("Authorized on account %s", bot.Self.UserName)
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

package main

import (
	"log"

	"fmt"

	"github.com/joho/godotenv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var GlobalConfig *Config
var GlobalBot *tgbotapi.BotAPI

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Panic(err)
		fmt.Println("Error loading .env file")
		return
	}
	GlobalConfig = NewConfig()
	initDB()

	bot, err := tgbotapi.NewBotAPI(GlobalConfig.BotToken)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true

	GlobalBot = bot

	log.Printf("Authorized on account %s", bot.Self.UserName)
}

func main() {

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := GlobalBot.GetUpdatesChan(u)

	for update := range updates {
		go orchestrator(update)
	}
}

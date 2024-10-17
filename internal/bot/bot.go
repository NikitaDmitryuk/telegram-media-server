package bot

import (
	"fmt"
	"log"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api *tgbotapi.BotAPI
}

func InitBot(config *tmsconfig.Config) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(config.BotToken)
	if err != nil {
		return nil, fmt.Errorf("Error creating bot: %v", err)
	}

	log.Printf("Authorized on account %s", api.Self.UserName)
	return &Bot{api: api}, nil
}

func (b *Bot) SendErrorMessage(chatID int64, message string) {
	log.Print(message)
	msg := tgbotapi.NewMessage(chatID, message)
	b.api.Send(msg)
}

func (b *Bot) SendSuccessMessage(chatID int64, message string) {
	msg := tgbotapi.NewMessage(chatID, message)
	b.api.Send(msg)
}

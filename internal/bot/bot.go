package bot

import (
	"fmt"
	"log"

	"database/sql"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	tmsdownloader "github.com/NikitaDmitryuk/telegram-media-server/internal/download-manager"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api             *tgbotapi.BotAPI
	config          *tmsconfig.Config
	db              *sql.DB
	DownloadManager *tmsdownloader.DownloadManager
}

func InitBot(config *tmsconfig.Config, db *sql.DB) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(config.BotToken)
	if err != nil {
		return nil, fmt.Errorf("error creating bot: %v", err)
	}
	log.Printf("Authorized on account %s", api.Self.UserName)
	return &Bot{api: api, config: config, db: db, DownloadManager: tmsdownloader.NewDownloadManager()}, nil
}

func (b *Bot) SendErrorMessage(chatID int64, message string) {
	log.Println(message)
	msg := tgbotapi.NewMessage(chatID, message)
	if smsg, err := b.api.Send(msg); err != nil {
		log.Printf("Message (%s) not send: %v", smsg.Text, err)
	}
}

func (b *Bot) SendSuccessMessage(chatID int64, message string) {
	msg := tgbotapi.NewMessage(chatID, message)
	if smsg, err := b.api.Send(msg); err != nil {
		log.Printf("Message (%s) not send: %v", smsg.Text, err)
	}
}

func (b *Bot) GetAPI() *tgbotapi.BotAPI {
	return b.api
}

func (b *Bot) GetConfig() *tmsconfig.Config {
	return b.config
}

func (b *Bot) GetDB() *sql.DB {
	return b.db
}

func (b *Bot) GetDownloadManager() *tmsdownloader.DownloadManager {
	return b.DownloadManager
}

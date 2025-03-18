package bot

import (
	"fmt"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

type Bot struct {
	Api *tgbotapi.BotAPI
}

func InitBot(config *tmsconfig.Config) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(config.BotToken)
	if err != nil {
		logrus.WithError(err).Error("Error creating bot")
		return nil, fmt.Errorf("error creating bot: %v", err)
	}
	logrus.Infof("Authorized on account %s", api.Self.UserName)
	return &Bot{Api: api}, nil
}

func (b *Bot) SendErrorMessage(chatID int64, message string) {
	logrus.Error(message)
	msg := tgbotapi.NewMessage(chatID, message)
	if smsg, err := b.Api.Send(msg); err != nil {
		logrus.WithError(err).Errorf("Message (%s) not sent", smsg.Text)
	}
}

func (b *Bot) SendSuccessMessage(chatID int64, message string) {
	msg := tgbotapi.NewMessage(chatID, message)
	if smsg, err := b.Api.Send(msg); err != nil {
		logrus.WithError(err).Errorf("Message (%s) not sent", smsg.Text)
	} else {
		logrus.Infof("Message (%s) sent successfully", smsg.Text)
	}
}

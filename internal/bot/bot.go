package bot

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

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
	logrus.Warn(message)
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

func (b *Bot) Send(msg tgbotapi.Chattable) {
	if smsg, err := b.Api.Send(msg); err != nil {
		logrus.WithError(err).Errorf("Message (%s) not sent", smsg.Text)
	} else {
		logrus.Infof("Message (%s) sent successfully", smsg.Text)
	}
}

func (b *Bot) DownloadFile(fileID, fileName string) error {
	file, err := b.Api.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		logrus.WithError(err).Error("Failed to get file")
		return err
	}

	fileURL := file.Link(b.Api.Token)
	resp, err := http.Get(fileURL)
	if err != nil {
		logrus.WithError(err).Error("Failed to download file")
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath.Join(tmsconfig.GlobalConfig.MoviePath, fileName))
	if err != nil {
		logrus.WithError(err).Error("Failed to create file")
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		logrus.WithError(err).Error("Failed to save file")
		return err
	}

	logrus.Info("File downloaded successfully")
	return nil
}

func (b *Bot) AnswerCallbackQuery(callbackConfig tgbotapi.CallbackConfig) {
	if _, err := b.Api.Request(callbackConfig); err != nil {
		logrus.WithError(err).Error("Failed to answer callback query")
	}
}

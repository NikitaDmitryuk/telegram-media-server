package bot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	Api *tgbotapi.BotAPI
}

func InitBot(config *tmsconfig.Config) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(config.BotToken)
	if err != nil {
		logutils.Log.WithError(err).Error("Error creating bot")
		return nil, fmt.Errorf("error creating bot: %w", err)
	}
	logutils.Log.Infof("Authorized on account %s", api.Self.UserName)
	return &Bot{Api: api}, nil
}

func (b *Bot) SendMessage(chatID int64, text string, keyboard any) {
	msg := tgbotapi.NewMessage(chatID, text)
	if keyboard != nil {
		switch k := keyboard.(type) {
		case tgbotapi.ReplyKeyboardMarkup:
			msg.ReplyMarkup = k
		case tgbotapi.ReplyKeyboardRemove:
			msg.ReplyMarkup = k
		case tgbotapi.InlineKeyboardMarkup:
			msg.ReplyMarkup = k
		}
	}
	if _, err := b.Api.Send(msg); err != nil {
		logutils.Log.WithError(err).Errorf("Message not sent: %s", text)
	}
}

func (b *Bot) DownloadFile(fileID, fileName string) error {
	file, err := b.Api.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get file")
		return err
	}

	fileURL := file.Link(b.Api.Token)
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, http.NoBody)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to create HTTP request")
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to download file")
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath.Join(tmsconfig.GlobalConfig.MoviePath, fileName))
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to create file")
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, resp.Body); err != nil {
		logutils.Log.WithError(err).Error("Failed to save file")
		return err
	}

	logutils.Log.Info("File downloaded successfully")
	return nil
}

func (b *Bot) AnswerCallbackQuery(callbackConfig tgbotapi.CallbackConfig) {
	if _, err := b.Api.Request(callbackConfig); err != nil {
		logutils.Log.WithError(err).Error("Failed to answer callback query")
	} else {
		logutils.Log.Info("Callback query answered successfully")
	}
}

func (b *Bot) DeleteMessage(chatID int64, messageID int) error {
	deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
	_, err := b.Api.Request(deleteMsg)
	if err != nil {
		logutils.Log.WithError(err).Errorf("Failed to delete message %d in chat %d", messageID, chatID)
	}
	return err
}

func SaveFile(fileName string, data []byte) error {
	path := filepath.Join(tmsconfig.GlobalConfig.MoviePath, fileName)
	f, err := os.Create(path)
	if err != nil {
		logutils.Log.WithError(err).Errorf("Failed to create file %s", path)
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	if err != nil {
		logutils.Log.WithError(err).Errorf("Failed to write file %s", path)
		return err
	}
	logutils.Log.Infof("File saved: %s", path)
	return nil
}

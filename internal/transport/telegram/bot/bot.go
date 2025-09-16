package bot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot реализует интерфейс BotInterface
type Bot struct {
	Api    *tgbotapi.BotAPI
	Config *domain.Config
}

// Проверяем, что Bot реализует интерфейс BotInterface
var _ domain.BotInterface = (*Bot)(nil)

// NewBot создает новый экземпляр бота
func NewBot(botToken string) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		logger.Log.WithError(err).Error("Error creating bot")
		return nil, fmt.Errorf("error creating bot: %w", err)
	}
	logger.Log.Infof("Authorized on account %s", api.Self.UserName)
	return &Bot{Api: api, Config: nil}, nil
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
		logger.Log.WithError(err).Errorf("Message not sent: %s", text)
	}
}

func (b *Bot) DownloadFile(fileID, fileName string) error {
	file, err := b.Api.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get file")
		return err
	}

	fileURL := file.Link(b.Api.Token)
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, http.NoBody)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create HTTP request")
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to download file")
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath.Join(b.Config.MoviePath, fileName))
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create file")
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, resp.Body); err != nil {
		logger.Log.WithError(err).Error("Failed to save file")
		return err
	}

	logger.Log.Info("File downloaded successfully")
	return nil
}

// AnswerCallbackQuery отвечает на callback query
func (b *Bot) AnswerCallbackQuery(callbackConfig tgbotapi.CallbackConfig) {
	if _, err := b.Api.Request(callbackConfig); err != nil {
		logger.Log.WithError(err).Error("Failed to answer callback query")
	} else {
		logger.Log.Info("Callback query answered successfully")
	}
}

func (b *Bot) DeleteMessage(chatID int64, messageID int) error {
	deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
	_, err := b.Api.Request(deleteMsg)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to delete message %d in chat %d", messageID, chatID)
	}
	return err
}

// SendMessageWithMarkup отправляет сообщение с inline клавиатурой
func (b *Bot) SendMessageWithMarkup(chatID int64, text string, markup tgbotapi.InlineKeyboardMarkup) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = markup

	_, err := b.Api.Send(msg)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send message with markup to chat %d", chatID)
		return err
	}

	logger.Log.Debugf("Message with markup sent to chat %d", chatID)
	return nil
}

func (b *Bot) SaveFile(fileName string, data []byte) error {
	path := filepath.Join(b.Config.MoviePath, fileName)
	f, err := os.Create(path)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to create file %s", path)
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to write file %s", path)
		return err
	}
	logger.Log.Infof("File saved: %s", path)
	return nil
}

// GetConfig возвращает конфигурацию бота
func (b *Bot) GetConfig() *domain.Config {
	return b.Config
}

// GetFileDirectURL получает прямую ссылку на файл
func (b *Bot) GetFileDirectURL(fileID string) (string, error) {
	file, err := b.Api.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return "", fmt.Errorf("failed to get file info: %w", err)
	}

	return file.Link(b.Api.Token), nil
}

// GetUpdatesChan возвращает канал для получения обновлений
func (b *Bot) GetUpdatesChan(config tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel {
	return b.Api.GetUpdatesChan(config)
}

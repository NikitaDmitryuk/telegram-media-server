package notification

import (
	"context"
	"fmt"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
)

// TelegramNotificationService отправляет уведомления через Telegram
type TelegramNotificationService struct {
	bot domain.BotInterface
}

// NewTelegramNotificationService создает новый сервис уведомлений
func NewTelegramNotificationService(bot domain.BotInterface) domain.NotificationService {
	return &TelegramNotificationService{
		bot: bot,
	}
}

// NotifyDownloadStarted уведомляет о начале загрузки
func (s *TelegramNotificationService) NotifyDownloadStarted(_ context.Context, chatID int64, title string) error {
	message := lang.Translate("general.downloads.notifications.started", map[string]any{
		"title": title,
	})

	// Если локализация не сработала, используем fallback из старой версии
	if message == "general.downloads.notifications.started" {
		message = lang.Translate("general.video_downloading", map[string]any{
			"Title": title,
		})
	}

	s.bot.SendMessage(chatID, message, nil)
	return nil
}

// NotifyDownloadProgress уведомляет о прогрессе загрузки
func (s *TelegramNotificationService) NotifyDownloadProgress(_ context.Context, chatID int64, title string, progress int) error {
	// Отправляем уведомления о прогрессе только каждые 25%
	if progress%25 != 0 {
		return nil
	}

	// Пробуем основной ключ
	message := lang.Translate("general.downloads.notifications.progress", map[string]any{
		"title":    title,
		"progress": progress,
	})

	// Если не сработал, используем fallback
	if message == "general.downloads.notifications.progress" {
		message = lang.Translate("general.download.progress", map[string]any{
			"Name":     title,
			"Progress": progress,
		})

		// Если и fallback не сработал, используем простое сообщение
		if message == "general.download.progress" {
			message = fmt.Sprintf("📊 %s: %d%%", title, progress)
		}
	}

	s.bot.SendMessage(chatID, message, nil)
	return nil
}

// NotifyDownloadCompleted уведомляет о завершении загрузки
func (s *TelegramNotificationService) NotifyDownloadCompleted(_ context.Context, chatID int64, title string) error {
	// Пробуем основной ключ
	message := lang.Translate("general.downloads.notifications.completed", map[string]any{
		"title": title,
	})

	// Если не сработал, используем fallback
	if message == "general.downloads.notifications.completed" {
		message = lang.Translate("general.video_successfully_downloaded", map[string]any{
			"Title": title,
		})

		// Если и fallback не сработал, используем простое сообщение
		if message == "general.video_successfully_downloaded" {
			message = fmt.Sprintf("✅ Загрузка завершена: %s", title)
		}
	}

	s.bot.SendMessage(chatID, message, nil)
	return nil
}

// NotifyDownloadFailed уведомляет об ошибке загрузки
func (s *TelegramNotificationService) NotifyDownloadFailed(_ context.Context, chatID int64, title string, downloadErr error) error {
	message := lang.Translate("general.downloads.notifications.failed", map[string]any{
		"title": title,
		"error": downloadErr.Error(),
	})

	// Если локализация не сработала, используем fallback
	if message == "general.downloads.notifications.failed" {
		message = lang.Translate("error.downloads.video_download_error", map[string]any{
			"Error": downloadErr.Error(),
		})
	}

	s.bot.SendMessage(chatID, message, nil)
	return nil
}

// MockNotificationService для тестирования
type MockNotificationService struct {
	StartedCalls   []NotificationCall
	ProgressCalls  []ProgressCall
	CompletedCalls []NotificationCall
	FailedCalls    []FailedCall
	ShouldError    bool
	ErrorToReturn  error
}

// NotificationCall представляет вызов уведомления
type NotificationCall struct {
	ChatID int64
	Title  string
}

// ProgressCall представляет вызов уведомления о прогрессе
type ProgressCall struct {
	ChatID   int64
	Title    string
	Progress int
}

// FailedCall представляет вызов уведомления об ошибке
type FailedCall struct {
	ChatID int64
	Title  string
	Error  error
}

// NewMockNotificationService создает mock сервис уведомлений
func NewMockNotificationService() *MockNotificationService {
	return &MockNotificationService{
		StartedCalls:   make([]NotificationCall, 0),
		ProgressCalls:  make([]ProgressCall, 0),
		CompletedCalls: make([]NotificationCall, 0),
		FailedCalls:    make([]FailedCall, 0),
	}
}

// SetError заставляет сервис возвращать ошибку
func (m *MockNotificationService) SetError(err error) {
	m.ShouldError = true
	m.ErrorToReturn = err
}

// Reset очищает все записанные вызовы
func (m *MockNotificationService) Reset() {
	m.StartedCalls = nil
	m.ProgressCalls = nil
	m.CompletedCalls = nil
	m.FailedCalls = nil
	m.ShouldError = false
	m.ErrorToReturn = nil
}

// NotifyDownloadStarted mock реализация
func (m *MockNotificationService) NotifyDownloadStarted(_ context.Context, chatID int64, title string) error {
	m.StartedCalls = append(m.StartedCalls, NotificationCall{
		ChatID: chatID,
		Title:  title,
	})

	if m.ShouldError {
		return m.ErrorToReturn
	}
	return nil
}

// NotifyDownloadProgress mock реализация
func (m *MockNotificationService) NotifyDownloadProgress(_ context.Context, chatID int64, title string, progress int) error {
	m.ProgressCalls = append(m.ProgressCalls, ProgressCall{
		ChatID:   chatID,
		Title:    title,
		Progress: progress,
	})

	if m.ShouldError {
		return m.ErrorToReturn
	}
	return nil
}

// NotifyDownloadCompleted mock реализация
func (m *MockNotificationService) NotifyDownloadCompleted(_ context.Context, chatID int64, title string) error {
	m.CompletedCalls = append(m.CompletedCalls, NotificationCall{
		ChatID: chatID,
		Title:  title,
	})

	if m.ShouldError {
		return m.ErrorToReturn
	}
	return nil
}

// NotifyDownloadFailed mock реализация
func (m *MockNotificationService) NotifyDownloadFailed(_ context.Context, chatID int64, title string, err error) error {
	m.FailedCalls = append(m.FailedCalls, FailedCall{
		ChatID: chatID,
		Title:  title,
		Error:  err,
	})

	if m.ShouldError {
		return m.ErrorToReturn
	}
	return nil
}

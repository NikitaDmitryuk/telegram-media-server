package notification

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MockBot implements BotInterface for testing
type MockBot struct {
	SentMessages []MockMessage
}

type MockMessage struct {
	ChatID   int64
	Text     string
	Keyboard any
}

func (m *MockBot) SendMessage(chatID int64, text string, keyboard any) {
	m.SentMessages = append(m.SentMessages, MockMessage{
		ChatID:   chatID,
		Text:     text,
		Keyboard: keyboard,
	})
}

// Implement other required methods with defaults
func (m *MockBot) SendMessageWithMarkup(chatID int64, text string, markup tgbotapi.InlineKeyboardMarkup) error {
	m.SendMessage(chatID, text, markup)
	return nil
}

func (m *MockBot) DownloadFile(fileID, fileName string) error                          { return nil }
func (m *MockBot) SaveFile(fileName string, data []byte) error                         { return nil }
func (m *MockBot) AnswerCallbackQuery(callbackConfig tgbotapi.CallbackConfig)          {}
func (m *MockBot) DeleteMessage(chatID int64, messageID int) error                     { return nil }
func (m *MockBot) GetFileDirectURL(fileID string) (string, error)                      { return "", nil }
func (m *MockBot) GetConfig() *domain.Config                                           { return testutils.TestConfig("/tmp") }
func (m *MockBot) GetUpdatesChan(config tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel { return nil }

func TestNotificationService(t *testing.T) {
	// Initialize logger for tests
	logger.InitLogger("debug")

	// Initialize localization
	cfg := testutils.TestConfig("/tmp")
	cfg.LangPath = "../../../locales"
	err := lang.InitLocalizer(cfg)
	if err != nil {
		t.Logf("Failed to initialize localizer (using fallbacks): %v", err)
	}

	mockBot := &MockBot{}
	service := NewTelegramNotificationService(mockBot)

	ctx := context.Background()
	chatID := int64(12345)
	title := "test-download.torrent"

	t.Run("NotifyDownloadStarted", func(t *testing.T) {
		mockBot.SentMessages = nil // Reset

		err := service.NotifyDownloadStarted(ctx, chatID, title)
		if err != nil {
			t.Errorf("NotifyDownloadStarted failed: %v", err)
		}

		if len(mockBot.SentMessages) != 1 {
			t.Fatalf("Expected 1 message, got %d", len(mockBot.SentMessages))
		}

		msg := mockBot.SentMessages[0]
		if msg.ChatID != chatID {
			t.Errorf("Expected chat ID %d, got %d", chatID, msg.ChatID)
		}

		// Should contain either the localized message or fallback
		if !strings.Contains(msg.Text, title) {
			t.Errorf("Message should contain title '%s', got: %s", title, msg.Text)
		}

		// Should contain either 🚀 (new) or ⬇️ (fallback) emoji
		if !strings.Contains(msg.Text, "🚀") && !strings.Contains(msg.Text, "⬇️") {
			t.Errorf("Message should contain start emoji, got: %s", msg.Text)
		}
	})

	t.Run("NotifyDownloadProgress", func(t *testing.T) {
		mockBot.SentMessages = nil // Reset

		// Test progress notification (should only send at 25% intervals)
		for progress := 0; progress <= 100; progress += 10 {
			err := service.NotifyDownloadProgress(ctx, chatID, title, progress)
			if err != nil {
				t.Errorf("NotifyDownloadProgress(%d) failed: %v", progress, err)
			}
		}

		// Should send notifications only for 0%, 25%, 50%, 75%, 100%
		expectedCount := 5
		if len(mockBot.SentMessages) != expectedCount {
			t.Errorf("Expected %d progress messages, got %d", expectedCount, len(mockBot.SentMessages))
		}

		// Check each message contains the expected progress
		expectedProgresses := []int{0, 25, 50, 75, 100}
		for i, msg := range mockBot.SentMessages {
			expectedProgress := expectedProgresses[i]

			if !strings.Contains(msg.Text, fmt.Sprintf("%d%%", expectedProgress)) {
				t.Errorf("Message %d should contain progress %d%%, got: %s", i, expectedProgress, msg.Text)
			}

			if !strings.Contains(msg.Text, title) {
				t.Errorf("Progress message should contain title '%s', got: %s", title, msg.Text)
			}

			// Should contain either 📊 (new) or progress text (fallback)
			if !strings.Contains(msg.Text, "📊") && !strings.Contains(msg.Text, "Загрузка") {
				t.Errorf("Progress message should contain progress indicator, got: %s", msg.Text)
			}
		}
	})

	t.Run("NotifyDownloadCompleted", func(t *testing.T) {
		mockBot.SentMessages = nil // Reset

		err := service.NotifyDownloadCompleted(ctx, chatID, title)
		if err != nil {
			t.Errorf("NotifyDownloadCompleted failed: %v", err)
		}

		if len(mockBot.SentMessages) != 1 {
			t.Fatalf("Expected 1 message, got %d", len(mockBot.SentMessages))
		}

		msg := mockBot.SentMessages[0]
		if msg.ChatID != chatID {
			t.Errorf("Expected chat ID %d, got %d", chatID, msg.ChatID)
		}

		if !strings.Contains(msg.Text, title) {
			t.Errorf("Completion message should contain title '%s', got: %s", title, msg.Text)
		}

		// Should contain ✅ emoji for completion
		if !strings.Contains(msg.Text, "✅") {
			t.Errorf("Completion message should contain ✅ emoji, got: %s", msg.Text)
		}
	})

	t.Run("NotifyDownloadFailed", func(t *testing.T) {
		mockBot.SentMessages = nil // Reset

		testError := fmt.Errorf("test download error")
		err := service.NotifyDownloadFailed(ctx, chatID, title, testError)
		if err != nil {
			t.Errorf("NotifyDownloadFailed failed: %v", err)
		}

		if len(mockBot.SentMessages) != 1 {
			t.Fatalf("Expected 1 message, got %d", len(mockBot.SentMessages))
		}

		msg := mockBot.SentMessages[0]
		if msg.ChatID != chatID {
			t.Errorf("Expected chat ID %d, got %d", chatID, msg.ChatID)
		}

		if !strings.Contains(msg.Text, title) {
			t.Errorf("Error message should contain title '%s', got: %s", title, msg.Text)
		}

		// Should contain ❌ emoji for error
		if !strings.Contains(msg.Text, "❌") {
			t.Errorf("Error message should contain ❌ emoji, got: %s", msg.Text)
		}
	})

	t.Run("ProgressIntervals", func(t *testing.T) {
		mockBot.SentMessages = nil // Reset

		// Test that only 25% intervals trigger notifications
		testProgresses := []int{1, 12, 24, 25, 26, 49, 50, 51, 74, 75, 76, 99, 100}
		expectedNotifications := []int{25, 50, 75, 100} // Only these should trigger

		for _, progress := range testProgresses {
			err := service.NotifyDownloadProgress(ctx, chatID, title, progress)
			if err != nil {
				t.Errorf("NotifyDownloadProgress(%d) failed: %v", progress, err)
			}
		}

		if len(mockBot.SentMessages) != len(expectedNotifications) {
			t.Errorf("Expected %d notifications for intervals, got %d", len(expectedNotifications), len(mockBot.SentMessages))
		}

		for i, expectedProgress := range expectedNotifications {
			if i >= len(mockBot.SentMessages) {
				break
			}
			msg := mockBot.SentMessages[i]
			if !strings.Contains(msg.Text, fmt.Sprintf("%d%%", expectedProgress)) {
				t.Errorf("Notification %d should contain %d%%, got: %s", i, expectedProgress, msg.Text)
			}
		}
	})
}

func TestNotificationServiceFallbacks(t *testing.T) {
	// Test without proper localization to ensure fallbacks work
	mockBot := &MockBot{}
	service := NewTelegramNotificationService(mockBot)

	ctx := context.Background()
	chatID := int64(12345)
	title := "fallback-test.torrent"

	t.Run("FallbackMessages", func(t *testing.T) {
		// Test all notification types to ensure they never fail
		mockBot.SentMessages = nil

		// Start notification
		err := service.NotifyDownloadStarted(ctx, chatID, title)
		if err != nil {
			t.Errorf("Start notification failed: %v", err)
		}

		// Progress notification
		err = service.NotifyDownloadProgress(ctx, chatID, title, 50)
		if err != nil {
			t.Errorf("Progress notification failed: %v", err)
		}

		// Completion notification
		err = service.NotifyDownloadCompleted(ctx, chatID, title)
		if err != nil {
			t.Errorf("Completion notification failed: %v", err)
		}

		// Error notification
		testError := fmt.Errorf("test error")
		err = service.NotifyDownloadFailed(ctx, chatID, title, testError)
		if err != nil {
			t.Errorf("Error notification failed: %v", err)
		}

		// All should succeed and send messages
		expectedCount := 4
		if len(mockBot.SentMessages) != expectedCount {
			t.Errorf("Expected %d fallback messages, got %d", expectedCount, len(mockBot.SentMessages))
		}

		// Each message should contain the title
		for i, msg := range mockBot.SentMessages {
			if !strings.Contains(msg.Text, title) {
				t.Errorf("Fallback message %d should contain title '%s', got: %s", i, title, msg.Text)
			}

			// Should not be empty
			if strings.TrimSpace(msg.Text) == "" {
				t.Errorf("Fallback message %d should not be empty", i)
			}
		}
	})
}

func TestNotificationServiceMockImplementation(t *testing.T) {
	// Test the mock notification service
	mockService := NewMockNotificationService()

	ctx := context.Background()
	chatID := int64(12345)
	title := "mock-test.torrent"

	t.Run("MockRecordsAllCalls", func(t *testing.T) {
		// Make all types of calls
		err := mockService.NotifyDownloadStarted(ctx, chatID, title)
		if err != nil {
			t.Errorf("Mock start failed: %v", err)
		}

		err = mockService.NotifyDownloadProgress(ctx, chatID, title, 75)
		if err != nil {
			t.Errorf("Mock progress failed: %v", err)
		}

		err = mockService.NotifyDownloadCompleted(ctx, chatID, title)
		if err != nil {
			t.Errorf("Mock completion failed: %v", err)
		}

		testError := fmt.Errorf("mock error")
		err = mockService.NotifyDownloadFailed(ctx, chatID, title, testError)
		if err != nil {
			t.Errorf("Mock error failed: %v", err)
		}

		// Check that all calls were recorded
		if len(mockService.StartedCalls) != 1 {
			t.Errorf("Expected 1 start call, got %d", len(mockService.StartedCalls))
		}

		if len(mockService.ProgressCalls) != 1 {
			t.Errorf("Expected 1 progress call, got %d", len(mockService.ProgressCalls))
		}

		if len(mockService.CompletedCalls) != 1 {
			t.Errorf("Expected 1 completion call, got %d", len(mockService.CompletedCalls))
		}

		if len(mockService.FailedCalls) != 1 {
			t.Errorf("Expected 1 error call, got %d", len(mockService.FailedCalls))
		}

		// Check call details
		if mockService.StartedCalls[0].Title != title {
			t.Errorf("Start call title mismatch: expected %s, got %s", title, mockService.StartedCalls[0].Title)
		}

		if mockService.ProgressCalls[0].Progress != 75 {
			t.Errorf("Progress call value mismatch: expected 75, got %d", mockService.ProgressCalls[0].Progress)
		}
	})

	t.Run("MockErrorHandling", func(t *testing.T) {
		mockService := NewMockNotificationService()
		testError := fmt.Errorf("mock service error")
		mockService.SetError(testError)

		// All calls should return the set error
		err := mockService.NotifyDownloadStarted(ctx, chatID, title)
		if err != testError {
			t.Errorf("Expected mock error, got: %v", err)
		}

		err = mockService.NotifyDownloadProgress(ctx, chatID, title, 50)
		if err != testError {
			t.Errorf("Expected mock error, got: %v", err)
		}

		err = mockService.NotifyDownloadCompleted(ctx, chatID, title)
		if err != testError {
			t.Errorf("Expected mock error, got: %v", err)
		}

		err = mockService.NotifyDownloadFailed(ctx, chatID, title, fmt.Errorf("original error"))
		if err != testError {
			t.Errorf("Expected mock error, got: %v", err)
		}
	})
}

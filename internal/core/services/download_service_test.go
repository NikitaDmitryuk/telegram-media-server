package services

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/factories"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/downloader/manager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/notification"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/validation"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestDownloadService_HandleTorrentFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Initialize logger for tests
	logger.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)
	cfg.DownloadSettings.DownloadTimeout = 5 * time.Second
	cfg.DownloadSettings.ProgressUpdateInterval = 100 * time.Millisecond

	db := testutils.TestDatabase(t)
	downloadManager := manager.NewDownloadManager(cfg, db)
	mockNotification := notification.NewMockNotificationService()
	mockBot := &MockBotForDownloadService{savedFiles: make(map[string][]byte), files: make(map[string][]byte)}

	urlValidator := validation.NewDefaultURLValidator()
	downloaderFactory := factories.NewDownloaderFactory(urlValidator)

	downloadService := NewDownloadService(
		downloadManager,
		downloaderFactory,
		mockNotification,
		db,
		cfg,
		mockBot,
	)

	ctx := context.Background()
	chatID := int64(12345)

	t.Run("ValidTorrentFile", func(t *testing.T) {
		// Reset mocks
		mockNotification.Reset()
		mockBot.savedFiles = make(map[string][]byte)

		// Create a real torrent file
		torrentPath := testutils.CreateRealTestTorrent(t, tempDir, "test-download")

		// Read the torrent file to simulate Telegram file
		torrentData, err := os.ReadFile(torrentPath)
		if err != nil {
			t.Fatalf("Failed to read test torrent: %v", err)
		}

		fileName := "test-download.torrent"

		err = downloadService.HandleTorrentFile(ctx, torrentData, fileName, chatID)
		if err != nil {
			t.Errorf("HandleTorrentFile failed: %v", err)
		}

		// Verify notification was called
		if len(mockNotification.StartedCalls) != 1 {
			t.Errorf("Expected 1 start notification, got %d", len(mockNotification.StartedCalls))
		}

		if mockNotification.StartedCalls[0].ChatID != chatID {
			t.Errorf("Expected start notification for chat %d, got %d", chatID, mockNotification.StartedCalls[0].ChatID)
		}
	})

	t.Run("InvalidTorrentFile", func(t *testing.T) {
		// Reset mocks
		mockNotification.Reset()
		mockBot.savedFiles = make(map[string][]byte)

		fileName := "invalid.torrent"

		// Mock invalid torrent data (HTML)
		invalidData := []byte(`<!DOCTYPE html><html><body>Error</body></html>`)

		err := downloadService.HandleTorrentFile(ctx, invalidData, fileName, chatID)
		if err == nil {
			t.Error("Expected error for invalid torrent file")
		}

		// Should not send start notification for invalid file
		if len(mockNotification.StartedCalls) != 0 {
			t.Errorf("Expected 0 start notifications for invalid file, got %d", len(mockNotification.StartedCalls))
		}
	})
}

func TestDownloadService_HandleVideoLink(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Initialize logger for tests
	logger.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)
	cfg.DownloadSettings.DownloadTimeout = 5 * time.Second

	db := testutils.TestDatabase(t)
	downloadManager := manager.NewDownloadManager(cfg, db)
	mockNotification := notification.NewMockNotificationService()
	mockBot := &MockBotForDownloadService{savedFiles: make(map[string][]byte), files: make(map[string][]byte)}

	urlValidator := validation.NewDefaultURLValidator()
	downloaderFactory := factories.NewDownloaderFactory(urlValidator)

	downloadService := NewDownloadService(
		downloadManager,
		downloaderFactory,
		mockNotification,
		db,
		cfg,
		mockBot,
	)

	ctx := context.Background()
	chatID := int64(12345)

	t.Run("ValidVideoURL", func(t *testing.T) {
		// Reset mocks
		mockNotification.Reset()

		videoURL := "https://example.com/test-video"

		err := downloadService.HandleVideoLink(ctx, videoURL, chatID)
		if err != nil {
			t.Errorf("HandleVideoLink failed: %v", err)
		}

		// Verify notification was called
		if len(mockNotification.StartedCalls) != 1 {
			t.Errorf("Expected 1 start notification, got %d", len(mockNotification.StartedCalls))
		}

		if mockNotification.StartedCalls[0].ChatID != chatID {
			t.Errorf("Expected start notification for chat %d, got %d", chatID, mockNotification.StartedCalls[0].ChatID)
		}
	})

	t.Run("InvalidVideoURL", func(t *testing.T) {
		// Reset mocks
		mockNotification.Reset()

		invalidURL := "not-a-url"

		err := downloadService.HandleVideoLink(ctx, invalidURL, chatID)
		if err == nil {
			t.Error("Expected error for invalid video URL")
		}

		// Should not send start notification for invalid URL
		if len(mockNotification.StartedCalls) != 0 {
			t.Errorf("Expected 0 start notifications for invalid URL, got %d", len(mockNotification.StartedCalls))
		}
	})
}

// MockBotForDownloadService implements BotInterface for testing download service
type MockBotForDownloadService struct {
	files       map[string][]byte
	savedFiles  map[string][]byte
	shouldError bool
}

func (m *MockBotForDownloadService) DownloadFile(fileID, fileName string) error {
	if m.shouldError {
		return fmt.Errorf("mock download error")
	}

	data, exists := m.files[fileID]
	if !exists {
		return fmt.Errorf("file not found: %s", fileID)
	}

	if m.savedFiles == nil {
		m.savedFiles = make(map[string][]byte)
	}
	m.savedFiles[fileName] = data
	return nil
}

func (m *MockBotForDownloadService) SaveFile(fileName string, data []byte) error {
	if m.shouldError {
		return fmt.Errorf("mock save error")
	}

	if m.savedFiles == nil {
		m.savedFiles = make(map[string][]byte)
	}
	m.savedFiles[fileName] = data
	return nil
}

// Implement other required methods with defaults
func (m *MockBotForDownloadService) SendMessage(chatID int64, text string, keyboard any) {}
func (m *MockBotForDownloadService) SendMessageWithMarkup(chatID int64, text string, markup tgbotapi.InlineKeyboardMarkup) error {
	return nil
}
func (m *MockBotForDownloadService) AnswerCallbackQuery(callbackConfig tgbotapi.CallbackConfig) {}
func (m *MockBotForDownloadService) DeleteMessage(chatID int64, messageID int) error            { return nil }
func (m *MockBotForDownloadService) GetFileDirectURL(fileID string) (string, error)             { return "", nil }
func (m *MockBotForDownloadService) GetConfig() *domain.Config                                  { return testutils.TestConfig("/tmp") }
func (m *MockBotForDownloadService) GetUpdatesChan(config tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel {
	return nil
}

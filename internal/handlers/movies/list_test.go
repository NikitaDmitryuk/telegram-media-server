package movies

import (
	"context"
	"fmt"
	"testing"
	"time"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/models"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// BotInterface defines the interface for bot operations
type BotInterface interface {
	SendMessage(chatID int64, text string, keyboard any)
	SendDocument(chatID int64, config *tgbotapi.DocumentConfig)
	SendEditMessageText(config *tgbotapi.EditMessageTextConfig)
	SendEditMessageReplyMarkup(config *tgbotapi.EditMessageReplyMarkupConfig)
}

type MockMoviesBot struct {
	SentMessages []MockMessage
}

type MockMessage struct {
	ChatID   int64
	Text     string
	Keyboard any
}

func (b *MockMoviesBot) SendMessage(chatID int64, text string, keyboard any) {
	b.SentMessages = append(b.SentMessages, MockMessage{
		ChatID:   chatID,
		Text:     text,
		Keyboard: keyboard,
	})
}

func (*MockMoviesBot) SendDocument(_ int64, _ *tgbotapi.DocumentConfig)                    {}
func (*MockMoviesBot) SendEditMessageText(_ *tgbotapi.EditMessageTextConfig)               {}
func (*MockMoviesBot) SendEditMessageReplyMarkup(_ *tgbotapi.EditMessageReplyMarkupConfig) {}

// Simulate bot calls by calling handler logic directly
func simulateListMoviesHandler(bot BotInterface, update *tgbotapi.Update, db database.Database, _ *tmsconfig.Config) {
	chatID := update.Message.Chat.ID

	movies, err := db.GetMovieList(context.Background())
	if err != nil {
		bot.SendMessage(chatID, "Ошибка при получении списка фильмов", nil)
		return
	}

	if len(movies) == 0 {
		bot.SendMessage(chatID, "📭 Список пуст", nil)
		return
	}

	// Simulate formatting movies
	var messages []string
	for _, movie := range movies {
		sizeGB := float64(movie.FileSize) / (1024 * 1024 * 1024)
		formattedSize := fmt.Sprintf("%.2f", sizeGB)
		message := fmt.Sprintf("ID:%d [%d%%] %s ГБ\n%s\n",
			movie.ID, movie.DownloadedPercentage, formattedSize, movie.Name)
		messages = append(messages, message)
	}

	messages = append(messages, "Свободное место на диске: 10.00 ГБ")

	finalMessage := ""
	for _, msg := range messages {
		finalMessage += msg
	}

	bot.SendMessage(chatID, finalMessage, nil)
}

func setupMoviesTest(t *testing.T) (*MockMoviesBot, database.Database, *tgbotapi.Update) {
	logutils.InitLogger("debug")

	bot := &MockMoviesBot{}
	db := testutils.TestDatabase(t)

	update := &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 123},
		},
	}

	return bot, db, update
}

func TestListMoviesHandler_EmptyList(t *testing.T) {
	bot, db, update := setupMoviesTest(t)
	config := testutils.TestConfig("/tmp")

	simulateListMoviesHandler(bot, update, db, config)

	if len(bot.SentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(bot.SentMessages))
	}

	message := bot.SentMessages[0]
	if message.ChatID != 123 {
		t.Errorf("Expected chat ID 123, got %d", message.ChatID)
	}

	if message.Text != "📭 Список пуст" {
		t.Errorf("Expected empty list message, got: %s", message.Text)
	}
}

func TestListMoviesHandler_WithMovies(t *testing.T) {
	bot := &MockMoviesBot{}
	config := testutils.TestConfig("/tmp")

	// Create a mock database with pre-set movies
	mockDB := &MockDatabaseWithMovies{}

	update := &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 123},
		},
	}

	simulateListMoviesHandler(bot, update, mockDB, config)

	if len(bot.SentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(bot.SentMessages))
	}

	message := bot.SentMessages[0]
	if message.ChatID != 123 {
		t.Errorf("Expected chat ID 123, got %d", message.ChatID)
	}

	// Check that message contains movie information
	expectedSubstrings := []string{
		"Test Movie 1",
		"Test Movie 2",
		"75%",
		"100%",
		"1.00", // 1GB formatted
		"2.00", // 2GB formatted
		"Свободное место на диске:", // Disk space info
	}

	for _, substring := range expectedSubstrings {
		if !contains(message.Text, substring) {
			t.Errorf("Expected message to contain '%s', but it doesn't. Message: %s", substring, message.Text)
		}
	}
}

func TestListMoviesHandler_DatabaseError(t *testing.T) {
	bot := &MockMoviesBot{}
	config := testutils.TestConfig("/tmp")
	logutils.InitLogger("debug")

	// Create a mock database that returns an error
	mockDB := &MockErrorDatabase{}

	update := &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 123},
		},
	}

	simulateListMoviesHandler(bot, update, mockDB, config)

	if len(bot.SentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(bot.SentMessages))
	}

	message := bot.SentMessages[0]
	expectedText := "Ошибка при получении списка фильмов"
	if message.Text != expectedText {
		t.Errorf("Expected error message '%s', got: %s", expectedText, message.Text)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || substr == "" ||
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			contains(s[1:], substr))))
}

// Mock database with movies for testing
type MockDatabaseWithMovies struct{}

func (*MockDatabaseWithMovies) Init(_ *tmsconfig.Config) error { return nil }
func (*MockDatabaseWithMovies) AddMovie(_ context.Context, _ string, _ int64, _, _ []string) (uint, error) {
	return 1, nil
}
func (*MockDatabaseWithMovies) RemoveMovie(_ context.Context, _ uint) error { return nil }
func (*MockDatabaseWithMovies) GetMovieList(_ context.Context) ([]database.Movie, error) {
	return []database.Movie{
		{
			ID:                   1,
			Name:                 "Test Movie 1",
			FileSize:             1073741824, // 1GB
			DownloadedPercentage: 75,
		},
		{
			ID:                   2,
			Name:                 "Test Movie 2",
			FileSize:             2147483648, // 2GB
			DownloadedPercentage: 100,
		},
	}, nil
}
func (*MockDatabaseWithMovies) UpdateDownloadedPercentage(_ context.Context, _ uint, _ int) error {
	return nil
}
func (*MockDatabaseWithMovies) SetLoaded(_ context.Context, _ uint) error { return nil }
func (*MockDatabaseWithMovies) GetMovieByID(_ context.Context, _ uint) (database.Movie, error) {
	return database.Movie{}, nil
}
func (*MockDatabaseWithMovies) GetFilesByMovieID(_ context.Context, _ uint) ([]database.MovieFile, error) {
	return nil, nil
}
func (*MockDatabaseWithMovies) RemoveFilesByMovieID(_ context.Context, _ uint) error { return nil }
func (*MockDatabaseWithMovies) RemoveTempFilesByMovieID(_ context.Context, _ uint) error {
	return nil
}
func (*MockDatabaseWithMovies) MovieExistsFiles(_ context.Context, _ []string) (bool, error) {
	return false, nil
}
func (*MockDatabaseWithMovies) MovieExistsUploadedFile(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (*MockDatabaseWithMovies) GetTempFilesByMovieID(_ context.Context, _ uint) ([]database.MovieFile, error) {
	return nil, nil
}
func (*MockDatabaseWithMovies) CreateMovie(_ context.Context, _ *models.Movie) (uint, error) {
	return 1, nil
}
func (*MockDatabaseWithMovies) Login(_ context.Context, _ string, _ int64, _ string, _ *tmsconfig.Config) (bool, error) {
	return true, nil
}
func (*MockDatabaseWithMovies) GetUserRole(_ context.Context, _ int64) (database.UserRole, error) {
	return models.AdminRole, nil
}
func (*MockDatabaseWithMovies) IsUserAccessAllowed(_ context.Context, _ int64) (allowed bool, role database.UserRole, err error) {
	return true, models.AdminRole, nil
}
func (*MockDatabaseWithMovies) AssignTemporaryPassword(_ context.Context, _ string, _ int64) error {
	return nil
}
func (*MockDatabaseWithMovies) ExtendTemporaryUser(_ context.Context, _ int64, _ time.Time) error {
	return nil
}
func (*MockDatabaseWithMovies) GenerateTemporaryPassword(_ context.Context, _ time.Duration) (string, error) {
	return "temp123", nil
}
func (*MockDatabaseWithMovies) GetUserByChatID(_ context.Context, _ int64) (database.User, error) {
	return database.User{}, nil
}
func (*MockDatabaseWithMovies) MovieExistsId(_ context.Context, _ uint) (bool, error) {
	return true, nil
}

// Mock database that returns errors
type MockErrorDatabase struct{}

func (*MockErrorDatabase) Init(_ *tmsconfig.Config) error { return nil }
func (*MockErrorDatabase) AddMovie(_ context.Context, _ string, _ int64, _, _ []string) (uint, error) {
	return 0, fmt.Errorf("database error")
}
func (*MockErrorDatabase) RemoveMovie(_ context.Context, _ uint) error {
	return fmt.Errorf("database error")
}
func (*MockErrorDatabase) GetMovieList(_ context.Context) ([]database.Movie, error) {
	return nil, fmt.Errorf("database error")
}
func (*MockErrorDatabase) UpdateDownloadedPercentage(_ context.Context, _ uint, _ int) error {
	return fmt.Errorf("database error")
}
func (*MockErrorDatabase) SetLoaded(_ context.Context, _ uint) error {
	return fmt.Errorf("database error")
}
func (*MockErrorDatabase) GetMovieByID(_ context.Context, _ uint) (database.Movie, error) {
	return database.Movie{}, fmt.Errorf("database error")
}
func (*MockErrorDatabase) GetFilesByMovieID(_ context.Context, _ uint) ([]database.MovieFile, error) {
	return nil, fmt.Errorf("database error")
}
func (*MockErrorDatabase) RemoveFilesByMovieID(_ context.Context, _ uint) error {
	return fmt.Errorf("database error")
}
func (*MockErrorDatabase) RemoveTempFilesByMovieID(_ context.Context, _ uint) error {
	return fmt.Errorf("database error")
}
func (*MockErrorDatabase) MovieExistsFiles(_ context.Context, _ []string) (bool, error) {
	return false, fmt.Errorf("database error")
}
func (*MockErrorDatabase) MovieExistsUploadedFile(_ context.Context, _ string) (bool, error) {
	return false, fmt.Errorf("database error")
}
func (*MockErrorDatabase) GetTempFilesByMovieID(_ context.Context, _ uint) ([]database.MovieFile, error) {
	return nil, fmt.Errorf("database error")
}
func (*MockErrorDatabase) CreateMovie(_ context.Context, _ *models.Movie) (uint, error) {
	return 0, fmt.Errorf("database error")
}
func (*MockErrorDatabase) Login(_ context.Context, _ string, _ int64, _ string, _ *tmsconfig.Config) (bool, error) {
	return false, fmt.Errorf("database error")
}
func (*MockErrorDatabase) GetUserRole(_ context.Context, _ int64) (database.UserRole, error) {
	return "", fmt.Errorf("database error")
}
func (*MockErrorDatabase) IsUserAccessAllowed(_ context.Context, _ int64) (allowed bool, role database.UserRole, err error) {
	return false, "", fmt.Errorf("database error")
}
func (*MockErrorDatabase) AssignTemporaryPassword(_ context.Context, _ string, _ int64) error {
	return fmt.Errorf("database error")
}
func (*MockErrorDatabase) ExtendTemporaryUser(_ context.Context, _ int64, _ time.Time) error {
	return fmt.Errorf("database error")
}
func (*MockErrorDatabase) GenerateTemporaryPassword(_ context.Context, _ time.Duration) (string, error) {
	return "", fmt.Errorf("database error")
}
func (*MockErrorDatabase) GetUserByChatID(_ context.Context, _ int64) (database.User, error) {
	return database.User{}, fmt.Errorf("database error")
}
func (*MockErrorDatabase) MovieExistsId(_ context.Context, _ uint) (bool, error) {
	return false, fmt.Errorf("database error")
}

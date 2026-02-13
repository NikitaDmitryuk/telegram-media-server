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

// Simulate bot calls by calling handler logic directly (mirrors list.go: episodes 2/8 for series, compat progress/sticker).
func simulateListMoviesHandler(bot BotInterface, update *tgbotapi.Update, db database.MovieReader, config *tmsconfig.Config) {
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

	compatMode := config != nil && config.VideoSettings.CompatibilityMode
	var messages []string
	for i := range movies {
		movie := &movies[i]
		sizeGB := float64(movie.FileSize) / (1024 * 1024 * 1024)
		formattedSize := fmt.Sprintf("%.2f", sizeGB)
		episodes := ""
		if movie.TotalEpisodes > 1 {
			episodes = fmt.Sprintf("%d/%d ", movie.CompletedEpisodes, movie.TotalEpisodes)
		}
		progressStr := fmt.Sprintf("%d%%", movie.DownloadedPercentage)
		sticker := ""
		if compatMode {
			progressStr = fmt.Sprintf("%d/%d", movie.DownloadedPercentage, movie.ConversionPercentage)
			switch movie.TvCompatibility {
			case "green":
				sticker = "🟢 "
			case "yellow":
				sticker = "🟡 "
			case "red":
				sticker = "🔴 "
			}
		}
		message := fmt.Sprintf("ID:%d %s[%s] %s%s ГБ\n%s\n",
			movie.ID, sticker, progressStr, episodes, formattedSize, movie.Name)
		messages = append(messages, message)
	}

	messages = append(messages, "Свободное место на диске: 10.00 ГБ")

	finalMessage := ""
	for _, msg := range messages {
		finalMessage += msg
	}

	bot.SendMessage(chatID, finalMessage, nil)
}

// Minimal MovieReader mock for tests that only need GetMovieList (e.g. empty list).
type mockMovieReaderEmpty struct{}

func (*mockMovieReaderEmpty) GetMovieList(_ context.Context) ([]database.Movie, error) {
	return nil, nil
}
func (*mockMovieReaderEmpty) GetMovieByID(_ context.Context, _ uint) (database.Movie, error) {
	return database.Movie{}, nil
}
func (*mockMovieReaderEmpty) GetFilesByMovieID(_ context.Context, _ uint) ([]database.MovieFile, error) {
	return nil, nil
}
func (*mockMovieReaderEmpty) GetTempFilesByMovieID(_ context.Context, _ uint) ([]database.MovieFile, error) {
	return nil, nil
}
func (*mockMovieReaderEmpty) MovieExistsId(_ context.Context, _ uint) (bool, error) {
	return false, nil
}
func (*mockMovieReaderEmpty) MovieExistsFiles(_ context.Context, _ []string) (bool, error) {
	return false, nil
}
func (*mockMovieReaderEmpty) MovieExistsUploadedFile(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func TestListMoviesHandler_EmptyList(t *testing.T) {
	bot := &MockMoviesBot{}
	update := &tgbotapi.Update{
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}},
	}
	config := testutils.TestConfig("/tmp")
	// Use minimal MovieReader mock instead of full Database.
	simulateListMoviesHandler(bot, update, &mockMovieReaderEmpty{}, config)

	if len(bot.SentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(bot.SentMessages))
	}
	if bot.SentMessages[0].ChatID != 123 {
		t.Errorf("Expected chat ID 123, got %d", bot.SentMessages[0].ChatID)
	}
	if bot.SentMessages[0].Text != "📭 Список пуст" {
		t.Errorf("Expected empty list message, got: %s", bot.SentMessages[0].Text)
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

	// Check that message contains movie information (TestConfig has CompatibilityMode: true → format [download/conversion])
	expectedSubstrings := []string{
		"Test Movie 1",
		"Test Movie 2",
		"[75/0]",  // first movie progress
		"[100/0]", // second movie progress
		"1.00",    // 1GB formatted
		"2.00",    // 2GB formatted
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

// MockDatabaseWithSeries returns one series (TotalEpisodes > 0) for testing episodes display 2/8.
type MockDatabaseWithSeries struct{}

func (*MockDatabaseWithSeries) Init(_ *tmsconfig.Config) error { return nil }
func (*MockDatabaseWithSeries) AddMovie(_ context.Context, _ string, _ int64, _, _ []string, _ int) (uint, error) {
	return 1, nil
}
func (*MockDatabaseWithSeries) RemoveMovie(_ context.Context, _ uint) error { return nil }
func (*MockDatabaseWithSeries) GetMovieList(_ context.Context) ([]database.Movie, error) {
	return []database.Movie{
		{
			ID:                   1,
			Name:                 "Test Series S01",
			FileSize:             1073741824,
			DownloadedPercentage: 25,
			TotalEpisodes:        8,
			CompletedEpisodes:    2,
		},
	}, nil
}
func (*MockDatabaseWithSeries) UpdateDownloadedPercentage(_ context.Context, _ uint, _ int) error {
	return nil
}
func (*MockDatabaseWithSeries) UpdateEpisodesProgress(_ context.Context, _ uint, _ int) error {
	return nil
}
func (*MockDatabaseWithSeries) SetLoaded(_ context.Context, _ uint) error { return nil }
func (*MockDatabaseWithSeries) UpdateConversionStatus(_ context.Context, _ uint, _ string) error {
	return nil
}
func (*MockDatabaseWithSeries) UpdateConversionPercentage(_ context.Context, _ uint, _ int) error {
	return nil
}
func (*MockDatabaseWithSeries) SetTvCompatibility(_ context.Context, _ uint, _ string) error {
	return nil
}
func (*MockDatabaseWithSeries) GetMovieByID(_ context.Context, _ uint) (database.Movie, error) {
	return database.Movie{}, nil
}
func (*MockDatabaseWithSeries) GetFilesByMovieID(_ context.Context, _ uint) ([]database.MovieFile, error) {
	return nil, nil
}
func (*MockDatabaseWithSeries) RemoveFilesByMovieID(_ context.Context, _ uint) error {
	return nil
}
func (*MockDatabaseWithSeries) RemoveTempFilesByMovieID(_ context.Context, _ uint) error {
	return nil
}
func (*MockDatabaseWithSeries) MovieExistsFiles(_ context.Context, _ []string) (bool, error) {
	return false, nil
}
func (*MockDatabaseWithSeries) MovieExistsUploadedFile(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (*MockDatabaseWithSeries) GetTempFilesByMovieID(_ context.Context, _ uint) ([]database.MovieFile, error) {
	return nil, nil
}
func (*MockDatabaseWithSeries) CreateMovie(_ context.Context, _ *models.Movie) (uint, error) {
	return 1, nil
}
func (*MockDatabaseWithSeries) Login(_ context.Context, _ string, _ int64, _ string, _ *tmsconfig.Config) (bool, error) {
	return true, nil
}
func (*MockDatabaseWithSeries) GetUserRole(_ context.Context, _ int64) (database.UserRole, error) {
	return models.AdminRole, nil
}
func (*MockDatabaseWithSeries) IsUserAccessAllowed(_ context.Context, _ int64) (allowed bool, role database.UserRole, err error) {
	return true, models.AdminRole, nil
}
func (*MockDatabaseWithSeries) AssignTemporaryPassword(_ context.Context, _ string, _ int64) error {
	return nil
}
func (*MockDatabaseWithSeries) ExtendTemporaryUser(_ context.Context, _ int64, _ time.Time) error {
	return nil
}
func (*MockDatabaseWithSeries) GenerateTemporaryPassword(_ context.Context, _ time.Duration) (string, error) {
	return mockTempPassword, nil
}
func (*MockDatabaseWithSeries) GetUserByChatID(_ context.Context, _ int64) (database.User, error) {
	return database.User{}, nil
}
func (*MockDatabaseWithSeries) MovieExistsId(_ context.Context, _ uint) (bool, error) {
	return true, nil
}

// Mock database that returns one movie with conversion fields set (for compatibility mode list test).
type MockDatabaseWithCompatMovie struct{}

func (*MockDatabaseWithCompatMovie) Init(_ *tmsconfig.Config) error { return nil }
func (*MockDatabaseWithCompatMovie) AddMovie(_ context.Context, _ string, _ int64, _, _ []string, _ int) (uint, error) {
	return 1, nil
}
func (*MockDatabaseWithCompatMovie) RemoveMovie(_ context.Context, _ uint) error { return nil }
func (*MockDatabaseWithCompatMovie) GetMovieList(_ context.Context) ([]database.Movie, error) {
	return []database.Movie{
		{
			ID:                   1,
			Name:                 "TV Ready Movie",
			FileSize:             1073741824,
			DownloadedPercentage: 100,
			ConversionPercentage: 100,
			TvCompatibility:      "green",
		},
	}, nil
}
func (*MockDatabaseWithCompatMovie) UpdateDownloadedPercentage(_ context.Context, _ uint, _ int) error {
	return nil
}
func (*MockDatabaseWithCompatMovie) UpdateEpisodesProgress(_ context.Context, _ uint, _ int) error {
	return nil
}
func (*MockDatabaseWithCompatMovie) SetLoaded(_ context.Context, _ uint) error { return nil }
func (*MockDatabaseWithCompatMovie) UpdateConversionStatus(_ context.Context, _ uint, _ string) error {
	return nil
}
func (*MockDatabaseWithCompatMovie) UpdateConversionPercentage(_ context.Context, _ uint, _ int) error {
	return nil
}
func (*MockDatabaseWithCompatMovie) SetTvCompatibility(_ context.Context, _ uint, _ string) error {
	return nil
}
func (*MockDatabaseWithCompatMovie) GetMovieByID(_ context.Context, _ uint) (database.Movie, error) {
	return database.Movie{}, nil
}
func (*MockDatabaseWithCompatMovie) GetFilesByMovieID(_ context.Context, _ uint) ([]database.MovieFile, error) {
	return nil, nil
}
func (*MockDatabaseWithCompatMovie) RemoveFilesByMovieID(_ context.Context, _ uint) error {
	return nil
}
func (*MockDatabaseWithCompatMovie) RemoveTempFilesByMovieID(_ context.Context, _ uint) error {
	return nil
}
func (*MockDatabaseWithCompatMovie) MovieExistsFiles(_ context.Context, _ []string) (bool, error) {
	return false, nil
}
func (*MockDatabaseWithCompatMovie) MovieExistsUploadedFile(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (*MockDatabaseWithCompatMovie) GetTempFilesByMovieID(_ context.Context, _ uint) ([]database.MovieFile, error) {
	return nil, nil
}
func (*MockDatabaseWithCompatMovie) CreateMovie(_ context.Context, _ *models.Movie) (uint, error) {
	return 1, nil
}
func (*MockDatabaseWithCompatMovie) Login(_ context.Context, _ string, _ int64, _ string, _ *tmsconfig.Config) (bool, error) {
	return true, nil
}
func (*MockDatabaseWithCompatMovie) GetUserRole(_ context.Context, _ int64) (database.UserRole, error) {
	return models.AdminRole, nil
}
func (*MockDatabaseWithCompatMovie) IsUserAccessAllowed(_ context.Context, _ int64) (allowed bool, role database.UserRole, err error) {
	return true, models.AdminRole, nil
}
func (*MockDatabaseWithCompatMovie) AssignTemporaryPassword(_ context.Context, _ string, _ int64) error {
	return nil
}
func (*MockDatabaseWithCompatMovie) ExtendTemporaryUser(_ context.Context, _ int64, _ time.Time) error {
	return nil
}
func (*MockDatabaseWithCompatMovie) GenerateTemporaryPassword(_ context.Context, _ time.Duration) (string, error) {
	return mockTempPassword, nil
}
func (*MockDatabaseWithCompatMovie) GetUserByChatID(_ context.Context, _ int64) (database.User, error) {
	return database.User{}, nil
}
func (*MockDatabaseWithCompatMovie) MovieExistsId(_ context.Context, _ uint) (bool, error) {
	return true, nil
}

func TestListMoviesHandler_WithSeries(t *testing.T) {
	bot := &MockMoviesBot{}
	config := testutils.TestConfig("/tmp")
	mockDB := &MockDatabaseWithSeries{}
	update := &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 123},
		},
	}

	simulateListMoviesHandler(bot, update, mockDB, config)

	if len(bot.SentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(bot.SentMessages))
	}
	msg := bot.SentMessages[0]
	if msg.ChatID != 123 {
		t.Errorf("Expected chat ID 123, got %d", msg.ChatID)
	}
	// Сериал: рядом с процентами отображается 2/8 без подписей
	if !contains(msg.Text, "2/8") {
		t.Errorf("Expected message to contain '2/8' for series, got: %s", msg.Text)
	}
	if !contains(msg.Text, "Test Series S01") {
		t.Errorf("Expected message to contain series name, got: %s", msg.Text)
	}
}

func TestListMoviesHandler_CompatibilityMode(t *testing.T) {
	bot := &MockMoviesBot{}
	config := testutils.TestConfig("/tmp")
	config.VideoSettings.CompatibilityMode = true
	mockDB := &MockDatabaseWithCompatMovie{}
	update := &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 123},
		},
	}

	simulateListMoviesHandler(bot, update, mockDB, config)

	if len(bot.SentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(bot.SentMessages))
	}
	msg := bot.SentMessages[0]
	if msg.ChatID != 123 {
		t.Errorf("Expected chat ID 123, got %d", msg.ChatID)
	}
	// Режим совместимости: прогресс в формате скачивание/конвертация и стикер
	if !contains(msg.Text, "100/100") {
		t.Errorf("Expected message to contain '100/100' in compatibility mode, got: %s", msg.Text)
	}
	if !contains(msg.Text, "🟢") {
		t.Errorf("Expected message to contain green sticker for TvCompatibility=green, got: %s", msg.Text)
	}
	if !contains(msg.Text, "TV Ready Movie") {
		t.Errorf("Expected message to contain movie name, got: %s", msg.Text)
	}
}

const mockTempPassword = "temp123"

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || substr == "" ||
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			contains(s[1:], substr))))
}

// Mock database with movies for testing
type MockDatabaseWithMovies struct{}

func (*MockDatabaseWithMovies) Init(_ *tmsconfig.Config) error { return nil }
func (*MockDatabaseWithMovies) AddMovie(_ context.Context, _ string, _ int64, _, _ []string, _ int) (uint, error) {
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
func (*MockDatabaseWithMovies) UpdateEpisodesProgress(_ context.Context, _ uint, _ int) error {
	return nil
}
func (*MockDatabaseWithMovies) SetLoaded(_ context.Context, _ uint) error { return nil }
func (*MockDatabaseWithMovies) UpdateConversionStatus(_ context.Context, _ uint, _ string) error {
	return nil
}
func (*MockDatabaseWithMovies) UpdateConversionPercentage(_ context.Context, _ uint, _ int) error {
	return nil
}
func (*MockDatabaseWithMovies) SetTvCompatibility(_ context.Context, _ uint, _ string) error {
	return nil
}
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
	return mockTempPassword, nil
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
func (*MockErrorDatabase) AddMovie(_ context.Context, _ string, _ int64, _, _ []string, _ int) (uint, error) {
	return 0, fmt.Errorf("database error")
}
func (*MockErrorDatabase) UpdateEpisodesProgress(_ context.Context, _ uint, _ int) error {
	return fmt.Errorf("database error")
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
func (*MockErrorDatabase) UpdateConversionStatus(_ context.Context, _ uint, _ string) error {
	return fmt.Errorf("database error")
}
func (*MockErrorDatabase) UpdateConversionPercentage(_ context.Context, _ uint, _ int) error {
	return fmt.Errorf("database error")
}
func (*MockErrorDatabase) SetTvCompatibility(_ context.Context, _ uint, _ string) error {
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

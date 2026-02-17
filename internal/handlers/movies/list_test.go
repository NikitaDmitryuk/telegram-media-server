package movies

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/models"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MockMoviesBot implements bot.Service for testing.
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

func (*MockMoviesBot) SendMessageReturningID(_ int64, _ string, _ any) (int, error) {
	return 0, nil
}

func (*MockMoviesBot) DownloadFile(_, _ string) error { return nil }

func (*MockMoviesBot) AnswerCallbackQuery(_ tgbotapi.CallbackConfig) {}

func (*MockMoviesBot) DeleteMessage(_ int64, _ int) error { return nil }

func (*MockMoviesBot) SaveFile(_ string, _ []byte) error { return nil }

func (*MockMoviesBot) EditMessageTextAndMarkup(_ int64, _ int, _ string, _ tgbotapi.InlineKeyboardMarkup) error {
	return nil
}

// newTestApp builds an *app.App with the given dependencies.
func newTestApp(bot *MockMoviesBot, db database.Database, cfg *tmsconfig.Config) *app.App {
	return &app.App{
		Bot:    bot,
		DB:     db,
		Config: cfg,
	}
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

// emptyDatabaseStub is a full database.Database stub that always returns empty/nil results.
// It embeds mockMovieReaderEmpty for MovieReader methods and adds stubs for the rest.
type emptyDatabaseStub struct {
	mockMovieReaderEmpty
}

func (*emptyDatabaseStub) Init(_ *tmsconfig.Config) error { return nil }
func (*emptyDatabaseStub) AddMovie(_ context.Context, _ string, _ int64, _, _ []string, _ int) (uint, error) {
	return 0, nil
}
func (*emptyDatabaseStub) UpdateEpisodesProgress(_ context.Context, _ uint, _ int) error { return nil }
func (*emptyDatabaseStub) UpdateDownloadedPercentage(_ context.Context, _ uint, _ int) error {
	return nil
}
func (*emptyDatabaseStub) SetLoaded(_ context.Context, _ uint) error   { return nil }
func (*emptyDatabaseStub) RemoveMovie(_ context.Context, _ uint) error { return nil }
func (*emptyDatabaseStub) UpdateConversionStatus(_ context.Context, _ uint, _ string) error {
	return nil
}
func (*emptyDatabaseStub) UpdateConversionPercentage(_ context.Context, _ uint, _ int) error {
	return nil
}
func (*emptyDatabaseStub) SetTvCompatibility(_ context.Context, _ uint, _ string) error { return nil }
func (*emptyDatabaseStub) RemoveFilesByMovieID(_ context.Context, _ uint) error         { return nil }
func (*emptyDatabaseStub) RemoveTempFilesByMovieID(_ context.Context, _ uint) error     { return nil }
func (*emptyDatabaseStub) Login(_ context.Context, _ string, _ int64, _ string, _ *tmsconfig.Config) (bool, error) {
	return false, nil
}
func (*emptyDatabaseStub) GetUserRole(_ context.Context, _ int64) (database.UserRole, error) {
	return "", nil
}
func (*emptyDatabaseStub) IsUserAccessAllowed(_ context.Context, _ int64) (allowed bool, role database.UserRole, err error) {
	return false, "", nil
}
func (*emptyDatabaseStub) AssignTemporaryPassword(_ context.Context, _ string, _ int64) error {
	return nil
}
func (*emptyDatabaseStub) ExtendTemporaryUser(_ context.Context, _ int64, _ time.Time) error {
	return nil
}
func (*emptyDatabaseStub) GenerateTemporaryPassword(_ context.Context, _ time.Duration) (string, error) {
	return "", nil
}
func (*emptyDatabaseStub) GetUserByChatID(_ context.Context, _ int64) (database.User, error) {
	return database.User{}, nil
}

func TestListMoviesHandler_EmptyList(t *testing.T) {
	bot := &MockMoviesBot{}
	cfg := testutils.TestConfig(t.TempDir())

	a := newTestApp(bot, &emptyDatabaseStub{}, cfg)
	update := &tgbotapi.Update{
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}},
	}

	ListMoviesHandler(a, update)

	if len(bot.SentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(bot.SentMessages))
	}
	if bot.SentMessages[0].ChatID != 123 {
		t.Errorf("Expected chat ID 123, got %d", bot.SentMessages[0].ChatID)
	}
	// English locale: "📭 The list is empty"
	if !strings.Contains(bot.SentMessages[0].Text, "empty") {
		t.Errorf("Expected empty list message, got: %s", bot.SentMessages[0].Text)
	}
}

// mockDatabaseWithMovies returns two movies for testing.
type mockDatabaseWithMovies struct {
	emptyDatabaseStub
}

func (*mockDatabaseWithMovies) GetMovieList(_ context.Context) ([]database.Movie, error) {
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

func TestListMoviesHandler_WithMovies(t *testing.T) {
	bot := &MockMoviesBot{}
	cfg := testutils.TestConfig(t.TempDir())

	a := newTestApp(bot, &mockDatabaseWithMovies{}, cfg)
	update := &tgbotapi.Update{
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}},
	}

	ListMoviesHandler(a, update)

	if len(bot.SentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(bot.SentMessages))
	}

	msg := bot.SentMessages[0]
	if msg.ChatID != 123 {
		t.Errorf("Expected chat ID 123, got %d", msg.ChatID)
	}

	// TestConfig sets CompatibilityMode=true → progress format is download/conversion.
	expectedSubstrings := []string{
		"Test Movie 1",
		"Test Movie 2",
		"[75/0]",
		"[100/0]",
		"1.00",
		"2.00",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(msg.Text, sub) {
			t.Errorf("Expected message to contain %q, got: %s", sub, msg.Text)
		}
	}
}

// mockErrorDatabase returns an error from GetMovieList.
type mockErrorDatabase struct {
	emptyDatabaseStub
}

func (*mockErrorDatabase) GetMovieList(_ context.Context) ([]database.Movie, error) {
	return nil, fmt.Errorf("database error")
}

func TestListMoviesHandler_DatabaseError(t *testing.T) {
	bot := &MockMoviesBot{}
	cfg := testutils.TestConfig(t.TempDir())

	a := newTestApp(bot, &mockErrorDatabase{}, cfg)
	update := &tgbotapi.Update{
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}},
	}

	ListMoviesHandler(a, update)

	if len(bot.SentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(bot.SentMessages))
	}

	msg := bot.SentMessages[0]
	// English locale: "An error occurred while fetching the movie list..."
	if !strings.Contains(msg.Text, "error") && !strings.Contains(msg.Text, "fetch") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

// mockDatabaseWithSeries returns one series (TotalEpisodes > 1) for testing episodes display.
type mockDatabaseWithSeries struct {
	emptyDatabaseStub
}

func (*mockDatabaseWithSeries) GetMovieList(_ context.Context) ([]database.Movie, error) {
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

func TestListMoviesHandler_WithSeries(t *testing.T) {
	bot := &MockMoviesBot{}
	cfg := testutils.TestConfig(t.TempDir())

	a := newTestApp(bot, &mockDatabaseWithSeries{}, cfg)
	update := &tgbotapi.Update{
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}},
	}

	ListMoviesHandler(a, update)

	if len(bot.SentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(bot.SentMessages))
	}

	msg := bot.SentMessages[0]
	if msg.ChatID != 123 {
		t.Errorf("Expected chat ID 123, got %d", msg.ChatID)
	}
	if !strings.Contains(msg.Text, "2/8") {
		t.Errorf("Expected message to contain '2/8' for series, got: %s", msg.Text)
	}
	if !strings.Contains(msg.Text, "Test Series S01") {
		t.Errorf("Expected message to contain series name, got: %s", msg.Text)
	}
}

// mockDatabaseWithCompatMovie returns one movie with conversion fields set (for compatibility mode list test).
type mockDatabaseWithCompatMovie struct {
	emptyDatabaseStub
}

func (*mockDatabaseWithCompatMovie) GetMovieList(_ context.Context) ([]database.Movie, error) {
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

func TestListMoviesHandler_CompatibilityMode(t *testing.T) {
	bot := &MockMoviesBot{}
	cfg := testutils.TestConfig(t.TempDir())
	cfg.VideoSettings.CompatibilityMode = true

	a := newTestApp(bot, &mockDatabaseWithCompatMovie{}, cfg)
	update := &tgbotapi.Update{
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}},
	}

	ListMoviesHandler(a, update)

	if len(bot.SentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(bot.SentMessages))
	}

	msg := bot.SentMessages[0]
	if msg.ChatID != 123 {
		t.Errorf("Expected chat ID 123, got %d", msg.ChatID)
	}
	if !strings.Contains(msg.Text, "100/100") {
		t.Errorf("Expected message to contain '100/100' in compatibility mode, got: %s", msg.Text)
	}
	if !strings.Contains(msg.Text, "🟢") {
		t.Errorf("Expected green sticker for TvCompatibility=green, got: %s", msg.Text)
	}
	if !strings.Contains(msg.Text, "TV Ready Movie") {
		t.Errorf("Expected message to contain movie name, got: %s", msg.Text)
	}
}

// unused constant kept for mock consistency with other test files in the package.
const mockTempPassword = "temp123"

// Kept for backward compatibility with the integration test file that uses models.AdminRole via these full mocks.
type MockDatabaseWithSeries struct{ emptyDatabaseStub }

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
func (*MockDatabaseWithSeries) GetUserRole(_ context.Context, _ int64) (database.UserRole, error) {
	return models.AdminRole, nil
}
func (*MockDatabaseWithSeries) IsUserAccessAllowed(_ context.Context, _ int64) (allowed bool, role database.UserRole, err error) {
	return true, models.AdminRole, nil
}
func (*MockDatabaseWithSeries) GenerateTemporaryPassword(_ context.Context, _ time.Duration) (string, error) {
	return mockTempPassword, nil
}

type MockDatabaseWithCompatMovie struct{ emptyDatabaseStub }

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
func (*MockDatabaseWithCompatMovie) GetUserRole(_ context.Context, _ int64) (database.UserRole, error) {
	return models.AdminRole, nil
}
func (*MockDatabaseWithCompatMovie) IsUserAccessAllowed(_ context.Context, _ int64) (allowed bool, role database.UserRole, err error) {
	return true, models.AdminRole, nil
}
func (*MockDatabaseWithCompatMovie) GenerateTemporaryPassword(_ context.Context, _ time.Duration) (string, error) {
	return mockTempPassword, nil
}

type MockDatabaseWithMovies struct{ emptyDatabaseStub }

func (*MockDatabaseWithMovies) GetMovieList(_ context.Context) ([]database.Movie, error) {
	return []database.Movie{
		{
			ID:                   1,
			Name:                 "Test Movie 1",
			FileSize:             1073741824,
			DownloadedPercentage: 75,
		},
		{
			ID:                   2,
			Name:                 "Test Movie 2",
			FileSize:             2147483648,
			DownloadedPercentage: 100,
		},
	}, nil
}
func (*MockDatabaseWithMovies) GetUserRole(_ context.Context, _ int64) (database.UserRole, error) {
	return models.AdminRole, nil
}
func (*MockDatabaseWithMovies) IsUserAccessAllowed(_ context.Context, _ int64) (allowed bool, role database.UserRole, err error) {
	return true, models.AdminRole, nil
}
func (*MockDatabaseWithMovies) GenerateTemporaryPassword(_ context.Context, _ time.Duration) (string, error) {
	return mockTempPassword, nil
}

type MockErrorDatabase struct{ emptyDatabaseStub }

func (*MockErrorDatabase) GetMovieList(_ context.Context) ([]database.Movie, error) {
	return nil, fmt.Errorf("database error")
}
func (*MockErrorDatabase) GetUserRole(_ context.Context, _ int64) (database.UserRole, error) {
	return "", fmt.Errorf("database error")
}
func (*MockErrorDatabase) IsUserAccessAllowed(_ context.Context, _ int64) (allowed bool, role database.UserRole, err error) {
	return false, "", fmt.Errorf("database error")
}
func (*MockErrorDatabase) GenerateTemporaryPassword(_ context.Context, _ time.Duration) (string, error) {
	return "", fmt.Errorf("database error")
}

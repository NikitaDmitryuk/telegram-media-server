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

// newTestApp builds an *app.App with the given dependencies.
func newTestApp(bot *testutils.MockBot, db database.Database, cfg *tmsconfig.Config) *app.App {
	return &app.App{
		Bot:    bot,
		DB:     db,
		Config: cfg,
	}
}

func TestListMoviesHandler_EmptyList(t *testing.T) {
	bot := &testutils.MockBot{}
	cfg := testutils.TestConfig(t.TempDir())

	a := newTestApp(bot, &testutils.DatabaseStub{}, cfg)
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
	if !strings.Contains(bot.SentMessages[0].Text, "empty") {
		t.Errorf("Expected empty list message, got: %s", bot.SentMessages[0].Text)
	}
}

// mockDatabaseWithMovies returns two movies.
type mockDatabaseWithMovies struct {
	testutils.DatabaseStub
}

func (*mockDatabaseWithMovies) GetMovieList(_ context.Context) ([]database.Movie, error) {
	return []database.Movie{
		{ID: 1, Name: "Test Movie 1", FileSize: 1073741824, DownloadedPercentage: 75},
		{ID: 2, Name: "Test Movie 2", FileSize: 2147483648, DownloadedPercentage: 100},
	}, nil
}

func TestListMoviesHandler_WithMovies(t *testing.T) {
	bot := &testutils.MockBot{}
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

	for _, sub := range []string{"Test Movie 1", "Test Movie 2", "[75/0]", "[100/0]", "1.00", "2.00"} {
		if !strings.Contains(msg.Text, sub) {
			t.Errorf("Expected message to contain %q, got: %s", sub, msg.Text)
		}
	}
}

// mockErrorDatabase returns an error from GetMovieList.
type mockErrorDatabase struct {
	testutils.DatabaseStub
}

func (*mockErrorDatabase) GetMovieList(_ context.Context) ([]database.Movie, error) {
	return nil, fmt.Errorf("database error")
}

func TestListMoviesHandler_DatabaseError(t *testing.T) {
	bot := &testutils.MockBot{}
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
	if !strings.Contains(msg.Text, "error") && !strings.Contains(msg.Text, "fetch") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

// mockDatabaseWithSeries returns one series (TotalEpisodes > 1).
type mockDatabaseWithSeries struct {
	testutils.DatabaseStub
}

func (*mockDatabaseWithSeries) GetMovieList(_ context.Context) ([]database.Movie, error) {
	return []database.Movie{
		{ID: 1, Name: "Test Series S01", FileSize: 1073741824, DownloadedPercentage: 25, TotalEpisodes: 8, CompletedEpisodes: 2},
	}, nil
}

func TestListMoviesHandler_WithSeries(t *testing.T) {
	bot := &testutils.MockBot{}
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
	if !strings.Contains(msg.Text, "2/8") {
		t.Errorf("Expected '2/8' for series, got: %s", msg.Text)
	}
	if !strings.Contains(msg.Text, "Test Series S01") {
		t.Errorf("Expected series name, got: %s", msg.Text)
	}
}

// mockDatabaseWithCompatMovie returns one movie with conversion fields.
type mockDatabaseWithCompatMovie struct {
	testutils.DatabaseStub
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
	bot := &testutils.MockBot{}
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
	if !strings.Contains(msg.Text, "100/100") {
		t.Errorf("Expected '100/100' in compat mode, got: %s", msg.Text)
	}
	if !strings.Contains(msg.Text, "🟢") {
		t.Errorf("Expected green sticker, got: %s", msg.Text)
	}
	if !strings.Contains(msg.Text, "TV Ready Movie") {
		t.Errorf("Expected movie name, got: %s", msg.Text)
	}
}

// Exported mock types kept for integration_test.go in the same package.

const mockTempPassword = "temp123"

type MockDatabaseWithSeries struct{ testutils.DatabaseStub }

func (*MockDatabaseWithSeries) GetMovieList(_ context.Context) ([]database.Movie, error) {
	return []database.Movie{
		{ID: 1, Name: "Test Series S01", FileSize: 1073741824, DownloadedPercentage: 25, TotalEpisodes: 8, CompletedEpisodes: 2},
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

type MockDatabaseWithCompatMovie struct{ testutils.DatabaseStub }

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

type MockDatabaseWithMovies struct{ testutils.DatabaseStub }

func (*MockDatabaseWithMovies) GetMovieList(_ context.Context) ([]database.Movie, error) {
	return []database.Movie{
		{ID: 1, Name: "Test Movie 1", FileSize: 1073741824, DownloadedPercentage: 75},
		{ID: 2, Name: "Test Movie 2", FileSize: 2147483648, DownloadedPercentage: 100},
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

type MockErrorDatabase struct{ testutils.DatabaseStub }

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

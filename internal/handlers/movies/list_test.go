package movies

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/models"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestListMoviesHandler_EmptyList(t *testing.T) {
	bot := &testutils.MockBot{}
	cfg := testutils.TestConfig(t.TempDir())

	a := &app.App{Bot: bot, DB: &testutils.DatabaseStub{}, Config: cfg}
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

func TestListMoviesHandler_WithMovies(t *testing.T) {
	bot := &testutils.MockBot{}
	cfg := testutils.TestConfig(t.TempDir())

	a := &app.App{Bot: bot, DB: &MockDatabaseWithMovies{}, Config: cfg}
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

	for _, sub := range []string{
		"Test Movie 1\n⬇️ Downloading — 75% · 1.00 GB · ID:1",
		"Test Movie 2\n📦 Downloaded · TV compatibility unknown · 2.00 GB · ID:2",
		"1.00",
		"2.00",
	} {
		if !strings.Contains(msg.Text, sub) {
			t.Errorf("Expected message to contain %q, got: %s", sub, msg.Text)
		}
	}
}

func TestListMoviesHandler_DatabaseError(t *testing.T) {
	bot := &testutils.MockBot{}
	cfg := testutils.TestConfig(t.TempDir())

	a := &app.App{Bot: bot, DB: &MockErrorDatabase{}, Config: cfg}
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

func TestListMoviesHandler_WithSeries(t *testing.T) {
	bot := &testutils.MockBot{}
	cfg := testutils.TestConfig(t.TempDir())

	a := &app.App{Bot: bot, DB: &MockDatabaseWithSeries{}, Config: cfg}
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
	if !strings.Contains(msg.Text, "Test Series S01\n⬇️ Downloading — 25% · 2/8 1.00 GB · ID:1") {
		t.Errorf("Expected formatted series card preserving name, ID, size and episodes, got: %s", msg.Text)
	}
}

func TestListMoviesHandler_FormattingHasNoTechnicalStatusOrBrackets(t *testing.T) {
	bot := &testutils.MockBot{}
	cfg := testutils.TestConfig(t.TempDir())
	cfg.VideoSettings.CompatibilityMode = true
	a := &app.App{Bot: bot, DB: &MockDatabaseWithCompatMovie{}, Config: cfg}
	update := &tgbotapi.Update{Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}}

	ListMoviesHandler(a, update)
	msg := bot.SentMessages[0].Text
	for _, technical := range []string{"[", "]", "DL ", "CV ", "TV green", "done", "pending", "in_progress", "failed"} {
		if strings.Contains(msg, technical) {
			t.Errorf("formatted list contains technical marker %q: %s", technical, msg)
		}
	}
}

func TestListMoviesHandler_CompatibilityMode(t *testing.T) {
	bot := &testutils.MockBot{}
	cfg := testutils.TestConfig(t.TempDir())
	cfg.VideoSettings.CompatibilityMode = true

	a := &app.App{Bot: bot, DB: &MockDatabaseWithCompatMovie{}, Config: cfg}
	update := &tgbotapi.Update{
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}},
	}

	ListMoviesHandler(a, update)

	if len(bot.SentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(bot.SentMessages))
	}

	msg := bot.SentMessages[0]
	if !strings.Contains(msg.Text, "TV Ready Movie\n✅ Ready to watch · 1.00 GB · ID:1") {
		t.Errorf("Expected readable ready status in compat mode, got: %s", msg.Text)
	}
	if !strings.Contains(msg.Text, "TV Ready Movie") {
		t.Errorf("Expected movie name, got: %s", msg.Text)
	}
}

func TestFormatListProgressAndSticker_StatusVariants(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		movie        database.Movie
		compatMode   bool
		wantProgress string
		wantSticker  string
	}{
		{
			name:         "downloading_unknown_hides_compat",
			movie:        database.Movie{DownloadedPercentage: 50, TvCompatibility: "yellow"},
			compatMode:   true,
			wantProgress: "Downloading — 50%",
			wantSticker:  "⬇️ ",
		},
		{
			name:         "compat_disabled_downloaded",
			movie:        database.Movie{DownloadedPercentage: 100, ConversionStatus: "pending"},
			wantProgress: "Downloaded",
			wantSticker:  "✅ ",
		},
		{
			name:         "legacy_green_empty_conversion",
			movie:        database.Movie{DownloadedPercentage: 100, TvCompatibility: "green"},
			compatMode:   true,
			wantProgress: "Ready to watch",
			wantSticker:  "✅ ",
		},
		{
			name:         "done_green_ignores_stale_conversion_percent",
			movie:        database.Movie{DownloadedPercentage: 100, ConversionStatus: "done", ConversionPercentage: 12, TvCompatibility: "green"},
			compatMode:   true,
			wantProgress: "Ready to watch",
			wantSticker:  "✅ ",
		},
		{
			name:         "skipped_green_is_ready",
			movie:        database.Movie{DownloadedPercentage: 100, ConversionStatus: "skipped", ConversionPercentage: 20, TvCompatibility: "green"},
			compatMode:   true,
			wantProgress: "Ready to watch",
			wantSticker:  "✅ ",
		},
		{
			name:         "pending_precedes_green",
			movie:        database.Movie{DownloadedPercentage: 100, ConversionStatus: "pending", TvCompatibility: "green"},
			compatMode:   true,
			wantProgress: "Queued for processing",
			wantSticker:  "⏳ ",
		},
		{
			name:         "in_progress_precedes_green",
			movie:        database.Movie{DownloadedPercentage: 100, ConversionStatus: "in_progress", ConversionPercentage: 35, TvCompatibility: "green"},
			compatMode:   true,
			wantProgress: "Preparing for TV — 35%",
			wantSticker:  "🎞️ ",
		},
		{
			name:         "failed_precedes_green",
			movie:        database.Movie{DownloadedPercentage: 100, ConversionStatus: "failed", TvCompatibility: "green"},
			compatMode:   true,
			wantProgress: "Failed to prepare for TV",
			wantSticker:  "⚠️ ",
		},
		{
			name:         "unknown_compatibility",
			movie:        database.Movie{DownloadedPercentage: 100, ConversionStatus: "done", TvCompatibility: "yellow"},
			compatMode:   true,
			wantProgress: "Downloaded · TV compatibility unknown",
			wantSticker:  "📦 ",
		},
		{
			name:         "completed_red_unsupported",
			movie:        database.Movie{DownloadedPercentage: 100, ConversionStatus: "done", TvCompatibility: "red"},
			compatMode:   true,
			wantProgress: "Not supported on TV",
			wantSticker:  "🚫 ",
		},
		{
			name:         "early_green_hint",
			movie:        database.Movie{DownloadedPercentage: 25, TvCompatibility: "green"},
			compatMode:   true,
			wantProgress: "Downloading — 25% · 📺 Suitable for TV",
			wantSticker:  "⬇️ ",
		},
		{
			name:         "early_red_hint",
			movie:        database.Movie{DownloadedPercentage: 25, TvCompatibility: "red"},
			compatMode:   true,
			wantProgress: "Downloading — 25% · ⚠️ Not supported on TV",
			wantSticker:  "⬇️ ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			progress, sticker := formatListProgressAndSticker(&tc.movie, tc.compatMode)
			if progress != tc.wantProgress {
				t.Fatalf("progress = %q, want %q", progress, tc.wantProgress)
			}
			if sticker != tc.wantSticker {
				t.Fatalf("sticker = %q, want %q", sticker, tc.wantSticker)
			}
		})
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

package utils

import (
	"os"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

func TestMain(m *testing.M) {
	logutils.InitLogger("error")
	os.Exit(m.Run())
}

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Simple ASCII", "hello", "hello"},
		{"Spaces to underscores", "hello world", "hello_world"},
		{"Special characters", "file<>name:with|bad*chars", "file_name_with_bad_chars"},
		{"Russian characters preserved", "Фильм", "Фильм"},
		{"Mixed ASCII and Russian", "Фильм 2024 (HD)", "Фильм_2024_HD_"},
		{"Consecutive specials collapsed", "a!!!b", "a_b"},
		{"Leading special chars", "---test", "_test"},
		{"Trailing special chars", "test---", "test_"},
		{"Numbers preserved", "file123", "file123"},
		{"Empty string", "", ""},
		{"Only special characters", "!@#$%", "_"},
		{"Dots replaced", "file.name.txt", "file_name_txt"},
		{"Slashes replaced", "path/to/file", "path_to_file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFileName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeFileName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGenerateFileName(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		expected string
	}{
		{"Simple title", "movie", "movie.mp4"},
		{"Title with spaces", "my movie", "my_movie.mp4"},
		{"Russian title", "Фильм", "Фильм.mp4"},
		{"Complex title", "Movie (2024) [HD]", "Movie_2024_HD_.mp4"},
		{"Empty title", "", ".mp4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateFileName(tt.title)
			if result != tt.expected {
				t.Errorf("GenerateFileName(%q) = %q, want %q", tt.title, result, tt.expected)
			}
		})
	}
}

func TestIsValidLink(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Valid HTTPS", "https://example.com", true},
		{"Valid HTTP", "http://example.com", true},
		{"Valid with path", "https://example.com/path", true},
		{"Valid with query", "https://example.com/path?q=1", true},
		{"Valid YouTube", "https://www.youtube.com/watch?v=abc123", true},
		{"Valid subdomain", "https://sub.domain.example.com", true},
		{"FTP rejected", "ftp://example.com", false},
		{"No scheme", "example.com", false},
		{"Empty string", "", false},
		{"Just text", "hello world", false},
		{"Magnet link", "magnet:?xt=urn:btih:abc", false},
		{"File path", "/etc/passwd", false},
		{"No TLD", "https://localhost", false},
		{"IP address", "https://192.168.1.1", false},
		{"Single char TLD", "https://example.a", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidLink(tt.input)
			if result != tt.expected {
				t.Errorf("IsValidLink(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateDurationString(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    time.Duration
		expectError bool
	}{
		{"Hours", "3h", 3 * time.Hour, false},
		{"Minutes", "30m", 30 * time.Minute, false},
		{"Days", "1d", 24 * time.Hour, false},
		{"Large hours", "100h", 100 * time.Hour, false},
		{"Single minute", "1m", 1 * time.Minute, false},
		{"7 days", "7d", 7 * 24 * time.Hour, false},
		{"Invalid unit", "3s", 0, true},
		{"No unit", "3", 0, true},
		{"No number", "h", 0, true},
		{"Empty string", "", 0, true},
		{"Negative", "-3h", 0, true},
		{"Float", "1.5h", 0, true},
		{"Random text", "abc", 0, true},
		{"Mixed invalid", "3h30m", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateDurationString(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("ValidateDurationString(%q): expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ValidateDurationString(%q): unexpected error: %v", tt.input, err)
				return
			}
			if result != tt.expected {
				t.Errorf("ValidateDurationString(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLogAndReturnError(t *testing.T) {
	originalErr := ErrDownloadFailed
	result := LogAndReturnError("test context", originalErr)

	if result == nil {
		t.Fatal("Expected non-nil error")
	}
	expected := "test context: download failed"
	if result.Error() != expected {
		t.Errorf("Error message = %q, want %q", result.Error(), expected)
	}
}

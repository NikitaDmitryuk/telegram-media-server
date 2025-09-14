package config

import (
	"os"
	"testing"
	"time"
)

func TestDownloadTimeoutValidation(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		expectedResult time.Duration
		expectError    bool
	}{
		{
			name:           "No timeout set (default)",
			envValue:       "",
			expectedResult: 0,
			expectError:    false,
		},
		{
			name:           "Valid timeout",
			envValue:       "60m",
			expectedResult: 60 * time.Minute,
			expectError:    false,
		},
		{
			name:           "Zero timeout (no timeout)",
			envValue:       "0",
			expectedResult: 0,
			expectError:    false,
		},
		{
			name:        "Negative timeout (invalid)",
			envValue:    "-30m",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("DOWNLOAD_TIMEOUT")
			if tt.envValue != "" {
				os.Setenv("DOWNLOAD_TIMEOUT", tt.envValue)
				defer os.Unsetenv("DOWNLOAD_TIMEOUT")
			}

			os.Setenv("BOT_TOKEN", "test_token")
			os.Setenv("ADMIN_PASSWORD", "test_password_123")
			os.Setenv("MOVIE_PATH", "/tmp")
			defer func() {
				os.Unsetenv("BOT_TOKEN")
				os.Unsetenv("ADMIN_PASSWORD")
				os.Unsetenv("MOVIE_PATH")
			}()

			config, err := NewConfig()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for timeout value %s, but got none", tt.envValue)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if config.DownloadSettings.DownloadTimeout != tt.expectedResult {
				t.Errorf("Expected timeout %v, got %v", tt.expectedResult, config.DownloadSettings.DownloadTimeout)
			}
		})
	}
}

func TestDownloadTimeoutFromEnvExample(t *testing.T) {
	os.Setenv("DOWNLOAD_TIMEOUT", "60m")
	os.Setenv("BOT_TOKEN", "test_token")
	os.Setenv("ADMIN_PASSWORD", "test_password_123")
	os.Setenv("MOVIE_PATH", "/tmp")
	defer func() {
		os.Unsetenv("DOWNLOAD_TIMEOUT")
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("ADMIN_PASSWORD")
		os.Unsetenv("MOVIE_PATH")
	}()

	config, err := NewConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if config.DownloadSettings.DownloadTimeout != 60*time.Minute {
		t.Errorf("Expected timeout 60m from env example, got %v", config.DownloadSettings.DownloadTimeout)
	}
}

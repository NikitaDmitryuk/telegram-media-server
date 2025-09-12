package config

import (
	"os"
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name          string
		setupEnv      func()
		cleanupEnv    func()
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid configuration",
			setupEnv: func() {
				tempDir := os.TempDir()
				os.Setenv("BOT_TOKEN", "test-token")
				os.Setenv("MOVIE_PATH", tempDir)
				os.Setenv("ADMIN_PASSWORD", "admin123")
				os.Setenv("REGULAR_PASSWORD", "regular123")
			},
			cleanupEnv: func() {
				os.Unsetenv("BOT_TOKEN")
				os.Unsetenv("MOVIE_PATH")
				os.Unsetenv("ADMIN_PASSWORD")
				os.Unsetenv("REGULAR_PASSWORD")
			},
			expectError: false,
		},
		{
			name: "Missing bot token",
			setupEnv: func() {
				os.Setenv("MOVIE_PATH", "/tmp/test")
				os.Setenv("ADMIN_PASSWORD", "admin123")
				os.Setenv("REGULAR_PASSWORD", "regular123")
			},
			cleanupEnv: func() {
				os.Unsetenv("MOVIE_PATH")
				os.Unsetenv("ADMIN_PASSWORD")
				os.Unsetenv("REGULAR_PASSWORD")
			},
			expectError:   true,
			errorContains: "configuration validation failed",
		},
		{
			name: "Missing movie path",
			setupEnv: func() {
				os.Setenv("BOT_TOKEN", "test-token")
				os.Setenv("ADMIN_PASSWORD", "admin123")
				os.Setenv("REGULAR_PASSWORD", "regular123")
			},
			cleanupEnv: func() {
				os.Unsetenv("BOT_TOKEN")
				os.Unsetenv("ADMIN_PASSWORD")
				os.Unsetenv("REGULAR_PASSWORD")
			},
			expectError:   true,
			errorContains: "configuration validation failed",
		},
		{
			name: "Password too short",
			setupEnv: func() {
				tempDir := os.TempDir()
				os.Setenv("BOT_TOKEN", "test-token")
				os.Setenv("MOVIE_PATH", tempDir)
				os.Setenv("ADMIN_PASSWORD", "123")
				os.Setenv("REGULAR_PASSWORD", "regular123")
			},
			cleanupEnv: func() {
				os.Unsetenv("BOT_TOKEN")
				os.Unsetenv("MOVIE_PATH")
				os.Unsetenv("ADMIN_PASSWORD")
				os.Unsetenv("REGULAR_PASSWORD")
			},
			expectError:   true,
			errorContains: "configuration validation failed",
		},
		{
			name: "Invalid prowlarr configuration",
			setupEnv: func() {
				tempDir := os.TempDir()
				os.Setenv("BOT_TOKEN", "test-token")
				os.Setenv("MOVIE_PATH", tempDir)
				os.Setenv("ADMIN_PASSWORD", "admin123")
				os.Setenv("REGULAR_PASSWORD", "regular123")
				os.Setenv("PROWLARR_URL", "http://localhost:9696")
				// Missing API key
			},
			cleanupEnv: func() {
				os.Unsetenv("BOT_TOKEN")
				os.Unsetenv("MOVIE_PATH")
				os.Unsetenv("ADMIN_PASSWORD")
				os.Unsetenv("REGULAR_PASSWORD")
				os.Unsetenv("PROWLARR_URL")
			},
			expectError:   true,
			errorContains: "configuration validation failed",
		},
		{
			name: "Invalid download settings",
			setupEnv: func() {
				tempDir := os.TempDir()
				os.Setenv("BOT_TOKEN", "test-token")
				os.Setenv("MOVIE_PATH", tempDir)
				os.Setenv("ADMIN_PASSWORD", "admin123")
				os.Setenv("REGULAR_PASSWORD", "regular123")
				os.Setenv("MAX_CONCURRENT_DOWNLOADS", "0")
			},
			cleanupEnv: func() {
				os.Unsetenv("BOT_TOKEN")
				os.Unsetenv("MOVIE_PATH")
				os.Unsetenv("ADMIN_PASSWORD")
				os.Unsetenv("REGULAR_PASSWORD")
				os.Unsetenv("MAX_CONCURRENT_DOWNLOADS")
			},
			expectError:   true,
			errorContains: "configuration validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment
			tt.setupEnv()
			defer tt.cleanupEnv()

			// Create config
			config, err := NewConfig()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got no error", tt.errorContains)
				} else if !containsError(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing '%s', but got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
				if config == nil {
					t.Error("Expected config to be non-nil")
				}
			}
		})
	}
}

//nolint:gocyclo // Test function validating many default values
func TestConfigDefaults(t *testing.T) {
	// Clear all environment variables
	envVars := []string{
		"BOT_TOKEN", "MOVIE_PATH", "ADMIN_PASSWORD", "REGULAR_PASSWORD",
		"LANG", "LOG_LEVEL", "MAX_CONCURRENT_DOWNLOADS", "DOWNLOAD_TIMEOUT",
		"PROGRESS_UPDATE_INTERVAL", "PASSWORD_MIN_LENGTH",
		"ARIA2_MAX_PEERS", "ARIA2_SPLIT", "VIDEO_ENABLE_REENCODING",
	}

	for _, envVar := range envVars {
		os.Unsetenv(envVar)
	}

	// Set only required variables
	tempDir := os.TempDir()
	os.Setenv("BOT_TOKEN", "test-token")
	os.Setenv("MOVIE_PATH", tempDir)
	os.Setenv("ADMIN_PASSWORD", "admin123")
	os.Setenv("REGULAR_PASSWORD", "regular123")

	defer func() {
		for _, envVar := range envVars {
			os.Unsetenv(envVar)
		}
	}()

	config, err := NewConfig()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Check defaults
	if config.Lang != "en" {
		t.Errorf("Expected default lang 'en', got '%s'", config.Lang)
	}

	if config.LogLevel != "info" {
		t.Errorf("Expected default log level 'info', got '%s'", config.LogLevel)
	}

	if config.DownloadSettings.MaxConcurrentDownloads != 3 {
		t.Errorf("Expected default max concurrent downloads 3, got %d", config.DownloadSettings.MaxConcurrentDownloads)
	}

	if config.DownloadSettings.DownloadTimeout != 0 && config.DownloadSettings.DownloadTimeout != 30*time.Minute {
		t.Errorf("Expected default download timeout 0 or 30m, got %v", config.DownloadSettings.DownloadTimeout)
	}

	if config.DownloadSettings.ProgressUpdateInterval != 3*time.Second {
		t.Errorf(
			"Expected default progress update interval %v, got %v",
			3*time.Second,
			config.DownloadSettings.ProgressUpdateInterval,
		)
	}

	if config.SecuritySettings.PasswordMinLength != 8 {
		t.Errorf("Expected default password min length %d, got %d", 8, config.SecuritySettings.PasswordMinLength)
	}

	// Check Aria2 defaults
	if config.Aria2Settings.MaxPeers != 200 {
		t.Errorf("Expected default aria2 max peers %d, got %d", 200, config.Aria2Settings.MaxPeers)
	}

	if config.Aria2Settings.Split != 16 {
		t.Errorf("Expected default aria2 split %d, got %d", 16, config.Aria2Settings.Split)
	}

	if config.Aria2Settings.EnableDHT != true {
		t.Errorf("Expected default aria2 enable DHT true, got %v", config.Aria2Settings.EnableDHT)
	}

	// Check Video defaults
	if config.VideoSettings.EnableReencoding != false {
		t.Errorf("Expected default video enable reencoding false, got %v", config.VideoSettings.EnableReencoding)
	}

	if config.VideoSettings.QualitySelector != "bv*+ba/b" {
		t.Errorf("Expected default video quality selector 'bv*+ba/b', got '%s'", config.VideoSettings.QualitySelector)
	}
}

func TestConfigDockerEnvironment(t *testing.T) {
	// Skip this test since it requires specific Docker paths
	t.Skip("Docker environment test requires actual Docker setup")
}

func TestConfigGetters(t *testing.T) {
	// Set up environment
	tempDir := os.TempDir()
	os.Setenv("BOT_TOKEN", "test-token")
	os.Setenv("MOVIE_PATH", tempDir)
	os.Setenv("ADMIN_PASSWORD", "admin123")
	os.Setenv("REGULAR_PASSWORD", "regular123")

	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("MOVIE_PATH")
		os.Unsetenv("ADMIN_PASSWORD")
		os.Unsetenv("REGULAR_PASSWORD")
	}()

	config, err := NewConfig()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Test getter methods
	downloadConfig := config.GetDownloadSettings()
	if downloadConfig.MaxConcurrentDownloads != config.DownloadSettings.MaxConcurrentDownloads {
		t.Error("GetDownloadSettings() returned different values")
	}

	aria2Config := config.GetAria2Settings()
	if aria2Config.MaxPeers != config.Aria2Settings.MaxPeers {
		t.Error("GetAria2Settings() returned different values")
	}

	videoConfig := config.GetVideoSettings()
	if videoConfig.EnableReencoding != config.VideoSettings.EnableReencoding {
		t.Error("GetVideoSettings() returned different values")
	}

	securityConfig := config.GetSecuritySettings()
	if securityConfig.PasswordMinLength != config.SecuritySettings.PasswordMinLength {
		t.Error("GetSecuritySettings() returned different values")
	}
}

func TestConfigEnvironmentVariableParsing(t *testing.T) {
	tests := []struct {
		name     string
		envVar   string
		envValue string
		checkFn  func(*Config) bool
	}{
		{
			name:     "Integer parsing",
			envVar:   "MAX_CONCURRENT_DOWNLOADS",
			envValue: "5",
			checkFn:  func(c *Config) bool { return c.DownloadSettings.MaxConcurrentDownloads == 5 },
		},
		{
			name:     "Duration parsing",
			envVar:   "DOWNLOAD_TIMEOUT",
			envValue: "30s",
			checkFn:  func(c *Config) bool { return c.DownloadSettings.DownloadTimeout == 30*time.Second },
		},
		{
			name:     "Boolean parsing true",
			envVar:   "ARIA2_ENABLE_DHT",
			envValue: "true",
			checkFn:  func(c *Config) bool { return c.Aria2Settings.EnableDHT == true },
		},
		{
			name:     "Boolean parsing false",
			envVar:   "ARIA2_ENABLE_DHT",
			envValue: "false",
			checkFn:  func(c *Config) bool { return c.Aria2Settings.EnableDHT == false },
		},
		{
			name:     "Float parsing",
			envVar:   "ARIA2_SEED_RATIO",
			envValue: "1.5",
			checkFn:  func(c *Config) bool { return c.Aria2Settings.SeedRatio == 1.5 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up required environment
			tempDir := os.TempDir()
			os.Setenv("BOT_TOKEN", "test-token")
			os.Setenv("MOVIE_PATH", tempDir)
			os.Setenv("ADMIN_PASSWORD", "admin123")
			os.Setenv("REGULAR_PASSWORD", "regular123")
			os.Setenv(tt.envVar, tt.envValue)

			defer func() {
				os.Unsetenv("BOT_TOKEN")
				os.Unsetenv("MOVIE_PATH")
				os.Unsetenv("ADMIN_PASSWORD")
				os.Unsetenv("REGULAR_PASSWORD")
				os.Unsetenv(tt.envVar)
			}()

			config, err := NewConfig()
			if err != nil {
				t.Fatalf("Failed to create config: %v", err)
			}

			if !tt.checkFn(config) {
				t.Errorf("Environment variable %s=%s was not parsed correctly", tt.envVar, tt.envValue)
			}
		})
	}
}

// Helper function to check if error message contains expected text
func containsError(actual, expected string) bool {
	return expected == "" || (actual != "" &&
		(actual == expected ||
			len(actual) >= len(expected) &&
				actual[:len(expected)] == expected))
}

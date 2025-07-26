package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
)

const (
	DefaultDownloadTimeout        = 0 // 0 means no timeout (infinite)
	DefaultProgressUpdateInterval = 5 * time.Second
	DefaultPasswordMinLength      = 8
)

var GlobalConfig *Config

func InitConfig() error {
	var err error
	GlobalConfig, err = NewConfig()
	if err != nil {
		log.Fatalf("Failed to create configuration: %v", err)
		return err
	}

	return nil
}

type Config struct {
	BotToken        string
	MoviePath       string
	AdminPassword   string
	RegularPassword string
	Lang            string
	Proxy           string
	ProxyHost       string
	LogLevel        string
	LangPath        string
	ProwlarrURL     string
	ProwlarrAPIKey  string

	DownloadSettings DownloadConfig
	SecuritySettings SecurityConfig
}

type DownloadConfig struct {
	MaxConcurrentDownloads int
	DownloadTimeout        time.Duration
	ProgressUpdateInterval time.Duration
}

type SecurityConfig struct {
	PasswordMinLength int
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value, exists := os.LookupEnv(key); exists {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func NewConfig() (*Config, error) {
	config := &Config{
		BotToken:        getEnv("BOT_TOKEN", ""),
		MoviePath:       getEnv("MOVIE_PATH", ""),
		AdminPassword:   getEnv("ADMIN_PASSWORD", ""),
		RegularPassword: getEnv("REGULAR_PASSWORD", ""),
		Lang:            getEnv("LANG", "en"),
		Proxy:           getEnv("PROXY", ""),
		ProxyHost:       getEnv("PROXY_HOST", ""),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		LangPath:        getEnv("LANG_PATH", "/usr/local/share/telegram-media-server/locales"),
		ProwlarrURL:     getEnv("PROWLARR_URL", ""),
		ProwlarrAPIKey:  getEnv("PROWLARR_API_KEY", ""),

		DownloadSettings: DownloadConfig{
			MaxConcurrentDownloads: getEnvInt("MAX_CONCURRENT_DOWNLOADS", 3),
			DownloadTimeout:        getEnvDuration("DOWNLOAD_TIMEOUT", DefaultDownloadTimeout),
			ProgressUpdateInterval: getEnvDuration("PROGRESS_UPDATE_INTERVAL", DefaultProgressUpdateInterval),
		},

		SecuritySettings: SecurityConfig{
			PasswordMinLength: getEnvInt("PASSWORD_MIN_LENGTH", DefaultPasswordMinLength),
		},
	}

	if getEnv("RUNNING_IN_DOCKER", "false") == "true" {
		config.MoviePath = "/app/media"
		config.LangPath = "/app/locales"
		log.Printf("Running inside Docker, setting MOVIE_PATH to %s and LANG_PATH to %s", config.MoviePath, config.LangPath)
	}

	if err := config.validate(); err != nil {
		log.Printf("Configuration validation failed: %v", err)
		return nil, utils.WrapError(err, "configuration validation failed", map[string]any{
			"config": config,
		})
	}

	log.Println("Configuration loaded successfully")
	return config, nil
}

func (c *Config) validate() error {
	if err := c.validateRequiredFields(); err != nil {
		return err
	}

	if err := c.validatePasswords(); err != nil {
		return err
	}

	if err := c.validateProwlarr(); err != nil {
		return err
	}

	if err := c.validateDownloadSettings(); err != nil {
		return err
	}

	return nil
}

func (c *Config) validateRequiredFields() error {
	var missingFields []string

	if c.BotToken == "" {
		missingFields = append(missingFields, "BOT_TOKEN")
	}
	if c.MoviePath == "" {
		missingFields = append(missingFields, "MOVIE_PATH")
	}
	if c.AdminPassword == "" {
		missingFields = append(missingFields, "ADMIN_PASSWORD")
	}

	if len(missingFields) > 0 {
		return utils.WrapError(utils.ErrConfigurationError, "missing required environment variables", map[string]any{
			"missing_fields": missingFields,
		})
	}

	return nil
}

func (c *Config) validatePasswords() error {
	if len(c.AdminPassword) < c.SecuritySettings.PasswordMinLength {
		return utils.WrapError(utils.ErrConfigurationError, "admin password too short", map[string]any{
			"min_length":    c.SecuritySettings.PasswordMinLength,
			"actual_length": len(c.AdminPassword),
		})
	}

	if c.RegularPassword == "" {
		log.Println("REGULAR_PASSWORD not set, using ADMIN_PASSWORD as REGULAR_PASSWORD")
		c.RegularPassword = c.AdminPassword
	} else if len(c.RegularPassword) < c.SecuritySettings.PasswordMinLength {
		return utils.WrapError(utils.ErrConfigurationError, "regular password too short", map[string]any{
			"min_length":    c.SecuritySettings.PasswordMinLength,
			"actual_length": len(c.RegularPassword),
		})
	}

	return nil
}

func (c *Config) validateProwlarr() error {
	if (c.ProwlarrURL != "" || c.ProwlarrAPIKey != "") && (c.ProwlarrURL == "" || c.ProwlarrAPIKey == "") {
		var missingFields []string
		if c.ProwlarrURL == "" {
			missingFields = append(missingFields, "PROWLARR_URL (required if PROWLARR_API_KEY is set)")
		}
		if c.ProwlarrAPIKey == "" {
			missingFields = append(missingFields, "PROWLARR_API_KEY (required if PROWLARR_URL is set)")
		}
		return utils.WrapError(utils.ErrConfigurationError, "missing required environment variables", map[string]any{
			"missing_fields": missingFields,
		})
	}

	return nil
}

func (c *Config) validateDownloadSettings() error {
	if c.DownloadSettings.MaxConcurrentDownloads <= 0 {
		return utils.WrapError(utils.ErrConfigurationError, "max concurrent downloads must be positive", nil)
	}

	if c.DownloadSettings.DownloadTimeout < 0 {
		return utils.WrapError(utils.ErrConfigurationError, "download timeout cannot be negative", nil)
	}

	return nil
}

func (c *Config) GetDownloadSettings() DownloadConfig {
	return c.DownloadSettings
}

func (c *Config) GetSecuritySettings() SecurityConfig {
	return c.SecuritySettings
}

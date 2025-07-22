package config

import (
	"fmt"
	"log"
	"os"
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
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
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
	}

	if getEnv("RUNNING_IN_DOCKER", "false") == "true" {
		config.MoviePath = "/app/media"
		config.LangPath = "/app/locales"
		log.Printf("Running inside Docker, setting MOVIE_PATH to %s and LANG_PATH to %s", config.MoviePath, config.LangPath)
	}

	if err := config.validate(); err != nil {
		log.Printf("Configuration validation failed: %v", err)
		return nil, err
	}

	log.Println("Configuration loaded successfully")
	return config, nil
}

func (c *Config) validate() error {
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
	if c.RegularPassword == "" {
		log.Println("REGULAR_PASSWORD not set, using ADMIN_PASSWORD as REGULAR_PASSWORD")
		c.RegularPassword = c.AdminPassword
	}

	if (c.ProwlarrURL != "" || c.ProwlarrAPIKey != "") && (c.ProwlarrURL == "" || c.ProwlarrAPIKey == "") {
		if c.ProwlarrURL == "" {
			missingFields = append(missingFields, "PROWLARR_URL (required if PROWLARR_API_KEY is set)")
		}
		if c.ProwlarrAPIKey == "" {
			missingFields = append(missingFields, "PROWLARR_API_KEY (required if PROWLARR_URL is set)")
		}
	}
	if len(missingFields) > 0 {
		return fmt.Errorf("missing required environment variables: %v", missingFields)
	}
	return nil
}

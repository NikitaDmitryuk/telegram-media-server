package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

var GlobalConfig *Config

func init() {
	var err error
	GlobalConfig, err = NewConfig()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create configuration")
	}

	if err := setLogLevel(GlobalConfig.LogLevel); err != nil {
		logrus.WithError(err).Fatal("Invalid log level configuration")
	}
}

type Config struct {
	BotToken        string
	MoviePath       string
	AdminPassword   string
	RegularPassword string
	Lang            string
	Proxy           string
	ProxyHost       string
	MessageFilePath string
	LogLevel        string
}

func getEnv(key string, defaultValue string) string {
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
		MessageFilePath: getEnv("MESSAGE_FILE_PATH", ""),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
	}

	if getEnv("RUNNING_IN_DOCKER", "false") == "true" {
		config.MoviePath = "/app/media"
		logrus.Infof("Running inside Docker, setting MOVIE_PATH to %s", config.MoviePath)
	}

	if err := config.validate(); err != nil {
		logrus.WithError(err).Error("Configuration validation failed")
		return nil, err
	}

	logrus.Info("Configuration loaded successfully")
	return config, nil
}

func (c *Config) validate() error {
	missingFields := []string{}
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
		logrus.Warn("REGULAR_PASSWORD not set, using ADMIN_PASSWORD as REGULAR_PASSWORD")
		c.RegularPassword = c.AdminPassword
	}

	if len(missingFields) > 0 {
		return fmt.Errorf("missing required environment variables: %v", missingFields)
	}
	return nil
}

func setLogLevel(level string) error {
	parsedLevel, err := logrus.ParseLevel(strings.ToLower(level))
	if err != nil {
		return fmt.Errorf("invalid log level: %s", level)
	}
	logrus.SetLevel(parsedLevel)
	logrus.Infof("Log level set to %s", level)
	return nil
}

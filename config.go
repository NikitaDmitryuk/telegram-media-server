package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
)

type Config struct {
	BotToken              string
	MoviePath             string
	Password              string
	Lang                  string
	UpdateIntervalSeconds int
	UpdatePercentageStep  int
	MaxWaitTimeMinutes    int
	MinDownloadPercentage int
	MessageFilePath       string
}

func getEnv(key string, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if valueStr, exists := os.LookupEnv(key); exists {
		if value, err := strconv.Atoi(valueStr); err == nil {
			return value
		} else {
			log.Fatalf("Error converting %s to int: %v", key, err)
		}
	}
	return defaultValue
}

func NewConfig() *Config {
	config := &Config{
		BotToken:              getEnv("BOT_TOKEN", ""),
		MoviePath:             getEnv("MOVIE_PATH", ""),
		Password:              getEnv("PASSWORD", ""),
		Lang:                  getEnv("LANG", "en"),
		MessageFilePath:       getEnv("MESSAGE_FILE_PATH", "/etc/telegram-media-server/messages.yaml"),
		UpdateIntervalSeconds: getEnvInt("UPDATE_INTERVAL_SECONDS", 30),
		UpdatePercentageStep:  getEnvInt("UPDATE_PERCENTAGE_STEP", 20),
		MaxWaitTimeMinutes:    getEnvInt("MAX_WAIT_TIME_MINUTES", 10),
		MinDownloadPercentage: getEnvInt("MIN_DOWNLOAD_PERCENTAGE", 10),
	}
	return config
}

func (c *Config) Validate() error {
	missingFields := []string{}
	if c.BotToken == "" {
		missingFields = append(missingFields, "BOT_TOKEN")
	}
	if c.MoviePath == "" {
		missingFields = append(missingFields, "MOVIE_PATH")
	}
	if c.Password == "" {
		missingFields = append(missingFields, "PASSWORD")
	}
	if len(missingFields) > 0 {
		return fmt.Errorf("Missing required environment variables: %v", missingFields)
	}
	return nil
}

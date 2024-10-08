package main

import (
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

func NewConfig() *Config {
	updateIntervalSeconds, err := strconv.Atoi(os.Getenv("UPDATE_INTERVAL_SECONDS"))
	if err != nil {
		log.Fatalf("Error converting UPDATE_INTERVAL_SECONDS to int: %v", err)
	}

	updatePercentageStep, err := strconv.Atoi(os.Getenv("UPDATE_PERCENTAGE_STEP"))
	if err != nil {
		log.Fatalf("Error converting UPDATE_PERCENTAGE_STEP to int: %v", err)
	}

	maxWaitTimeMinutes, err := strconv.Atoi(os.Getenv("MAX_WAIT_TIME_MINUTES"))
	if err != nil {
		log.Fatalf("Error converting MAX_WAIT_TIME_MINUTES to int: %v", err)
	}

	minDownloadPercentage, err := strconv.Atoi(os.Getenv("MIN_DOWNLOAD_PERCENTAGE"))
	if err != nil {
		log.Fatalf("Error converting MIN_DOWNLOAD_PERCENTAGE to int: %v", err)
	}

	return &Config{
		BotToken:              os.Getenv("BOT_TOKEN"),
		MoviePath:             os.Getenv("MOVIE_PATH"),
		Password:              os.Getenv("PASSWORD"),
		Lang:                  os.Getenv("LANG"),
		MessageFilePath:       os.Getenv("MESSAGE_FILE_PATH"),
		UpdateIntervalSeconds: updateIntervalSeconds,
		UpdatePercentageStep:  updatePercentageStep,
		MaxWaitTimeMinutes:    maxWaitTimeMinutes,
		MinDownloadPercentage: minDownloadPercentage,
	}
}

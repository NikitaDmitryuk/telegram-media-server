package main

import (
	"os"
)

type Config struct {
	BotToken string
	MoviePath string
	Password string
}

func NewConfig() *Config {
    return &Config{
	    BotToken: os.Getenv("BOT_TOKEN"),
	    MoviePath: os.Getenv("MOVIE_PATH"),
	    Password: os.Getenv("PASSWORD"),
    }
}

// config.go
package config

import (
    "os"
)

type Config struct {
    BotToken string
}

func NewConfig() *Config {
    return &Config{
        BotToken: os.Getenv("BOT_TOKEN"),
    }
}

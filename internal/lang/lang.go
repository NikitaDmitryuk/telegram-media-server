package lang

import (
	"fmt"
	"log"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
)

var lang string

func SetupLang(config *tmsconfig.Config) error {
	lang = config.Lang
	return nil
}

func GetMessage(id MessageID, args ...interface{}) string {
	if m, ok := messages[id]; ok {
		if msg, ok := m[lang]; ok {
			return fmt.Sprintf(msg, args...)
		}
		if msg, ok := m["en"]; ok {
			return fmt.Sprintf(msg, args...)
		}
	}
	log.Printf("Message not found for ID: %s", id)
	return "Message not found"
}

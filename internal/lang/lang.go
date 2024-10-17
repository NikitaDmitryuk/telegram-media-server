package lang

import (
	"fmt"
	"log"
	"os"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"gopkg.in/yaml.v2"
)

var messages map[MessageID]map[string]string
var lang string

func SetupLang(config *tmsconfig.Config) error {

	data, err := os.ReadFile(config.MessageFilePath)
	if err != nil {
		return err
	}
	lang = config.Lang
	return yaml.Unmarshal(data, &messages)
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
	log.Printf("Message not found")
	return "Message not found"
}

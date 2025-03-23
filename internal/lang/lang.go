package lang

import (
	"fmt"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/sirupsen/logrus"
)

var lang string

func SetupLang(config *tmsconfig.Config) error {
	lang = config.Lang
	logrus.Infof("Language set to: %s", lang)
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
	logrus.Warnf("Message not found for ID: %s", id)
	return "Message not found"
}

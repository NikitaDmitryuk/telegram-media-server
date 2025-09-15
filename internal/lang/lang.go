package lang

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

var localizer *i18n.Localizer

func InitLocalizer(config *domain.Config) error {
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	mainLang := config.Lang
	mainLangFile := filepath.Join(config.LangPath, fmt.Sprintf("active.%s.json", mainLang))
	_, err := bundle.LoadMessageFile(mainLangFile)
	if err != nil {
		logger.Log.Warnf("Failed to load main language file %s: %v", mainLangFile, err)
	}

	if mainLang != "en" {
		englishFile := filepath.Join(config.LangPath, "active.en.json")
		_, err = bundle.LoadMessageFile(englishFile)
		if err != nil {
			logger.Log.Warnf("Failed to load fallback language file %s: %v", englishFile, err)
		}
	}

	localizer = i18n.NewLocalizer(bundle, mainLang, "en")
	logger.Log.Info("Localizer initialized successfully")
	return nil
}

func Translate(key string, data map[string]any) string {
	translation, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    key,
		TemplateData: data,
	})
	if err != nil {
		logger.Log.Errorf("Localization error for key '%s': %v", key, err)
		return key
	}

	if translation == "" {
		logger.Log.Errorf("Translation not found for key: %s", key)
		return key
	}

	return translation
}

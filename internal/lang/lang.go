package lang

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

var localizer *i18n.Localizer

func InitLocalizer() error {
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	mainLang := tmsconfig.GlobalConfig.Lang
	mainLangFile := filepath.Join(tmsconfig.GlobalConfig.LangPath, fmt.Sprintf("active.%s.json", mainLang))
	_, err := bundle.LoadMessageFile(mainLangFile)
	if err != nil {
		logutils.Log.Warnf("Failed to load main language file %s: %v", mainLangFile, err)
	}

	if mainLang != "en" {
		englishFile := filepath.Join(tmsconfig.GlobalConfig.LangPath, "active.en.json")
		_, err = bundle.LoadMessageFile(englishFile)
		if err != nil {
			logutils.Log.Warnf("Failed to load fallback language file %s: %v", englishFile, err)
		}
	}

	localizer = i18n.NewLocalizer(bundle, mainLang, "en")
	logutils.Log.Info("Localizer initialized successfully")
	return nil
}

func Translate(key string, data map[string]any) string {
	translation, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    key,
		TemplateData: data,
	})
	if err != nil {
		logutils.Log.Errorf("Localization error for key '%s': %v", key, err)
		return key
	}

	if translation == "" {
		logutils.Log.Errorf("Translation not found for key: %s", key)
		return key
	}

	return translation
}

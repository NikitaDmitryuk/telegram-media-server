package lang

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

var localizer *i18n.Localizer

// flattenMap преобразует вложенную структуру в плоские ключи
func flattenMap(data map[string]any, prefix string) map[string]string {
	result := make(map[string]string)

	for key, value := range data {
		newKey := key
		if prefix != "" {
			newKey = prefix + "." + key
		}

		switch v := value.(type) {
		case string:
			result[newKey] = v
		case map[string]any:
			for k, val := range flattenMap(v, newKey) {
				result[k] = val
			}
		}
	}

	return result
}

// loadNestedJSONFile загружает файл с вложенной структурой и преобразует его в формат для go-i18n
func loadNestedJSONFile(bundle *i18n.Bundle, filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	var nestedData map[string]any
	if unmarshalErr := json.Unmarshal(data, &nestedData); unmarshalErr != nil {
		return fmt.Errorf("failed to unmarshal JSON from %s: %w", filename, unmarshalErr)
	}

	// Преобразуем в плоскую структуру
	flatData := flattenMap(nestedData, "")

	// Создаем массив сообщений для go-i18n
	var messages []map[string]string
	for key, value := range flatData {
		messages = append(messages, map[string]string{
			"id":          key,
			"translation": value,
		})
	}

	// Преобразуем обратно в JSON для go-i18n
	flatJSON, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("failed to marshal flattened data: %w", err)
	}

	// Определяем язык из имени файла
	baseName := filepath.Base(filename)
	parts := strings.Split(baseName, ".")
	if len(parts) < 3 {
		return fmt.Errorf("invalid filename format: %s", filename)
	}
	langCode := parts[1]

	// Загружаем сообщения в bundle
	_, err = bundle.ParseMessageFileBytes(flatJSON, langCode+".json")
	if err != nil {
		return fmt.Errorf("failed to parse messages: %w", err)
	}

	return nil
}

func InitLocalizer(config *domain.Config) error {
	bundle := i18n.NewBundle(language.English)

	mainLang := config.Lang
	mainLangFile := filepath.Join(config.LangPath, fmt.Sprintf("active.%s.json", mainLang))
	err := loadNestedJSONFile(bundle, mainLangFile)
	if err != nil {
		logger.Log.Warnf("Failed to load main language file %s: %v", mainLangFile, err)
	}

	if mainLang != "en" {
		englishFile := filepath.Join(config.LangPath, "active.en.json")
		err = loadNestedJSONFile(bundle, englishFile)
		if err != nil {
			logger.Log.Warnf("Failed to load fallback language file %s: %v", englishFile, err)
		}
	}

	localizer = i18n.NewLocalizer(bundle, mainLang, "en")
	logger.Log.Info("Localizer initialized successfully")
	return nil
}

func Translate(key string, data map[string]any) string {
	if localizer == nil {
		logger.Log.Errorf("Localizer not initialized")
		return key
	}

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

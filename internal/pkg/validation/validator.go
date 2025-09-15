package validation

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/errors"
)

// Validator представляет валидатор для различных типов данных
type Validator struct {
	errors []error
}

// NewValidator создает новый валидатор
func NewValidator() *Validator {
	return &Validator{
		errors: make([]error, 0),
	}
}

// AddError добавляет ошибку в валидатор
func (v *Validator) AddError(err error) {
	v.errors = append(v.errors, err)
}

// HasErrors проверяет наличие ошибок
func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

// GetErrors возвращает все ошибки
func (v *Validator) GetErrors() []error {
	return v.errors
}

// GetFirstError возвращает первую ошибку или nil
func (v *Validator) GetFirstError() error {
	if len(v.errors) > 0 {
		return v.errors[0]
	}
	return nil
}

// Clear очищает все ошибки
func (v *Validator) Clear() {
	v.errors = v.errors[:0]
}

// ValidateRequired проверяет, что строка не пустая
func (v *Validator) ValidateRequired(value, fieldName string) *Validator {
	if strings.TrimSpace(value) == "" {
		v.AddError(errors.NewDomainError(
			errors.ErrorTypeValidation,
			"required_field",
			fmt.Sprintf("%s is required", fieldName),
		).WithDetails(map[string]any{
			"field": fieldName,
		}))
	}
	return v
}

// ValidateMinLength проверяет минимальную длину строки
func (v *Validator) ValidateMinLength(value, fieldName string, minLength int) *Validator {
	if len(value) < minLength {
		v.AddError(errors.NewDomainError(
			errors.ErrorTypeValidation,
			"min_length",
			fmt.Sprintf("%s must be at least %d characters long", fieldName, minLength),
		).WithDetails(map[string]any{
			"field":      fieldName,
			"min_length": minLength,
			"actual":     len(value),
		}))
	}
	return v
}

// ValidateMaxLength проверяет максимальную длину строки
func (v *Validator) ValidateMaxLength(value, fieldName string, maxLength int) *Validator {
	if len(value) > maxLength {
		v.AddError(errors.NewDomainError(
			errors.ErrorTypeValidation,
			"max_length",
			fmt.Sprintf("%s must be no more than %d characters long", fieldName, maxLength),
		).WithDetails(map[string]any{
			"field":      fieldName,
			"max_length": maxLength,
			"actual":     len(value),
		}))
	}
	return v
}

// ValidateURL проверяет валидность URL
func (v *Validator) ValidateURL(value, fieldName string) *Validator {
	if value == "" {
		return v // Пропускаем пустые значения
	}

	if _, err := url.Parse(value); err != nil {
		v.AddError(errors.NewDomainError(
			errors.ErrorTypeValidation,
			"invalid_url",
			fmt.Sprintf("%s must be a valid URL", fieldName),
		).WithDetails(map[string]any{
			"field": fieldName,
			"value": value,
		}))
	}
	return v
}

// ValidateRegex проверяет соответствие регулярному выражению
func (v *Validator) ValidateRegex(value, fieldName, pattern, errorMessage string) *Validator {
	if value == "" {
		return v // Пропускаем пустые значения
	}

	matched, err := regexp.MatchString(pattern, value)
	if err != nil {
		v.AddError(errors.NewDomainError(
			errors.ErrorTypeValidation,
			"regex_error",
			fmt.Sprintf("regex validation failed for %s", fieldName),
		).WithDetails(map[string]any{
			"field":   fieldName,
			"pattern": pattern,
			"error":   err.Error(),
		}))
		return v
	}

	if !matched {
		v.AddError(errors.NewDomainError(
			errors.ErrorTypeValidation,
			"regex_mismatch",
			errorMessage,
		).WithDetails(map[string]any{
			"field":   fieldName,
			"pattern": pattern,
			"value":   value,
		}))
	}
	return v
}

// ValidatePositive проверяет, что число положительное
func (v *Validator) ValidatePositive(value int, fieldName string) *Validator {
	if value <= 0 {
		v.AddError(errors.NewDomainError(
			errors.ErrorTypeValidation,
			"positive_number",
			fmt.Sprintf("%s must be greater than 0", fieldName),
		).WithDetails(map[string]any{
			"field": fieldName,
			"value": value,
		}))
	}
	return v
}

// ValidateNonNegative проверяет, что число неотрицательное
func (v *Validator) ValidateNonNegative(value int, fieldName string) *Validator {
	if value < 0 {
		v.AddError(errors.NewDomainError(
			errors.ErrorTypeValidation,
			"non_negative",
			fmt.Sprintf("%s cannot be negative", fieldName),
		).WithDetails(map[string]any{
			"field": fieldName,
			"value": value,
		}))
	}
	return v
}

// ValidateRange проверяет, что число в заданном диапазоне
func (v *Validator) ValidateRange(value, minVal, maxVal int, fieldName string) *Validator {
	if value < minVal || value > maxVal {
		v.AddError(errors.NewDomainError(
			errors.ErrorTypeValidation,
			"out_of_range",
			fmt.Sprintf("%s must be between %d and %d", fieldName, minVal, maxVal),
		).WithDetails(map[string]any{
			"field":     fieldName,
			"value":     value,
			"min_value": minVal,
			"max_value": maxVal,
		}))
	}
	return v
}

// ValidateDuration проверяет валидность duration
func (v *Validator) ValidateDuration(value time.Duration, fieldName string) *Validator {
	if value < 0 {
		v.AddError(errors.NewDomainError(
			errors.ErrorTypeValidation,
			"negative_duration",
			fmt.Sprintf("%s cannot be negative", fieldName),
		).WithDetails(map[string]any{
			"field": fieldName,
			"value": value.String(),
		}))
	}
	return v
}

// ValidateOneOf проверяет, что значение входит в список допустимых
func (v *Validator) ValidateOneOf(value, fieldName string, allowedValues []string) *Validator {
	if value == "" {
		return v // Пропускаем пустые значения
	}

	for _, allowed := range allowedValues {
		if value == allowed {
			return v
		}
	}

	v.AddError(errors.NewDomainError(
		errors.ErrorTypeValidation,
		"invalid_value",
		fmt.Sprintf("%s must be one of: %s", fieldName, strings.Join(allowedValues, ", ")),
	).WithDetails(map[string]any{
		"field":          fieldName,
		"value":          value,
		"allowed_values": allowedValues,
	}))
	return v
}

// ValidateConditional выполняет валидацию только если условие истинно
func (v *Validator) ValidateConditional(condition bool, validationFunc func(*Validator) *Validator) *Validator {
	if condition {
		return validationFunc(v)
	}
	return v
}

// ValidateCustom выполняет пользовательскую валидацию
func (v *Validator) ValidateCustom(validationFunc func() error) *Validator {
	if err := validationFunc(); err != nil {
		v.AddError(err)
	}
	return v
}

// ConfigValidator специализированный валидатор для конфигурации
type ConfigValidator struct {
	*Validator
}

// NewConfigValidator создает новый валидатор конфигурации
func NewConfigValidator() *ConfigValidator {
	return &ConfigValidator{
		Validator: NewValidator(),
	}
}

// ValidateBotToken валидирует токен бота
func (cv *ConfigValidator) ValidateBotToken(token string) *ConfigValidator {
	cv.ValidateRequired(token, "BOT_TOKEN")

	// Telegram bot token format: 123456789:ABCdefGHIjklMNOpqrsTUVwxyz
	if token != "" {
		cv.ValidateRegex(
			token,
			"BOT_TOKEN",
			`^\d+:[A-Za-z0-9_-]+$`,
			"BOT_TOKEN must be in format: 123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
		)
	}

	return cv
}

// ValidateMoviePath валидирует путь к фильмам
func (cv *ConfigValidator) ValidateMoviePath(path string) *ConfigValidator {
	cv.ValidateRequired(path, "MOVIE_PATH")
	return cv
}

// ValidatePassword валидирует пароль
func (cv *ConfigValidator) ValidatePassword(password, fieldName string, minLength int) *ConfigValidator {
	cv.ValidateRequired(password, fieldName).
		ValidateMinLength(password, fieldName, minLength)
	return cv
}

// ValidateProwlarrConfig валидирует конфигурацию Prowlarr
func (cv *ConfigValidator) ValidateProwlarrConfig(prowlarrURL, apiKey string) *ConfigValidator {
	// Если URL задан, то API ключ обязателен
	cv.ValidateConditional(prowlarrURL != "", func(v *Validator) *Validator {
		return v.ValidateRequired(apiKey, "PROWLARR_API_KEY")
	})

	// Если API ключ задан, то URL обязателен
	cv.ValidateConditional(apiKey != "", func(v *Validator) *Validator {
		return v.ValidateRequired(prowlarrURL, "PROWLARR_URL")
	})

	cv.ValidateURL(prowlarrURL, "PROWLARR_URL")
	return cv
}

// ValidateLogLevel валидирует уровень логирования
func (cv *ConfigValidator) ValidateLogLevel(level string) *ConfigValidator {
	allowedLevels := []string{"debug", "info", "warn", "error", "fatal", "panic"}
	cv.ValidateOneOf(level, "LOG_LEVEL", allowedLevels)
	return cv
}

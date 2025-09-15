package utils

import (
	"fmt"
)

// AppError представляет кастомную ошибку приложения
type AppError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Context map[string]any `json:"context,omitempty"`
	Cause   error          `json:"-"`
}

// Error реализует интерфейс error
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap возвращает исходную ошибку для поддержки errors.Unwrap
func (e *AppError) Unwrap() error {
	return e.Cause
}

// NewAppError создает новую ошибку приложения
func NewAppError(code, message string, context map[string]any) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Context: context,
	}
}

// WrapError оборачивает ошибку в AppError
func WrapError(err error, message string, context map[string]any) *AppError {
	return &AppError{
		Code:    "wrapped_error",
		Message: message,
		Context: context,
		Cause:   err,
	}
}

// IsAppError проверяет, является ли ошибка AppError
func IsAppError(err error) bool {
	_, ok := err.(*AppError)
	return ok
}

// GetAppError извлекает AppError из ошибки
func GetAppError(err error) (*AppError, bool) {
	appErr, ok := err.(*AppError)
	return appErr, ok
}

// ErrorCode возвращает код ошибки, если это AppError
func ErrorCode(err error) string {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Code
	}
	return "unknown_error"
}

// ErrorContext возвращает контекст ошибки, если это AppError
func ErrorContext(err error) map[string]any {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Context
	}
	return nil
}

package errors

import (
	"fmt"
)

// ErrorType представляет тип ошибки для лучшей категоризации
type ErrorType string

const (
	// Validation errors
	ErrorTypeValidation ErrorType = "validation"
	ErrorTypeConfig     ErrorType = "config"

	// Authentication and authorization errors
	ErrorTypeAuth       ErrorType = "auth"
	ErrorTypePermission ErrorType = "permission"

	// External service errors
	ErrorTypeExternal ErrorType = "external"
	ErrorTypeNetwork  ErrorType = "network"
	ErrorTypeTelegram ErrorType = "telegram"
	ErrorTypeProwlarr ErrorType = "prowlarr"

	// Internal errors
	ErrorTypeDatabase   ErrorType = "database"
	ErrorTypeFileSystem ErrorType = "filesystem"
	ErrorTypeDownload   ErrorType = "download"
	ErrorTypeInternal   ErrorType = "internal"

	// Business logic errors
	ErrorTypeBusiness ErrorType = "business"
	ErrorTypeNotFound ErrorType = "not_found"
	ErrorTypeConflict ErrorType = "conflict"
)

// DomainError представляет доменную ошибку с типизацией
type DomainError struct {
	Type    ErrorType      `json:"type"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
	Cause   error          `json:"-"`
	UserMsg string         `json:"user_message,omitempty"` // Сообщение для пользователя
}

// Error реализует интерфейс error
func (e *DomainError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s:%s] %s: %v", e.Type, e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s:%s] %s", e.Type, e.Code, e.Message)
}

// Unwrap возвращает исходную ошибку
func (e *DomainError) Unwrap() error {
	return e.Cause
}

// Is проверяет тип ошибки
func (e *DomainError) Is(target error) bool {
	if t, ok := target.(*DomainError); ok {
		return e.Type == t.Type && e.Code == t.Code
	}
	return false
}

// GetUserMessage возвращает сообщение для пользователя или дефолтное
func (e *DomainError) GetUserMessage() string {
	if e.UserMsg != "" {
		return e.UserMsg
	}
	return e.Message
}

// IsRetryable определяет, можно ли повторить операцию
func (e *DomainError) IsRetryable() bool {
	switch e.Type {
	case ErrorTypeNetwork, ErrorTypeExternal, ErrorTypeTelegram, ErrorTypeProwlarr:
		return true
	case ErrorTypeDatabase:
		// Некоторые ошибки БД можно повторить
		return e.Code == "connection_lost" || e.Code == "timeout"
	default:
		return false
	}
}

// NewDomainError создает новую доменную ошибку
func NewDomainError(errType ErrorType, code, message string) *DomainError {
	return &DomainError{
		Type:    errType,
		Code:    code,
		Message: message,
		Details: make(map[string]any),
	}
}

// WrapDomainError оборачивает существующую ошибку в доменную
func WrapDomainError(err error, errType ErrorType, code, message string) *DomainError {
	return &DomainError{
		Type:    errType,
		Code:    code,
		Message: message,
		Cause:   err,
		Details: make(map[string]any),
	}
}

// WithDetails добавляет детали к ошибке
func (e *DomainError) WithDetails(details map[string]any) *DomainError {
	for k, v := range details {
		e.Details[k] = v
	}
	return e
}

// WithUserMessage добавляет сообщение для пользователя
func (e *DomainError) WithUserMessage(msg string) *DomainError {
	e.UserMsg = msg
	return e
}

// Предопределенные ошибки для частых случаев

// Validation errors
var (
	ErrInvalidInput  = NewDomainError(ErrorTypeValidation, "invalid_input", "invalid input provided")
	ErrMissingField  = NewDomainError(ErrorTypeValidation, "missing_field", "required field is missing")
	ErrInvalidFormat = NewDomainError(ErrorTypeValidation, "invalid_format", "invalid format")
)

// Auth errors
var (
	ErrUnauthorized = NewDomainError(ErrorTypeAuth, "unauthorized", "authentication required").
			WithUserMessage("error.general.unauthorized")
	ErrInvalidCredentials = NewDomainError(ErrorTypeAuth, "invalid_credentials", "invalid credentials").
				WithUserMessage("error.general.invalid_credentials")
	ErrAccessDenied = NewDomainError(ErrorTypePermission, "access_denied", "access denied").
			WithUserMessage("error.general.access_denied")
)

// Business logic errors
var (
	ErrNotFound = NewDomainError(ErrorTypeNotFound, "not_found", "resource not found").
			WithUserMessage("error.general.not_found")
	ErrAlreadyExists = NewDomainError(ErrorTypeConflict, "already_exists", "resource already exists").
				WithUserMessage("error.general.already_exists")
	ErrInvalidState = NewDomainError(ErrorTypeBusiness, "invalid_state", "invalid state for operation").
			WithUserMessage("error.general.invalid_state")
)

// External service errors
var (
	ErrExternalService = NewDomainError(ErrorTypeExternal, "service_unavailable", "external service unavailable").
				WithUserMessage("error.general.external_service")
	ErrNetworkTimeout = NewDomainError(ErrorTypeNetwork, "timeout", "network timeout").
				WithUserMessage("error.general.network_timeout")
)

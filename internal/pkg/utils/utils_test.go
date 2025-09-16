package utils

import (
	"errors"
	"testing"
)

func TestWrapError(t *testing.T) {
	originalErr := errors.New("original error")
	context := map[string]any{
		"key1": "value1",
		"key2": 123,
	}

	wrappedErr := WrapError(originalErr, "wrapped message", context)

	// Test that the wrapped error contains the original error
	if !errors.Is(wrappedErr, originalErr) {
		t.Errorf("Wrapped error should contain the original error")
	}

	// Test error message format
	errorMsg := wrappedErr.Error()
	if errorMsg != "wrapped_error: wrapped message (caused by: original error)" {
		t.Errorf("Expected error message to be 'wrapped_error: wrapped message (caused by: original error)', got '%s'", errorMsg)
	}

	// Test that we can unwrap the error
	var wrappedError *AppError
	if !errors.As(wrappedErr, &wrappedError) {
		t.Errorf("Should be able to assert as AppError")
	}

	if !errors.Is(wrappedError.Cause, originalErr) {
		t.Errorf("AppError.Cause should be the original error")
	}

	if wrappedError.Message != "wrapped message" {
		t.Errorf("Expected message 'wrapped message', got '%s'", wrappedError.Message)
	}

	if wrappedError.Context["key1"] != "value1" {
		t.Errorf("Expected context key1 to be 'value1', got '%v'", wrappedError.Context["key1"])
	}

	if wrappedError.Context["key2"] != 123 {
		t.Errorf("Expected context key2 to be 123, got '%v'", wrappedError.Context["key2"])
	}
}

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		code     string
		message  string
		expected string
	}{
		{
			name:     "error with cause",
			err:      errors.New("original"),
			code:     "test_error",
			message:  "test message",
			expected: "test_error: test message (caused by: original)",
		},
		{
			name:     "error without cause",
			err:      nil,
			code:     "test_error",
			message:  "test message",
			expected: "test_error: test message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appErr := &AppError{
				Code:    tt.code,
				Cause:   tt.err,
				Message: tt.message,
			}

			if appErr.Error() != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, appErr.Error())
			}
		})
	}
}

func TestNewAppError(t *testing.T) {
	code := "test_error"
	message := "test error"
	context := map[string]any{"key": "value"}

	appErr := NewAppError(code, message, context)

	// appErr is already *AppError, no need to cast

	if appErr.Cause != nil {
		t.Errorf("Expected Cause to be nil for NewAppError, got %v", appErr.Cause)
	}

	if appErr.Code != code {
		t.Errorf("Expected code '%s', got '%s'", code, appErr.Code)
	}

	if appErr.Message != message {
		t.Errorf("Expected message '%s', got '%s'", message, appErr.Message)
	}

	if appErr.Context["key"] != "value" {
		t.Errorf("Expected context key to be 'value', got '%v'", appErr.Context["key"])
	}
}

func TestIsAppError(t *testing.T) {
	appErr := NewAppError("test", "test message", nil)
	regularErr := errors.New("regular error")

	if !IsAppError(appErr) {
		t.Errorf("Expected IsAppError to return true for AppError")
	}

	if IsAppError(regularErr) {
		t.Errorf("Expected IsAppError to return false for regular error")
	}
}

func TestGetAppError(t *testing.T) {
	appErr := NewAppError("test", "test message", nil)
	regularErr := errors.New("regular error")

	// Test with AppError
	extracted, ok := GetAppError(appErr)
	if !ok {
		t.Errorf("Expected GetAppError to return true for AppError")
	}
	if extracted.Code != "test" {
		t.Errorf("Expected extracted code to be 'test', got '%s'", extracted.Code)
	}

	// Test with regular error
	_, ok = GetAppError(regularErr)
	if ok {
		t.Errorf("Expected GetAppError to return false for regular error")
	}
}

func TestErrorCode(t *testing.T) {
	appErr := NewAppError("test_code", "test message", nil)
	regularErr := errors.New("regular error")

	if ErrorCode(appErr) != "test_code" {
		t.Errorf("Expected ErrorCode to return 'test_code', got '%s'", ErrorCode(appErr))
	}

	if ErrorCode(regularErr) != "unknown_error" {
		t.Errorf("Expected ErrorCode to return 'unknown_error' for regular error, got '%s'", ErrorCode(regularErr))
	}
}

func TestErrorContext(t *testing.T) {
	context := map[string]any{"key": "value"}
	appErr := NewAppError("test", "test message", context)
	regularErr := errors.New("regular error")

	appContext := ErrorContext(appErr)
	if appContext["key"] != "value" {
		t.Errorf("Expected context key to be 'value', got '%v'", appContext["key"])
	}

	regularContext := ErrorContext(regularErr)
	if regularContext != nil {
		t.Errorf("Expected ErrorContext to return nil for regular error, got %v", regularContext)
	}
}

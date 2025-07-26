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

	// Test that the error message includes the wrapper message
	errorMsg := wrappedErr.Error()
	if errorMsg != "wrapped message: original error" {
		t.Errorf("Expected error message to be 'wrapped message: original error', got '%s'", errorMsg)
	}

	// Test that we can unwrap the error
	var wrappedError *WrappedError
	if !errors.As(wrappedErr, &wrappedError) {
		t.Errorf("Should be able to assert as WrappedError")
	}

	if !errors.Is(wrappedError.Err, originalErr) {
		t.Errorf("WrappedError.Err should be the original error")
	}

	if wrappedError.Message != "wrapped message" {
		t.Errorf("Expected message 'wrapped message', got '%s'", wrappedError.Message)
	}

	if len(wrappedError.Context) != 2 {
		t.Errorf("Expected 2 context items, got %d", len(wrappedError.Context))
	}
}

func TestWrappedError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		message  string
		expected string
	}{
		{
			name:     "with message",
			err:      errors.New("test error"),
			message:  "wrapper message",
			expected: "wrapper message: test error",
		},
		{
			name:     "without message",
			err:      errors.New("test error"),
			message:  "",
			expected: "test error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := &WrappedError{
				Err:     tt.err,
				Message: tt.message,
			}

			if wrapped.Error() != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, wrapped.Error())
			}
		})
	}
}

func TestWrappedError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	wrapped := &WrappedError{
		Err: originalErr,
	}

	unwrapped := wrapped.Unwrap()
	if !errors.Is(unwrapped, originalErr) {
		t.Errorf("Unwrap() should return the original error")
	}
}

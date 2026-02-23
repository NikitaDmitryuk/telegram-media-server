package utils

import (
	"errors"
)

var (
	ErrInvalidURL           = errors.New("invalid URL provided")
	ErrDownloadFailed       = errors.New("download failed")
	ErrInsufficientSpace    = errors.New("insufficient disk space")
	ErrFileAlreadyExists    = errors.New("file already exists")
	ErrUnauthorized         = errors.New("unauthorized access")
	ErrInvalidPassword      = errors.New("invalid password")
	ErrUserNotFound         = errors.New("user not found")
	ErrDatabaseError        = errors.New("database operation failed")
	ErrExternalServiceError = errors.New("external service error")
	ErrConfigurationError   = errors.New("configuration error")
)

type WrappedError struct {
	Err     error
	Message string
	Context map[string]any
}

func (w *WrappedError) Error() string {
	if w.Message != "" {
		return w.Message + ": " + w.Err.Error()
	}
	return w.Err.Error()
}

func (w *WrappedError) Unwrap() error {
	return w.Err
}

func WrapError(err error, message string, ctx map[string]any) error {
	return &WrappedError{
		Err:     err,
		Message: message,
		Context: ctx,
	}
}

// RootError returns the innermost error in the chain (for user-facing messages without wrapper text).
func RootError(err error) error {
	for e := err; e != nil; e = errors.Unwrap(e) {
		err = e
	}
	return err
}

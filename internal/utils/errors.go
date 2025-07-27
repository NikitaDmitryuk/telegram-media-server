package utils

import (
	"context"
	"errors"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/models"
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

type DownloadService interface {
	StartDownload(ctx context.Context, url string) (uint, chan float64, chan error, error)
	StopDownload(movieID uint) error
	GetDownloadStatus(movieID uint) (float64, error)
}

type AuthService interface {
	Authenticate(chatID int64, password string) (bool, models.UserRole, error)
	CheckAccess(chatID int64) (bool, models.UserRole, error)
}

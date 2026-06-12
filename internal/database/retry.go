package database

import (
	"context"
	"strings"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

const (
	sqliteRetryAttempts = 10
	sqliteRetryDelay    = 50 * time.Millisecond
	sqliteRetryMaxDelay = 500 * time.Millisecond
)

func (*SQLiteDatabase) withRetry(ctx context.Context, operation string, fn func() error) error {
	var err error
	for attempt := range sqliteRetryAttempts {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err = fn()
		if !isSQLiteBusyError(err) {
			return err
		}
		if attempt == 0 && logutils.Log != nil {
			logutils.Log.WithError(err).WithField("operation", operation).Debug("SQLite busy; retrying operation")
		}
		delay := sqliteRetryDelay * time.Duration(1<<attempt)
		if delay > sqliteRetryMaxDelay {
			delay = sqliteRetryMaxDelay
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database is busy") ||
		strings.Contains(msg, "sqlite_busy") ||
		strings.Contains(msg, "sqlite_locked")
}

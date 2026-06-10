package database

import (
	"context"
	"strings"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

const (
	sqliteRetryAttempts = 5
	sqliteRetryDelay    = 25 * time.Millisecond
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
		timer := time.NewTimer(sqliteRetryDelay * time.Duration(attempt+1))
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

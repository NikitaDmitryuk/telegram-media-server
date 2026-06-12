package database

import (
	"context"
	"errors"
	"testing"
)

func TestWithRetryRetriesSQLiteBusyError(t *testing.T) {
	t.Parallel()
	db := &SQLiteDatabase{}
	attempts := 0
	err := db.withRetry(context.Background(), "test", func() error {
		attempts++
		if attempts == 1 {
			return errors.New("database is locked")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withRetry returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestWithRetryDoesNotRetryNonSQLiteBusyError(t *testing.T) {
	t.Parallel()
	db := &SQLiteDatabase{}
	attempts := 0
	wantErr := errors.New("other error")
	err := db.withRetry(context.Background(), "test", func() error {
		attempts++
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

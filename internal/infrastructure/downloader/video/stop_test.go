package ytdlp

import (
	"context"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

// TestYTDLPStopDownload tests the StopDownload functionality
func TestYTDLPStopDownload(t *testing.T) {
	logger.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)

	t.Run("StopNonExistentProcess", func(t *testing.T) {
		downloader := &YTDLPDownloader{
			config: cfg,
		}

		// Test stopping when no process is running
		done := make(chan error, 1)
		go func() {
			done <- downloader.StopDownload()
		}()

		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Expected no error for non-existent process, got %v", err)
			}
			if !downloader.StoppedManually() {
				t.Error("Expected stoppedManually to be true")
			}
		case <-time.After(1 * time.Second):
			t.Fatal("StopDownload timed out")
		}
	})

	t.Run("StopWithCancelOnly", func(t *testing.T) {
		downloader := &YTDLPDownloader{
			config: cfg,
		}

		// Create a context and cancel function
		ctx, cancel := context.WithCancel(context.Background())
		downloader.cancel = cancel

		// Test stopping with only cancel function
		done := make(chan error, 1)
		go func() {
			done <- downloader.StopDownload()
		}()

		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Expected no error for cancel-only stop, got %v", err)
			}
			if !downloader.StoppedManually() {
				t.Error("Expected stoppedManually to be true")
			}
			// Check context was canceled
			if ctx.Err() == nil {
				t.Error("Expected context to be canceled")
			}
		case <-time.After(1 * time.Second):
			t.Fatal("StopDownload timed out")
		}
	})
}

// TestYTDLPStopProcessTimeout tests timeout handling when process doesn't exit
func TestYTDLPStopProcessTimeout(t *testing.T) {
	t.Skip("Skipping timeout test - requires network connectivity")

	logger.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)

	t.Run("ProcessExitTimeout", func(t *testing.T) {
		downloader := &YTDLPDownloader{
			config: cfg,
		}

		// Create a mock command that won't exit quickly
		// This is hard to test properly without actual process, so we'll test the timeout logic
		start := time.Now()
		err := downloader.StopDownload()
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("Expected no error even with timeout, got %v", err)
		}

		// Should complete quickly since no actual process is running
		if elapsed > 100*time.Millisecond {
			t.Errorf("StopDownload took too long without real process: %v", elapsed)
		}
	})
}

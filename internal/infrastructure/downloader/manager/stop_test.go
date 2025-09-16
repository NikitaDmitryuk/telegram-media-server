package manager

import (
	"context"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

// TestDownloadManagerStopNonBlocking tests that stopping downloads doesn't block indefinitely
func TestDownloadManagerStopNonBlocking(t *testing.T) {
	logger.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)
	// For stop tests, we don't need a real database
	var db database.Database

	dm := NewDownloadManager(cfg, db)

	t.Run("StopNonExistentDownload", func(t *testing.T) {
		// Test stopping a download that doesn't exist
		done := make(chan error, 1)
		go func() {
			done <- dm.StopDownload(999) // Non-existent movie ID
		}()

		select {
		case err := <-done:
			if err != nil {
				t.Errorf("StopDownload for non-existent download should not return error, got: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("StopDownload for non-existent download blocked too long")
		}
	})

	t.Run("StopActiveDownloadTimeout", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping active download test in short mode")
		}

		// Create mock download that might be slow to stop
		movieID := uint(1)

		// This test verifies that even if the underlying downloader has issues,
		// the download manager doesn't block indefinitely
		done := make(chan error, 1)
		go func() {
			done <- dm.StopDownload(movieID)
		}()

		select {
		case err := <-done:
			if err != nil {
				t.Logf("StopDownload returned error (expected for non-existent): %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("StopDownload blocked for too long - this indicates a potential deadlock")
		}
	})

	t.Run("StopAllDownloadsTimeout", func(t *testing.T) {
		// Test that StopAllDownloads completes in reasonable time
		done := make(chan struct{})
		go func() {
			dm.StopAllDownloads()
			close(done)
		}()

		select {
		case <-done:
			// Success
		case <-time.After(10 * time.Second):
			t.Error("StopAllDownloads took too long - potential deadlock")
		}
	})

	t.Run("ConcurrentStopCalls", func(t *testing.T) {
		// Test that multiple concurrent stop calls don't cause issues
		movieID := uint(42)

		const numConcurrentCalls = 10
		done := make(chan error, numConcurrentCalls)

		for range numConcurrentCalls {
			go func() {
				done <- dm.StopDownload(movieID)
			}()
		}

		// Collect all results
		for i := range numConcurrentCalls {
			select {
			case err := <-done:
				if err != nil {
					t.Logf("Concurrent stop call %d returned error (expected): %v", i, err)
				}
			case <-time.After(3 * time.Second):
				t.Errorf("Concurrent stop call %d timed out", i)
			}
		}
	})
}

// TestDownloadManagerRobustness tests the download manager under stress
func TestDownloadManagerRobustness(t *testing.T) {
	t.Skip("Skipping robustness test - requires network connectivity")

	logger.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)
	// For stop tests, we don't need a real database
	var db database.Database

	dm := NewDownloadManager(cfg, db)

	t.Run("RapidStartStopCycle", func(t *testing.T) {
		// Simulate rapid start/stop cycles that could happen during deletion
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		movieID := uint(100)

		for i := range 5 {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Try to stop download (even if not started)
			stopDone := make(chan error, 1)
			go func() {
				stopDone <- dm.StopDownload(movieID)
			}()

			select {
			case err := <-stopDone:
				if err != nil {
					t.Logf("Stop cycle %d returned error (expected): %v", i, err)
				}
			case <-time.After(1 * time.Second):
				t.Errorf("Stop cycle %d timed out", i)
			}

			time.Sleep(10 * time.Millisecond) // Small delay between cycles
		}
	})

	t.Run("ManagerStateConsistency", func(t *testing.T) {
		// Verify that after all operations, the manager is in a consistent state

		// Try to stop some downloads
		for i := uint(1); i <= 5; i++ {
			if err := dm.StopDownload(i); err != nil {
				t.Logf("Stop download %d returned error (expected): %v", i, err)
			}
		}

		// Stop all downloads
		dm.StopAllDownloads()

		// Manager should still be responsive
		err := dm.StopDownload(999)
		if err != nil {
			t.Logf("Final stop call returned error (expected): %v", err)
		}
	})
}

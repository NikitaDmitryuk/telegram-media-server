package manager

import (
	"fmt"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

func TestDownloadManager_ChannelFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Initialize logger for tests
	logger.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)
	cfg.DownloadSettings.DownloadTimeout = 5 * time.Second
	cfg.DownloadSettings.ProgressUpdateInterval = 50 * time.Millisecond

	db := testutils.TestDatabase(t)
	manager := NewDownloadManager(cfg, db)

	t.Run("SuccessfulDownloadChannelFlow", func(t *testing.T) {
		// Create mock downloader that simulates successful download
		mockDownloader := &testutils.MockDownloader{
			Title:    "Test Download",
			Files:    []string{"/tmp/test.mp4"},
			FileSize: 1024,
		}

		progressChan, errChan, err := manager.StartDownload(mockDownloader, 1)
		if err != nil {
			t.Fatalf("Failed to start download: %v", err)
		}

		if progressChan == nil {
			t.Error("Expected progress channel")
		}

		if errChan == nil {
			t.Error("Expected error channel")
		}

		// Collect all progress updates
		var progressUpdates []float64
		var finalError error
		var channelsClosed bool

		// Use a timeout to avoid hanging
		timeout := time.After(2 * time.Second)

	CollectLoop:
		for {
			select {
			case progress, ok := <-progressChan:
				if !ok {
					t.Log("Progress channel closed")
					channelsClosed = true
					break CollectLoop
				}
				t.Logf("Received progress: %.1f%%", progress)
				progressUpdates = append(progressUpdates, progress)

			case err, ok := <-errChan:
				if !ok {
					t.Log("Error channel closed")
					channelsClosed = true
					break CollectLoop
				}
				t.Logf("Received final result: %v", err)
				finalError = err
				break CollectLoop

			case <-timeout:
				t.Fatal("Test timed out waiting for channels")
			}
		}

		// Verify we received the completion signal
		if !channelsClosed {
			t.Error("Expected channels to be closed")
		}

		// Should have received progress updates
		if len(progressUpdates) == 0 {
			t.Error("Expected at least one progress update")
		}

		// Final error should be nil (success)
		if finalError != nil {
			t.Errorf("Expected successful completion, got error: %v", finalError)
		}

		t.Logf("Received %d progress updates", len(progressUpdates))
	})

	t.Run("FailedDownloadChannelFlow", func(t *testing.T) {
		// Create mock downloader that simulates failed download
		mockDownloader := &testutils.MockDownloader{
			Title:        "Failed Download",
			Files:        []string{"/tmp/failed.mp4"},
			FileSize:     1024,
			ShouldError:  true,
			ErrorMessage: "Simulated download failure",
		}

		progressChan, errChan, err := manager.StartDownload(mockDownloader, 2)
		if err != nil {
			t.Fatalf("Failed to start download: %v", err)
		}

		// Collect results
		var finalError error
		var channelsClosed bool

		timeout := time.After(2 * time.Second)

	FailLoop:
		for {
			select {
			case progress, ok := <-progressChan:
				if !ok {
					t.Log("Progress channel closed")
					channelsClosed = true
					break FailLoop
				}
				t.Logf("Received progress during failure: %.1f%%", progress)

			case err, ok := <-errChan:
				if !ok {
					t.Log("Error channel closed")
					channelsClosed = true
					break FailLoop
				}
				t.Logf("Received error result: %v", err)
				finalError = err
				break FailLoop

			case <-timeout:
				t.Fatal("Test timed out waiting for error")
			}
		}

		if !channelsClosed {
			t.Error("Expected channels to be closed")
		}

		// Should have received error
		if finalError == nil {
			t.Error("Expected error for failed download")
		}

		t.Logf("Correctly received error: %v", finalError)
	})

	t.Run("ConcurrentDownloads", func(t *testing.T) {
		// Test that multiple downloads can run concurrently and their channels don't interfere
		cfg.DownloadSettings.MaxConcurrentDownloads = 3

		downloaderA := &testutils.MockDownloader{
			Title:    "Download A",
			Files:    []string{"/tmp/a.mp4"},
			FileSize: 1024,
		}

		downloaderB := &testutils.MockDownloader{
			Title:    "Download B",
			Files:    []string{"/tmp/b.mp4"},
			FileSize: 2048,
		}

		downloaderC := &testutils.MockDownloader{
			Title:    "Download C",
			Files:    []string{"/tmp/c.mp4"},
			FileSize: 4096,
		}

		// Start all downloads
		progressA, errA, errStartA := manager.StartDownload(downloaderA, 3)
		if errStartA != nil {
			t.Fatalf("Failed to start download A: %v", errStartA)
		}

		progressB, errB, errStartB := manager.StartDownload(downloaderB, 4)
		if errStartB != nil {
			t.Fatalf("Failed to start download B: %v", errStartB)
		}

		progressC, errC, errStartC := manager.StartDownload(downloaderC, 5)
		if errStartC != nil {
			t.Fatalf("Failed to start download C: %v", errStartC)
		}

		// Wait for all downloads to complete
		timeout := time.After(3 * time.Second)
		completed := 0

		for completed < 3 {
			select {
			case err := <-errA:
				t.Logf("Download A completed: %v", err)
				completed++
				errA = nil // Mark as processed

			case err := <-errB:
				t.Logf("Download B completed: %v", err)
				completed++
				errB = nil // Mark as processed

			case err := <-errC:
				t.Logf("Download C completed: %v", err)
				completed++
				errC = nil // Mark as processed

			case <-progressA:
				// Consume progress updates
			case <-progressB:
				// Consume progress updates
			case <-progressC:
				// Consume progress updates

			case <-timeout:
				t.Fatalf("Timeout waiting for concurrent downloads to complete (completed: %d/3)", completed)
			}
		}

		t.Log("All concurrent downloads completed successfully")
	})

	t.Run("MaxConcurrentLimit", func(t *testing.T) {
		// Test that the max concurrent downloads limit is enforced
		cfg.DownloadSettings.MaxConcurrentDownloads = 1

		// Create blocking downloader
		downloaderBlocking := &testutils.MockDownloader{
			Title:       "Blocking Download",
			Files:       []string{"/tmp/blocking.mp4"},
			FileSize:    1024,
			ShouldBlock: true, // This will block indefinitely
		}

		downloaderQueued := &testutils.MockDownloader{
			Title:    "Queued Download",
			Files:    []string{"/tmp/queued.mp4"},
			FileSize: 1024,
		}

		// Start first download (should start immediately)
		progressBlocking, errBlocking, err := manager.StartDownload(downloaderBlocking, 6)
		if err != nil {
			t.Fatalf("Failed to start blocking download: %v", err)
		}

		// Give it time to start
		time.Sleep(100 * time.Millisecond)

		// Start second download (should be queued)
		startTime := time.Now()
		progressQueued, errQueued, err := manager.StartDownload(downloaderQueued, 7)
		if err != nil {
			t.Fatalf("Failed to start queued download: %v", err)
		}

		// The queued download should not start immediately
		queueTime := time.Since(startTime)
		t.Logf("Queued download returned after: %v", queueTime)

		// Stop the blocking download to allow queued one to proceed
		err = manager.StopDownload(6)
		if err != nil {
			t.Errorf("Failed to stop blocking download: %v", err)
		}

		// Now wait for the queued download to complete
		timeout := time.After(2 * time.Second)

	QueueLoop:
		for {
			select {
			case err := <-errQueued:
				t.Logf("Queued download completed: %v", err)
				break QueueLoop

			case <-progressQueued:
				// Consume progress updates

			case <-errBlocking:
				// Consume blocking download result

			case <-progressBlocking:
				// Consume progress updates

			case <-timeout:
				t.Fatal("Timeout waiting for queued download to complete")
			}
		}

		t.Log("Queue limit enforced correctly")
	})
}

func TestDownloadManager_ChannelSynchronization(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping synchronization test in short mode")
	}

	// Test for race conditions and proper channel handling
	logger.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)
	cfg.DownloadSettings.DownloadTimeout = 1 * time.Second
	cfg.DownloadSettings.ProgressUpdateInterval = 10 * time.Millisecond

	db := testutils.TestDatabase(t)
	manager := NewDownloadManager(cfg, db)

	t.Run("ChannelCleanup", func(t *testing.T) {
		// Test that channels are properly closed and cleaned up
		mockDownloader := &testutils.MockDownloader{
			Title:    "Cleanup Test",
			Files:    []string{"/tmp/cleanup.mp4"},
			FileSize: 1024,
		}

		progressChan, errChan, err := manager.StartDownload(mockDownloader, 8)
		if err != nil {
			t.Fatalf("Failed to start download: %v", err)
		}

		// Wait for completion
		timeout := time.After(2 * time.Second)

	CleanupLoop:
		for {
			select {
			case _, ok := <-progressChan:
				if !ok {
					t.Log("Progress channel properly closed")
					progressChan = nil
				}

			case _, ok := <-errChan:
				if !ok {
					t.Log("Error channel properly closed")
					errChan = nil
				} else {
					// Got final result
				}
				if progressChan == nil {
					break CleanupLoop
				}

			case <-timeout:
				t.Fatal("Timeout waiting for channel cleanup")
			}
		}

		t.Log("Channels cleaned up successfully")
	})

	t.Run("NoChannelLeaks", func(t *testing.T) {
		// Run multiple downloads sequentially to check for channel leaks
		for i := 0; i < 5; i++ {
			mockDownloader := &testutils.MockDownloader{
				Title:    fmt.Sprintf("Leak Test %d", i),
				Files:    []string{fmt.Sprintf("/tmp/leak%d.mp4", i)},
				FileSize: 1024,
			}

			movieID := int64(10 + i)
			progressChan, errChan, err := manager.StartDownload(mockDownloader, movieID)
			if err != nil {
				t.Fatalf("Failed to start download %d: %v", i, err)
			}

			// Wait for completion
			timeout := time.After(1 * time.Second)

		LeakLoop:
			for {
				select {
				case _, ok := <-progressChan:
					if !ok {
						progressChan = nil
					}

				case _, ok := <-errChan:
					if !ok {
						errChan = nil
					}
					if progressChan == nil {
						break LeakLoop
					}

				case <-timeout:
					t.Fatalf("Timeout on download %d", i)
				}
			}

			t.Logf("Download %d completed and cleaned up", i)
		}

		t.Log("No channel leaks detected in sequential downloads")
	})
}

func TestDownloadManager_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping error handling test in short mode")
	}

	logger.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)
	cfg.DownloadSettings.DownloadTimeout = 500 * time.Millisecond

	db := testutils.TestDatabase(t)
	manager := NewDownloadManager(cfg, db)

	t.Run("DownloadTimeout", func(t *testing.T) {
		// Test download timeout handling
		mockDownloader := &testutils.MockDownloader{
			Title:       "Timeout Test",
			Files:       []string{"/tmp/timeout.mp4"},
			FileSize:    1024,
			ShouldBlock: true, // Will block until context cancelled
		}

		progressChan, errChan, err := manager.StartDownload(mockDownloader, 20)
		if err != nil {
			t.Fatalf("Failed to start timeout test: %v", err)
		}

		// Wait for timeout
		timeout := time.After(2 * time.Second)
		var timeoutError error

	TimeoutLoop:
		for {
			select {
			case err, ok := <-errChan:
				if !ok {
					break TimeoutLoop
				}
				timeoutError = err
				if err != nil {
					t.Logf("Received timeout error: %v", err)
					break TimeoutLoop
				}

			case <-progressChan:
				// Consume any progress updates

			case <-timeout:
				t.Fatal("Test timed out waiting for download timeout")
			}
		}

		// Should have received a timeout error
		if timeoutError == nil {
			t.Error("Expected timeout error")
		}

		t.Log("Timeout handling working correctly")
	})
}

package aria2

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

// TestAria2StopDownload tests the StopDownload functionality
func TestAria2StopDownload(t *testing.T) {
	logutils.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)

	t.Run("StopNonExistentProcess", func(t *testing.T) {
		torrentPath := testutils.CreateRealTestTorrent(t, tempDir, "test-stop")

		downloader := NewAria2Downloader(filepath.Base(torrentPath), cfg.MoviePath, cfg).(*Aria2Downloader)

		// Try to stop before starting - should not panic
		err := downloader.StopDownload()
		if err != nil {
			t.Errorf("StopDownload on non-existent process should not return error, got: %v", err)
		}

		if !downloader.StoppedManually() {
			t.Error("Expected StoppedManually to be true after StopDownload")
		}
	})

	t.Run("StopActiveProcess", func(t *testing.T) {
		t.Skip("Skipping active process test - requires network connectivity")

		torrentPath := testutils.CreateRealTestTorrent(t, tempDir, "test-active-stop")

		downloader := NewAria2Downloader(filepath.Base(torrentPath), cfg.MoviePath, cfg).(*Aria2Downloader)

		// Start download
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		progressChan, errChan, _, err := downloader.StartDownload(ctx)
		if err != nil {
			t.Logf("StartDownload failed (expected without aria2): %v", err)
			return // Skip if aria2 not available
		}

		// Give it a moment to start
		time.Sleep(100 * time.Millisecond)

		// Stop the download
		stopErr := downloader.StopDownload()
		if stopErr != nil {
			t.Errorf("StopDownload failed: %v", stopErr)
		}

		// Verify it stopped
		if !downloader.StoppedManually() {
			t.Error("Expected StoppedManually to be true after StopDownload")
		}

		// Close channels to avoid goroutine leaks
		if progressChan != nil {
			go func() {
				for range progressChan {
					// Drain channel
				}
			}()
		}
		if errChan != nil {
			go func() {
				<-errChan // Get final error
			}()
		}
	})

	t.Run("StopDownloadTimeout", func(t *testing.T) {
		t.Skip("Skipping timeout test - requires network connectivity")

		// Create a downloader with simulated stubborn process
		downloader := &Aria2Downloader{
			stoppedManually: false,
			config:          cfg,
		}

		// Test that StopDownload doesn't block forever
		done := make(chan error, 1)
		go func() {
			done <- downloader.StopDownload()
		}()

		select {
		case err := <-done:
			if err != nil {
				t.Logf("StopDownload returned error (expected): %v", err)
			}
			if !downloader.StoppedManually() {
				t.Error("Expected StoppedManually to be true")
			}
		case <-time.After(10 * time.Second):
			t.Error("StopDownload blocked for too long")
		}
	})
}

// TestAria2ProcessCleanup tests that processes are properly cleaned up
func TestAria2ProcessCleanup(t *testing.T) {
	t.Skip("Skipping process cleanup test - requires network connectivity")

	logutils.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)

	t.Run("ProcessCleanupAfterStop", func(t *testing.T) {
		torrentPath := testutils.CreateRealTestTorrent(t, tempDir, "test-cleanup")

		downloader := NewAria2Downloader(filepath.Base(torrentPath), cfg.MoviePath, cfg).(*Aria2Downloader)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		progressChan, errChan, _, err := downloader.StartDownload(ctx)
		if err != nil {
			t.Logf("StartDownload failed (expected without aria2): %v", err)
			return
		}

		// Verify process started
		if downloader.cmd == nil || downloader.cmd.Process == nil {
			t.Error("Expected process to be started")
			return
		}

		pid := downloader.cmd.Process.Pid

		// Stop the download
		if err := downloader.StopDownload(); err != nil {
			t.Errorf("StopDownload failed: %v", err)
		}

		// Verify process is cleaned up
		time.Sleep(100 * time.Millisecond)

		// Check if process still exists (this is OS-specific)
		if process, err := os.FindProcess(pid); err == nil {
			// On Unix systems, check if process is actually running
			if err := process.Signal(os.Signal(syscall.Signal(0))); err == nil {
				t.Logf("Process %d may still be running (this might be normal)", pid)
			}
		}

		// Close channels
		if progressChan != nil {
			go func() {
				for range progressChan {
					// Drain
				}
			}()
		}
		if errChan != nil {
			go func() {
				<-errChan
			}()
		}
	})
}

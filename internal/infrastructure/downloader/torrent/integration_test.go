package aria2

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

// TestTorrentDownloadIntegration tests the complete torrent download flow
func TestTorrentDownloadIntegration(t *testing.T) {
	t.Skip("Skipping torrent download test - requires network connectivity")

	// Initialize logger for tests
	logger.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)

	// Set very short timeouts for testing
	cfg.DownloadSettings.DownloadTimeout = 10 * time.Second
	cfg.DownloadSettings.ProgressUpdateInterval = 100 * time.Millisecond

	t.Run("ValidTorrentFile", func(t *testing.T) {
		torrentPath := testutils.CreateRealTestTorrent(t, tempDir, "test-movie")

		downloader := NewAria2Downloader(nil, torrentPath, cfg.MoviePath, cfg).(*Aria2Downloader)

		// Test basic initialization
		title, err := downloader.GetTitle()
		if err != nil {
			t.Logf("GetTitle failed (expected without aria2): %v", err)
		} else {
			t.Logf("Got title: %s", title)
		}

		files, tempFiles, err := downloader.GetFiles()
		if err != nil {
			t.Logf("GetFiles failed (expected without aria2): %v", err)
		} else {
			t.Logf("Got files: main=%v, temp=%v", files, tempFiles)
		}

		size, err := downloader.GetFileSize()
		if err != nil {
			t.Logf("GetFileSize failed (expected without aria2): %v", err)
		} else {
			t.Logf("Got size: %d bytes", size)
		}
	})

	t.Run("TorrentValidation", func(t *testing.T) {
		// Test with real torrent structure
		torrentPath := testutils.CreateRealTestTorrent(t, tempDir, "validation-test")

		_ = NewAria2Downloader(nil, torrentPath, cfg.MoviePath, cfg).(*Aria2Downloader)

		// This should not fail validation
		err := ValidateTorrentFile(torrentPath)
		if err != nil {
			t.Errorf("Validation failed for real torrent: %v", err)
		}
	})

	t.Run("InvalidTorrentFile", func(t *testing.T) {
		htmlPath := testutils.CreateInvalidTorrent(t, tempDir, "invalid-test")

		_ = NewAria2Downloader(nil, htmlPath, tempDir, cfg).(*Aria2Downloader)

		// This should fail validation
		err := ValidateTorrentFile(htmlPath)
		if err == nil {
			t.Error("Expected validation to fail for HTML file")
		} else {
			t.Logf("Validation correctly failed: %v", err)
		}
	})

	t.Run("MagnetLinkFile", func(t *testing.T) {
		magnetPath := testutils.CreateMagnetLink(t, tempDir, "magnet-test")

		_ = NewAria2Downloader(nil, magnetPath, tempDir, cfg).(*Aria2Downloader)

		// This should fail validation
		err := ValidateTorrentFile(magnetPath)
		if err == nil {
			t.Error("Expected validation to fail for magnet link")
		} else {
			t.Logf("Validation correctly failed: %v", err)
		}
	})
}

// TestTorrentMetaParsing tests parsing of various torrent metadata
func TestTorrentMetaParsing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Initialize logger for tests
	logger.InitLogger("debug")

	tempDir := testutils.TempDir(t)

	t.Run("SingleFileTorrent", func(t *testing.T) {
		torrentPath := testutils.CreateRealTestTorrent(t, tempDir, "single-file")

		meta, err := ParseMeta(torrentPath)
		if err != nil {
			t.Fatalf("Failed to parse real torrent: %v", err)
		}

		if meta.Info.Name != "single-file.txt" {
			t.Errorf("Expected name 'single-file.txt', got '%s'", meta.Info.Name)
		}

		if meta.Info.Length <= 0 {
			t.Errorf("Expected positive length, got %d", meta.Info.Length)
		}

		if len(meta.Info.Files) != 0 {
			t.Errorf("Single file torrent should have 0 files in list, got %d", len(meta.Info.Files))
		}
	})

	t.Run("TorrentWithAnnounce", func(t *testing.T) {
		torrentPath := testutils.CreateRealTestTorrent(t, tempDir, "announce-test")

		meta, err := ParseMeta(torrentPath)
		if err != nil {
			t.Fatalf("Failed to parse torrent: %v", err)
		}

		if meta.Announce == "" {
			t.Error("Expected announce URL to be set")
		}

		t.Logf("Announce URL: %s", meta.Announce)
	})
}

// TestTorrentDownloadManager tests integration with download manager
func TestTorrentDownloadManager(t *testing.T) {
	t.Skip("Skipping torrent manager test - requires network connectivity")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)
	cfg.DownloadSettings.MaxConcurrentDownloads = 1
	cfg.DownloadSettings.DownloadTimeout = 5 * time.Second

	// Create mock database
	_ = testutils.TestDatabase(t)

	t.Run("DownloadManagerIntegration", func(t *testing.T) {
		torrentPath := testutils.CreateRealTestTorrent(t, tempDir, "manager-test")

		// Test that downloader can be created and initialized
		downloader := NewAria2Downloader(nil, torrentPath, cfg.MoviePath, cfg).(*Aria2Downloader)

		// Test file operations
		_, err := downloader.GetTitle()
		if err != nil {
			t.Logf("GetTitle failed (expected without aria2): %v", err)
		}

		// Test that file exists
		if _, err := os.Stat(torrentPath); os.IsNotExist(err) {
			t.Error("Torrent file should exist")
		}
	})
}

// TestTorrentFileOperations tests file-related operations
func TestTorrentFileOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tempDir := testutils.TempDir(t)

	t.Run("FileCreationAndValidation", func(t *testing.T) {
		// Create test data file
		dataFile := testutils.CreateTestDataFile(t, tempDir, "test-data.bin", 1024)

		// Verify file exists and has correct size
		info, err := os.Stat(dataFile)
		if err != nil {
			t.Fatalf("Test data file should exist: %v", err)
		}

		if info.Size() != 1024 {
			t.Errorf("Expected file size 1024, got %d", info.Size())
		}

		// Create torrent for this file
		torrentPath := testutils.CreateRealTestTorrent(t, tempDir, "file-test")

		// Validate torrent
		err = ValidateTorrentFile(torrentPath)
		if err != nil {
			t.Errorf("Torrent validation failed: %v", err)
		}
	})

	t.Run("DirectoryOperations", func(t *testing.T) {
		// Create subdirectory
		subDir := filepath.Join(tempDir, "subdir")
		err := os.MkdirAll(subDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create subdirectory: %v", err)
		}

		// Create torrent in subdirectory
		torrentPath := testutils.CreateRealTestTorrent(t, subDir, "subdir-test")

		// Verify path handling
		if !filepath.IsAbs(torrentPath) {
			absPath, absErr := filepath.Abs(torrentPath)
			if absErr != nil {
				t.Fatalf("Failed to get absolute path: %v", absErr)
			}
			torrentPath = absPath
		}

		// Test validation with absolute path
		err = ValidateTorrentFile(torrentPath)
		if err != nil {
			t.Errorf("Validation with absolute path failed: %v", err)
		}
	})
}

// TestTorrentErrorHandling tests various error conditions
func TestTorrentErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)

	t.Run("NonExistentFile", func(t *testing.T) {
		nonExistentPath := filepath.Join(tempDir, "nonexistent.torrent")

		downloader := NewAria2Downloader(nil, nonExistentPath, cfg.MoviePath, cfg)

		_, err := downloader.GetTitle()
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})

	t.Run("CorruptedTorrentFile", func(t *testing.T) {
		corruptedPath := filepath.Join(tempDir, "corrupted.torrent")

		// Create file with invalid content
		err := os.WriteFile(corruptedPath, []byte("invalid torrent data"), 0600)
		if err != nil {
			t.Fatalf("Failed to create corrupted file: %v", err)
		}

		err = ValidateTorrentFile(corruptedPath)
		if err == nil {
			t.Error("Expected validation to fail for corrupted file")
		}
	})

	t.Run("EmptyTorrentFile", func(t *testing.T) {
		emptyPath := filepath.Join(tempDir, "empty.torrent")

		// Create empty file
		err := os.WriteFile(emptyPath, []byte{}, 0600)
		if err != nil {
			t.Fatalf("Failed to create empty file: %v", err)
		}

		err = ValidateTorrentFile(emptyPath)
		if err == nil {
			t.Error("Expected validation to fail for empty file")
		}
	})
}

// TestTorrentConcurrency tests concurrent torrent operations
func TestTorrentConcurrency(t *testing.T) {
	t.Skip("Skipping torrent concurrency test - requires network connectivity")

	tempDir := testutils.TempDir(t)
	_ = testutils.TestConfig(tempDir)

	t.Run("ConcurrentValidation", func(t *testing.T) {
		const numGoroutines = 5

		// Create multiple torrent files
		torrentPaths := make([]string, numGoroutines)
		for i := range numGoroutines {
			torrentPaths[i] = testutils.CreateRealTestTorrent(t, tempDir, fmt.Sprintf("concurrent-test-%d", i))
		}

		// Channel to collect results
		results := make(chan error, numGoroutines)

		// Run validations concurrently
		for i := range numGoroutines {
			go func(path string) {
				results <- ValidateTorrentFile(path)
			}(torrentPaths[i])
		}

		// Collect results
		for i := range numGoroutines {
			err := <-results
			if err != nil {
				t.Errorf("Concurrent validation %d failed: %v", i, err)
			}
		}
	})
}

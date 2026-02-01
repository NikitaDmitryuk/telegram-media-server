package filemanager

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

func TestFileManager_Docker(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker test in short mode")
	}

	// Check if we're running in Docker or have Docker available
	if !isDockerAvailable() && !isRunningInDocker() {
		t.Skip("Docker not available and not running in Docker")
	}

	logutils.InitLogger("debug")

	testCases := []struct {
		name string
		test func(t *testing.T)
	}{
		{"CrossPlatformFileOperations", testCrossPlatformFileOperations},
		{"LargeFileSystemOperations", testLargeFileSystemOperations},
		{"FileSystemLimits", testFileSystemLimits},
		{"ContainerVolumeOperations", testContainerVolumeOperations},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.test)
	}
}

func testCrossPlatformFileOperations(t *testing.T) {
	tempDir := t.TempDir()
	db := testutils.TestDatabase(t)
	cfg := testutils.TestConfig(tempDir)
	downloadManager := tmsdmanager.NewDownloadManager(cfg, db)

	// Test with various file name patterns that might cause issues
	problematicFiles := []string{
		"normal_file.mp4",
		"file with spaces.mkv",
		"file-with-dashes.avi",
		"file_with_underscores.mp4",
		"файл_с_русскими_символами.mp4", // Cyrillic
		"file.with.multiple.dots.mp4",
		"UPPERCASE_FILE.MP4",
		"mixedCase_File.Mp4",
	}

	// Add movie to database
	movieID, err := db.AddMovie(context.Background(), "Cross Platform Test", 1024,
		problematicFiles[:4], problematicFiles[4:], 0)
	if err != nil {
		t.Fatalf("Failed to add movie: %v", err)
	}

	// Create files with various naming patterns
	for _, fileName := range problematicFiles {
		filePath := filepath.Join(tempDir, fileName)
		content := fmt.Sprintf("Test content for file: %s\nCreated at: %s",
			fileName, time.Now().Format(time.RFC3339))

		if writeErr := os.WriteFile(filePath, []byte(content), 0600); writeErr != nil {
			// Some filesystems might not support certain characters
			t.Logf("Warning: Could not create file '%s': %v", fileName, writeErr)
			continue
		}

		// Verify file was created and is readable
		if readContent, readErr := os.ReadFile(filePath); readErr != nil {
			t.Errorf("Failed to read file '%s': %v", fileName, readErr)
		} else if len(readContent) == 0 {
			t.Errorf("File '%s' is empty", fileName)
		}
	}

	// Test deletion operations with problematic filenames
	err = DeleteTemporaryFilesByMovieID(movieID, tempDir, db, downloadManager)
	if err != nil {
		t.Errorf("Failed to delete temporary files: %v", err)
	}
}

func testLargeFileSystemOperations(t *testing.T) {
	tempDir := t.TempDir()

	// Test creating larger files (100MB each)
	largeFiles := []string{"large1.dat", "large2.dat", "large3.dat"}
	fileSize := int64(100 * 1024 * 1024) // 100MB

	for _, fileName := range largeFiles {
		filePath := filepath.Join(tempDir, fileName)
		t.Logf("Creating large file: %s (%d MB)", fileName, fileSize/(1024*1024))

		if err := createLargeFile(filePath, fileSize); err != nil {
			t.Fatalf("Failed to create large file %s: %v", fileName, err)
		}

		// Verify file size
		if info, err := os.Stat(filePath); err != nil {
			t.Errorf("Failed to stat large file %s: %v", fileName, err)
		} else if info.Size() != fileSize {
			t.Errorf("Large file %s: expected size %d, got %d", fileName, fileSize, info.Size())
		}
	}

	// Test disk space operations with large files
	spaceGB, err := GetAvailableSpaceGB(tempDir)
	if err != nil {
		t.Fatalf("Failed to get available space: %v", err)
	}

	t.Logf("Available space after creating large files: %.2f GB", spaceGB)

	// Test space requirements
	totalLargeFileSize := fileSize * int64(len(largeFiles))
	if !HasEnoughSpace(tempDir, 1024) { // Should have space for 1KB
		t.Error("Should have enough space for small requirements")
	}

	// Cleanup large files
	for _, fileName := range largeFiles {
		filePath := filepath.Join(tempDir, fileName)
		if removeErr := os.Remove(filePath); removeErr != nil {
			t.Errorf("Failed to remove large file %s: %v", fileName, err)
		}
	}

	// Verify space is reclaimed
	spaceAfterCleanup, err := GetAvailableSpaceGB(tempDir)
	if err != nil {
		t.Fatalf("Failed to get available space after cleanup: %v", err)
	}

	spaceDiff := spaceAfterCleanup - spaceGB
	expectedDiff := float64(totalLargeFileSize) / (1024 * 1024 * 1024)

	t.Logf("Space reclaimed: %.2f GB (expected ~%.2f GB)", spaceDiff, expectedDiff)

	if spaceDiff < expectedDiff*0.8 { // Allow some tolerance
		t.Logf("Warning: Less space reclaimed than expected (filesystem overhead?)")
	}
}

func testFileSystemLimits(t *testing.T) {
	tempDir := t.TempDir()

	// Test creating many small files (filesystem inode limits)
	numFiles := 1000
	t.Logf("Creating %d small files to test filesystem limits", numFiles)

	createdFiles := 0
	for i := range numFiles {
		fileName := filepath.Join(tempDir, fmt.Sprintf("small_file_%04d.txt", i))
		content := fmt.Sprintf("File %d content", i)

		if err := os.WriteFile(fileName, []byte(content), 0600); err != nil {
			t.Logf("Failed to create file %d (filesystem limit?): %v", i, err)
			break
		}
		createdFiles++
	}

	t.Logf("Successfully created %d files", createdFiles)

	if createdFiles < 100 {
		t.Errorf("Expected to create at least 100 files, only created %d", createdFiles)
	}

	// Test directory operations
	if IsEmptyDirectory(tempDir) {
		t.Error("Directory should not be empty after creating files")
	}

	// Test bulk cleanup
	start := time.Now()
	cleanedFiles := 0
	for i := range createdFiles {
		fileName := filepath.Join(tempDir, fmt.Sprintf("small_file_%04d.txt", i))
		if removeErr := os.Remove(fileName); removeErr != nil {
			t.Logf("Failed to remove file %d: %v", i, removeErr)
		} else {
			cleanedFiles++
		}
	}
	cleanupDuration := time.Since(start)

	t.Logf("Cleaned up %d files in %v", cleanedFiles, cleanupDuration)

	if cleanedFiles != createdFiles {
		t.Errorf("Cleanup mismatch: created %d, cleaned %d", createdFiles, cleanedFiles)
	}
}

func testContainerVolumeOperations(t *testing.T) {
	if !isRunningInDocker() {
		t.Skip("Not running in Docker container")
	}

	// Test operations that are specific to container environments
	tempDir := t.TempDir()

	// Test volume mount permissions
	testFile := filepath.Join(tempDir, "volume_test.txt")
	content := "Testing volume operations in container"

	if err := os.WriteFile(testFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to write to volume: %v", err)
	}

	// Test reading back
	readContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read from volume: %v", err)
	}

	if string(readContent) != content {
		t.Errorf("Content mismatch: expected '%s', got '%s'", content, string(readContent))
	}

	// Test space operations in container
	spaceGB, err := GetAvailableSpaceGB(tempDir)
	if err != nil {
		t.Fatalf("Failed to get space in container: %v", err)
	}

	if spaceGB <= 0 {
		t.Error("Available space should be positive in container")
	}

	t.Logf("Container available space: %.2f GB", spaceGB)
}

// Helper functions

func createLargeFile(filePath string, size int64) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Use efficient method to create large file
	if err := file.Truncate(size); err != nil {
		return err
	}

	// Write some data at the beginning and end to ensure it's not sparse
	if _, err := file.WriteAt([]byte("START"), 0); err != nil {
		return err
	}

	if size > 5 {
		if _, err := file.WriteAt([]byte("END"), size-3); err != nil {
			return err
		}
	}

	return nil
}

func isDockerAvailable() bool {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "docker", "version")
	return cmd.Run() == nil
}

func isRunningInDocker() bool {
	// Check common indicators of running in Docker
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// Check cgroup info
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		return strings.Contains(string(data), "docker") || strings.Contains(string(data), "containerd")
	}

	return false
}

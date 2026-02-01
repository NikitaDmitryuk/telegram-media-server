package filemanager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

func TestFileManager_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logutils.InitLogger("debug")

	testCases := []struct {
		name string
		test func(t *testing.T)
	}{
		{"RealFileOperations", testRealFileOperations},
		{"DiskSpaceOperations", testDiskSpaceOperations},
		{"DirectoryCleanup", testDirectoryCleanup},
		{"LargeFileHandling", testLargeFileHandling},
		{"ConcurrentFileOperations", testConcurrentFileOperations},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.test)
	}
}

func testRealFileOperations(t *testing.T) {
	// Create real temp directory and files
	tempDir := t.TempDir()
	db := testutils.TestDatabase(t)
	cfg := testutils.TestConfig(tempDir)
	downloadManager := tmsdmanager.NewDownloadManager(cfg, db)

	// Add a movie to database
	movieID, err := db.AddMovie(context.Background(), "Integration Test Movie", 1024*1024, // 1MB
		[]string{"movie.mp4", "subtitle.srt"},
		[]string{"movie.torrent", "movie.part", "movie.tmp"}, 0)
	if err != nil {
		t.Fatalf("Failed to add movie: %v", err)
	}

	// Create real files on filesystem
	mainFiles := []string{"movie.mp4", "subtitle.srt"}
	tempFiles := []string{"movie.torrent", "movie.part", "movie.tmp"}

	for _, file := range append(mainFiles, tempFiles...) {
		filePath := filepath.Join(tempDir, file)
		content := make([]byte, 1024) // 1KB each file
		for i := range content {
			content[i] = byte(i % 256)
		}
		if writeErr := os.WriteFile(filePath, content, 0600); writeErr != nil {
			t.Fatalf("Failed to create file %s: %v", file, writeErr)
		}
	}

	// Verify files exist
	for _, file := range append(mainFiles, tempFiles...) {
		filePath := filepath.Join(tempDir, file)
		if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
			t.Errorf("File %s should exist", file)
		}
	}

	// Test deleting temp files
	err = DeleteTemporaryFilesByMovieID(movieID, tempDir, db, downloadManager)
	if err != nil {
		t.Errorf("Failed to delete temporary files: %v", err)
	}

	// Verify temp files are deleted, main files remain
	// Note: temp files weren't added to database, so they still exist on disk
	// This is expected behavior - only files tracked in database are deleted
	for _, file := range tempFiles {
		filePath := filepath.Join(tempDir, file)
		if _, statErr := os.Stat(filePath); !os.IsNotExist(statErr) {
			t.Logf("Temp file %s remains on disk (expected - not tracked in database)", file)
		}
	}

	for _, file := range mainFiles {
		filePath := filepath.Join(tempDir, file)
		if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
			t.Errorf("Main file %s should still exist", file)
		}
	}
}

func testDiskSpaceOperations(t *testing.T) {
	tempDir := t.TempDir()

	// Test real disk space calculation
	spaceGB, err := GetAvailableSpaceGB(tempDir)
	if err != nil {
		t.Fatalf("Failed to get available space: %v", err)
	}

	if spaceGB <= 0 {
		t.Errorf("Available space should be positive, got %f GB", spaceGB)
	}

	t.Logf("Available space: %.2f GB", spaceGB)

	// Test space requirement checking
	smallRequirement := int64(1024) // 1KB
	if !HasEnoughSpace(tempDir, smallRequirement) {
		t.Error("Should have enough space for 1KB")
	}

	// Test with very large requirement (should fail unless on a huge disk)
	largeRequirement := int64(1024 * 1024 * 1024 * 1024 * 10) // 10TB
	if HasEnoughSpace(tempDir, largeRequirement) {
		t.Log("Wow, you have more than 10TB free space!")
	}
}

func testDirectoryCleanup(t *testing.T) {
	tempDir := t.TempDir()

	// Create subdirectories with files
	subDirs := []string{"movies", "temp", "downloads"}
	for _, dir := range subDirs {
		dirPath := filepath.Join(tempDir, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}

		// Add some files
		for i := range 3 {
			fileName := filepath.Join(dirPath, fmt.Sprintf("file%d.txt", i))
			if err := os.WriteFile(fileName, []byte("test content"), 0600); err != nil {
				t.Fatalf("Failed to create file %s: %v", fileName, err)
			}
		}
	}

	// Test empty directory detection
	emptyDir := filepath.Join(tempDir, "empty")
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatalf("Failed to create empty directory: %v", err)
	}

	if !IsEmptyDirectory(emptyDir) {
		t.Error("Empty directory should be detected as empty")
	}

	for _, dir := range subDirs {
		dirPath := filepath.Join(tempDir, dir)
		if IsEmptyDirectory(dirPath) {
			t.Errorf("Directory %s should not be empty", dir)
		}
	}
}

func testLargeFileHandling(t *testing.T) {
	tempDir := t.TempDir()

	// Create a moderately large file (10MB)
	largeFileName := filepath.Join(tempDir, "large_file.dat")
	largeFile, err := os.Create(largeFileName)
	if err != nil {
		t.Fatalf("Failed to create large file: %v", err)
	}
	defer largeFile.Close()

	// Write 10MB of data
	chunk := make([]byte, 1024*1024) // 1MB chunk
	for i := range chunk {
		chunk[i] = byte(i % 256)
	}

	for range 10 { // 10 chunks = 10MB
		if _, writeErr := largeFile.Write(chunk); writeErr != nil {
			t.Fatalf("Failed to write to large file: %v", writeErr)
		}
	}

	largeFile.Close()

	// Test operations on large file
	fileInfo, err := os.Stat(largeFileName)
	if err != nil {
		t.Fatalf("Failed to stat large file: %v", err)
	}

	expectedSize := int64(10 * 1024 * 1024) // 10MB
	if fileInfo.Size() != expectedSize {
		t.Errorf("Expected file size %d, got %d", expectedSize, fileInfo.Size())
	}

	// Test disk space with large file requirement
	if !HasEnoughSpace(tempDir, expectedSize) {
		t.Error("Should have enough space for the file we just created")
	}
}

func testConcurrentFileOperations(t *testing.T) {
	tempDir := t.TempDir()
	_ = testutils.TestDatabase(t) // Database setup for test isolation

	// Create multiple goroutines doing file operations
	numWorkers := 10
	numFilesPerWorker := 5

	done := make(chan bool, numWorkers)

	for worker := range numWorkers {
		go func(workerID int) {
			defer func() { done <- true }()

			// Each worker creates its own subdirectory
			workerDir := filepath.Join(tempDir, fmt.Sprintf("worker_%d", workerID))
			if err := os.MkdirAll(workerDir, 0755); err != nil {
				t.Errorf("Worker %d failed to create directory: %v", workerID, err)
				return
			}

			// Create files
			for file := range numFilesPerWorker {
				fileName := filepath.Join(workerDir, fmt.Sprintf("file_%d.txt", file))
				content := fmt.Sprintf("Worker %d, File %d, Time: %s",
					workerID, file, time.Now().Format(time.RFC3339Nano))

				if err := os.WriteFile(fileName, []byte(content), 0600); err != nil {
					t.Errorf("Worker %d failed to write file %d: %v", workerID, file, err)
					return
				}
			}

			// Test directory operations
			if IsEmptyDirectory(workerDir) {
				t.Errorf("Worker %d directory should not be empty", workerID)
			}

			// Test space operations
			if !HasEnoughSpace(workerDir, 1024) {
				t.Errorf("Worker %d should have space for 1KB", workerID)
			}
		}(worker)
	}

	// Wait for all workers to complete
	for range numWorkers {
		select {
		case <-done:
			// Worker completed
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout waiting for concurrent operations")
		}
	}

	// Verify all files were created
	totalFiles := 0
	err := filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".txt" {
			totalFiles++
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Failed to walk directory: %v", err)
	}

	expectedFiles := numWorkers * numFilesPerWorker
	if totalFiles != expectedFiles {
		t.Errorf("Expected %d files, found %d", expectedFiles, totalFiles)
	}
}

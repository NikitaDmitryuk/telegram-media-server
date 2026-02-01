package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

func TestHasEnoughSpace(t *testing.T) {
	logutils.InitLogger("debug")

	tests := []struct {
		name          string
		path          string
		requiredSpace int64
		expectResult  bool
		expectValid   bool // whether the path is valid for testing
	}{
		{
			name:          "Valid path with small space requirement",
			path:          "/tmp",
			requiredSpace: 1024, // 1KB
			expectResult:  true,
			expectValid:   true,
		},
		{
			name:          "Valid path with zero space requirement",
			path:          "/tmp",
			requiredSpace: 0,
			expectResult:  true,
			expectValid:   true,
		},
		{
			name:          "Negative space requirement",
			path:          "/tmp",
			requiredSpace: -1,
			expectResult:  false,
			expectValid:   true,
		},
		{
			name:          "Invalid path",
			path:          "/nonexistent/path/that/does/not/exist",
			requiredSpace: 1024,
			expectResult:  false,
			expectValid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasEnoughSpace(tt.path, tt.requiredSpace)

			if !tt.expectValid {
				// For invalid paths, we expect false
				if result != false {
					t.Errorf("Expected false for invalid path, got %v", result)
				}
				return
			}

			if tt.requiredSpace < 0 {
				// Negative space should always return false
				if result != false {
					t.Errorf("Expected false for negative space requirement, got %v", result)
				}
				return
			}

			// For valid paths and non-negative space, result depends on actual disk space
			// We can't predict the exact result, but it should not panic
		})
	}
}

func TestIsEmptyDirectory(t *testing.T) {
	logutils.InitLogger("debug")

	// Create temporary directory for testing
	tempDir := t.TempDir()

	// Test empty directory
	t.Run("Empty directory", func(t *testing.T) {
		if !IsEmptyDirectory(tempDir) {
			t.Error("Expected empty directory to return true")
		}
	})

	// Test directory with files
	t.Run("Directory with files", func(t *testing.T) {
		testFile := filepath.Join(tempDir, "test.txt")
		err := os.WriteFile(testFile, []byte("test"), 0600)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		if IsEmptyDirectory(tempDir) {
			t.Error("Expected non-empty directory to return false")
		}
	})

	// Test non-existent directory
	t.Run("Non-existent directory", func(t *testing.T) {
		nonExistentDir := filepath.Join(tempDir, "nonexistent")
		if IsEmptyDirectory(nonExistentDir) {
			t.Error("Expected non-existent directory to return false")
		}
	})
}

func TestDeleteTemporaryFilesByMovieID(t *testing.T) {
	logutils.InitLogger("debug")

	tempDir := t.TempDir()
	db := testutils.TestDatabase(t)

	// We can't easily test the download manager integration without complex setup
	// So we'll test with a nil download manager for basic functionality
	// For testing file manager functions, we can use nil download manager
	// as the actual download manager logic is tested elsewhere
	downloadManager := (*tmsdmanager.DownloadManager)(nil)

	// Add a test movie
	movieID, err := db.AddMovie(context.Background(), "Test Movie", 1024,
		[]string{"movie.mp4"}, []string{"movie.torrent", "movie.temp"}, 0)
	if err != nil {
		t.Fatalf("Failed to add test movie: %v", err)
	}

	// Create some test files in the temp directory
	tempFiles := []string{"movie.torrent", "movie.temp"}
	for _, file := range tempFiles {
		filePath := filepath.Join(tempDir, file)
		if writeErr := os.WriteFile(filePath, []byte("test content"), 0600); writeErr != nil {
			t.Fatalf("Failed to create test file %s: %v", file, writeErr)
		}
	}

	// Test deleting temporary files
	err = DeleteTemporaryFilesByMovieID(movieID, tempDir, db, downloadManager)
	if err != nil {
		t.Errorf("DeleteTemporaryFilesByMovieID failed: %v", err)
	}

	// Verify files are deleted (this is a simplified test)
	// In a real scenario, we'd check if the files were actually removed
}

func TestDeleteMainFilesByMovieID(t *testing.T) {
	logutils.InitLogger("debug")

	tempDir := t.TempDir()
	db := testutils.TestDatabase(t)
	// For testing file manager functions, we can use nil download manager
	// as the actual download manager logic is tested elsewhere
	downloadManager := (*tmsdmanager.DownloadManager)(nil)

	// Add a test movie
	movieID, err := db.AddMovie(context.Background(), "Test Movie", 1024,
		[]string{"movie.mp4"}, []string{"movie.torrent"}, 0)
	if err != nil {
		t.Fatalf("Failed to add test movie: %v", err)
	}

	// Create test files
	allFiles := []string{"movie.mp4", "movie.torrent"}
	for _, file := range allFiles {
		filePath := filepath.Join(tempDir, file)
		if writeErr := os.WriteFile(filePath, []byte("test content"), 0600); writeErr != nil {
			t.Fatalf("Failed to create test file %s: %v", file, writeErr)
		}
	}

	// Test deleting main files
	err = DeleteMainFilesByMovieID(movieID, tempDir, db, downloadManager)
	if err != nil {
		t.Errorf("DeleteMainFilesByMovieID failed: %v", err)
	}
}

func TestGetAvailableSpaceGB(t *testing.T) {
	logutils.InitLogger("debug")

	tests := []struct {
		name        string
		path        string
		expectError bool
	}{
		{
			name:        "Valid path",
			path:        "/tmp",
			expectError: false,
		},
		{
			name:        "Non-existent path",
			path:        "/nonexistent/path",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spaceGB, err := GetAvailableSpaceGB(tt.path)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error for invalid path, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if spaceGB < 0 {
				t.Errorf("Expected non-negative space, got %f", spaceGB)
			}
		})
	}
}

func BenchmarkHasEnoughSpace(b *testing.B) {
	logutils.InitLogger("error") // Reduce log noise during benchmarking

	b.ResetTimer()
	for range b.N {
		HasEnoughSpace("/tmp", 1024)
	}
}

func BenchmarkIsEmptyDirectory(b *testing.B) {
	logutils.InitLogger("error")
	tempDir := b.TempDir()

	b.ResetTimer()
	for range b.N {
		IsEmptyDirectory(tempDir)
	}
}

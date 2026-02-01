package movies

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

func TestMoviesHandler_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logutils.InitLogger("debug")

	testCases := []struct {
		name string
		test func(t *testing.T)
	}{
		{"ListMoviesWithRealFiles", testListMoviesWithRealFiles},
		{"MovieDeletionIntegration", testMovieDeletionIntegration},
		{"LargeMovieListHandling", testLargeMovieListHandling},
		{"MovieListWithDiskSpace", testMovieListWithDiskSpace},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.test)
	}
}

func setupMoviesIntegrationTest(t *testing.T) (*database.SQLiteDatabase, *config.Config, string) {
	tempDir := t.TempDir()

	cfg := &config.Config{
		BotToken:      "test_token",
		MoviePath:     tempDir,
		AdminPassword: "testpassword123",
	}

	db := &database.SQLiteDatabase{}
	if err := db.Init(cfg); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	return db, cfg, tempDir
}

func testListMoviesWithRealFiles(t *testing.T) {
	db, _, tempDir := setupMoviesIntegrationTest(t)

	movieFiles := createTestMovieFiles(t, tempDir)
	createMoviesInDatabase(t, db, movieFiles)
	verifyFilesExistOnFilesystem(t, tempDir, movieFiles)
	verifyMovieListFromDatabase(t, db, movieFiles)
	verifyDiskSpaceCalculation(t, tempDir, len(movieFiles))
}

func createTestMovieFiles(t *testing.T, tempDir string) map[string]int64 {
	movieFiles := map[string]int64{
		"action_movie.mp4": 1024 * 1024 * 500,  // 500MB
		"comedy_movie.mkv": 1024 * 1024 * 800,  // 800MB
		"drama_movie.avi":  1024 * 1024 * 1200, // 1.2GB
		"documentary.mp4":  1024 * 1024 * 300,  // 300MB
	}

	for movieName, size := range movieFiles {
		filePath := filepath.Join(tempDir, movieName)
		if err := createTestFile(filePath, size); err != nil {
			t.Fatalf("Failed to create movie file %s: %v", movieName, err)
		}
	}

	return movieFiles
}

func createMoviesInDatabase(t *testing.T, db *database.SQLiteDatabase, movieFiles map[string]int64) {
	percentages := map[string]int{
		"action_movie.mp4": 100,
		"comedy_movie.mkv": 75,
		"drama_movie.avi":  50,
		"documentary.mp4":  25,
	}

	for movieName, size := range movieFiles {
		movieID, err := db.AddMovie(context.Background(), movieName, size,
			[]string{movieName}, []string{movieName + ".torrent"}, 0)
		if err != nil {
			t.Fatalf("Failed to add movie %s to database: %v", movieName, err)
		}

		percentage := percentages[movieName]
		if err := db.UpdateDownloadedPercentage(context.Background(), movieID, percentage); err != nil {
			t.Fatalf("Failed to update percentage for %s: %v", movieName, err)
		}
	}
}

func verifyFilesExistOnFilesystem(t *testing.T, tempDir string, movieFiles map[string]int64) {
	for movieName := range movieFiles {
		filePath := filepath.Join(tempDir, movieName)
		if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
			t.Errorf("Movie file %s should exist on filesystem", movieName)
		}
	}
}

func verifyMovieListFromDatabase(t *testing.T, db *database.SQLiteDatabase, movieFiles map[string]int64) {
	movies, err := db.GetMovieList(context.Background())
	if err != nil {
		t.Fatalf("Failed to get movie list: %v", err)
	}

	if len(movies) != len(movieFiles) {
		t.Errorf("Expected %d movies, got %d", len(movieFiles), len(movies))
	}

	for i := range movies {
		movie := &movies[i]
		expectedSize, exists := movieFiles[movie.Name]
		if !exists {
			t.Errorf("Unexpected movie in list: %s", movie.Name)
			continue
		}

		if movie.FileSize != expectedSize {
			t.Errorf("Movie %s: expected size %d, got %d", movie.Name, expectedSize, movie.FileSize)
		}

		if movie.DownloadedPercentage < 0 || movie.DownloadedPercentage > 100 {
			t.Errorf("Movie %s: invalid percentage %d", movie.Name, movie.DownloadedPercentage)
		}
	}
}

func verifyDiskSpaceCalculation(t *testing.T, tempDir string, movieCount int) {
	availableSpace, err := filemanager.GetAvailableSpaceGB(tempDir)
	if err != nil {
		t.Fatalf("Failed to get available space: %v", err)
	}

	if availableSpace <= 0 {
		t.Error("Available space should be positive")
	}

	t.Logf("Created %d movies, available space: %.2f GB", movieCount, availableSpace)
}

func testMovieDeletionIntegration(t *testing.T) {
	db, cfg, tempDir := setupMoviesIntegrationTest(t)

	// Create a movie with multiple files
	movieName := "test_movie_for_deletion"
	mainFiles := []string{"movie.mp4", "subtitle.srt", "info.txt"}
	tempFiles := []string{"movie.torrent", "movie.part", "download.tmp"}

	// Create actual files
	allFiles := make([]string, 0, len(mainFiles)+len(tempFiles))
	allFiles = append(allFiles, mainFiles...)
	allFiles = append(allFiles, tempFiles...)
	for _, fileName := range allFiles {
		filePath := filepath.Join(tempDir, fileName)
		if err := createTestFile(filePath, 1024); err != nil {
			t.Fatalf("Failed to create file %s: %v", fileName, err)
		}
	}

	// Add movie to database
	movieID, err := db.AddMovie(context.Background(), movieName, 1024*int64(len(allFiles)),
		mainFiles, tempFiles, 0)
	if err != nil {
		t.Fatalf("Failed to add movie: %v", err)
	}

	// Verify all files exist
	for _, fileName := range allFiles {
		filePath := filepath.Join(tempDir, fileName)
		if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
			t.Errorf("File %s should exist before deletion", fileName)
		}
	}

	// Create download manager for deletion
	downloadManager := tmsdmanager.NewDownloadManager(cfg, db)

	// Test deleting temporary files
	err = filemanager.DeleteTemporaryFilesByMovieID(movieID, tempDir, db, downloadManager)
	if err != nil {
		t.Fatalf("Failed to delete temporary files: %v", err)
	}

	// Verify temp files are deleted, main files remain
	for _, fileName := range tempFiles {
		filePath := filepath.Join(tempDir, fileName)
		if _, statErr := os.Stat(filePath); !os.IsNotExist(statErr) {
			t.Errorf("Temp file %s should be deleted", fileName)
		}
	}

	for _, fileName := range mainFiles {
		filePath := filepath.Join(tempDir, fileName)
		if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
			t.Errorf("Main file %s should still exist", fileName)
		}
	}

	// Test deleting main files
	err = filemanager.DeleteMainFilesByMovieID(movieID, tempDir, db, downloadManager)
	if err != nil {
		t.Fatalf("Failed to delete main files: %v", err)
	}

	// Verify all files are now deleted
	for _, fileName := range mainFiles {
		filePath := filepath.Join(tempDir, fileName)
		if _, statErr := os.Stat(filePath); !os.IsNotExist(statErr) {
			t.Errorf("Main file %s should be deleted", fileName)
		}
	}
}

func testLargeMovieListHandling(t *testing.T) {
	db, _, tempDir := setupMoviesIntegrationTest(t)

	// Create a large number of movies
	numMovies := 100

	for i := range numMovies {
		movieName := fmt.Sprintf("movie_%03d.mp4", i)
		fileSize := int64(1024 * 1024 * (i + 1)) // Increasing sizes

		// Create actual file (small for testing)
		filePath := filepath.Join(tempDir, movieName)
		if err := createTestFile(filePath, 1024); err != nil {
			t.Fatalf("Failed to create movie file %d: %v", i, err)
		}

		movieID, err := db.AddMovie(context.Background(), movieName, fileSize,
			[]string{movieName}, []string{movieName + ".torrent"}, 0)
		if err != nil {
			t.Fatalf("Failed to add movie %d: %v", i, err)
		}

		// Set random progress
		progress := (i * 7) % 101 // Pseudo-random percentage 0-100
		if err := db.UpdateDownloadedPercentage(context.Background(), movieID, progress); err != nil {
			t.Fatalf("Failed to set progress for movie %d: %v", i, err)
		}
	}

	// Test retrieving large movie list
	movies, err := db.GetMovieList(context.Background())
	if err != nil {
		t.Fatalf("Failed to get large movie list: %v", err)
	}

	if len(movies) != numMovies {
		t.Errorf("Expected %d movies, got %d", numMovies, len(movies))
	}

	// Test performance of list operations
	totalSize := int64(0)
	for i := range movies {
		movie := &movies[i]
		totalSize += movie.FileSize
		if movie.DownloadedPercentage < 0 || movie.DownloadedPercentage > 100 {
			t.Errorf("Invalid percentage for movie %s: %d", movie.Name, movie.DownloadedPercentage)
		}
	}

	expectedTotalSize := int64(1024 * 1024 * numMovies * (numMovies + 1) / 2) // Sum of 1+2+...+numMovies MB
	if totalSize != expectedTotalSize {
		t.Errorf("Expected total size %d, got %d", expectedTotalSize, totalSize)
	}

	t.Logf("Successfully handled %d movies with total size %.2f GB",
		numMovies, float64(totalSize)/(1024*1024*1024))
}

func testMovieListWithDiskSpace(t *testing.T) {
	db, _, tempDir := setupMoviesIntegrationTest(t)

	// Create movies and verify disk space calculations
	movieSizes := []int64{
		100 * 1024 * 1024,  // 100MB
		500 * 1024 * 1024,  // 500MB
		1024 * 1024 * 1024, // 1GB
	}

	var totalMovieSize int64
	for i, size := range movieSizes {
		movieName := fmt.Sprintf("size_test_%d.mp4", i)

		// Create file with approximate size (smaller for testing)
		filePath := filepath.Join(tempDir, movieName)
		testFileSize := min(size, 10*1024) // Max 10KB for test files
		if err := createTestFile(filePath, testFileSize); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		_, err := db.AddMovie(context.Background(), movieName, size,
			[]string{movieName}, []string{}, 0)
		if err != nil {
			t.Fatalf("Failed to add movie: %v", err)
		}

		totalMovieSize += size
	}

	// Test disk space before and after
	availableSpace, err := filemanager.GetAvailableSpaceGB(tempDir)
	if err != nil {
		t.Fatalf("Failed to get available space: %v", err)
	}

	// Test space requirement checking
	spaceNeeded := totalMovieSize
	hasSpace := filemanager.HasEnoughSpace(tempDir, spaceNeeded)

	t.Logf("Total movie size: %.2f GB, Available space: %.2f GB, Has enough space: %v",
		float64(totalMovieSize)/(1024*1024*1024), availableSpace, hasSpace)

	if availableSpace <= 0 {
		t.Error("Available space should be positive")
	}
}

// Helper function to create a test file with specified size
func createTestFile(filePath string, size int64) error {
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Write data in chunks to avoid memory issues
	chunkSize := int64(1024) // 1KB chunks
	written := int64(0)
	chunk := make([]byte, chunkSize)

	for i := range chunk {
		chunk[i] = byte(i % 256)
	}

	for written < size {
		toWrite := min(chunkSize, size-written)
		if _, err := file.Write(chunk[:toWrite]); err != nil {
			return fmt.Errorf("failed to write to file: %w", err)
		}
		written += toWrite
	}

	return nil
}

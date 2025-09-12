package testutils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/jackpal/bencode-go"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	testFileSize   = 1024
	tickerInterval = 10 * time.Millisecond
	testFileMode   = 0600
	byteRange      = 256
)

// TestConfig creates a configuration suitable for testing
func TestConfig(tempDir string) *config.Config {
	return &config.Config{
		BotToken:        "test-bot-token",
		MoviePath:       tempDir,
		AdminPassword:   "test-admin",
		RegularPassword: "test-regular",
		Lang:            "en",
		LangPath:        "./locales",
		LogLevel:        "debug",

		DownloadSettings: config.DownloadConfig{
			MaxConcurrentDownloads: 1,
			DownloadTimeout:        30 * time.Second,
			ProgressUpdateInterval: 100 * time.Millisecond,
		},

		SecuritySettings: config.SecurityConfig{
			PasswordMinLength: 4,
		},

		Aria2Settings: config.Aria2Config{
			MaxPeers:                 50,
			MaxConnectionsPerServer:  4,
			Split:                    4,
			MinSplitSize:             "1M",
			BTMaxPeers:               50,
			BTRequestPeerSpeedLimit:  "0",
			BTMaxOpenFiles:           10,
			MaxOverallUploadLimit:    "100K",
			MaxUploadLimit:           "50K",
			SeedRatio:                0.0,
			SeedTime:                 0,
			BTTrackerTimeout:         10,
			BTTrackerInterval:        0,
			EnableDHT:                false,
			EnablePeerExchange:       false,
			EnableLocalPeerDiscovery: false,
			FollowTorrent:            true,
			ListenPort:               "6881-6882",
			DHTPorts:                 "6881-6882",
			BTSaveMetadata:           false,
			BTHashCheckSeed:          false,
			BTRequireCrypto:          false,
			BTMinCryptoLevel:         "plain",
			CheckIntegrity:           false,
			ContinueDownload:         true,
			RemoteTime:               false,
			FileAllocation:           "none",
			Timeout:                  10,
			MaxTries:                 3,
			RetryWait:                1,
		},

		VideoSettings: config.VideoConfig{
			EnableReencoding:  false,
			ForceReencoding:   false,
			VideoCodec:        "h264",
			AudioCodec:        "mp3",
			OutputFormat:      "mp4",
			FFmpegExtraArgs:   "-pix_fmt yuv420p",
			QualitySelector:   "worst",
			CompatibilityMode: true,
		},
	}
}

// TestDatabase creates an in-memory SQLite database for testing
func TestDatabase(t *testing.T) database.Database {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	testDB := &TestSQLiteDatabase{db: db}
	if err := testDB.runMigrations(); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	return testDB
}

// TestSQLiteDatabase is a test-specific implementation
type TestSQLiteDatabase struct {
	db *gorm.DB
}

func (t *TestSQLiteDatabase) runMigrations() error {
	// Copy migration logic from the main database package
	return t.db.AutoMigrate(
		&database.Movie{},
		&database.MovieFile{},
		&database.User{},
		&database.TemporaryPassword{},
	)
}

func (*TestSQLiteDatabase) Init(_ *config.Config) error {
	return nil // Already initialized
}

// Implement all database interface methods by delegating to the real implementation
func (t *TestSQLiteDatabase) AddMovie(ctx context.Context, name string, fileSize int64, mainFiles, tempFiles []string) (uint, error) {
	movie := database.Movie{
		Name:     name,
		FileSize: fileSize,
	}
	result := t.db.WithContext(ctx).Create(&movie)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to add movie to database: %w", result.Error)
	}

	if err := t.addFiles(ctx, movie.ID, mainFiles, false); err != nil {
		return 0, fmt.Errorf("failed to add main files for movie %d: %w", movie.ID, err)
	}

	if err := t.addFiles(ctx, movie.ID, tempFiles, true); err != nil {
		return 0, fmt.Errorf("failed to add temporary files for movie %d: %w", movie.ID, err)
	}

	return movie.ID, nil
}

func (t *TestSQLiteDatabase) addFiles(ctx context.Context, movieID uint, files []string, isTemp bool) error {
	for _, file := range files {
		movieFile := database.MovieFile{
			MovieID:  movieID,
			FilePath: file,
			TempFile: isTemp,
		}
		if result := t.db.WithContext(ctx).Create(&movieFile); result.Error != nil {
			return fmt.Errorf("failed to add file %s: %w", file, result.Error)
		}
	}
	return nil
}

// Add other required methods...
func (*TestSQLiteDatabase) RemoveMovie(_ context.Context, _ uint) error { return nil }
func (*TestSQLiteDatabase) GetMovieList(_ context.Context) ([]database.Movie, error) {
	return nil, nil
}
func (*TestSQLiteDatabase) UpdateDownloadedPercentage(_ context.Context, _ uint, _ int) error {
	return nil
}
func (*TestSQLiteDatabase) SetLoaded(_ context.Context, _ uint) error { return nil }
func (*TestSQLiteDatabase) GetMovieByID(_ context.Context, _ uint) (database.Movie, error) {
	return database.Movie{}, nil
}
func (*TestSQLiteDatabase) GetFilesByMovieID(_ context.Context, _ uint) ([]database.MovieFile, error) {
	return nil, nil
}
func (*TestSQLiteDatabase) RemoveFilesByMovieID(_ context.Context, _ uint) error {
	return nil
}
func (*TestSQLiteDatabase) RemoveTempFilesByMovieID(_ context.Context, _ uint) error {
	return nil
}
func (*TestSQLiteDatabase) MovieExistsFiles(_ context.Context, _ []string) (bool, error) {
	return false, nil
}
func (*TestSQLiteDatabase) MovieExistsUploadedFile(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (*TestSQLiteDatabase) GetTempFilesByMovieID(_ context.Context, _ uint) ([]database.MovieFile, error) {
	return nil, nil
}

func (*TestSQLiteDatabase) Login(
	_ context.Context,
	_ string,
	_ int64,
	_ string,
	_ *config.Config,
) (bool, error) {
	return false, nil
}
func (*TestSQLiteDatabase) GetUserRole(_ context.Context, _ int64) (database.UserRole, error) {
	return "guest", nil
}
func (*TestSQLiteDatabase) IsUserAccessAllowed(_ context.Context, _ int64) (isAllowed bool, userRole database.UserRole, err error) {
	return false, "guest", nil
}
func (*TestSQLiteDatabase) AssignTemporaryPassword(_ context.Context, _ string, _ int64) error {
	return nil
}
func (*TestSQLiteDatabase) ExtendTemporaryUser(_ context.Context, _ int64, _ time.Time) error {
	return nil
}
func (*TestSQLiteDatabase) GenerateTemporaryPassword(_ context.Context, _ time.Duration) (string, error) {
	return "", nil
}
func (*TestSQLiteDatabase) GetUserByChatID(_ context.Context, _ int64) (database.User, error) {
	return database.User{}, nil
}
func (*TestSQLiteDatabase) MovieExistsId(_ context.Context, _ uint) (bool, error) {
	return false, nil
}

// TempDir creates a temporary directory for testing
func TempDir(t *testing.T) string {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "telegram-media-server-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	return tempDir
}

// CreateTestTorrent creates a minimal valid torrent file for testing
func CreateTestTorrent(t *testing.T, dir, name string) string {
	t.Helper()

	// Create a simple bencode torrent file
	// This is a minimal valid torrent structure
	nameLen := len(name)
	torrentContent := fmt.Sprintf(
		"d8:announce9:test-url13:creation datei1609459200e4:infod6:lengthi1024e4:name%d:%s"+
			"12:piece lengthi16384e6:pieces20:01234567890123456789ee",
		nameLen,
		name,
	)

	filePath := filepath.Join(dir, name+".torrent")
	if err := os.WriteFile(filePath, []byte(torrentContent), testFileMode); err != nil {
		t.Fatalf("Failed to create test torrent: %v", err)
	}

	return filePath
}

// CreateRealTestTorrent creates a more realistic torrent file using bencode library
func CreateRealTestTorrent(t *testing.T, dir, name string) string {
	t.Helper()

	// Create actual test file content
	testFilePath := filepath.Join(dir, name+".txt")
	testContent := []byte("This is a test file for torrent creation")
	if err := os.WriteFile(testFilePath, testContent, testFileMode); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create torrent metadata
	torrentMeta := map[string]any{
		"announce":      "http://tracker.example.com:8080/announce",
		"creation date": time.Now().Unix(),
		"comment":       "Test torrent created for integration testing",
		"created by":    "telegram-media-server-test",
		"info": map[string]any{
			"name":         name + ".txt",
			"length":       int64(len(testContent)),
			"piece length": 16384,
			"pieces":       "12345678901234567890", // 20-byte SHA1 hash placeholder
		},
	}

	torrentPath := filepath.Join(dir, name+".torrent")
	f, err := os.Create(torrentPath)
	if err != nil {
		t.Fatalf("Failed to create torrent file: %v", err)
	}
	defer f.Close()

	if err := bencode.Marshal(f, torrentMeta); err != nil {
		t.Fatalf("Failed to encode torrent: %v", err)
	}

	return torrentPath
}

// CreateTestDataFile creates a test data file with specified size
func CreateTestDataFile(t *testing.T, dir, name string, size int64) string {
	t.Helper()

	filePath := filepath.Join(dir, name)
	f, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("Failed to create test data file: %v", err)
	}
	defer f.Close()

	// Write test data
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % byteRange)
	}

	if _, err := f.Write(data); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	return filePath
}

// CreateInvalidTorrent creates an invalid torrent file (HTML) for testing
func CreateInvalidTorrent(t *testing.T, dir, name string) string {
	t.Helper()

	htmlContent := `<!DOCTYPE html>
<html>
<head><title>Error</title></head>
<body><h1>File not found</h1></body>
</html>`

	filePath := filepath.Join(dir, name+".torrent")
	if err := os.WriteFile(filePath, []byte(htmlContent), testFileMode); err != nil {
		t.Fatalf("Failed to create invalid torrent: %v", err)
	}

	return filePath
}

// CreateMagnetLink creates a magnet link file for testing
func CreateMagnetLink(t *testing.T, dir, name string) string {
	t.Helper()

	magnetContent := "magnet:?xt=urn:btih:1234567890abcdef1234567890abcdef12345678&dn=test"

	filePath := filepath.Join(dir, name+".torrent")
	if err := os.WriteFile(filePath, []byte(magnetContent), testFileMode); err != nil {
		t.Fatalf("Failed to create magnet link file: %v", err)
	}

	return filePath
}

// MockHTTPServer creates a mock HTTP server for testing
func MockHTTPServer(t *testing.T, responses map[string]string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if response, exists := responses[path]; exists {
			w.Header().Set("Content-Type", "application/x-bittorrent")
			_, err := io.WriteString(w, response)
			if err != nil {
				t.Errorf("Failed to write response: %v", err)
			}
		} else {
			http.NotFound(w, r)
		}
	}))

	t.Cleanup(server.Close)
	return server
}

// MockYTDLPServer creates a mock server that responds like a video hosting service
func MockYTDLPServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case strings.Contains(path, "/test-video"):
			// Simulate a simple video page
			w.Header().Set("Content-Type", "text/html")
			_, err := io.WriteString(w, `
<!DOCTYPE html>
<html>
<head><title>Test Video</title></head>
<body>
	<video src="/test-video.mp4"></video>
	<script>
		window.videoData = {
			title: "Test Video",
			duration: 60,
			formats: [
				{url: "/test-video.mp4", quality: "720p"}
			]
		};
	</script>
</body>
</html>`)
			if err != nil {
				t.Errorf("Failed to write response: %v", err)
			}
		case strings.Contains(path, "/test-video.mp4"):
			// Simulate a small video file
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Content-Length", "1024")
			_, err := w.Write(make([]byte, testFileSize))
			if err != nil {
				t.Errorf("Failed to write video response: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))

	t.Cleanup(server.Close)
	return server
}

// AssertFileExists checks if a file exists
func AssertFileExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Expected file %s to exist, but it doesn't", path)
	}
}

// AssertFileNotExists checks if a file doesn't exist
func AssertFileNotExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err == nil {
		t.Errorf("Expected file %s to not exist, but it does", path)
	}
}

// WaitForCondition waits for a condition to be true or times out
func WaitForCondition(t *testing.T, condition func() bool, timeout time.Duration, message string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for condition: %s", message)
		case <-ticker.C:
			if condition() {
				return
			}
		}
	}
}

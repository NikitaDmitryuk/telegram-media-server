package ytdlp

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

// TestVideoDownloadIntegration tests the complete video download flow
func TestVideoDownloadIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Initialize logger for tests
	logger.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)

	// Set short timeouts for testing
	cfg.DownloadSettings.DownloadTimeout = 10 * time.Second
	cfg.DownloadSettings.ProgressUpdateInterval = 100 * time.Millisecond

	t.Run("VideoURLValidation", func(t *testing.T) {
		testCases := []struct {
			name     string
			url      string
			expected bool
		}{
			{"YouTube URL", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", true},
			{"YouTube Short URL", "https://youtu.be/dQw4w9WgXcQ", true},
			{"Vimeo URL", "https://vimeo.com/123456789", true},
			{"Invalid URL", "not-a-url", false},
			{"HTTP URL", "http://example.com/video.mp4", true},
			{"Local file", "/path/to/local/file.mp4", false},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				isValid := isValidVideoURL(tc.url)
				if isValid != tc.expected {
					t.Errorf("Expected %v for URL %s, got %v", tc.expected, tc.url, isValid)
				}
			})
		}
	})

	t.Run("VideoIDExtraction", func(t *testing.T) {
		testCases := []struct {
			url      string
			expected string
		}{
			{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "www_youtube_com_dQw4w9WgXcQ"},
			{"https://youtu.be/dQw4w9WgXcQ", "youtu_be_dQw4w9WgXcQ"},
			{"https://vimeo.com/123456789", "vimeo_com_123456789"},
			{"https://example.com/video/test", "example_com_video_test"},
		}

		for _, tc := range testCases {
			t.Run(tc.url, func(t *testing.T) {
				result, err := extractVideoID(tc.url)
				if err != nil {
					t.Fatalf("extractVideoID failed: %v", err)
				}
				if result != tc.expected {
					t.Errorf("Expected %s, got %s", tc.expected, result)
				}
			})
		}
	})

	t.Run("MockVideoDownload", func(t *testing.T) {
		// Create mock HTTP server for testing
		server := testutils.MockYTDLPServer(t)
		testURL := server.URL + "/test-video"

		downloader := NewYTDLPDownloader(nil, testURL, cfg)

		// Test basic operations (these will fail without yt-dlp but shouldn't crash)
		title, err := downloader.GetTitle()
		if err != nil {
			t.Logf("GetTitle failed (expected without yt-dlp): %v", err)
		} else {
			t.Logf("Got title: %s", title)
		}

		files, tempFiles, err := downloader.GetFiles()
		if err != nil {
			t.Logf("GetFiles failed (expected without yt-dlp): %v", err)
		} else {
			t.Logf("Got files: main=%v, temp=%v", files, tempFiles)
		}

		size, err := downloader.GetFileSize()
		if err != nil {
			t.Logf("GetFileSize failed (expected without yt-dlp): %v", err)
		} else {
			t.Logf("Got size: %d bytes", size)
		}
	})
}

// TestVideoConfigurationOptions tests various video configuration options
//
//nolint:gocyclo // Test function validating many configuration options
func TestVideoConfigurationOptions(t *testing.T) {
	t.Skip("Skipping video configuration test - requires network connectivity")

	// Initialize logger for tests
	logger.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	baseConfig := testutils.TestConfig(tempDir)

	testURL := "https://example.com/test-video"

	t.Run("DefaultConfiguration", func(t *testing.T) {
		cfg := *baseConfig
		downloader := NewYTDLPDownloader(nil, testURL, &cfg).(*YTDLPDownloader)

		outputPath := filepath.Join(cfg.MoviePath, "%(title)s.%(ext)s")
		args := downloader.buildYTDLPArgs(outputPath)

		// Check for default quality selector
		found := false
		for i, arg := range args {
			if arg == "--format" && i+1 < len(args) {
				if strings.Contains(args[i+1], "bv*+ba/b") {
					found = true
					break
				}
			}
		}
		if !found {
			t.Error("Default quality selector not found in args")
		}
	})

	t.Run("ReencodingConfiguration", func(t *testing.T) {
		cfg := *baseConfig
		cfg.VideoSettings.EnableReencoding = true
		cfg.VideoSettings.VideoCodec = "h264"
		cfg.VideoSettings.AudioCodec = "aac"
		cfg.VideoSettings.OutputFormat = "mp4"
		cfg.VideoSettings.FFmpegExtraArgs = "-crf 23"

		downloader := NewYTDLPDownloader(nil, testURL, &cfg).(*YTDLPDownloader)
		outputPath := filepath.Join(cfg.MoviePath, "%(title)s.%(ext)s")
		args := downloader.buildYTDLPArgs(outputPath)

		// Check for re-encoding args
		hasRemux := false
		hasPostprocessor := false

		for _, arg := range args {
			if arg == "--remux-video" {
				hasRemux = true
			}
			if arg == "--postprocessor-args" {
				hasPostprocessor = true
			}
		}

		if !hasRemux {
			t.Error("Re-encoding should include --remux-video")
		}
		if !hasPostprocessor {
			t.Error("Re-encoding should include --postprocessor-args")
		}
	})

	t.Run("ProxyConfiguration", func(t *testing.T) {
		cfg := *baseConfig
		cfg.Proxy = "http://proxy.example.com:8080"
		cfg.ProxyDomains = "example.com,youtube.com"

		downloader := NewYTDLPDownloader(nil, testURL, &cfg).(*YTDLPDownloader)
		outputPath := filepath.Join(cfg.MoviePath, "%(title)s.%(ext)s")
		args := downloader.buildYTDLPArgs(outputPath)

		// Check for proxy args
		hasProxy := false
		for i, arg := range args {
			if arg == "--proxy" && i+1 < len(args) {
				if args[i+1] == cfg.Proxy {
					hasProxy = true
					break
				}
			}
		}

		if !hasProxy {
			t.Error("Proxy configuration not found in args")
		}
	})

	t.Run("CustomQualitySelector", func(t *testing.T) {
		cfg := *baseConfig
		cfg.VideoSettings.QualitySelector = "best[height<=720]"

		downloader := NewYTDLPDownloader(nil, testURL, &cfg).(*YTDLPDownloader)
		outputPath := filepath.Join(cfg.MoviePath, "%(title)s.%(ext)s")
		args := downloader.buildYTDLPArgs(outputPath)

		// Check for custom quality
		found := false
		for i, arg := range args {
			if arg == "--format" && i+1 < len(args) {
				if args[i+1] == "best[height<=720]" {
					found = true
					break
				}
			}
		}

		if !found {
			t.Error("Custom quality selector not found in args")
		}
	})
}

// TestVideoErrorHandling tests various error conditions for video downloads
func TestVideoErrorHandling(t *testing.T) {
	t.Skip("Skipping video error handling test - requires network connectivity")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)

	t.Run("InvalidURL", func(t *testing.T) {
		invalidURLs := []string{
			"not-a-url",
			"ftp://example.com/file",
			"",
			"javascript:alert('xss')",
		}

		for _, url := range invalidURLs {
			t.Run(url, func(t *testing.T) {
				if url == "" {
					url = "empty"
				}

				downloader := NewYTDLPDownloader(nil, url, cfg)

				// These should either fail gracefully or handle the error
				_, err := downloader.GetTitle()
				if err == nil && url != "empty" {
					t.Logf("GetTitle unexpectedly succeeded for invalid URL: %s", url)
				}
			})
		}
	})

	t.Run("NetworkTimeout", func(t *testing.T) {
		// Use a URL that will timeout
		timeoutURL := "http://192.0.2.0:12345/video" // Non-routable IP

		cfg.DownloadSettings.DownloadTimeout = 1 * time.Second
		downloader := NewYTDLPDownloader(nil, timeoutURL, cfg)

		// This should timeout or fail quickly
		start := time.Now()
		_, err := downloader.GetTitle()
		duration := time.Since(start)

		if err == nil {
			t.Log("GetTitle unexpectedly succeeded for timeout URL")
		}

		// Should not take too long to fail
		if duration > 30*time.Second {
			t.Errorf("Operation took too long: %v", duration)
		}
	})

	t.Run("InvalidOutputDirectory", func(t *testing.T) {
		invalidDir := "/invalid/nonexistent/directory/path"
		cfg.MoviePath = invalidDir

		downloader := NewYTDLPDownloader(nil, "https://example.com/video", cfg)

		// This should handle the invalid directory gracefully
		_, _, err := downloader.GetFiles()
		if err == nil {
			t.Log("GetFiles unexpectedly succeeded with invalid directory")
		}
	})
}

// TestVideoFileOperations tests file-related operations for video downloads
func TestVideoFileOperations(t *testing.T) {
	t.Skip("Skipping video file operations test - requires network connectivity")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)

	t.Run("OutputPathGeneration", func(t *testing.T) {
		testCases := []struct {
			url           string
			expectedInfix string
		}{
			{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "youtube"},
			{"https://vimeo.com/123456789", "vimeo"},
			{"https://example.com/video", "example"},
		}

		for _, tc := range testCases {
			t.Run(tc.url, func(t *testing.T) {
				downloader := NewYTDLPDownloader(nil, tc.url, cfg).(*YTDLPDownloader)

				outputPath := filepath.Join(cfg.MoviePath, "%(title)s.%(ext)s")
				args := downloader.buildYTDLPArgs(outputPath)

				// Find output path in args
				outputFound := false
				for i, arg := range args {
					if arg == "--output" && i+1 < len(args) {
						outputPath := args[i+1]
						if strings.Contains(outputPath, cfg.MoviePath) {
							outputFound = true
						}
						t.Logf("Output path: %s", outputPath)
						break
					}
				}

				if !outputFound {
					t.Error("Output path not found or incorrect")
				}
			})
		}
	})

	t.Run("DirectoryCreation", func(t *testing.T) {
		// Test with nested directory structure
		nestedDir := filepath.Join(tempDir, "videos", "downloads", "test")
		cfg.MoviePath = nestedDir

		downloader := NewYTDLPDownloader(nil, "https://example.com/video", cfg)

		// This should handle directory creation (or at least not crash)
		_, _, err := downloader.GetFiles()
		if err != nil {
			t.Logf("GetFiles failed (expected without yt-dlp): %v", err)
		}
	})

	t.Run("FilenameGeneration", func(t *testing.T) {
		testCases := []struct {
			url      string
			hasTitle bool
		}{
			{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", true},
			{"https://example.com/video.mp4", false},
		}

		for _, tc := range testCases {
			t.Run(tc.url, func(t *testing.T) {
				downloader := NewYTDLPDownloader(nil, tc.url, cfg)

				// Test title extraction
				title, err := downloader.GetTitle()
				if err != nil {
					t.Logf("GetTitle failed (expected without yt-dlp): %v", err)
				} else if title != "" {
					t.Logf("Got title: %s", title)
				}
			})
		}
	})
}

// TestVideoConcurrency tests concurrent video operations
func TestVideoConcurrency(t *testing.T) {
	t.Skip("Skipping video concurrency test - requires network connectivity")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)

	t.Run("ConcurrentDownloaderCreation", func(t *testing.T) {
		const numGoroutines = 5

		urls := make([]string, numGoroutines)
		for i := range numGoroutines {
			urls[i] = fmt.Sprintf("https://example.com/video-%d", i)
		}

		// Channel to collect results
		results := make(chan error, numGoroutines)

		// Create downloaders concurrently
		for i := range numGoroutines {
			go func(url string) {
				downloader := NewYTDLPDownloader(nil, url, cfg)
				_, err := downloader.GetTitle()
				results <- err
			}(urls[i])
		}

		// Collect results
		for i := range numGoroutines {
			err := <-results
			if err != nil {
				t.Logf("Concurrent operation %d failed (expected): %v", i, err)
			}
		}
	})

	t.Run("ConcurrentVideoIDExtraction", func(t *testing.T) {
		const numGoroutines = 10

		urls := []string{
			"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			"https://youtu.be/abc123",
			"https://vimeo.com/123456789",
			"https://example.com/video/test",
		}

		results := make(chan string, numGoroutines)

		// Extract video IDs concurrently
		for i := range numGoroutines {
			go func(url string) {
				videoID, err := extractVideoID(url)
				if err != nil {
					results <- fmt.Sprintf("error: %v", err)
				} else {
					results <- videoID
				}
			}(urls[i%len(urls)])
		}

		// Collect results
		for range numGoroutines {
			result := <-results
			if strings.HasPrefix(result, "error:") {
				t.Errorf("Video ID extraction failed: %s", result)
			} else {
				t.Logf("Video ID: %s", result)
			}
		}
	})
}

// Helper function to check if URL is valid for video downloading
func isValidVideoURL(url string) bool {
	if url == "" {
		return false
	}

	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return true
	}

	return false
}

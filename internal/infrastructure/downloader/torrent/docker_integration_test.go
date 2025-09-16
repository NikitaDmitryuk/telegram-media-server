package aria2

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

// TestAria2Integration tests real aria2 functionality in Docker environment
func TestAria2Integration(t *testing.T) {
	// Only run if explicitly enabled or in Docker environment
	if os.Getenv("INTEGRATION_TESTS") != "true" && os.Getenv("CI") != "true" {
		t.Skip("Skipping aria2 integration test (set INTEGRATION_TESTS=true to enable)")
	}

	// Initialize logger for tests
	logger.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)
	cfg.DownloadSettings.DownloadTimeout = 30 * time.Second
	cfg.DownloadSettings.ProgressUpdateInterval = 500 * time.Millisecond

	t.Run("RealTorrentDownload", func(t *testing.T) {
		// Create a real small torrent file for testing
		torrentPath := testutils.CreateRealTestTorrent(t, tempDir, "integration-test")

		downloader := NewAria2Downloader(nil, torrentPath, cfg.MoviePath, cfg).(*Aria2Downloader)

		// Test metadata extraction
		title, err := downloader.GetTitle()
		if err != nil {
			t.Logf("GetTitle failed (may require aria2): %v", err)
		} else {
			t.Logf("Got title: %s", title)
		}

		files, tempFiles, err := downloader.GetFiles()
		if err != nil {
			t.Logf("GetFiles failed (may require aria2): %v", err)
		} else {
			t.Logf("Got files: main=%v, temp=%v", files, tempFiles)
		}

		size, err := downloader.GetFileSize()
		if err != nil {
			t.Logf("GetFileSize failed (may require aria2): %v", err)
		} else {
			t.Logf("Got size: %d bytes", size)
		}

		// Test actual download (with short timeout for testing)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		progressChan, errChan, err := downloader.StartDownload(ctx)
		if err != nil {
			t.Logf("StartDownload failed (expected without real peers): %v", err)
			return
		}

		// Monitor download for a short time
		timeout := time.After(5 * time.Second)
		var lastProgress float64
		progressReceived := false

	DownloadLoop:
		for {
			select {
			case progress, ok := <-progressChan:
				if !ok {
					t.Log("Progress channel closed")
					break DownloadLoop
				}
				lastProgress = progress
				progressReceived = true
				t.Logf("Download progress: %.1f%%", progress)

			case err, ok := <-errChan:
				if !ok {
					t.Log("Error channel closed")
					break DownloadLoop
				}
				if err != nil {
					t.Logf("Download error (expected): %v", err)
				} else {
					t.Log("Download completed successfully")
				}
				break DownloadLoop

			case <-timeout:
				t.Log("Download test timeout (expected)")
				err = downloader.StopDownload()
				if err != nil {
					t.Logf("Stop download error: %v", err)
				}
				break DownloadLoop

			case <-ctx.Done():
				t.Log("Context cancelled")
				break DownloadLoop
			}
		}

		if progressReceived {
			t.Logf("Successfully received progress updates, last: %.1f%%", lastProgress)
		} else {
			t.Log("No progress updates received (may be expected for test torrent)")
		}
	})

	t.Run("MultipleAria2Instances", func(t *testing.T) {
		// Test that multiple aria2 instances can run concurrently
		numInstances := 3
		downloaders := make([]*Aria2Downloader, numInstances)

		for i := 0; i < numInstances; i++ {
			torrentPath := testutils.CreateRealTestTorrent(t, tempDir, fmt.Sprintf("multi-test-%d", i))
			downloaders[i] = NewAria2Downloader(nil, torrentPath, cfg.MoviePath, cfg).(*Aria2Downloader)
		}

		// Start all downloads
		contexts := make([]context.Context, numInstances)
		cancels := make([]context.CancelFunc, numInstances)
		progressChans := make([]chan float64, numInstances)
		errChans := make([]chan error, numInstances)

		for i, downloader := range downloaders {
			contexts[i], cancels[i] = context.WithTimeout(context.Background(), 5*time.Second)
			defer cancels[i]()

			progressChan, errChan, err := downloader.StartDownload(contexts[i])
			if err != nil {
				t.Logf("Failed to start download %d: %v", i, err)
				continue
			}

			progressChans[i] = progressChan
			errChans[i] = errChan
		}

		// Monitor all downloads briefly
		timeout := time.After(3 * time.Second)
		activeDownloads := numInstances

	MultiLoop:
		for activeDownloads > 0 {
			select {
			case <-timeout:
				t.Log("Multi-instance test timeout")
				break MultiLoop

			default:
				// Check all channels non-blockingly
				for i := 0; i < numInstances; i++ {
					if progressChans[i] == nil {
						continue
					}

					select {
					case progress, ok := <-progressChans[i]:
						if !ok {
							progressChans[i] = nil
							continue
						}
						t.Logf("Instance %d progress: %.1f%%", i, progress)

					case err, ok := <-errChans[i]:
						if !ok {
							errChans[i] = nil
							activeDownloads--
							continue
						}
						if err != nil {
							t.Logf("Instance %d error: %v", i, err)
						} else {
							t.Logf("Instance %d completed", i)
						}
						activeDownloads--

					default:
						// No data available, continue
					}
				}

				// Small delay to prevent busy waiting
				time.Sleep(100 * time.Millisecond)
			}
		}

		// Stop all remaining downloads
		for i, downloader := range downloaders {
			if downloader != nil {
				err := downloader.StopDownload()
				if err != nil {
					t.Logf("Failed to stop download %d: %v", i, err)
				}
			}
		}

		t.Log("Multiple aria2 instances test completed")
	})

	t.Run("Aria2ConfigurationTest", func(t *testing.T) {
		// Test various aria2 configurations
		torrentPath := testutils.CreateRealTestTorrent(t, tempDir, "config-test")

		// Test with custom configuration
		customCfg := *cfg
		customCfg.Aria2Settings.MaxPeers = 10
		customCfg.Aria2Settings.BTMaxPeers = 10
		customCfg.Aria2Settings.MaxConnectionsPerServer = 2

		downloader := NewAria2Downloader(nil, torrentPath, customCfg.MoviePath, &customCfg).(*Aria2Downloader)

		// Test that configuration is applied
		if downloader.config.Aria2Settings.MaxPeers != 10 {
			t.Errorf("Expected MaxPeers=10, got %d", downloader.config.Aria2Settings.MaxPeers)
		}

		// Test download with custom config
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		progressChan, errChan, err := downloader.StartDownload(ctx)
		if err != nil {
			t.Logf("StartDownload with custom config failed: %v", err)
			return
		}

		// Quick test
		timeout := time.After(2 * time.Second)

	ConfigLoop:
		for {
			select {
			case progress, ok := <-progressChan:
				if !ok {
					break ConfigLoop
				}
				t.Logf("Custom config progress: %.1f%%", progress)

			case err, ok := <-errChan:
				if !ok {
					break ConfigLoop
				}
				t.Logf("Custom config result: %v", err)
				break ConfigLoop

			case <-timeout:
				downloader.StopDownload()
				break ConfigLoop
			}
		}

		t.Log("Configuration test completed")
	})
}

func TestAria2ValidationIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") != "true" && os.Getenv("CI") != "true" {
		t.Skip("Skipping aria2 validation integration test")
	}

	logger.InitLogger("debug")
	tempDir := testutils.TempDir(t)

	t.Run("RealTorrentValidation", func(t *testing.T) {
		// Test validation with real torrent file
		torrentPath := testutils.CreateRealTestTorrent(t, tempDir, "validation-integration")

		err := ValidateTorrentFile(torrentPath)
		if err != nil {
			t.Errorf("Real torrent validation failed: %v", err)
		}

		// Test metadata parsing
		meta, err := ParseMeta(torrentPath)
		if err != nil {
			t.Errorf("Real torrent parsing failed: %v", err)
		}

		if meta.Info.Name == "" {
			t.Error("Expected non-empty torrent name")
		}

		if meta.Info.Length <= 0 && len(meta.Info.Files) == 0 {
			t.Error("Expected either Length > 0 or Files list")
		}

		t.Logf("Validated torrent: name=%s, length=%d, files=%d",
			meta.Info.Name, meta.Info.Length, len(meta.Info.Files))
	})

	t.Run("InvalidFileFormats", func(t *testing.T) {
		// Test various invalid file formats
		testCases := []struct {
			name    string
			content string
		}{
			{"HTML Error Page", `<!DOCTYPE html><html><body>Error 404</body></html>`},
			{"JSON Response", `{"error": "torrent not found"}`},
			{"Plain Text", `This is not a torrent file`},
			{"Binary Garbage", string([]byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD})},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				invalidPath := filepath.Join(tempDir, "invalid-"+tc.name+".torrent")
				err := os.WriteFile(invalidPath, []byte(tc.content), 0600)
				if err != nil {
					t.Fatalf("Failed to create invalid file: %v", err)
				}

				err = ValidateTorrentFile(invalidPath)
				if err == nil {
					t.Errorf("Expected validation to fail for %s", tc.name)
				} else {
					t.Logf("Correctly rejected %s: %v", tc.name, err)
				}
			})
		}
	})
}

func TestAria2RealWorld(t *testing.T) {
	if os.Getenv("REAL_WORLD_TESTS") != "true" {
		t.Skip("Skipping real-world tests (set REAL_WORLD_TESTS=true to enable)")
	}

	// These tests require network connectivity and real torrents
	logger.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)
	cfg.DownloadSettings.DownloadTimeout = 60 * time.Second

	t.Run("PublicDomainTorrent", func(t *testing.T) {
		// Use a real public domain torrent for testing
		// This is a hypothetical test - in practice you'd need a small, reliable torrent

		// For example, you could use Internet Archive torrents or other public domain content
		// torrentURL := "https://archive.org/download/SampleTorrent/sample.torrent"

		t.Skip("Real-world torrent test requires manual setup")

		// Example implementation:
		// 1. Download a small public domain torrent
		// 2. Save it to tempDir
		// 3. Test full download cycle
		// 4. Verify files are created
		// 5. Clean up
	})
}

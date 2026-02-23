package aria2

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jackpal/bencode-go"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

// writeBencode marshals data to a torrent file and returns the path.
func writeBencode(t *testing.T, dir, name string, data any) string {
	t.Helper()
	filePath := filepath.Join(dir, name+".torrent")
	f, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	defer f.Close()
	if err := bencode.Marshal(f, data); err != nil {
		t.Fatalf("bencode marshal: %v", err)
	}
	return filePath
}

func TestParseMeta(t *testing.T) {
	tests := []struct {
		name           string
		setupFile      func(t *testing.T, dir string) string
		expectedName   string
		expectedLength int64
		expectError    bool
		errorContains  string
	}{
		{
			name: "Valid single file torrent",
			setupFile: func(t *testing.T, dir string) string {
				return writeBencode(t, dir, "single-file", map[string]any{
					"announce": "http://tracker.example.com/announce",
					"info": map[string]any{
						"name":         "test.file",
						"length":       int64(1048576),
						"piece length": int64(32768),
						"pieces":       "01234567890123456789",
					},
				})
			},
			expectedName:   "test.file",
			expectedLength: 1048576,
			expectError:    false,
		},
		{
			name: "Valid multi-file torrent",
			setupFile: func(t *testing.T, dir string) string {
				return writeBencode(t, dir, "multi-file", map[string]any{
					"announce": "http://tracker.example.com/announce",
					"info": map[string]any{
						"name":         "multi-test",
						"piece length": int64(32768),
						"pieces":       "01234567890123456789",
						"files": []any{
							map[string]any{
								"length": int64(524288),
								"path":   []any{"folder1", "file1.txt"},
							},
							map[string]any{
								"length": int64(262144),
								"path":   []any{"folder2", "file2.txt"},
							},
						},
					},
				})
			},
			expectedName:   "multi-test",
			expectedLength: 0, // Multi-file torrents don't have length in root info
			expectError:    false,
		},
		{
			name: "File does not exist",
			setupFile: func(_ *testing.T, dir string) string {
				return filepath.Join(dir, "non-existent.torrent")
			},
			expectError:   true,
			errorContains: "failed to open torrent file",
		},
		{
			name: "Invalid bencode format",
			setupFile: func(t *testing.T, dir string) string {
				filePath := filepath.Join(dir, "invalid.torrent")
				if err := os.WriteFile(filePath, []byte("invalid bencode"), 0600); err != nil {
					t.Fatalf("Failed to create invalid torrent: %v", err)
				}
				return filePath
			},
			expectError:   true,
			errorContains: "failed to decode torrent meta",
		},
		{
			name: "Missing name field",
			setupFile: func(t *testing.T, dir string) string {
				return writeBencode(t, dir, "no-name", map[string]any{
					"announce": "http://tracker.example.com/announce",
					"info": map[string]any{
						"length":       int64(1024),
						"piece length": int64(32768),
						"pieces":       "01234567890123456789",
					},
				})
			},
			expectError:   true,
			errorContains: "torrent meta does not contain a file name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := testutils.TempDir(t)
			filePath := tt.setupFile(t, tempDir)

			meta, err := ParseMeta(filePath)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got no error", tt.errorContains)
				} else if tt.errorContains != "" && !containsError(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing '%s', but got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Errorf("Expected no error, but got: %v", err)
				return
			}

			if meta == nil {
				t.Error("Expected meta to be non-nil")
				return
			}

			if meta.Info.Name != tt.expectedName {
				t.Errorf("Expected name '%s', but got '%s'", tt.expectedName, meta.Info.Name)
			}

			if tt.expectedLength > 0 && meta.Info.Length != tt.expectedLength {
				t.Errorf("Expected length %d, but got %d", tt.expectedLength, meta.Info.Length)
			}
		})
	}
}

func TestAria2DownloaderParseTorrentMeta(t *testing.T) {
	tempDir := testutils.TempDir(t)

	// Use CreateRealTestTorrent which uses bencode.Marshal for valid format
	torrentPath := testutils.CreateRealTestTorrent(t, tempDir, "integration-test")

	downloader := &Aria2Downloader{
		downloadDir:     tempDir,
		torrentFileName: filepath.Base(torrentPath),
	}

	meta, err := downloader.parseTorrentMeta()
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
		return
	}

	if meta == nil {
		t.Error("Expected meta to be non-nil")
		return
	}

	if meta.Info.Name != "integration-test.txt" {
		t.Errorf("Expected name 'integration-test.txt', but got '%s'", meta.Info.Name)
	}
}

func TestMagnetDummyMetaExactLength(t *testing.T) {
	tempDir := testutils.TempDir(t)
	cfg := &config.Config{}

	// Magnet with xl= (exact length in bytes, BEP 9) — size should be used
	magnetWithXL := "magnet:?xt=urn:btih:1234567890abcdef1234567890abcdef12345678&dn=Test+Movie&xl=1234567890"
	magnetPath := filepath.Join(tempDir, "magnet_xl.magnet")
	if err := os.WriteFile(magnetPath, []byte(magnetWithXL), 0600); err != nil {
		t.Fatalf("write magnet file: %v", err)
	}

	dl := NewAria2Downloader(filepath.Base(magnetPath), tempDir, cfg).(*Aria2Downloader)
	size, err := dl.GetFileSize()
	if err != nil {
		t.Fatalf("GetFileSize: %v", err)
	}
	if size != 1234567890 {
		t.Errorf("expected size 1234567890 from xl=, got %d", size)
	}
}

//nolint:gocyclo // Test function validating torrent structures
func TestMetaStructure(t *testing.T) {
	tempDir := testutils.TempDir(t)

	t.Run("Single file structure", func(t *testing.T) {
		filePath := writeBencode(t, tempDir, "single", map[string]any{
			"announce": "http://tracker.example.com/announce",
			"info": map[string]any{
				"name":         "test.mp4",
				"length":       int64(2048),
				"piece length": int64(32768),
				"pieces":       "01234567890123456789",
			},
		})

		meta, err := ParseMeta(filePath)
		if err != nil {
			t.Fatalf("Failed to parse meta: %v", err)
		}

		if meta.Info.Name != "test.mp4" {
			t.Errorf("Expected name 'test.mp4', got '%s'", meta.Info.Name)
		}

		if meta.Info.Length != 2048 {
			t.Errorf("Expected length 2048, got %d", meta.Info.Length)
		}

		if len(meta.Info.Files) != 0 {
			t.Errorf("Expected empty files array for single file torrent, got %d files", len(meta.Info.Files))
		}
	})

	t.Run("Multi file structure", func(t *testing.T) {
		filePath := writeBencode(t, tempDir, "multi", map[string]any{
			"announce": "http://tracker.example.com/announce",
			"info": map[string]any{
				"name":         "test-folder",
				"piece length": int64(32768),
				"pieces":       "01234567890123456789",
				"files": []any{
					map[string]any{
						"length": int64(1024),
						"path":   []any{"file1"},
					},
					map[string]any{
						"length": int64(2048),
						"path":   []any{"subdir", "file2"},
					},
				},
			},
		})

		meta, err := ParseMeta(filePath)
		if err != nil {
			t.Fatalf("Failed to parse meta: %v", err)
		}

		if meta.Info.Name != "test-folder" {
			t.Errorf("Expected name 'test-folder', got '%s'", meta.Info.Name)
		}

		if meta.Info.Length != 0 {
			t.Errorf("Expected length 0 for multi-file torrent, got %d", meta.Info.Length)
		}

		if len(meta.Info.Files) != 2 {
			t.Errorf("Expected 2 files, got %d", len(meta.Info.Files))
		}

		if meta.Info.Files[0].Length != 1024 {
			t.Errorf("Expected first file length 1024, got %d", meta.Info.Files[0].Length)
		}

		if len(meta.Info.Files[0].Path) != 1 || meta.Info.Files[0].Path[0] != "file1" {
			t.Errorf("Expected first file path ['file1'], got %v", meta.Info.Files[0].Path)
		}

		if meta.Info.Files[1].Length != 2048 {
			t.Errorf("Expected second file length 2048, got %d", meta.Info.Files[1].Length)
		}

		if len(meta.Info.Files[1].Path) != 2 ||
			meta.Info.Files[1].Path[0] != "subdir" ||
			meta.Info.Files[1].Path[1] != "file2" {
			t.Errorf("Expected second file path ['subdir', 'file2'], got %v", meta.Info.Files[1].Path)
		}
	})
}

func TestSortedFileIndicesByPath(t *testing.T) {
	meta := &Meta{}
	meta.Info.Name = "Series"
	meta.Info.Files = []struct {
		Length int64    `bencode:"length"`
		Path   []string `bencode:"path"`
	}{
		{100, []string{"Episode 03.mkv"}},
		{200, []string{"Episode 01.mkv"}},
		{300, []string{"Episode 02.mkv"}},
	}

	indices, sizes, isVideo, totalSize := sortedFileIndicesByPath(meta)

	// Lexicographic by path: "Series/Episode 01.mkv" < "Series/Episode 02.mkv" < "Series/Episode 03.mkv"
	// Original aria2 indices: 1=Episode 03, 2=Episode 01, 3=Episode 02 → sorted: 2, 3, 1
	if want := []int{2, 3, 1}; len(indices) != len(want) || indices[0] != want[0] || indices[1] != want[1] || indices[2] != want[2] {
		t.Errorf("sortedFileIndicesByPath indices = %v, want %v", indices, want)
	}
	if want := []int64{200, 300, 100}; len(sizes) != len(want) || sizes[0] != want[0] || sizes[1] != want[1] || sizes[2] != want[2] {
		t.Errorf("sortedFileIndicesByPath sizes = %v, want %v", sizes, want)
	}
	if len(isVideo) != 3 || !isVideo[0] || !isVideo[1] || !isVideo[2] {
		t.Errorf("sortedFileIndicesByPath isVideo = %v, want [true, true, true] (all .mkv)", isVideo)
	}
	if totalSize != 600 {
		t.Errorf("sortedFileIndicesByPath totalSize = %d, want 600", totalSize)
	}
}

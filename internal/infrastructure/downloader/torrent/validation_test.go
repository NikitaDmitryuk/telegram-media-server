package aria2

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

func TestValidateContent(t *testing.T) {
	tests := []struct {
		name          string
		setupFile     func(t *testing.T, dir string) string
		expectedError string
	}{
		{
			name: "Valid torrent file",
			setupFile: func(t *testing.T, dir string) string {
				return testutils.CreateTestTorrent(t, dir, "valid-torrent")
			},
			expectedError: "",
		},
		{
			name: "HTML file instead of torrent",
			setupFile: func(t *testing.T, dir string) string {
				return testutils.CreateInvalidTorrent(t, dir, "html-file")
			},
			expectedError: "file appears to be HTML, not a torrent file",
		},
		{
			name: "Magnet link file",
			setupFile: func(t *testing.T, dir string) string {
				return testutils.CreateMagnetLink(t, dir, "magnet-link")
			},
			expectedError: "file appears to be a magnet link, not a torrent file",
		},
		{
			name: "Empty file",
			setupFile: func(t *testing.T, dir string) string {
				filePath := filepath.Join(dir, "empty.torrent")
				err := os.WriteFile(filePath, []byte{}, 0600)
				if err != nil {
					t.Fatalf("Failed to create empty file: %v", err)
				}
				return filePath
			},
			expectedError: "invalid torrent file format",
		},
		{
			name: "Invalid bencode",
			setupFile: func(t *testing.T, dir string) string {
				filePath := filepath.Join(dir, "invalid.torrent")
				err := os.WriteFile(filePath, []byte("invalid bencode data"), 0600)
				if err != nil {
					t.Fatalf("Failed to create invalid file: %v", err)
				}
				return filePath
			},
			expectedError: "invalid torrent file format",
		},
		{
			name: "Too small file",
			setupFile: func(t *testing.T, dir string) string {
				filePath := filepath.Join(dir, "tiny.torrent")
				err := os.WriteFile(filePath, []byte("d"), 0600)
				if err != nil {
					t.Fatalf("Failed to create tiny file: %v", err)
				}
				return filePath
			},
			expectedError: "invalid torrent file format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := testutils.TempDir(t)
			filePath := tt.setupFile(t, tempDir)

			// Read file content
			data, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("Failed to read test file: %v", err)
			}

			// Test validation
			err = ValidateContent(data, len(data))

			if tt.expectedError == "" {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got no error", tt.expectedError)
				} else if !containsError(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing '%s', but got: %v", tt.expectedError, err)
				}
			}
		})
	}
}

func TestValidateTorrentFile(t *testing.T) {
	tests := []struct {
		name          string
		setupFile     func(t *testing.T, dir string) string
		expectedError string
	}{
		{
			name: "Valid torrent file integration",
			setupFile: func(t *testing.T, dir string) string {
				return testutils.CreateTestTorrent(t, dir, "valid-integration")
			},
			expectedError: "",
		},
		{
			name: "File does not exist",
			setupFile: func(_ *testing.T, dir string) string {
				return filepath.Join(dir, "non-existent.torrent")
			},
			expectedError: "cannot open file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := testutils.TempDir(t)
			filePath := tt.setupFile(t, tempDir)

			// Create a minimal downloader instance for testing
			downloader := &Aria2Downloader{}
			err := downloader.validateTorrentFile(filePath)

			if tt.expectedError == "" {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got no error", tt.expectedError)
				} else if !containsError(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing '%s', but got: %v", tt.expectedError, err)
				}
			}
		})
	}
}

func TestIsClearlyHTML(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "HTML with DOCTYPE",
			content:  "<!DOCTYPE html><html><body>test</body></html>",
			expected: true,
		},
		{
			name:     "HTML with html tag",
			content:  "<html><body>test</body></html>",
			expected: true,
		},
		{
			name:     "HTML with head tag",
			content:  "<head><title>Test</title></head>",
			expected: true,
		},
		{
			name:     "HTML with body tag",
			content:  "<body>Test content</body>",
			expected: true,
		},
		{
			name:     "Valid bencode",
			content:  "d8:announce9:test-url4:infod4:name4:testee",
			expected: false,
		},
		{
			name:     "Random text",
			content:  "This is just random text without HTML tags",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isClearlyHTML(tt.content)
			if result != tt.expected {
				t.Errorf("isClearlyHTML(%q) = %v, expected %v", tt.content, result, tt.expected)
			}
		})
	}
}

func TestIsMagnetLink(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Standard magnet link",
			content:  "magnet:?xt=urn:btih:1234567890abcdef1234567890abcdef12345678&dn=test",
			expected: true,
		},
		{
			name:     "Magnet link with whitespace",
			content:  "  magnet:?xt=urn:btih:abcd  ",
			expected: true,
		},
		{
			name:     "Case insensitive magnet",
			content:  "MAGNET:?xt=urn:btih:1234",
			expected: true,
		},
		{
			name:     "Valid bencode",
			content:  "d8:announce9:test-url4:infod4:name4:testee",
			expected: false,
		},
		{
			name:     "HTML content",
			content:  "<!DOCTYPE html><html></html>",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMagnetLink(tt.content)
			if result != tt.expected {
				t.Errorf("isMagnetLink(%q) = %v, expected %v", tt.content, result, tt.expected)
			}
		})
	}
}

func TestHasValidBencodeStructure(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name:     "Valid torrent with announce",
			data:     []byte("d8:announce9:test-url4:infod4:name4:testee"),
			expected: true,
		},
		{
			name:     "Valid torrent with info only",
			data:     []byte("d4:infod4:name9:test-fileee"),
			expected: true,
		},
		{
			name:     "Bencode without torrent fields",
			data:     []byte("d4:spam5:eggse"),
			expected: false,
		},
		{
			name:     "Invalid bencode",
			data:     []byte("invalid data"),
			expected: false,
		},
		{
			name:     "HTML content",
			data:     []byte("<!DOCTYPE html>"),
			expected: false,
		},
		{
			name:     "Empty data",
			data:     []byte{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasValidBencodeStructure(tt.data, len(tt.data))
			if result != tt.expected {
				t.Errorf("hasValidBencodeStructure() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// Helper function to check if error message contains expected text
func containsError(actual, expected string) bool {
	return expected == "" || (actual != "" &&
		(actual == expected ||
			len(actual) >= len(expected) &&
				actual[:len(expected)] == expected))
}

package common

import (
	"testing"
)

func TestIsValidLink(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "Valid HTTP URL",
			input:    "http://example.com",
			expected: true,
		},
		{
			name:     "Valid HTTPS URL",
			input:    "https://example.com",
			expected: true,
		},
		{
			name:     "Valid URL with path",
			input:    "https://example.com/path/to/resource",
			expected: true,
		},
		{
			name:     "Valid URL with query parameters",
			input:    "https://example.com/search?q=test&category=video",
			expected: true,
		},
		{
			name:     "Valid YouTube URL",
			input:    "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			expected: true,
		},
		{
			name:     "Invalid URL without protocol",
			input:    "example.com",
			expected: false,
		},
		{
			name:     "Invalid URL with FTP protocol",
			input:    "ftp://example.com",
			expected: false,
		},
		{
			name:     "Empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "Invalid URL with spaces",
			input:    "https://example.com/path with spaces",
			expected: false,
		},
		{
			name:     "Invalid URL with newlines",
			input:    "https://example.com\nmalicious.com",
			expected: false,
		},
		{
			name:     "Magnet link",
			input:    "magnet:?xt=urn:btih:example",
			expected: true,
		},
		{
			name:     "Magnet link with leading and trailing spaces",
			input:    "  magnet:?xt=urn:btih:abc123&dn=Test  ",
			expected: true,
		},
		{
			name:     "Local file path",
			input:    "/path/to/file.txt",
			expected: false,
		},
		{
			name:     "Just text",
			input:    "this is just text",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidLink(tt.input)
			if result != tt.expected {
				t.Errorf("IsValidLink(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractLink(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantFound bool
	}{
		{
			name:      "Plain HTTPS URL",
			input:     "https://example.com/movie",
			want:      "https://example.com/movie",
			wantFound: true,
		},
		{
			name:      "URL inside Russian text",
			input:     "Скачай пожалуйста https://youtu.be/dQw4w9WgXcQ когда будет время",
			want:      "https://youtu.be/dQw4w9WgXcQ",
			wantFound: true,
		},
		{
			name:      "URL with query params",
			input:     "нашел https://example.com/watch?v=abc123&category=video",
			want:      "https://example.com/watch?v=abc123&category=video",
			wantFound: true,
		},
		{
			name:      "URL in angle brackets",
			input:     "download <https://example.com/file.torrent>",
			want:      "https://example.com/file.torrent",
			wantFound: true,
		},
		{
			name:      "URL in quotes",
			input:     `download "https://example.com/file.torrent"`,
			want:      "https://example.com/file.torrent",
			wantFound: true,
		},
		{
			name:      "URL with trailing punctuation",
			input:     "вот ссылка https://example.com/movie.",
			want:      "https://example.com/movie",
			wantFound: true,
		},
		{
			name:      "Magnet link",
			input:     "вот magnet:?xt=urn:btih:abcdef1234567890&dn=Movie",
			want:      "magnet:?xt=urn:btih:abcdef1234567890&dn=Movie",
			wantFound: true,
		},
		{
			name:      "First supported URL wins",
			input:     "first https://example.com/one second https://example.com/two",
			want:      "https://example.com/one",
			wantFound: true,
		},
		{
			name:      "No URL",
			input:     "this is just text",
			wantFound: false,
		},
		{
			name:      "Unsupported scheme",
			input:     "download ftp://example.com/file",
			wantFound: false,
		},
		{
			name:      "Domain without scheme",
			input:     "download example.com/file",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := ExtractLink(tt.input)
			if found != tt.wantFound {
				t.Fatalf("ExtractLink(%q) found = %v, want %v", tt.input, found, tt.wantFound)
			}
			if got != tt.want {
				t.Fatalf("ExtractLink(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsTorrentFile(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		expected bool
	}{
		{
			name:     "Valid torrent file",
			fileName: "example.torrent",
			expected: true,
		},
		{
			name:     "Valid torrent file with path",
			fileName: "/path/to/file.torrent",
			expected: true,
		},
		{
			name:     "Valid torrent file with complex name",
			fileName: "Ubuntu-20.04-desktop-amd64.torrent",
			expected: true,
		},
		{
			name:     "Not a torrent file - mp4",
			fileName: "video.mp4",
			expected: false,
		},
		{
			name:     "Not a torrent file - txt",
			fileName: "document.txt",
			expected: false,
		},
		{
			name:     "Empty string",
			fileName: "",
			expected: false,
		},
		{
			name:     "Just .torrent extension",
			fileName: ".torrent",
			expected: true,
		},
		{
			name:     "Torrent in filename but not extension",
			fileName: "torrent-file.zip",
			expected: false,
		},
		{
			name:     "Case sensitivity test",
			fileName: "file.TORRENT",
			expected: false,
		},
		{
			name:     "Multiple extensions",
			fileName: "file.tar.torrent",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTorrentFile(tt.fileName)
			if result != tt.expected {
				t.Errorf("IsTorrentFile(%q) = %v, expected %v", tt.fileName, result, tt.expected)
			}
		})
	}
}

func BenchmarkIsValidLink(b *testing.B) {
	testURL := "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

	b.ResetTimer()
	for range b.N {
		IsValidLink(testURL)
	}
}

func BenchmarkIsTorrentFile(b *testing.B) {
	fileName := "Ubuntu-20.04-desktop-amd64.torrent"

	b.ResetTimer()
	for range b.N {
		IsTorrentFile(fileName)
	}
}

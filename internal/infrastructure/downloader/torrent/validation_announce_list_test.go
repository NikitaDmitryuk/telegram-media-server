package aria2

import (
	"testing"
)

func TestValidateContentWithAnnounceList(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Valid torrent with announce field",
			content:  "d8:announce22:http://tracker.example.com4:infod4:name9:test.file12:piece lengthi32768e6:pieces20:xxxxxxxxxxxxxxxxxxee",
			expected: true,
		},
		{
			name:     "Valid torrent with announce-list field",
			content:  "d13:announce-listll22:http://tracker.example.comel23:http://tracker2.example.comee4:infod4:name9:test.file12:piece lengthi32768e6:pieces20:xxxxxxxxxxxxxxxxxxee",
			expected: true,
		},
		{
			name:     "Valid torrent with both announce and announce-list",
			content:  "d8:announce22:http://tracker.example.com13:announce-listll22:http://tracker.example.comel23:http://tracker2.example.comee4:infod4:name9:test.file12:piece lengthi32768e6:pieces20:xxxxxxxxxxxxxxxxxxee",
			expected: true,
		},
		{
			name:     "Invalid content - not a torrent",
			content:  "This is not a torrent file",
			expected: false,
		},
		{
			name:     "Invalid content - HTML",
			content:  "<html><head><title>Error</title></head><body>Not found</body></html>",
			expected: false,
		},
		{
			name:     "Invalid content - magnet link",
			content:  "magnet:?xt=urn:btih:1234567890abcdef1234567890abcdef12345678",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(tt.content)
			result := hasValidBencodeStructure(data, len(data))

			if result != tt.expected {
				t.Errorf(
					"hasValidBencodeStructure() = %v, expected %v for content: %s",
					result,
					tt.expected,
					tt.content[:minInt(50, len(tt.content))],
				)
			}
		})
	}
}

func TestValidateContentIntegration(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "Valid torrent with announce-list only",
			content: "d13:announce-listll22:http://tracker.example.comel23:http://tracker2.example.comee4:infod4:name9:test.file12:piece lengthi32768e6:pieces20:xxxxxxxxxxxxxxxxxxee",
			wantErr: false,
		},
		{
			name:    "Valid torrent with announce only",
			content: "d8:announce22:http://tracker.example.com4:infod4:name9:test.file12:piece lengthi32768e6:pieces20:xxxxxxxxxxxxxxxxxxee",
			wantErr: false,
		},
		{
			name:    "Invalid - no torrent patterns",
			content: "d4:test5:valuee",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(tt.content)
			err := ValidateContent(data, len(data))

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateContent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

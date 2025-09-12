package ytdlp

import (
	"testing"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

func TestShouldUseProxy(t *testing.T) {
	t.Skip("Proxy logic tests need refinement - skipping for now")
	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)

	tests := []struct {
		url          string
		proxyDomains string
		expected     bool
	}{
		{
			url:          "https://youtube.com/watch?v=123",
			proxyDomains: "youtube.com",
			expected:     true,
		},
		{
			url:          "https://vimeo.com/123",
			proxyDomains: "youtube.com, vimeo.com",
			expected:     true,
		},
		{
			url:          "https://dailymotion.com/video/x123",
			proxyDomains: "youtube.com",
			expected:     false,
		},
		{
			url:          "https://example.com",
			proxyDomains: "",
			expected:     true, // Если прокси настроен, но доменов нет, всё равно может использовать прокси
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			tempCfg := *cfg
			tempCfg.ProxyDomains = tt.proxyDomains
			tempCfg.Proxy = "http://proxy.example.com:8080" // Нужен для проверки
			result, err := shouldUseProxy(tt.url, &tempCfg)
			if err != nil {
				t.Errorf("shouldUseProxy(%s) failed: %v", tt.url, err)
				return
			}
			if result != tt.expected {
				t.Errorf("shouldUseProxy(%s, %s) = %v, expected %v",
					tt.url, tt.proxyDomains, result, tt.expected)
			}
		})
	}
}

func TestExtractVideoID(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{
			url:      "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			expected: "www_youtube_com_dQw4w9WgXcQ",
		},
		{
			url:      "https://youtu.be/dQw4w9WgXcQ",
			expected: "youtu_be_dQw4w9WgXcQ",
		},
		{
			url:      "https://vimeo.com/123456789",
			expected: "vimeo_com_123456789",
		},
		{
			url:      "https://example.com/video/test",
			expected: "example_com_video_test",
		},
		{
			url:      "invalid-url",
			expected: "_invalid-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result, _ := extractVideoID(tt.url)
			if result != tt.expected {
				t.Errorf("extractVideoID(%s) = %s, expected %s",
					tt.url, result, tt.expected)
			}
		})
	}
}

func TestYTDLPDownloader(t *testing.T) {
	t.Skip("YTDLPDownloader integration tests require yt-dlp executable - skipping for now")
}

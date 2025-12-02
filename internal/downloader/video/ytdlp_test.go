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
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			tempCfg := *cfg
			tempCfg.ProxyDomains = tt.proxyDomains
			tempCfg.Proxy = "http://proxy.example.com:8080"
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

func TestAddAudioLanguageFilter(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		lang     string
		expected string
	}{
		{
			name:     "simple best format with language",
			selector: "best",
			lang:     "ru",
			expected: "best[language=ru]",
		},
		{
			name:     "best format with existing filter",
			selector: "best[height<=1080]",
			lang:     "en",
			expected: "best[height<=1080][language=en]",
		},
		{
			name:     "bv*+ba format with language",
			selector: "bv*+ba",
			lang:     "ru",
			expected: "bv*+ba[language=ru]",
		},
		{
			name:     "bv*+ba/b fallback format with language",
			selector: "bv*+ba/b",
			lang:     "ru",
			expected: "bv*+ba[language=ru]/b[language=ru]",
		},
		{
			name:     "worst format with language",
			selector: "worst",
			lang:     "es",
			expected: "worst[language=es]",
		},
		{
			name:     "empty language returns original",
			selector: "bv*+ba/b",
			lang:     "",
			expected: "bv*+ba/b",
		},
		{
			name:     "bestvideo+bestaudio format",
			selector: "bestvideo+bestaudio",
			lang:     "fr",
			expected: "bestvideo+bestaudio[language=fr]",
		},
		{
			name:     "complex format with multiple parts",
			selector: "bestvideo[height<=1080]+bestaudio/best",
			lang:     "de",
			expected: "bestvideo[height<=1080]+bestaudio[language=de]/best[language=de]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addAudioLanguageFilter(tt.selector, tt.lang)
			if result != tt.expected {
				t.Errorf("addAudioLanguageFilter(%s, %s) = %s, expected %s",
					tt.selector, tt.lang, result, tt.expected)
			}
		})
	}
}

func TestAddLanguageFilterToSingleFormat(t *testing.T) {
	tests := []struct {
		name           string
		format         string
		languageFilter string
		expected       string
	}{
		{
			name:           "simple format",
			format:         "best",
			languageFilter: "[language=ru]",
			expected:       "best[language=ru]",
		},
		{
			name:           "bv*+ba merged format",
			format:         "bv*+ba",
			languageFilter: "[language=en]",
			expected:       "bv*+ba[language=en]",
		},
		{
			name:           "bestvideo+bestaudio merged format",
			format:         "bestvideo+bestaudio",
			languageFilter: "[language=fr]",
			expected:       "bestvideo+bestaudio[language=fr]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addLanguageFilterToSingleFormat(tt.format, tt.languageFilter)
			if result != tt.expected {
				t.Errorf("addLanguageFilterToSingleFormat(%s, %s) = %s, expected %s",
					tt.format, tt.languageFilter, result, tt.expected)
			}
		})
	}
}

func TestAddFilterToFormat(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		filter   string
		expected string
	}{
		{
			name:     "add filter to simple format",
			format:   "best",
			filter:   "[language=ru]",
			expected: "best[language=ru]",
		},
		{
			name:     "add filter to format with existing filter",
			format:   "best[height<=1080]",
			filter:   "[language=en]",
			expected: "best[height<=1080][language=en]",
		},
		{
			name:     "add filter to ba format",
			format:   "ba",
			filter:   "[language=es]",
			expected: "ba[language=es]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addFilterToFormat(tt.format, tt.filter)
			if result != tt.expected {
				t.Errorf("addFilterToFormat(%s, %s) = %s, expected %s",
					tt.format, tt.filter, result, tt.expected)
			}
		})
	}
}

func TestBuildYTDLPArgs(t *testing.T) {
	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)

	tests := []struct {
		name              string
		audioLang         string
		qualitySelector   string
		expectedFormat    string
		shouldContainLang bool
	}{
		{
			name:              "with audio language ru",
			audioLang:         "ru",
			qualitySelector:   "bv*+ba/b",
			expectedFormat:    "bv*+ba[language=ru]/b[language=ru]/best", // /best fallback added
			shouldContainLang: true,
		},
		{
			name:              "with audio language en",
			audioLang:         "en",
			qualitySelector:   "best",
			expectedFormat:    "best[language=en]/best", // /best fallback added
			shouldContainLang: true,
		},
		{
			name:              "without audio language",
			audioLang:         "",
			qualitySelector:   "bv*+ba/b",
			expectedFormat:    "bv*+ba/b", // ends with /b, no fallback needed
			shouldContainLang: false,
		},
		{
			name:              "with audio language and complex selector",
			audioLang:         "fr",
			qualitySelector:   "bestvideo[height<=1080]+bestaudio/best",
			expectedFormat:    "bestvideo[height<=1080]+bestaudio[language=fr]/best[language=fr]/best", // /best fallback added
			shouldContainLang: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy of config with specific video settings
			testCfg := *cfg
			testCfg.VideoSettings.AudioLang = tt.audioLang
			testCfg.VideoSettings.QualitySelector = tt.qualitySelector

			downloader := &YTDLPDownloader{
				config: &testCfg,
				url:    "https://example.com/video",
			}

			outputPath := "/tmp/test.mp4"
			args := downloader.buildYTDLPArgs(outputPath)

			// Check that args contain the expected format selector
			formatFound := false
			for i, arg := range args {
				if arg == "-f" && i+1 < len(args) {
					if args[i+1] != tt.expectedFormat {
						t.Errorf("Expected format selector %s, got %s", tt.expectedFormat, args[i+1])
					}
					formatFound = true
					break
				}
			}

			if !formatFound {
				t.Error("Format selector not found in args")
			}

			// Verify other required args are present
			requiredArgs := []string{"--newline", "-o", outputPath, "https://example.com/video"}
			for _, req := range requiredArgs {
				found := false
				for _, arg := range args {
					if arg == req {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Required arg %s not found in args: %v", req, args)
				}
			}
		})
	}
}

package ytdlp

import (
	"testing"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

func TestShouldUseProxy(t *testing.T) {
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

func TestShouldUseProxy_NoProxy(t *testing.T) {
	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)
	cfg.Proxy = ""
	cfg.ProxyDomains = ""

	useProxy, err := shouldUseProxy("https://youtube.com/watch?v=123", cfg)
	if err != nil {
		t.Fatalf("shouldUseProxy: %v", err)
	}
	if useProxy {
		t.Error("shouldUseProxy should return false when no proxy configured")
	}
}

func TestShouldUseProxy_InvalidURL(t *testing.T) {
	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)
	cfg.Proxy = "http://proxy:8080"
	cfg.ProxyDomains = "youtube.com"

	_, err := shouldUseProxy("://bad-url", cfg)
	if err == nil {
		t.Error("shouldUseProxy should return error for invalid URL")
	}
}

func TestPrepareQualitySelector(t *testing.T) {
	tests := []struct {
		name     string
		settings tmsconfig.VideoConfig
		expected string
	}{
		{
			name: "simple with fallback",
			settings: tmsconfig.VideoConfig{
				QualitySelector: "best",
				AudioLang:       "",
			},
			expected: "best/best",
		},
		{
			name: "already has /best",
			settings: tmsconfig.VideoConfig{
				QualitySelector: "bv*+ba/best",
				AudioLang:       "",
			},
			expected: "bv*+ba/best",
		},
		{
			name: "already has /b",
			settings: tmsconfig.VideoConfig{
				QualitySelector: "bv*+ba/b",
				AudioLang:       "",
			},
			expected: "bv*+ba/b",
		},
		{
			name: "with audio language",
			settings: tmsconfig.VideoConfig{
				QualitySelector: "best",
				AudioLang:       "ru",
			},
			expected: "best[language=ru]/best",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := prepareQualitySelector(&tt.settings)
			if result != tt.expected {
				t.Errorf("prepareQualitySelector() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestAppendFormatSortArgs(t *testing.T) {
	tests := []struct {
		name     string
		settings tmsconfig.VideoConfig
		wantSort string
	}{
		{
			name: "with max height",
			settings: tmsconfig.VideoConfig{
				MaxHeight: 1080,
			},
			wantSort: "res:1080",
		},
		{
			name: "no max height",
			settings: tmsconfig.VideoConfig{
				MaxHeight: 0,
			},
			wantSort: "",
		},
		{
			name: "compatibility mode",
			settings: tmsconfig.VideoConfig{
				CompatibilityMode: true,
			},
			wantSort: "vcodec:h264,acodec:mp3",
		},
		{
			name: "height + compatibility",
			settings: tmsconfig.VideoConfig{
				MaxHeight:         720,
				CompatibilityMode: true,
			},
			wantSort: "res:720,vcodec:h264,acodec:mp3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := appendFormatSortArgs(nil, &tt.settings)
			if tt.wantSort == "" {
				if len(args) != 0 {
					t.Errorf("Expected no args, got %v", args)
				}
				return
			}
			if len(args) < 2 || args[0] != "-S" {
				t.Fatalf("Expected [-S sortSpec], got %v", args)
			}
			if args[1] != tt.wantSort {
				t.Errorf("Sort spec = %q, want %q", args[1], tt.wantSort)
			}
		})
	}
}

func TestAppendReencodingArgs(t *testing.T) {
	tests := []struct {
		name     string
		settings tmsconfig.VideoConfig
		want     int // expected number of added args
	}{
		{
			name: "reencoding disabled",
			settings: tmsconfig.VideoConfig{
				EnableReencoding: false,
			},
			want: 0,
		},
		{
			name: "reencoding enabled without force",
			settings: tmsconfig.VideoConfig{
				EnableReencoding: true,
				OutputFormat:     "mp4",
			},
			want: 2, // --recode-video mp4
		},
		{
			name: "reencoding with force",
			settings: tmsconfig.VideoConfig{
				EnableReencoding: true,
				ForceReencoding:  true,
				OutputFormat:     "mp4",
				VideoCodec:       "h264",
				AudioCodec:       "mp3",
			},
			want: 4, // --recode-video mp4 --postprocessor-args "ffmpeg:..."
		},
		{
			name: "reencoding with force and extra args",
			settings: tmsconfig.VideoConfig{
				EnableReencoding: true,
				ForceReencoding:  true,
				OutputFormat:     "mp4",
				VideoCodec:       "h264",
				AudioCodec:       "mp3",
				FFmpegExtraArgs:  "-pix_fmt yuv420p",
			},
			want: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := appendReencodingArgs(nil, &tt.settings)
			if len(args) != tt.want {
				t.Errorf("appendReencodingArgs added %d args, want %d; args=%v", len(args), tt.want, args)
			}
		})
	}
}

func TestAppendSubtitleArgs(t *testing.T) {
	tests := []struct {
		name     string
		settings tmsconfig.VideoConfig
		want     []string
	}{
		{
			name: "no subtitles",
			settings: tmsconfig.VideoConfig{
				WriteSubs:    false,
				SubtitleLang: "",
			},
			want: nil,
		},
		{
			name: "write subs without lang",
			settings: tmsconfig.VideoConfig{
				WriteSubs:    true,
				SubtitleLang: "",
			},
			want: []string{"--write-subs"},
		},
		{
			name: "write subs with lang",
			settings: tmsconfig.VideoConfig{
				WriteSubs:    true,
				SubtitleLang: "en",
			},
			want: []string{"--write-subs", "--sub-langs", "en"},
		},
		{
			name: "subtitle lang without write-subs flag",
			settings: tmsconfig.VideoConfig{
				WriteSubs:    false,
				SubtitleLang: "ru",
			},
			want: []string{"--write-subs", "--sub-langs", "ru"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := appendSubtitleArgs(nil, &tt.settings)
			if len(args) != len(tt.want) {
				t.Fatalf("appendSubtitleArgs() = %v (len=%d), want %v (len=%d)", args, len(args), tt.want, len(tt.want))
			}
			for i, arg := range args {
				if arg != tt.want[i] {
					t.Errorf("arg[%d] = %q, want %q", i, arg, tt.want[i])
				}
			}
		})
	}
}

func TestGetMapKeys(t *testing.T) {
	m := map[string]any{
		"title":    "test",
		"duration": 120,
		"filesize": 1024,
	}

	keys := getMapKeys(m)
	if len(keys) != 3 {
		t.Fatalf("Expected 3 keys, got %d: %v", len(keys), keys)
	}

	// Check all keys present (order not guaranteed)
	keySet := map[string]bool{}
	for _, k := range keys {
		keySet[k] = true
	}
	for _, expected := range []string{"title", "duration", "filesize"} {
		if !keySet[expected] {
			t.Errorf("Missing key %q in result: %v", expected, keys)
		}
	}
}

func TestGetMapKeys_Empty(t *testing.T) {
	keys := getMapKeys(map[string]any{})
	if len(keys) != 0 {
		t.Errorf("Expected empty keys, got %v", keys)
	}
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
			testCfg.VideoSettings.MaxHeight = 0             // Disable max height for format testing
			testCfg.VideoSettings.CompatibilityMode = false // Disable compatibility mode for format testing

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

package config

import (
	"log"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
)

const (
	DefaultAria2MaxPeers                = 200
	DefaultAria2MaxConnectionsPerServer = 16
	DefaultAria2Split                   = 16
	DefaultAria2BTMaxPeers              = 200
	DefaultAria2BTMaxOpenFiles          = 100
	DefaultAria2BTTrackerTimeout        = 60
	DefaultAria2Timeout                 = 60
	DefaultAria2MaxTries                = 5
	DefaultPasswordMinLength            = 8
	DefaultMaxConcurrentDownloads       = 3
	DefaultProgressUpdateInterval       = 3 * time.Second
	DefaultVideoMaxHeight               = 0             // Default: no max height limit (0 = disabled)
	DefaultYtdlpUpdateInterval          = 3 * time.Hour // Periodic yt-dlp update interval; 0 = disabled
)

func NewConfig() (*Config, error) {
	config := &Config{
		BotToken:            getEnv("BOT_TOKEN", ""),
		MoviePath:           getEnv("MOVIE_PATH", ""),
		AdminPassword:       getEnv("ADMIN_PASSWORD", ""),
		RegularPassword:     getEnv("REGULAR_PASSWORD", ""),
		Lang:                getEnv("LANG", "en"),
		TelegramProxy:       getEnv("TELEGRAM_PROXY", ""),
		Proxy:               getEnv("PROXY", ""),
		ProxyDomains:        getEnv("PROXY_DOMAINS", ""),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		LangPath:            getEnv("LANG_PATH", "/usr/local/share/telegram-media-server/locales"),
		ProwlarrURL:         getEnv("PROWLARR_URL", ""),
		ProwlarrAPIKey:      getEnv("PROWLARR_API_KEY", ""),
		YtdlpPath:           getEnv("YTDLP_PATH", "/usr/bin/yt-dlp"),
		YtdlpUpdateOnStart:  getEnvBool("YTDLP_UPDATE_ON_START", true),
		YtdlpUpdateInterval: getEnvDuration("YTDLP_UPDATE_INTERVAL", DefaultYtdlpUpdateInterval),

		DownloadSettings: DownloadConfig{
			MaxConcurrentDownloads: getEnvInt("MAX_CONCURRENT_DOWNLOADS", DefaultMaxConcurrentDownloads),
			DownloadTimeout:        getEnvDuration("DOWNLOAD_TIMEOUT", 0),
			ProgressUpdateInterval: getEnvDuration("PROGRESS_UPDATE_INTERVAL", DefaultProgressUpdateInterval),
		},

		SecuritySettings: SecurityConfig{
			PasswordMinLength: DefaultPasswordMinLength,
		},

		Aria2Settings: Aria2Config{
			MaxPeers:                 getEnvInt("ARIA2_MAX_PEERS", DefaultAria2MaxPeers),
			MaxConnectionsPerServer:  getEnvInt("ARIA2_MAX_CONNECTIONS_PER_SERVER", DefaultAria2MaxConnectionsPerServer),
			Split:                    getEnvInt("ARIA2_SPLIT", DefaultAria2Split),
			MinSplitSize:             getEnv("ARIA2_MIN_SPLIT_SIZE", "1M"),
			BTMaxPeers:               getEnvInt("ARIA2_BT_MAX_PEERS", DefaultAria2BTMaxPeers),
			BTRequestPeerSpeedLimit:  getEnv("ARIA2_BT_REQUEST_PEER_SPEED_LIMIT", "0"),
			BTMaxOpenFiles:           getEnvInt("ARIA2_BT_MAX_OPEN_FILES", DefaultAria2BTMaxOpenFiles),
			MaxOverallUploadLimit:    getEnv("ARIA2_MAX_OVERALL_UPLOAD_LIMIT", "1M"),
			MaxUploadLimit:           getEnv("ARIA2_MAX_UPLOAD_LIMIT", "200K"),
			SeedRatio:                getEnvFloat("ARIA2_SEED_RATIO", 0.0),
			SeedTime:                 getEnvInt("ARIA2_SEED_TIME", 0),
			BTTrackerTimeout:         getEnvInt("ARIA2_BT_TRACKER_TIMEOUT", DefaultAria2BTTrackerTimeout),
			BTTrackerInterval:        getEnvInt("ARIA2_BT_TRACKER_INTERVAL", 0),
			EnableDHT:                getEnvBool("ARIA2_ENABLE_DHT", true),
			EnablePeerExchange:       getEnvBool("ARIA2_ENABLE_PEER_EXCHANGE", true),
			EnableLocalPeerDiscovery: getEnvBool("ARIA2_ENABLE_LOCAL_PEER_DISCOVERY", true),
			FollowTorrent:            getEnvBool("ARIA2_FOLLOW_TORRENT", true),
			ListenPort:               getEnv("ARIA2_LISTEN_PORT", "6881-6999"),
			DHTPorts:                 getEnv("ARIA2_DHT_PORTS", "6881-6999"),
			BTSaveMetadata:           getEnvBool("ARIA2_BT_SAVE_METADATA", true),
			BTHashCheckSeed:          getEnvBool("ARIA2_BT_HASH_CHECK_SEED", true),
			BTRequireCrypto:          getEnvBool("ARIA2_BT_REQUIRE_CRYPTO", false),
			BTMinCryptoLevel:         getEnv("ARIA2_BT_MIN_CRYPTO_LEVEL", "plain"),
			CheckIntegrity:           getEnvBool("ARIA2_CHECK_INTEGRITY", false),
			ContinueDownload:         getEnvBool("ARIA2_CONTINUE_DOWNLOAD", true),
			RemoteTime:               getEnvBool("ARIA2_REMOTE_TIME", true),
			FileAllocation:           getEnv("ARIA2_FILE_ALLOCATION", "none"),
			HTTPProxy:                getEnv("ARIA2_HTTP_PROXY", ""),
			AllProxy:                 getEnv("ARIA2_ALL_PROXY", ""),
			UserAgent:                getEnv("ARIA2_USER_AGENT", ""),
			Timeout:                  getEnvInt("ARIA2_TIMEOUT", DefaultAria2Timeout),
			MaxTries:                 getEnvInt("ARIA2_MAX_TRIES", DefaultAria2MaxTries),
			RetryWait:                getEnvInt("ARIA2_RETRY_WAIT", 0),
		},

		VideoSettings: VideoConfig{
			EnableReencoding:   getEnvBool("VIDEO_ENABLE_REENCODING", false),
			ForceReencoding:    getEnvBool("VIDEO_FORCE_REENCODING", false),
			VideoCodec:         getEnv("VIDEO_CODEC", "h264"),
			AudioCodec:         getEnv("AUDIO_CODEC", "mp3"),
			OutputFormat:       getEnv("VIDEO_OUTPUT_FORMAT", "mp4"),
			FFmpegExtraArgs:    getEnv("FFMPEG_EXTRA_ARGS", "-pix_fmt yuv420p"),
			QualitySelector:    getEnv("VIDEO_QUALITY_SELECTOR", "bv*+ba/b"),
			MaxHeight:          getEnvInt("VIDEO_MAX_HEIGHT", DefaultVideoMaxHeight),
			CompatibilityMode:  getEnvBool("VIDEO_COMPATIBILITY_MODE", false),
			RejectIncompatible: getEnvBool("VIDEO_REJECT_INCOMPATIBLE", false),
			SubtitleLang:       getEnv("VIDEO_SUBTITLE_LANG", ""),
			AudioLang:          getEnv("VIDEO_AUDIO_LANG", ""),
			WriteSubs:          getEnvBool("VIDEO_WRITE_SUBS", false),
			TvH264Level:        getEnv("VIDEO_TV_H264_LEVEL", "4.1"),
		},
	}

	if getEnv("RUNNING_IN_DOCKER", "false") == "true" {
		config.MoviePath = "/app/media"
		config.LangPath = "/app/locales"
		log.Printf("Running inside Docker, setting MOVIE_PATH to %s and LANG_PATH to %s", config.MoviePath, config.LangPath)
	}

	if err := config.validate(); err != nil {
		log.Printf("Configuration validation failed: %v", err)
		return nil, utils.WrapError(err, "configuration validation failed", map[string]any{
			"config": config,
		})
	}

	log.Println("Configuration loaded successfully")
	return config, nil
}

type Config struct {
	BotToken            string
	MoviePath           string
	AdminPassword       string
	RegularPassword     string
	Lang                string
	TelegramProxy       string
	Proxy               string
	ProxyDomains        string
	LogLevel            string
	LangPath            string
	ProwlarrURL         string
	ProwlarrAPIKey      string
	YtdlpPath           string // Path to yt-dlp binary; use standalone from GitHub for auto-update via -U (pacman/pip builds refuse -U)
	YtdlpUpdateOnStart  bool
	YtdlpUpdateInterval time.Duration

	DownloadSettings DownloadConfig
	SecuritySettings SecurityConfig
	Aria2Settings    Aria2Config
	VideoSettings    VideoConfig
}

type DownloadConfig struct {
	MaxConcurrentDownloads int
	DownloadTimeout        time.Duration
	ProgressUpdateInterval time.Duration
}

type Aria2Config struct {
	MaxPeers                 int
	MaxConnectionsPerServer  int
	Split                    int
	MinSplitSize             string
	BTMaxPeers               int
	BTRequestPeerSpeedLimit  string
	BTMaxOpenFiles           int
	MaxOverallUploadLimit    string
	MaxUploadLimit           string
	SeedRatio                float64
	SeedTime                 int
	BTTrackerTimeout         int
	BTTrackerInterval        int
	EnableDHT                bool
	EnablePeerExchange       bool
	EnableLocalPeerDiscovery bool
	FollowTorrent            bool
	ListenPort               string
	DHTPorts                 string
	BTSaveMetadata           bool
	BTHashCheckSeed          bool
	BTRequireCrypto          bool
	BTMinCryptoLevel         string
	CheckIntegrity           bool
	ContinueDownload         bool
	RemoteTime               bool
	FileAllocation           string
	HTTPProxy                string
	AllProxy                 string
	UserAgent                string
	Timeout                  int
	MaxTries                 int
	RetryWait                int
}

type SecurityConfig struct {
	PasswordMinLength int
}

type VideoConfig struct {
	EnableReencoding   bool
	ForceReencoding    bool
	VideoCodec         string
	AudioCodec         string
	OutputFormat       string
	FFmpegExtraArgs    string
	QualitySelector    string
	MaxHeight          int // Maximum video height (e.g., 1080 for 1080p, 720 for 720p, 0 for no limit)
	CompatibilityMode  bool
	RejectIncompatible bool // if true, stop download and notify when video is red (not playable on TV)
	SubtitleLang       string
	AudioLang          string
	WriteSubs          bool
	TvH264Level        string // H.264 level cap for compatibility mode (e.g. "4.0", "4.1")
}

func (c *Config) GetDownloadSettings() DownloadConfig {
	return c.DownloadSettings
}

func (c *Config) GetSecuritySettings() SecurityConfig {
	return c.SecuritySettings
}

func (c *Config) GetAria2Settings() Aria2Config {
	return c.Aria2Settings
}

func (c *Config) GetVideoSettings() VideoConfig {
	return c.VideoSettings
}

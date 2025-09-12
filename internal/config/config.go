package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
)

const (
	DefaultDownloadTimeout        = 0 // 0 means no timeout (infinite)
	DefaultProgressUpdateInterval = 3 * time.Second
	DefaultPasswordMinLength      = 8

	// Aria2 default values
	DefaultAria2MaxPeers                = 200
	DefaultAria2MaxConnectionsPerServer = 16
	DefaultAria2Split                   = 16
	DefaultAria2BTMaxPeers              = 200
	DefaultAria2BTMaxOpenFiles          = 100
	DefaultAria2BTTrackerTimeout        = 60
	DefaultAria2Timeout                 = 60
	DefaultAria2MaxTries                = 5
)

func NewConfig() (*Config, error) {
	config := &Config{
		BotToken:        getEnv("BOT_TOKEN", ""),
		MoviePath:       getEnv("MOVIE_PATH", ""),
		AdminPassword:   getEnv("ADMIN_PASSWORD", ""),
		RegularPassword: getEnv("REGULAR_PASSWORD", ""),
		Lang:            getEnv("LANG", "en"),
		Proxy:           getEnv("PROXY", ""),
		ProxyDomains:    getEnv("PROXY_DOMAINS", ""),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		LangPath:        getEnv("LANG_PATH", "/usr/local/share/telegram-media-server/locales"),
		ProwlarrURL:     getEnv("PROWLARR_URL", ""),
		ProwlarrAPIKey:  getEnv("PROWLARR_API_KEY", ""),

		DownloadSettings: DownloadConfig{
			MaxConcurrentDownloads: getEnvInt("MAX_CONCURRENT_DOWNLOADS", 3),
			DownloadTimeout:        getEnvDuration("DOWNLOAD_TIMEOUT", DefaultDownloadTimeout),
			ProgressUpdateInterval: getEnvDuration("PROGRESS_UPDATE_INTERVAL", DefaultProgressUpdateInterval),
		},

		SecuritySettings: SecurityConfig{
			PasswordMinLength: getEnvInt("PASSWORD_MIN_LENGTH", DefaultPasswordMinLength),
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
			EnableReencoding:  getEnvBool("VIDEO_ENABLE_REENCODING", false),
			ForceReencoding:   getEnvBool("VIDEO_FORCE_REENCODING", false),
			VideoCodec:        getEnv("VIDEO_CODEC", "h264"),
			AudioCodec:        getEnv("AUDIO_CODEC", "mp3"),
			OutputFormat:      getEnv("VIDEO_OUTPUT_FORMAT", "mp4"),
			FFmpegExtraArgs:   getEnv("FFMPEG_EXTRA_ARGS", "-pix_fmt yuv420p"),
			QualitySelector:   getEnv("VIDEO_QUALITY_SELECTOR", "bv*+ba/b"),
			CompatibilityMode: getEnvBool("VIDEO_COMPATIBILITY_MODE", false),
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
	BotToken        string
	MoviePath       string
	AdminPassword   string
	RegularPassword string
	Lang            string
	Proxy           string
	ProxyDomains    string
	LogLevel        string
	LangPath        string
	ProwlarrURL     string
	ProwlarrAPIKey  string

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
	EnableReencoding  bool
	ForceReencoding   bool
	VideoCodec        string
	AudioCodec        string
	OutputFormat      string
	FFmpegExtraArgs   string
	QualitySelector   string
	CompatibilityMode bool
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value, exists := os.LookupEnv(key); exists {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value, exists := os.LookupEnv(key); exists {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

func (c *Config) validate() error {
	if err := c.validateRequiredFields(); err != nil {
		return err
	}

	if err := c.validatePasswords(); err != nil {
		return err
	}

	if err := c.validateProwlarr(); err != nil {
		return err
	}

	if err := c.validateDownloadSettings(); err != nil {
		return err
	}

	return nil
}

func (c *Config) validateRequiredFields() error {
	var missingFields []string

	if c.BotToken == "" {
		missingFields = append(missingFields, "BOT_TOKEN")
	}
	if c.MoviePath == "" {
		missingFields = append(missingFields, "MOVIE_PATH")
	}
	if c.AdminPassword == "" {
		missingFields = append(missingFields, "ADMIN_PASSWORD")
	}

	if len(missingFields) > 0 {
		return utils.WrapError(utils.ErrConfigurationError, "missing required environment variables", map[string]any{
			"missing_fields": missingFields,
		})
	}

	return nil
}

func (c *Config) validatePasswords() error {
	if len(c.AdminPassword) < c.SecuritySettings.PasswordMinLength {
		return utils.WrapError(utils.ErrConfigurationError, "admin password too short", map[string]any{
			"min_length":    c.SecuritySettings.PasswordMinLength,
			"actual_length": len(c.AdminPassword),
		})
	}

	if c.RegularPassword == "" {
		log.Println("REGULAR_PASSWORD not set, using ADMIN_PASSWORD as REGULAR_PASSWORD")
		c.RegularPassword = c.AdminPassword
	} else if len(c.RegularPassword) < c.SecuritySettings.PasswordMinLength {
		return utils.WrapError(utils.ErrConfigurationError, "regular password too short", map[string]any{
			"min_length":    c.SecuritySettings.PasswordMinLength,
			"actual_length": len(c.RegularPassword),
		})
	}

	return nil
}

func (c *Config) validateProwlarr() error {
	if (c.ProwlarrURL != "" || c.ProwlarrAPIKey != "") && (c.ProwlarrURL == "" || c.ProwlarrAPIKey == "") {
		var missingFields []string
		if c.ProwlarrURL == "" {
			missingFields = append(missingFields, "PROWLARR_URL (required if PROWLARR_API_KEY is set)")
		}
		if c.ProwlarrAPIKey == "" {
			missingFields = append(missingFields, "PROWLARR_API_KEY (required if PROWLARR_URL is set)")
		}
		return utils.WrapError(utils.ErrConfigurationError, "missing required environment variables", map[string]any{
			"missing_fields": missingFields,
		})
	}

	return nil
}

func (c *Config) validateDownloadSettings() error {
	if c.DownloadSettings.MaxConcurrentDownloads <= 0 {
		return utils.WrapError(utils.ErrConfigurationError, "max concurrent downloads must be positive", nil)
	}

	if c.DownloadSettings.DownloadTimeout < 0 {
		return utils.WrapError(utils.ErrConfigurationError, "download timeout cannot be negative", nil)
	}

	return nil
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

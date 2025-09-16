package app

import (
	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
)

// initializeConfig инициализирует конфигурацию и логгер
func initializeConfig() (*domain.Config, error) {
	pkgConfig, err := config.NewConfig()
	if err != nil {
		return nil, err
	}

	// Инициализируем логгер
	logger.InitLogger(pkgConfig.LogLevel)

	// Конвертируем в domain.Config
	domainConfig := convertConfig(pkgConfig)

	return domainConfig, nil
}

// convertConfig конвертирует pkg.Config в domain.Config
func convertConfig(cfg *config.Config) *domain.Config {
	return &domain.Config{
		BotToken:        cfg.BotToken,
		MoviePath:       cfg.MoviePath,
		AdminPassword:   cfg.AdminPassword,
		RegularPassword: cfg.RegularPassword,
		Lang:            cfg.Lang,
		Proxy:           cfg.Proxy,
		ProxyDomains:    cfg.ProxyDomains,
		LogLevel:        cfg.LogLevel,
		LangPath:        cfg.LangPath,
		ProwlarrURL:     cfg.ProwlarrURL,
		ProwlarrAPIKey:  cfg.ProwlarrAPIKey,

		DownloadSettings: domain.DownloadConfig{
			MaxConcurrentDownloads: cfg.DownloadSettings.MaxConcurrentDownloads,
			DownloadTimeout:        cfg.DownloadSettings.DownloadTimeout,
			ProgressUpdateInterval: cfg.DownloadSettings.ProgressUpdateInterval,
		},
		SecuritySettings: domain.SecurityConfig{
			PasswordMinLength: cfg.SecuritySettings.PasswordMinLength,
		},
		Aria2Settings: domain.Aria2Config{
			MaxPeers:                 cfg.Aria2Settings.MaxPeers,
			MaxConnectionsPerServer:  cfg.Aria2Settings.MaxConnectionsPerServer,
			Split:                    cfg.Aria2Settings.Split,
			MinSplitSize:             cfg.Aria2Settings.MinSplitSize,
			BTMaxPeers:               cfg.Aria2Settings.BTMaxPeers,
			BTRequestPeerSpeedLimit:  cfg.Aria2Settings.BTRequestPeerSpeedLimit,
			BTMaxOpenFiles:           cfg.Aria2Settings.BTMaxOpenFiles,
			MaxOverallUploadLimit:    cfg.Aria2Settings.MaxOverallUploadLimit,
			MaxUploadLimit:           cfg.Aria2Settings.MaxUploadLimit,
			SeedRatio:                cfg.Aria2Settings.SeedRatio,
			SeedTime:                 cfg.Aria2Settings.SeedTime,
			BTTrackerTimeout:         cfg.Aria2Settings.BTTrackerTimeout,
			BTTrackerInterval:        cfg.Aria2Settings.BTTrackerInterval,
			EnableDHT:                cfg.Aria2Settings.EnableDHT,
			EnablePeerExchange:       cfg.Aria2Settings.EnablePeerExchange,
			EnableLocalPeerDiscovery: cfg.Aria2Settings.EnableLocalPeerDiscovery,
			FollowTorrent:            cfg.Aria2Settings.FollowTorrent,
			ListenPort:               cfg.Aria2Settings.ListenPort,
			DHTPorts:                 cfg.Aria2Settings.DHTPorts,
			BTSaveMetadata:           cfg.Aria2Settings.BTSaveMetadata,
			BTHashCheckSeed:          cfg.Aria2Settings.BTHashCheckSeed,
			BTRequireCrypto:          cfg.Aria2Settings.BTRequireCrypto,
			BTMinCryptoLevel:         cfg.Aria2Settings.BTMinCryptoLevel,
			CheckIntegrity:           cfg.Aria2Settings.CheckIntegrity,
			ContinueDownload:         cfg.Aria2Settings.ContinueDownload,
			RemoteTime:               cfg.Aria2Settings.RemoteTime,
			FileAllocation:           cfg.Aria2Settings.FileAllocation,
			HTTPProxy:                cfg.Aria2Settings.HTTPProxy,
			AllProxy:                 cfg.Aria2Settings.AllProxy,
			UserAgent:                cfg.Aria2Settings.UserAgent,
			Timeout:                  cfg.Aria2Settings.Timeout,
			MaxTries:                 cfg.Aria2Settings.MaxTries,
			RetryWait:                cfg.Aria2Settings.RetryWait,
		},
		VideoSettings: domain.VideoConfig{
			EnableReencoding:  cfg.VideoSettings.EnableReencoding,
			ForceReencoding:   cfg.VideoSettings.ForceReencoding,
			VideoCodec:        cfg.VideoSettings.VideoCodec,
			AudioCodec:        cfg.VideoSettings.AudioCodec,
			OutputFormat:      cfg.VideoSettings.OutputFormat,
			FFmpegExtraArgs:   cfg.VideoSettings.FFmpegExtraArgs,
			QualitySelector:   cfg.VideoSettings.QualitySelector,
			CompatibilityMode: cfg.VideoSettings.CompatibilityMode,
		},
	}
}

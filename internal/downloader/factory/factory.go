package factory

import (
	"context"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	aria2 "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/torrent"
	ytdlp "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/video"
)

func NewTorrentDownloader(torrentFileName, moviePath string, cfg *config.Config) downloader.Downloader {
	return aria2.NewAria2Downloader(torrentFileName, moviePath, cfg)
}

func NewVideoDownloader(videoURL string, cfg *config.Config) downloader.Downloader {
	return ytdlp.NewYTDLPDownloader(videoURL, cfg)
}

func RunUpdatersOnStart(ctx context.Context, cfg *config.Config) {
	if cfg.YtdlpUpdateOnStart {
		go newYtdlpUpdater(cfg.YtdlpPath).RunUpdate(ctx)
	}
}

func StartPeriodicUpdaters(ctx context.Context, cfg *config.Config) {
	if cfg.YtdlpUpdateInterval > 0 {
		go downloader.StartPeriodicUpdater(ctx, cfg.YtdlpUpdateInterval, newYtdlpUpdater(cfg.YtdlpPath))
	}
}

func newYtdlpUpdater(binaryPath string) downloader.Updater {
	return ytdlp.NewUpdater(binaryPath)
}

package factory

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	aria2 "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/torrent"
	ytdlp "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/video"
	"github.com/google/uuid"
)

const (
	torrentDownloadTimeoutSec = 60
	torrentMaxSizeBytes       = 10 * 1024 * 1024 // 10 MiB
	ownerOnlyFileMode         = 0o600
)

func NewTorrentDownloader(torrentFileName, moviePath string, cfg *config.Config) downloader.Downloader {
	return aria2.NewAria2Downloader(torrentFileName, moviePath, cfg)
}

func NewVideoDownloader(videoURL string, cfg *config.Config) downloader.Downloader {
	return ytdlp.NewYTDLPDownloader(videoURL, cfg)
}

// CreateDownloaderFromURL creates a downloader from a URL string: magnet link, .torrent URL, or video URL (yt-dlp).
func CreateDownloaderFromURL(ctx context.Context, rawURL, moviePath string, cfg *config.Config) (downloader.Downloader, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("empty URL")
	}

	// Magnet link: write to a .magnet file and use torrent downloader
	if strings.HasPrefix(strings.ToLower(rawURL), "magnet:") {
		name := "magnet_" + uuid.New().String()[:8] + ".magnet"
		path := filepath.Join(moviePath, name)
		if err := os.WriteFile(path, []byte(rawURL), ownerOnlyFileMode); err != nil {
			return nil, fmt.Errorf("write magnet file: %w", err)
		}
		return aria2.NewAria2Downloader(name, moviePath, cfg), nil
	}

	// HTTP(S) URL ending with .torrent or with torrent content: download to temp file
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		if strings.HasSuffix(strings.ToLower(rawURL), ".torrent") {
			localPath, err := downloadTorrentFile(ctx, rawURL, moviePath)
			if err != nil {
				return nil, err
			}
			base := filepath.Base(localPath)
			return aria2.NewAria2Downloader(base, moviePath, cfg), nil
		}
	}

	// Otherwise treat as video URL (yt-dlp)
	return ytdlp.NewYTDLPDownloader(rawURL, cfg), nil
}

func downloadTorrentFile(ctx context.Context, url, moviePath string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	client := &http.Client{Timeout: time.Duration(torrentDownloadTimeoutSec) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download .torrent: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download .torrent: status %d", resp.StatusCode)
	}
	if resp.ContentLength > torrentMaxSizeBytes {
		return "", fmt.Errorf("download .torrent: file too large (%d bytes)", resp.ContentLength)
	}
	body := io.LimitReader(resp.Body, torrentMaxSizeBytes+1)
	data, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("read .torrent: %w", err)
	}
	if int64(len(data)) > torrentMaxSizeBytes {
		return "", fmt.Errorf("download .torrent: file too large")
	}
	name := "torrent_" + uuid.New().String()[:8] + ".torrent"
	path := filepath.Join(moviePath, name)
	if err := os.WriteFile(path, data, ownerOnlyFileMode); err != nil {
		return "", fmt.Errorf("write .torrent file: %w", err)
	}
	return path, nil
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

package factory

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

	// Magnet link: validate first, then write to a .magnet file and use torrent downloader
	if strings.HasPrefix(strings.ToLower(rawURL), "magnet:") {
		if err := aria2.ValidateMagnetBtih(rawURL); err != nil {
			return nil, err
		}
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
		// Prowlarr download proxy URL: GET and resolve to magnet or .torrent
		if dl, err := resolveProwlarrDownload(ctx, rawURL, moviePath, cfg); err == nil {
			return dl, nil
		}
		// fall through to video
	}

	// Otherwise treat as video URL (yt-dlp)
	return ytdlp.NewYTDLPDownloader(rawURL, cfg), nil
}

// resolveProwlarrDownload fetches a Prowlarr proxy download URL and returns a downloader for the
// resulting magnet or .torrent. Returns an error if the URL is not a Prowlarr download or the
// response is not torrent/magnet (caller may then treat as video URL).
func resolveProwlarrDownload(ctx context.Context, rawURL, moviePath string, cfg *config.Config) (downloader.Downloader, error) {
	if _, err := isProwlarrDownloadURL(cfg, rawURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	if cfg.ProwlarrAPIKey != "" {
		req.Header.Set("X-Api-Key", cfg.ProwlarrAPIKey)
	}
	client := &http.Client{
		Timeout: time.Duration(torrentDownloadTimeoutSec) * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Redirect to magnet: use it
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc := resp.Header.Get("Location")
		if strings.HasPrefix(strings.TrimSpace(loc), "magnet:") {
			return writeMagnetAndReturnDownloader(moviePath, strings.TrimSpace(loc), cfg)
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prowlarr download returned status %d", resp.StatusCode)
	}

	// Body: .torrent (bencode dict starts with 'd') or Content-Type
	contentType := resp.Header.Get("Content-Type")
	if resp.ContentLength > torrentMaxSizeBytes {
		return nil, fmt.Errorf("prowlarr download response too large")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, torrentMaxSizeBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > torrentMaxSizeBytes {
		return nil, fmt.Errorf("prowlarr download response too large")
	}
	if isTorrentResponse(data, contentType) {
		return writeTorrentAndReturnDownloader(moviePath, data, cfg)
	}

	return nil, fmt.Errorf("prowlarr download URL did not return torrent or magnet")
}

func isProwlarrDownloadURL(cfg *config.Config, rawURL string) (bool, error) {
	if cfg.ProwlarrURL == "" {
		return false, fmt.Errorf("prowlarr not configured")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false, err
	}
	baseParsed, _ := url.Parse(strings.TrimSuffix(cfg.ProwlarrURL, "/"))
	if baseParsed == nil || parsed.Scheme != baseParsed.Scheme || parsed.Host != baseParsed.Host {
		return false, fmt.Errorf("url is not prowlarr base")
	}
	if !strings.Contains(parsed.Path, "download") {
		return false, fmt.Errorf("url path does not contain download")
	}
	return true, nil
}

// safeJoinPath joins base with name and returns the path only if it stays under base (no traversal).
func safeJoinPath(base, name string) (string, error) {
	path := filepath.Join(base, name)
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") || rel == ".." {
		return "", fmt.Errorf("path escapes base directory")
	}
	return path, nil
}

func writeMagnetAndReturnDownloader(moviePath, magnetURI string, cfg *config.Config) (downloader.Downloader, error) {
	name := "magnet_" + uuid.New().String()[:8] + ".magnet"
	path, err := safeJoinPath(moviePath, name)
	if err != nil {
		return nil, err
	}
	// path is safe: safeJoinPath ensures it does not escape moviePath
	// #nosec G703 -- path validated against traversal
	if err := os.WriteFile(path, []byte(magnetURI), ownerOnlyFileMode); err != nil {
		return nil, err
	}
	return aria2.NewAria2Downloader(name, moviePath, cfg), nil
}

func isTorrentResponse(data []byte, contentType string) bool {
	if len(data) > 0 && data[0] == 'd' {
		return true
	}
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "bittorrent") || strings.Contains(ct, "x-bittorrent")
}

func writeTorrentAndReturnDownloader(moviePath string, data []byte, cfg *config.Config) (downloader.Downloader, error) {
	fname := "torrent_" + uuid.New().String()[:8] + ".torrent"
	fpath, err := safeJoinPath(moviePath, fname)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(fpath, data, ownerOnlyFileMode); err != nil {
		return nil, err
	}
	return aria2.NewAria2Downloader(fname, moviePath, cfg), nil
}

func downloadTorrentFile(ctx context.Context, fileURL, moviePath string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, http.NoBody)
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
	path, err := safeJoinPath(moviePath, name)
	if err != nil {
		return "", fmt.Errorf("torrent path: %w", err)
	}
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

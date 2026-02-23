package qbittorrent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	aria2pkg "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/torrent"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/tvcompat"
	"github.com/go-bittorrent/magneturi"
)

const (
	pollInterval       = 3 * time.Second
	delayAfterAdd      = 500 * time.Millisecond
	progressPercentMax = 100
	stopTimeout        = 15 * time.Second
)

// QBittorrentDownloader implements downloader.Downloader using qBittorrent Web API.
type QBittorrentDownloader struct {
	torrentFileName string
	downloadDir     string
	cfg             *config.Config
	magnetURI       string
	client          *Client

	mu              sync.Mutex
	hash            string
	hashChan        chan string // sends qBittorrent torrent hash once when known (for DB persistence)
	stoppedManually bool
}

// NewQBittorrentDownloader creates a downloader that uses qBittorrent.
// torrentFileName is the .torrent or .magnet file name under downloadDir.
func NewQBittorrentDownloader(torrentFileName, moviePath string, cfg *config.Config) (downloader.Downloader, error) {
	if cfg.QBittorrentURL == "" {
		return nil, fmt.Errorf("QBittorrentURL is not set")
	}
	client, err := NewClient(cfg.QBittorrentURL, cfg.QBittorrentUsername, cfg.QBittorrentPassword)
	if err != nil {
		return nil, err
	}
	d := &QBittorrentDownloader{
		torrentFileName: torrentFileName,
		downloadDir:     moviePath,
		cfg:             cfg,
		client:          client,
	}
	if strings.HasSuffix(strings.ToLower(torrentFileName), ".magnet") {
		path := filepath.Join(moviePath, torrentFileName)
		if b, err := os.ReadFile(path); err == nil {
			d.magnetURI = strings.TrimSpace(string(b))
		}
	}
	return d, nil
}

func (d *QBittorrentDownloader) parseMeta() (*aria2pkg.Meta, error) {
	if d.magnetURI != "" {
		return d.magnetDummyMeta()
	}
	torrentPath := filepath.Join(d.downloadDir, d.torrentFileName)
	return aria2pkg.ParseMeta(torrentPath)
}

func (d *QBittorrentDownloader) magnetDummyMeta() (*aria2pkg.Meta, error) {
	m := &aria2pkg.Meta{}
	m.Info.Name = "Magnet download"
	if parsed, err := magneturi.Parse(d.magnetURI); err == nil {
		if parsed.DisplayName != "" {
			m.Info.Name = parsed.DisplayName
		}
		if parsed.ExactLength > 0 {
			m.Info.Length = parsed.ExactLength
		}
	}
	return m, nil
}

func (d *QBittorrentDownloader) GetTitle() (string, error) {
	meta, err := d.parseMeta()
	if err != nil {
		return "", err
	}
	return meta.Info.Name, nil
}

func (d *QBittorrentDownloader) GetFiles() (mainFiles, tempFiles []string, err error) {
	meta, err := d.parseMeta()
	if err != nil {
		return nil, nil, err
	}
	if len(meta.Info.Files) > 0 {
		for _, file := range meta.Info.Files {
			if meta.Info.Name == "" {
				return nil, nil, fmt.Errorf("torrent meta does not contain a root directory name")
			}
			if len(file.Path) == 0 {
				return nil, nil, fmt.Errorf("file path is empty in torrent meta")
			}
			mainFiles = append(mainFiles, filepath.Join(meta.Info.Name, filepath.Join(file.Path...)))
		}
	} else {
		mainFiles = append(mainFiles, meta.Info.Name)
	}
	tempFiles = []string{d.torrentFileName}
	return mainFiles, tempFiles, nil
}

func (d *QBittorrentDownloader) GetFileSize() (int64, error) {
	meta, err := d.parseMeta()
	if err != nil {
		logutils.Log.WithError(err).Warn("Failed to parse torrent metadata for file size, returning 0")
		return 0, nil
	}
	if len(meta.Info.Files) > 0 {
		var total int64
		for _, f := range meta.Info.Files {
			total += f.Length
		}
		return total, nil
	}
	return meta.Info.Length, nil
}

func (d *QBittorrentDownloader) TotalEpisodes() int {
	meta, err := d.parseMeta()
	if err != nil {
		return 0
	}
	var n int
	for i := range meta.Info.Files {
		path := filepath.Join(meta.Info.Name, filepath.Join(meta.Info.Files[i].Path...))
		if tvcompat.IsVideoFilePath(path) {
			n++
		}
	}
	if n == 0 && len(meta.Info.Files) == 0 && meta.Info.Name != "" {
		return 1
	}
	return n
}

func (d *QBittorrentDownloader) StoppedManually() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stoppedManually
}

func (d *QBittorrentDownloader) setHash(h string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.hash = h
}

func (d *QBittorrentDownloader) StartDownload(
	ctx context.Context,
) (progressChan chan float64, errChan chan error, episodesChan <-chan int, err error) {
	meta, metaErr := d.parseMeta()
	if metaErr != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse torrent meta: %w", metaErr)
	}

	progressChan = make(chan float64)
	errChan = make(chan error, 1)
	d.hashChan = make(chan string, 1)
	var epCh chan int
	totalVideo := d.TotalEpisodes()
	if totalVideo > 1 {
		epCh = make(chan int, 1)
		episodesChan = epCh
	}

	go d.run(ctx, meta, totalVideo, progressChan, errChan, epCh)
	return progressChan, errChan, episodesChan, nil
}

// QBittorrentHashChan implements downloader.QBittorrentHashDownloader.
func (d *QBittorrentDownloader) QBittorrentHashChan() <-chan string {
	return d.hashChan
}

//nolint:gocyclo // run orchestrates login, add, find hash, poll loop; splitting would obscure flow.
func (d *QBittorrentDownloader) run(
	ctx context.Context,
	meta *aria2pkg.Meta,
	totalVideo int,
	progressChan chan float64,
	errChan chan error,
	episodesChan chan int,
) {
	defer close(errChan)
	if episodesChan != nil {
		defer close(episodesChan)
	}
	defer close(progressChan)

	if err := d.client.Login(ctx); err != nil {
		errChan <- fmt.Errorf("qBittorrent login: %w", err)
		return
	}

	savepath := d.downloadDir
	if d.magnetURI != "" {
		if err := d.client.AddTorrentFromURLs(ctx, d.magnetURI, savepath); err != nil {
			errChan <- fmt.Errorf("qBittorrent add magnet: %w", err)
			return
		}
	} else {
		torrentPath := filepath.Join(d.downloadDir, d.torrentFileName)
		body, err := os.ReadFile(torrentPath)
		if err != nil {
			errChan <- fmt.Errorf("read torrent file: %w", err)
			return
		}
		if err := d.client.AddTorrentFromFile(ctx, d.torrentFileName, body, savepath); err != nil {
			errChan <- fmt.Errorf("qBittorrent add file: %w", err)
			return
		}
	}

	// Find our torrent: last added (most recent added_on).
	time.Sleep(delayAfterAdd)
	list, err := d.client.TorrentsInfo(ctx, "", "added_on", true)
	if err != nil {
		errChan <- fmt.Errorf("qBittorrent list after add: %w", err)
		return
	}
	if len(list) == 0 {
		errChan <- fmt.Errorf("qBittorrent: torrent not found after add")
		return
	}
	// Prefer match by name; otherwise take the newest.
	var our *TorrentInfo
	for i := range list {
		if list[i].Name == meta.Info.Name {
			our = &list[i]
			break
		}
	}
	if our == nil {
		our = &list[0]
	}
	d.setHash(our.Hash)
	// Notify manager so it can persist hash for removal from qBittorrent on movie delete
	if d.hashChan != nil {
		select {
		case d.hashChan <- our.Hash:
		default:
		}
		close(d.hashChan)
		d.hashChan = nil
	}

	// Send initial 0% so the UI shows progress from the start
	select {
	case progressChan <- 0:
	case <-ctx.Done():
		if d.StoppedManually() {
			errChan <- downloader.ErrStoppedByUser
		} else {
			errChan <- ctx.Err()
		}
		return
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var lastProgress float64
	for {
		select {
		case <-ctx.Done():
			if d.StoppedManually() {
				errChan <- downloader.ErrStoppedByUser
			} else {
				errChan <- ctx.Err()
			}
			return
		case <-ticker.C:
			// Refresh torrent info
			info, err := d.client.TorrentsInfo(ctx, our.Hash, "", false)
			if err != nil {
				errChan <- fmt.Errorf("qBittorrent torrents/info: %w", err)
				return
			}
			if len(info) == 0 {
				if d.StoppedManually() {
					errChan <- downloader.ErrStoppedByUser
				} else {
					errChan <- fmt.Errorf("qBittorrent: torrent no longer in list")
				}
				return
			}
			t := info[0]
			// Use API progress (0–1); fallback to downloaded/size when progress is 0 but we have partial data
			progress := t.Progress * 100
			if progress <= 0 && t.Size > 0 && t.Downloaded > 0 {
				progress = float64(t.Downloaded) / float64(t.Size) * 100
			}
			if progress > progressPercentMax {
				progress = progressPercentMax
			}
			// Send whenever progress changed so the UI updates (including intermediate values)
			if progress != lastProgress {
				lastProgress = progress
				select {
				case progressChan <- progress:
				case <-ctx.Done():
					if d.StoppedManually() {
						errChan <- downloader.ErrStoppedByUser
					} else {
						errChan <- ctx.Err()
					}
					return
				}
			}
			if t.Progress >= 1.0 {
				if episodesChan != nil && totalVideo > 0 {
					episodesChan <- totalVideo
				}
				errChan <- nil
				return
			}
		}
	}
}

func (d *QBittorrentDownloader) StopDownload() error {
	d.mu.Lock()
	d.stoppedManually = true
	hash := d.hash
	d.mu.Unlock()
	if hash == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	if err := d.client.Login(ctx); err != nil {
		logutils.Log.WithError(err).Warn("qBittorrent login before delete failed")
		return nil
	}
	if err := d.client.DeleteTorrent(ctx, hash, false); err != nil {
		logutils.Log.WithError(err).WithField("hash", hash).Warn("qBittorrent delete failed")
		return err
	}
	return nil
}

// GetEarlyTvCompatibility returns preliminary TV compatibility from torrent file names, or yellow for magnet.
func (d *QBittorrentDownloader) GetEarlyTvCompatibility(_ context.Context) (string, error) {
	mainFiles, _, err := d.GetFiles()
	if err != nil {
		return "", err
	}
	compat := tvcompat.CompatFromTorrentFileNames(mainFiles)
	if compat == "" && d.magnetURI != "" {
		return tvcompat.TvCompatYellow, nil
	}
	return compat, nil
}

// Ensure QBittorrentDownloader implements downloader.Downloader and optional EarlyCompatDownloader.
var (
	_ downloader.Downloader                = (*QBittorrentDownloader)(nil)
	_ downloader.EarlyCompatDownloader     = (*QBittorrentDownloader)(nil)
	_ downloader.QBittorrentHashDownloader = (*QBittorrentDownloader)(nil)
)

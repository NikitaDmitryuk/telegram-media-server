package qbittorrent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	hashChan        chan string       // sends qBittorrent torrent hash once when known (for DB persistence)
	onHashKnown     func(hash string) // optional; called synchronously when hash is known so manager can persist to DB before restart
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

// SetOnHashKnown implements downloader.OnHashKnownSetter.
func (d *QBittorrentDownloader) SetOnHashKnown(cb func(hash string)) {
	d.onHashKnown = cb
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
	isSeries := totalVideo > 1
	addOpts := &AddTorrentOptions{
		SequentialDownload: isSeries,
		FirstLastPiecePrio: isSeries,
	}
	if d.magnetURI != "" {
		if err := d.client.AddTorrentFromURLs(ctx, d.magnetURI, savepath, addOpts); err != nil {
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
		if err := d.client.AddTorrentFromFile(ctx, d.torrentFileName, body, savepath, addOpts); err != nil {
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
	// Persist hash synchronously so it survives process restart (callback runs in this goroutine before channel send).
	if d.onHashKnown != nil {
		d.onHashKnown(our.Hash)
	}
	// Notify manager via channel (fallback / idempotent second persist).
	if d.hashChan != nil {
		select {
		case d.hashChan <- our.Hash:
		default:
		}
		close(d.hashChan)
		d.hashChan = nil
	}

	if isSeries {
		d.applyLexicographicPriorities(ctx, our.Hash)
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
			logutils.Log.WithFields(map[string]any{
				"progress":    t.Progress,
				"size":        t.Size,
				"total_size":  t.TotalSize,
				"downloaded":  t.Downloaded,
				"amount_left": t.AmountLeft,
				"completed":   t.Completed,
				"state":       t.State,
			}).Debug("qBittorrent API torrents/info response")
			progress := t.Progress * 100
			if progress > progressPercentMax {
				progress = progressPercentMax
			}
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
			// Complete when API progress >= 1 or amount_left is 0 (e.g. state=stalledUP)
			if t.Progress >= 1.0 || (t.Size > 0 && t.AmountLeft == 0) {
				if episodesChan != nil && totalVideo > 0 {
					episodesChan <- totalVideo
				}
				errChan <- nil
				return
			}
		}
	}
}

const (
	priorityNormal  = 1
	priorityHigh    = 6
	priorityMaximal = 7
)

// applyLexicographicPriorities sets file download priorities so that earlier
// video files (lexicographic order by name) are downloaded first. The first
// video file gets maximal priority, the second gets high, and the rest get
// normal. Combined with sequential download mode this ensures the first
// episode finishes as soon as possible without blocking the rest.
func (d *QBittorrentDownloader) applyLexicographicPriorities(ctx context.Context, hash string) {
	files, err := d.client.TorrentFiles(ctx, hash)
	if err != nil {
		logutils.Log.WithError(err).Warn("Failed to get torrent files for priority setup")
		return
	}

	type videoEntry struct {
		index int
		name  string
	}
	var videos []videoEntry
	for _, f := range files {
		if tvcompat.IsVideoFilePath(f.Name) {
			videos = append(videos, videoEntry{index: f.Index, name: f.Name})
		}
	}
	if len(videos) <= 1 {
		return
	}
	sort.Slice(videos, func(i, j int) bool { return videos[i].name < videos[j].name })

	for rank, v := range videos {
		prio := priorityNormal
		switch rank {
		case 0:
			prio = priorityMaximal
		case 1:
			prio = priorityHigh
		}
		if err := d.client.SetFilePriority(ctx, hash, fmt.Sprintf("%d", v.index), prio); err != nil {
			logutils.Log.WithError(err).WithField("file", v.name).Warn("Failed to set file priority")
		}
	}

	logutils.Log.WithFields(map[string]any{
		"hash":        hash,
		"video_files": len(videos),
		"first_file":  videos[0].name,
	}).Info("Applied lexicographic file priorities for series")
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

// Ensure QBittorrentDownloader implements downloader.Downloader and optional interfaces.
var (
	_ downloader.Downloader                = (*QBittorrentDownloader)(nil)
	_ downloader.EarlyCompatDownloader     = (*QBittorrentDownloader)(nil)
	_ downloader.QBittorrentHashDownloader = (*QBittorrentDownloader)(nil)
	_ downloader.OnHashKnownSetter         = (*QBittorrentDownloader)(nil)
)

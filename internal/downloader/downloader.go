package downloader

import (
	"context"
)

type Downloader interface {
	GetTitle() (string, error)
	GetFiles() ([]string, []string, error)
	GetFileSize() (int64, error)
	TotalEpisodes() int
	StoppedManually() bool
	StartDownload(ctx context.Context) (progressChan chan float64, errChan chan error, episodesChan <-chan int, err error)
	StopDownload() error
}

// EarlyCompatDownloader: optional; manager uses GetEarlyTvCompatibility to show TV compat from metadata before file is ready.
type EarlyCompatDownloader interface {
	Downloader
	GetEarlyTvCompatibility(ctx context.Context) (string, error)
}

// QBittorrentHashDownloader: optional; manager uses QBittorrentHashChan to persist the torrent hash
// for removal from qBittorrent on movie delete.
type QBittorrentHashDownloader interface {
	Downloader
	QBittorrentHashChan() <-chan string // sends the qBittorrent torrent hash once when known, then closes
}

type Updater interface {
	RunUpdate(ctx context.Context)
}

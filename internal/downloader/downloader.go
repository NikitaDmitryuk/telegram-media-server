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

type Updater interface {
	RunUpdate(ctx context.Context)
}

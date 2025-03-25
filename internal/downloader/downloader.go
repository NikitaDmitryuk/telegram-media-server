package downloader

import (
	"context"
)

// Downloader defines the interface for all downloaders.
// Implementations must:
// - Return the final title of the content via GetTitle (e.g. the file name from the torrent metadata).
// - Return the main (downloaded) file and any temporary files (such as the torrent file or aria2 temp file) via GetFiles.
// - Return the size (in bytes) of the main downloaded file via GetFileSize.
// - Provide a flag indicating if the download was stopped manually via StoppedManually.
// - Start the download process and provide a progress channel (which gets closed when the download completes or is stopped).
// - Stop the download process, marking a manual stop if invoked, so that handlers can notify the user accordingly.
type Downloader interface {
	// GetTitle retrieves the title of the content.
	// For example, for a torrent downloader it returns the file name extracted from the torrent metadata.
	GetTitle() (string, error)

	// GetFiles retrieves the file paths associated with the content.
	// The first return value should be the path to the main downloaded file.
	// The second return value is a slice of paths to temporary files (like the torrent file itself and any .aria2 file).
	GetFiles() (string, []string, error)

	// GetFileSize retrieves the size (in bytes) of the main downloaded file.
	// For a single file downloader, the returned size relates to that one file.
	GetFileSize() (int64, error)

	// StoppedManually returns whether the download was stopped manually.
	StoppedManually() bool

	// StartDownload initiates the download process.
	// It returns a channel to receive progress updates (in percentage) and an error if the start fails.
	// The progress channel must be closed when the download completes or is stopped.
	StartDownload(ctx context.Context) (chan float64, chan error, error)

	// StopDownload stops the download process.
	// If the download is stopped manually, implementations should track that state so that handlers
	// can notify the user that the download was cancelled.
	StopDownload() error
}

package app

import (
	"context"
	"errors"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/notifier"
)

// RunCompletionLoop waits for the download to complete (via completionChan from the manager),
// then performs cleanup (temp files, delete on failure) and notifies via the given notifier.
// It must not read progressChan — the manager's monitor is the only consumer of progress so it can write progress to the DB.
func RunCompletionLoop(
	a *App,
	completionChan <-chan error,
	dl downloader.Downloader,
	movieID uint,
	title string,
	compl notifier.CompletionNotifier,
) {
	err := <-completionChan
	if errors.Is(err, downloader.ErrStoppedByDeletion) {
		logutils.Log.Info("Download stopped by deletion queue (no user notification)")
		return
	}
	if dl.StoppedManually() {
		logutils.Log.Info("Download was manually stopped")
		compl.OnStopped(movieID, title)
		return
	}
	if err != nil {
		logutils.Log.WithError(err).Error("Download failed")
		if deleteErr := filemanager.DeleteMovie(movieID, a.Config.MoviePath, a.DB, a.DownloadManager); deleteErr != nil {
			logutils.Log.WithError(deleteErr).Error("Failed to delete movie after download failed")
		}
		compl.OnFailed(movieID, title, err)
		return
	}
	logutils.Log.Info("Download completed successfully")
	if err := filemanager.DeleteTemporaryFilesByMovieID(movieID, a.Config.MoviePath, a.DB, a.DownloadManager); err != nil {
		logutils.Log.WithError(err).Error("Failed to delete temporary files after download")
	}
	if err := a.DownloadManager.RemoveQBittorrentTorrent(context.Background(), movieID); err != nil {
		logutils.Log.WithError(err).WithField("movie_id", movieID).Debug("Failed to remove torrent from qBittorrent after completion")
	}
	compl.OnCompleted(movieID, title)
}

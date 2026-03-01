package app

import (
	"context"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/qbittorrent"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/notifier"
)

// ResumeIncompleteDownloads finds movies with an active qBittorrent hash and downloaded_percentage < 100,
// then reattaches monitoring so progress and completion are tracked again after a bot restart.
func ResumeIncompleteDownloads(a *App) {
	if a.Config.QBittorrentURL == "" {
		return
	}
	ctx := context.Background()
	movies, err := a.DB.GetIncompleteQBittorrentDownloads(ctx)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get incomplete qBittorrent downloads for resume")
		return
	}
	if len(movies) == 0 {
		return
	}
	logutils.Log.WithField("count", len(movies)).Info("Resuming incomplete qBittorrent downloads")
	for i := range movies {
		movie := &movies[i]
		dl, err := qbittorrent.NewQBittorrentResumeDownloader(
			movie.QBittorrentHash,
			a.Config.MoviePath,
			movie.TotalEpisodes,
			movie.CompletedEpisodes,
			a.Config,
		)
		if err != nil {
			logutils.Log.WithError(err).WithField("movie_id", movie.ID).Warn("Failed to create resume downloader")
			continue
		}
		completionChan, err := a.DownloadManager.ResumeDownload(
			movie.ID,
			dl,
			movie.Name,
			movie.TotalEpisodes,
			notifier.Noop,
		)
		if err != nil {
			logutils.Log.WithError(err).WithField("movie_id", movie.ID).Warn("Failed to resume download")
			continue
		}
		go RunCompletionLoop(a, completionChan, dl, movie.ID, movie.Name, notifier.CompletionNoop)
	}
}

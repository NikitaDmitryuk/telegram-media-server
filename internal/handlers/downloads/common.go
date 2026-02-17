package downloads

import (
	"context"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	tmsdownloader "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	filemanager "github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

func HandleDownload(
	a *app.App,
	chatID int64,
	downloaderInstance tmsdownloader.Downloader,
) {
	go handleDownloadAsync(a, chatID, downloaderInstance)
}

func handleDownloadAsync(
	a *app.App,
	chatID int64,
	downloaderInstance tmsdownloader.Downloader,
) {
	mainFiles, tempFiles, err := downloaderInstance.GetFiles()
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get files from downloader instance")
		a.Bot.SendMessage(chatID, tmslang.Translate("error.movies.check_error", map[string]any{
			"Error": err.Error(),
		}), nil)
		return
	}
	mainFiles = append(mainFiles, tempFiles...)
	allFiles := mainFiles

	exists, err := a.DB.MovieExistsFiles(context.Background(), allFiles)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to check if media files exist in the database")
		a.Bot.SendMessage(chatID, tmslang.Translate("error.movies.check_error", map[string]any{
			"Error": err.Error(),
		}), nil)
		return
	}
	if exists {
		logutils.Log.Warn("Media already exists")
		a.Bot.SendMessage(chatID, tmslang.Translate("error.movies.already_exists", nil), nil)
		return
	}

	videoTitle, err := downloaderInstance.GetTitle()
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get video title")
		a.Bot.SendMessage(chatID, tmslang.Translate("error.downloads.video_title_error", map[string]any{
			"Error": err.Error(),
		}), nil)
		return
	}

	fileSize, err := downloaderInstance.GetFileSize()
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get file size")
		a.Bot.SendMessage(chatID, tmslang.Translate("error.downloads.video_size_error", nil), nil)
		return
	}
	if !filemanager.HasEnoughSpace(a.Config.MoviePath, fileSize) {
		logutils.Log.Warn("Not enough space for the download")
		a.Bot.SendMessage(chatID, tmslang.Translate("error.storage.not_enough_space", nil), nil)
		return
	}

	movieID, progressChan, errChan, err := a.DownloadManager.StartDownload(downloaderInstance, chatID)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to start download")
		a.Bot.SendMessage(chatID, tmslang.Translate("error.downloads.document_download_error", nil), nil)
		return
	}

	a.Bot.SendMessage(chatID, tmslang.Translate("general.video_downloading", map[string]any{
		"Title": videoTitle,
	}), nil)

	go handleDownloadCompletion(a, chatID, downloaderInstance, movieID, videoTitle, progressChan, errChan)
}

func handleDownloadCompletion(
	a *app.App,
	chatID int64,
	downloaderInstance tmsdownloader.Downloader,
	movieID uint,
	videoTitle string,
	progressChan <-chan float64,
	errChan <-chan error,
) {
	for range progressChan {
	}
	err := <-errChan
	if downloaderInstance.StoppedManually() {
		logutils.Log.Info("Download was manually stopped")
		a.Bot.SendMessage(chatID, tmslang.Translate("general.download_stopped", map[string]any{
			"Title": videoTitle,
		}), nil)
	} else if err != nil {
		logutils.Log.WithError(err).Error("Download failed")
		if deleteErr := filemanager.DeleteMovie(movieID, a.Config.MoviePath, a.DB, a.DownloadManager); deleteErr != nil {
			logutils.Log.WithError(deleteErr).Error("Failed to delete movie after download failed")
		}
		a.Bot.SendMessage(chatID, tmslang.Translate("error.downloads.video_download_error", map[string]any{
			"Error": err.Error(),
		}), nil)
	} else {
		logutils.Log.Info("Download completed successfully")
		a.Bot.SendMessage(chatID, tmslang.Translate("general.video_successfully_downloaded", map[string]any{
			"Title": videoTitle,
		}), nil)
		if err := filemanager.DeleteTemporaryFilesByMovieID(movieID, a.Config.MoviePath, a.DB, a.DownloadManager); err != nil {
			logutils.Log.WithError(err).Error("Failed to delete temporary files after download")
		}
	}
}

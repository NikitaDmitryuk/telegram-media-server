package downloads

import (
	"context"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	tmsdownloader "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	filemanager "github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

func HandleDownload(
	bot *tmsbot.Bot,
	chatID int64,
	downloaderInstance tmsdownloader.Downloader,
	config *tmsconfig.Config,
	db tmsdb.Database,
	downloadManager *tmsdmanager.DownloadManager,
) {
	go handleDownloadAsync(bot, chatID, downloaderInstance, config, db, downloadManager)
}

func handleDownloadAsync(
	bot *tmsbot.Bot,
	chatID int64,
	downloaderInstance tmsdownloader.Downloader,
	config *tmsconfig.Config,
	db tmsdb.Database,
	downloadManager *tmsdmanager.DownloadManager,
) {
	mainFiles, tempFiles, err := downloaderInstance.GetFiles()
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get files from downloader instance")
		bot.SendMessage(chatID, tmslang.Translate("error.movies.check_error", map[string]any{
			"Error": err.Error(),
		}), nil)
		return
	}
	mainFiles = append(mainFiles, tempFiles...)
	allFiles := mainFiles

	exists, err := db.MovieExistsFiles(context.Background(), allFiles)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to check if media files exist in the database")
		bot.SendMessage(chatID, tmslang.Translate("error.movies.check_error", map[string]any{
			"Error": err.Error(),
		}), nil)
		return
	}
	if exists {
		logutils.Log.Warn("Media already exists")
		bot.SendMessage(chatID, tmslang.Translate("error.movies.already_exists", nil), nil)
		return
	}

	videoTitle, err := downloaderInstance.GetTitle()
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get video title")
		bot.SendMessage(chatID, tmslang.Translate("error.downloads.video_title_error", map[string]any{
			"Error": err.Error(),
		}), nil)
		return
	}

	fileSize, err := downloaderInstance.GetFileSize()
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get file size")
		bot.SendMessage(chatID, tmslang.Translate("error.downloads.video_size_error", nil), nil)
		return
	}
	if !filemanager.HasEnoughSpace(config.MoviePath, fileSize) {
		logutils.Log.Warn("Not enough space for the download")
		bot.SendMessage(chatID, tmslang.Translate("error.storage.not_enough_space", nil), nil)
		return
	}

	movieID, progressChan, errChan, err := downloadManager.StartDownload(downloaderInstance, chatID)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to start download")
		bot.SendMessage(chatID, tmslang.Translate("error.downloads.document_download_error", nil), nil)
		return
	}

	bot.SendMessage(chatID, tmslang.Translate("general.video_downloading", map[string]any{
		"Title": videoTitle,
	}), nil)

	go handleDownloadCompletion(bot, chatID, downloaderInstance, movieID, videoTitle, progressChan, errChan, config, db, downloadManager)
}

func handleDownloadCompletion(
	bot *tmsbot.Bot,
	chatID int64,
	downloaderInstance tmsdownloader.Downloader,
	movieID uint,
	videoTitle string,
	progressChan <-chan float64,
	errChan <-chan error,
	config *tmsconfig.Config,
	db tmsdb.Database,
	downloadManager *tmsdmanager.DownloadManager,
) {
	for range progressChan {
	}
	err := <-errChan
	if downloaderInstance.StoppedManually() {
		logutils.Log.Info("Download was manually stopped")
		bot.SendMessage(chatID, tmslang.Translate("general.download_stopped", map[string]any{
			"Title": videoTitle,
		}), nil)
	} else if err != nil {
		logutils.Log.WithError(err).Error("Download failed")
		if deleteErr := filemanager.DeleteMovie(movieID, config.MoviePath, db, downloadManager); deleteErr != nil {
			logutils.Log.WithError(deleteErr).Error("Failed to delete movie after download failed")
		}
		bot.SendMessage(chatID, tmslang.Translate("error.downloads.video_download_error", map[string]any{
			"Error": err.Error(),
		}), nil)
	} else {
		logutils.Log.Info("Download completed successfully")
		bot.SendMessage(chatID, tmslang.Translate("general.video_successfully_downloaded", map[string]any{
			"Title": videoTitle,
		}), nil)
		if err := filemanager.DeleteTemporaryFilesByMovieID(movieID, config.MoviePath, db, downloadManager); err != nil {
			logutils.Log.WithError(err).Error("Failed to delete temporary files after download")
		}
	}
}

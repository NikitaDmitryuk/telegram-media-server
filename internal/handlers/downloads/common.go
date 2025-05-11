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

func handleDownload(bot *tmsbot.Bot, chatID int64, downloaderInstance tmsdownloader.Downloader) {
	mainFiles, tempFiles, err := downloaderInstance.GetFiles()
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get files from downloader instance")
		bot.SendErrorMessage(chatID, tmslang.Translate("error.movies.check_error", map[string]any{
			"Error": err.Error(),
		}))
		return
	}
	mainFiles = append(mainFiles, tempFiles...)
	allFiles := mainFiles

	exists, err := tmsdb.GlobalDB.MovieExistsFiles(context.Background(), allFiles)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to check if media files exist in the database")
		bot.SendErrorMessage(chatID, tmslang.Translate("error.movies.check_error", map[string]any{
			"Error": err.Error(),
		}))
		return
	}
	if exists {
		logutils.Log.Warn("Media already exists")
		bot.SendErrorMessage(chatID, tmslang.Translate("error.movies.already_exists", nil))
		return
	}

	videoTitle, err := downloaderInstance.GetTitle()
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get video title")
		bot.SendErrorMessage(chatID, tmslang.Translate("error.downloads.video_title_error", map[string]any{
			"Error": err.Error(),
		}))
		return
	}

	fileSize, err := downloaderInstance.GetFileSize()
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get file size")
		bot.SendErrorMessage(chatID, tmslang.Translate("error.downloads.video_size_error", nil))
		return
	}
	if !filemanager.HasEnoughSpace(tmsconfig.GlobalConfig.MoviePath, fileSize) {
		logutils.Log.Warn("Not enough space for the download")
		bot.SendErrorMessage(chatID, tmslang.Translate("error.storage.not_enough_space", nil))
		return
	}

	movieID, progressChan, errChan, err := tmsdmanager.GlobalDownloadManager.StartDownload(downloaderInstance)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to start download")
		bot.SendErrorMessage(chatID, tmslang.Translate("error.downloads.document_download_error", nil))
		return
	}

	user, err := tmsdb.GlobalDB.GetUserByChatID(context.Background(), chatID)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to fetch user for download history")
	}

	if err := tmsdb.GlobalDB.AddDownloadHistory(context.Background(), user.ID, movieID); err != nil {
		logutils.Log.WithError(err).Error("Failed to record download history")
	}

	bot.SendSuccessMessage(chatID, tmslang.Translate("general.video_downloading", map[string]any{
		"Title": videoTitle,
	}))

	go handleDownloadCompletion(bot, chatID, downloaderInstance, movieID, videoTitle, progressChan, errChan)
}

func handleDownloadCompletion(
	bot *tmsbot.Bot,
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
		bot.SendSuccessMessage(chatID, tmslang.Translate("general.download_stopped", map[string]any{
			"Title": videoTitle,
		}))
	} else if err != nil {
		logutils.Log.WithError(err).Error("Download failed")
		if deleteErr := filemanager.DeleteMovie(movieID); deleteErr != nil {
			logutils.Log.WithError(deleteErr).Error("Failed to delete movie after download failed")
		}
		bot.SendErrorMessage(chatID, tmslang.Translate("error.downloads.video_download_error", map[string]any{
			"Error": err.Error(),
		}))
	} else {
		logutils.Log.Info("Download completed successfully")
		bot.SendSuccessMessage(chatID, tmslang.Translate("general.video_successfully_downloaded", map[string]any{
			"Title": videoTitle,
		}))
		if err := filemanager.DeleteTemporaryFilesByMovieID(movieID); err != nil {
			logutils.Log.WithError(err).Error("Failed to delete temporary files after download")
		}
	}
}

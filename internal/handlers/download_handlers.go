package handlers

import (
	"context"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	tmsdownloader "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	tmstorrent "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/torrent"
	tmsytdlp "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/video"
	filemanager "github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

func handleDownload(bot *tmsbot.Bot, chatID int64, downloaderInstance tmsdownloader.Downloader) {
	mainFiles, tempFiles, err := downloaderInstance.GetFiles()
	if err != nil {
		logrus.WithError(err).Error("Failed to get files from downloader instance")
		bot.SendErrorMessage(chatID, tmslang.Translate("error.movies.check_error", map[string]interface{}{
			"Error": err.Error(),
		}))
		return
	}
	allFiles := append(tempFiles, mainFiles...)

	if exists, err := tmsdb.GlobalDB.MovieExistsFiles(context.Background(), allFiles); err != nil {
		logrus.WithError(err).Error("Failed to check if media files exist in the database")
		bot.SendErrorMessage(chatID, tmslang.Translate("error.movies.check_error", map[string]interface{}{
			"Error": err.Error(),
		}))
		return
	} else if exists {
		logrus.Warn("Media already exists")
		bot.SendErrorMessage(chatID, tmslang.Translate("error.movies.already_exists", nil))
		return
	}

	videoTitle, err := downloaderInstance.GetTitle()
	if err != nil {
		logrus.WithError(err).Error("Failed to get video title")
		bot.SendErrorMessage(chatID, tmslang.Translate("error.downloads.video_title_error", map[string]interface{}{
			"Error": err.Error(),
		}))
		return
	}

	fileSize, err := downloaderInstance.GetFileSize()
	if err != nil {
		logrus.WithError(err).Error("Failed to get file size")
		bot.SendErrorMessage(chatID, tmslang.Translate("error.downloads.video_size_error", nil))
		return
	}
	if !filemanager.HasEnoughSpace(tmsconfig.GlobalConfig.MoviePath, fileSize) {
		logrus.Warn("Not enough space for the download")
		bot.SendErrorMessage(chatID, tmslang.Translate("error.storage.not_enough_space", nil))
		return
	}

	movieID, progressChan, errChan, err := tmsdmanager.GlobalDownloadManager.StartDownload(downloaderInstance)
	if err != nil {
		logrus.WithError(err).Error("Failed to start download")
		bot.SendErrorMessage(chatID, tmslang.Translate("error.downloads.document_download_error", nil))
		return
	}

	user, err := tmsdb.GlobalDB.GetUserByChatID(context.Background(), chatID)
	if err != nil {
		logrus.WithError(err).Error("Failed to fetch user for download history")
	}

	if err := tmsdb.GlobalDB.AddDownloadHistory(context.Background(), user.ID, uint(movieID)); err != nil {
		logrus.WithError(err).Error("Failed to record download history")
	}

	bot.SendSuccessMessage(chatID, tmslang.Translate("general.video_downloading", map[string]interface{}{
		"Title": videoTitle,
	}))

	go handleDownloadCompletion(bot, chatID, downloaderInstance, movieID, videoTitle, progressChan, errChan)
}

func handleDownloadCompletion(bot *tmsbot.Bot, chatID int64, downloaderInstance tmsdownloader.Downloader, movieID int, videoTitle string, progressChan <-chan float64, errChan <-chan error) {
	for range progressChan {
	}
	err := <-errChan
	if downloaderInstance.StoppedManually() {
		logrus.Info("Download was manually stopped")
		bot.SendSuccessMessage(chatID, tmslang.Translate("general.download_stopped", map[string]interface{}{
			"Title": videoTitle,
		}))
	} else if err != nil {
		logrus.WithError(err).Error("Download failed")
		if err := filemanager.DeleteMovie(movieID); err != nil {
			logrus.WithError(err).Error("Failed to delete movie after download failed")
		}
		bot.SendErrorMessage(chatID, tmslang.Translate("error.downloads.video_download_error", map[string]interface{}{
			"Error": err.Error(),
		}))
	} else {
		logrus.Info("Download completed successfully")
		bot.SendSuccessMessage(chatID, tmslang.Translate("general.video_successfully_downloaded", map[string]interface{}{
			"Title": videoTitle,
		}))
		if err := filemanager.DeleteTemporaryFilesByMovieID(movieID); err != nil {
			logrus.WithError(err).Error("Failed to delete temporary files after download")
		}
	}
}

func HandleDownloadLink(bot *tmsbot.Bot, update tgbotapi.Update) {
	message := update.Message
	chatID := message.Chat.ID

	logrus.WithField("link", message.Text).Info("Starting download for a valid link")
	downloaderInstance := tmsytdlp.NewYTDLPDownloader(bot, message.Text)
	handleDownload(bot, chatID, downloaderInstance)
}

func HandleTorrentFile(bot *tmsbot.Bot, update tgbotapi.Update) {
	message := update.Message
	chatID := message.Chat.ID
	doc := message.Document

	fileName := doc.FileName
	logrus.WithField("file_name", fileName).Info("Received a torrent file")

	if err := bot.DownloadFile(doc.FileID, fileName); err != nil {
		logrus.WithError(err).Error("Failed to download torrent file")
		bot.SendErrorMessage(chatID, tmslang.Translate("error.downloads.document_download_error", nil))
		return
	}

	downloaderInstance := tmstorrent.NewAria2Downloader(bot, fileName)
	handleDownload(bot, chatID, downloaderInstance)
}

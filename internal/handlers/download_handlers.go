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
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.MovieCheckErrorMsgID, err))
		return
	}
	allFiles := append(tempFiles, mainFiles...)

	if exists, err := tmsdb.GlobalDB.MovieExistsFiles(context.Background(), allFiles); err != nil {
		logrus.WithError(err).Error("Failed to check if media files exist in the database")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.MovieCheckErrorMsgID, err))
		return
	} else if exists {
		logrus.Warn("Media already exists")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.VideoExistsMsgID))
		return
	}

	videoTitle, err := downloaderInstance.GetTitle()
	if err != nil {
		logrus.WithError(err).Error("Failed to get video title")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.VideoTitleErrorMsgID, err))
		return
	}

	fileSize, err := downloaderInstance.GetFileSize()
	if err != nil {
		logrus.WithError(err).Error("Failed to get file size")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.VideoGetSizeErrorMsgID))
		return
	}
	if !filemanager.HasEnoughSpace(tmsconfig.GlobalConfig.MoviePath, fileSize) {
		logrus.Warn("Not enough space for the download")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.NotEnoughSpaceMsgID))
		return
	}

	movieID, progressChan, errChan, err := tmsdmanager.GlobalDownloadManager.StartDownload(downloaderInstance)
	if err != nil {
		logrus.WithError(err).Error("Failed to start download")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.DownloadDocumentErrorMsgID))
		return
	}

	bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.VideoDownloadingMsgID))

	go handleDownloadCompletion(bot, chatID, downloaderInstance, movieID, videoTitle, progressChan, errChan)
}

func handleDownloadCompletion(bot *tmsbot.Bot, chatID int64, downloaderInstance tmsdownloader.Downloader, movieID int, videoTitle string, progressChan <-chan float64, errChan <-chan error) {
	for range progressChan {
	}
	err := <-errChan
	if downloaderInstance.StoppedManually() {
		logrus.Info("Download was manually stopped")
		bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.DownloadCancelledMsgID, videoTitle))
	} else if err != nil {
		logrus.WithError(err).Error("Download failed")
		if err := filemanager.DeleteMovie(movieID); err != nil {
			logrus.WithError(err).Error("Failed to delete movie after download failed")
		}
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.VideoDownloadErrorMsgID, err))
	} else {
		logrus.Info("Download completed successfully")
		bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.VideoSuccessfullyDownloadedMsgID, videoTitle))
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
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.DownloadDocumentErrorMsgID))
		return
	}

	downloaderInstance := tmstorrent.NewAria2Downloader(bot, fileName)
	handleDownload(bot, chatID, downloaderInstance)
}

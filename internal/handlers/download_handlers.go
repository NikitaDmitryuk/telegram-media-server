package handlers

import (
	"context"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	tmstorrent "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/torrent"
	tmsytdlp "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/video"
	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tmsutils "github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

func HandleDownloadLink(bot *tmsbot.Bot, update tgbotapi.Update) {
	message := update.Message
	chatID := message.Chat.ID

	logrus.WithField("link", message.Text).Info("Starting download for a valid link")
	downloaderInstance := tmsytdlp.NewYTDLPDownloader(bot, message.Text)

	mainFile, tempFiles, err := downloaderInstance.GetFiles()
	if err != nil {
		logrus.WithError(err).Error("Failed to get files from downloader instance")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.MovieCheckErrorMsgID, err))
		return
	}
	allfiles := append(tempFiles, mainFile)

	exist, err := tmsdb.GlobalDB.MovieExistsFiles(context.Background(), allfiles)
	if err != nil {
		logrus.WithError(err).Error("Failed to check if media files exist in the database")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.MovieCheckErrorMsgID, err))
		return
	}
	if exist {
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

	filesize, err := downloaderInstance.GetFileSize()
	if err != nil {
		logrus.WithError(err).Error("Failed to get file size")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.VideoGetSizeErrorMsgID))
		return
	}

	if !tmsutils.HasEnoughSpace(tmsconfig.GlobalConfig.MoviePath, filesize) {
		logrus.Warn("Not enough space for the download")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.NotEnoughSpaceMsgID))
		return
	}

	progressChan, err := tmsdmanager.GlobalDownloadManager.StartDownload(downloaderInstance)
	if err != nil {
		logrus.WithError(err).Error("Failed to start download")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.DownloadDocumentErrorMsgID))
		return
	}

	bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.VideoDownloadingMsgID))

	go func() {
		for range progressChan {
		}
		if downloaderInstance.StoppedManually() {
			logrus.Info("Download was manually stopped")
			bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.DownloadCancelledMsgID, videoTitle))
		} else {
			logrus.Info("Download completed successfully")
			bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.VideoSuccessfullyDownloadedMsgID, videoTitle))
		}
	}()
}

func HandleTorrentFile(bot *tmsbot.Bot, update tgbotapi.Update) {
	message := update.Message
	chatID := message.Chat.ID
	doc := message.Document

	fileName := doc.FileName
	logrus.WithField("file_name", fileName).Info("Received a torrent file")

	if err := tmsbot.DownloadFile(bot, doc.FileID, fileName); err != nil {
		logrus.WithError(err).Error("Failed to download torrent file")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.DownloadDocumentErrorMsgID))
		return
	}

	downloaderInstance := tmstorrent.NewAria2Downloader(bot, fileName)

	mainFile, tempFiles, err := downloaderInstance.GetFiles()
	if err != nil {
		logrus.WithError(err).Error("Failed to get files from downloader instance")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.MovieCheckErrorMsgID, err))
		return
	}
	allfiles := append(tempFiles, mainFile)

	exists, err := tmsdb.GlobalDB.MovieExistsFiles(context.Background(), allfiles)
	if err != nil {
		logrus.WithError(err).Error("Failed to check if media files exist in the database")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.MovieCheckErrorMsgID, err))
		return
	}

	if exists {
		logrus.WithField("file_name", fileName).Warn("Media already exist")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.VideoExistsMsgID))
		return
	}

	videoTitle, err := downloaderInstance.GetTitle()
	if err != nil {
		logrus.WithError(err).Error("Failed to get video title")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.VideoTitleErrorMsgID, err))
		return
	}

	filesize, err := downloaderInstance.GetFileSize()
	if err != nil {
		logrus.WithError(err).Error("Failed to get file size")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.VideoGetSizeErrorMsgID))
		return
	}

	if !tmsutils.HasEnoughSpace(tmsconfig.GlobalConfig.MoviePath, filesize) {
		logrus.Warn("Not enough space for the download")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.NotEnoughSpaceMsgID))
		return
	}

	progressChan, err := tmsdmanager.GlobalDownloadManager.StartDownload(downloaderInstance)
	if err != nil {
		logrus.WithError(err).Error("Failed to start download")
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.DownloadDocumentErrorMsgID))
		return
	}

	bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.VideoDownloadingMsgID))

	go func() {
		for range progressChan {
		}
		if downloaderInstance.StoppedManually() {
			logrus.Info("Download was manually stopped")
			bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.DownloadCancelledMsgID, videoTitle))
		} else {
			logrus.Info("Download completed successfully")
			bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.VideoSuccessfullyDownloadedMsgID, videoTitle))
		}
	}()
}

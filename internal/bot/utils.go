package bot

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tmsutils "github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

func DownloadFile(bot *Bot, fileID, fileName string) error {
	file, err := bot.Api.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		logrus.WithError(err).Error("Failed to get file")
		return err
	}

	fileURL := file.Link(bot.Api.Token)
	resp, err := http.Get(fileURL)
	if err != nil {
		logrus.WithError(err).Error("Failed to download file")
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath.Join(tmsconfig.GlobalConfig.MoviePath, fileName))
	if err != nil {
		logrus.WithError(err).Error("Failed to create file")
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		logrus.WithError(err).Error("Failed to save file")
		return err
	}

	logrus.Info("File downloaded successfully")
	return nil
}

func DeleteMovie(bot *Bot, id int) error {
	exist, err := tmsdb.GlobalDB.MovieExistsId(context.Background(), id)
	if !exist {
		logrus.WithError(err).Warn("Movie not found")
		return tmsutils.LogAndReturnError(tmslang.GetMessage(tmslang.MovieNotFoundMsgID), err)
	}

	tmsdmanager.GlobalDownloadManager.StopDownload(id)

	files, err := tmsdb.GlobalDB.GetFilesByMovieID(context.Background(), id)
	if err != nil {
		logrus.WithError(err).Error("Failed to get files by movie ID")
		return tmsutils.LogAndReturnError(tmslang.GetMessage(tmslang.GetFilesErrorMsgID), err)
	}

	logrus.Debugf("Files to delete: %v", files)

	var rootFolder string

	for _, file := range files {
		filePath := filepath.Join(tmsconfig.GlobalConfig.MoviePath, file.FilePath)

		if rootFolder == "" {
			rootFolder = filepath.Dir(filePath)
		}

		fileInfo, err := os.Stat(filePath)
		if err != nil {
			logrus.WithError(err).Warnf("Failed to stat file %s", filePath)
		} else {
			if fileInfo.IsDir() {
				if err := os.RemoveAll(filePath); err != nil {
					logrus.WithError(err).Warnf("Failed to delete folder %s", filePath)
				} else {
					logrus.Infof("Folder %s deleted successfully", filePath)
				}
			} else {
				if err := os.Remove(filePath); err != nil {
					logrus.WithError(err).Warnf("Failed to delete file %s", filePath)
				} else {
					logrus.Infof("File %s deleted successfully", filePath)
				}
			}
		}

		pattern := filePath + "*"
		matchedFiles, err := filepath.Glob(pattern)
		if err != nil {
			logrus.WithError(err).Warnf("Failed to find files matching pattern %s", pattern)
			continue
		}
		for _, matchedFile := range matchedFiles {
			if err := os.Remove(matchedFile); err != nil {
				logrus.WithError(err).Warnf("Failed to delete file %s", matchedFile)
			} else {
				logrus.Infof("File %s deleted successfully", matchedFile)
			}
		}
	}

	if err := tmsdb.GlobalDB.RemoveMovie(context.Background(), id); err != nil {
		logrus.WithError(err).Error("Failed to remove movie from database")
		return tmsutils.LogAndReturnError(tmslang.GetMessage(tmslang.DeleteMovieDBErrorMsgID), err)
	}

	if err := tmsdb.GlobalDB.RemoveFilesByMovieID(context.Background(), id); err != nil {
		logrus.WithError(err).Error("Failed to remove files from database")
		return tmsutils.LogAndReturnError(tmslang.GetMessage(tmslang.DeleteFilesDBErrorMsgID), err)
	}

	if rootFolder != "" && rootFolder != tmsconfig.GlobalConfig.MoviePath && tmsutils.IsEmptyDirectory(rootFolder) {
		if err := os.Remove(rootFolder); err != nil {
			logrus.WithError(err).Warnf("Failed to delete root folder %s", rootFolder)
		} else {
			logrus.Infof("Root folder %s deleted successfully", rootFolder)
		}
	}

	logrus.Infof("Movie with ID %d and all associated files deleted successfully", id)
	return nil
}

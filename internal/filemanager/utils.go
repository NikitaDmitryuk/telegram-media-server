package filemanager

import (
	"os"
	"syscall"

	"context"
	"path/filepath"

	"github.com/sirupsen/logrus"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	tmsutils "github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
)

func HasEnoughSpace(path string, requiredSpace int64) bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		logrus.WithError(err).Error("Failed to get filesystem stats")
		return false
	}
	availableSpace := stat.Bavail * uint64(stat.Bsize)

	logrus.WithFields(logrus.Fields{
		"required_space":  requiredSpace,
		"available_space": availableSpace,
	}).Info("Checking available disk space")

	return availableSpace >= uint64(requiredSpace)
}

func IsEmptyDirectory(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		logrus.WithError(err).Errorf("Failed to read directory: %s", dir)
		return false
	}

	logrus.WithField("directory", dir).Info("Checking if directory is empty")
	return len(entries) == 0
}

func DeleteTemporaryFilesByMovieID(movieID int) error {
	tempFiles, err := tmsdb.GlobalDB.GetTempFilesByMovieID(context.Background(), movieID)
	if err != nil {
		logrus.WithError(err).Error("Failed to get temporary files by movie ID")
		return tmsutils.LogAndReturnError("Failed to get temporary files", err)
	}

	logrus.Debugf("Temporary files to delete: %v", tempFiles)

	if err := deleteFiles(tempFiles); err != nil {
		logrus.WithError(err).Error("Failed to delete temporary files")
		return err
	}

	if err := tmsdb.GlobalDB.RemoveTempFilesByMovieID(context.Background(), movieID); err != nil {
		logrus.WithError(err).Error("Failed to remove temporary files from database")
		return tmsutils.LogAndReturnError("Failed to remove temporary files from database", err)
	}

	logrus.Infof("Temporary files for movie ID %d deleted successfully", movieID)
	return nil
}

func DeleteMainFilesByMovieID(movieID int) error {
	mainFiles, err := tmsdb.GlobalDB.GetFilesByMovieID(context.Background(), movieID)
	if err != nil {
		logrus.WithError(err).Error("Failed to get main files by movie ID")
		return tmsutils.LogAndReturnError("Failed to get main files", err)
	}

	logrus.Debugf("Main files to delete: %v", mainFiles)

	if err := deleteFiles(mainFiles); err != nil {
		logrus.WithError(err).Error("Failed to delete main files")
		return err
	}

	if err := tmsdb.GlobalDB.RemoveFilesByMovieID(context.Background(), movieID); err != nil {
		logrus.WithError(err).Error("Failed to remove main files from database")
		return tmsutils.LogAndReturnError("Failed to remove main files from database", err)
	}

	logrus.Infof("Main files for movie ID %d deleted successfully", movieID)
	return nil
}

func DeleteMovie(movieID int) error {
	exist, err := tmsdb.GlobalDB.MovieExistsId(context.Background(), movieID)
	if !exist {
		logrus.WithError(err).Warn("Movie not found")
		return tmsutils.LogAndReturnError("Movie not found", err)
	}

	tmsdmanager.GlobalDownloadManager.StopDownload(movieID)

	if err := DeleteTemporaryFilesByMovieID(movieID); err != nil {
		logrus.WithError(err).Error("Failed to delete temporary files")
	}

	if err := DeleteMainFilesByMovieID(movieID); err != nil {
		logrus.WithError(err).Error("Failed to delete main files")
	}

	if err := tmsdb.GlobalDB.RemoveMovie(context.Background(), movieID); err != nil {
		logrus.WithError(err).Error("Failed to remove movie from database")
		return tmsutils.LogAndReturnError("Failed to remove movie from database", err)
	}

	logrus.Infof("Movie with ID %d and all associated files deleted successfully", movieID)
	return nil
}

func deleteFiles(files []tmsdb.MovieFile) error {
	foldersToDelete := make(map[string]struct{})

	for _, file := range files {
		filePath := filepath.Join(tmsconfig.GlobalConfig.MoviePath, file.FilePath)

		folderPath := filepath.Dir(filePath)
		foldersToDelete[folderPath] = struct{}{}

		fileInfo, err := os.Stat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				logrus.Warnf("File %s does not exist, skipping", filePath)
			} else {
				logrus.WithError(err).Warnf("Failed to stat file %s", filePath)
			}
			continue
		}

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

	for folderPath := range foldersToDelete {
		if IsEmptyDirectory(folderPath) {
			if err := os.Remove(folderPath); err != nil {
				logrus.WithError(err).Warnf("Failed to delete folder %s", folderPath)
			} else {
				logrus.Infof("Folder %s deleted successfully", folderPath)
			}
		}
	}

	return nil
}

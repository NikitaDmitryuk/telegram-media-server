package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tmsutils "github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
)

func HasEnoughSpace(path string, requiredSpace int64) bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		logutils.Log.WithError(err).Error("Failed to get filesystem stats")
		return false
	}
	availableSpace := stat.Bavail * uint64(stat.Bsize) // #nosec G115

	logutils.Log.WithFields(map[string]any{
		"required_space":  requiredSpace,
		"available_space": availableSpace,
	}).Info("Checking available disk space")

	if requiredSpace < 0 {
		logutils.Log.Error("Required space cannot be negative")
		return false
	}
	return availableSpace >= uint64(requiredSpace)
}

func IsEmptyDirectory(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		logutils.Log.WithError(err).Errorf("Failed to read directory: %s", dir)
		return false
	}

	logutils.Log.WithField("directory", dir).Info("Checking if directory is empty")
	return len(entries) == 0
}

func DeleteTemporaryFilesByMovieID(movieID uint) error {
	return deleteFilesByType(movieID, true)
}

func DeleteMainFilesByMovieID(movieID uint) error {
	return deleteFilesByType(movieID, false)
}

func DeleteMovie(movieID uint) error {
	exist, err := tmsdb.GlobalDB.MovieExistsId(context.Background(), movieID)
	if !exist {
		logutils.Log.WithError(err).Warn("Movie not found")
		return tmsutils.LogAndReturnError("Movie not found", err)
	}

	tmsdmanager.GlobalDownloadManager.StopDownload(movieID)

	if err := DeleteTemporaryFilesByMovieID(movieID); err != nil {
		logutils.Log.WithError(err).Error("Failed to delete temporary files")
	}

	if err := DeleteMainFilesByMovieID(movieID); err != nil {
		logutils.Log.WithError(err).Error("Failed to delete main files")
	}

	if err := tmsdb.GlobalDB.RemoveMovie(context.Background(), movieID); err != nil {
		logutils.Log.WithError(err).Error("Failed to remove movie from database")
		return tmsutils.LogAndReturnError("Failed to remove movie from database", err)
	}

	logutils.Log.Infof("Movie with ID %d and all associated files deleted successfully", movieID)
	return nil
}

func deleteFilesByType(movieID uint, isTemp bool) error {
	var files []tmsdb.MovieFile
	var err error

	if isTemp {
		files, err = tmsdb.GlobalDB.GetTempFilesByMovieID(context.Background(), movieID)
	} else {
		files, err = tmsdb.GlobalDB.GetFilesByMovieID(context.Background(), movieID)
	}

	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get files by movie ID")
		return tmsutils.LogAndReturnError("Failed to get files", err)
	}

	logutils.Log.Debugf("Files to delete (relative): %v", files)

	var expandedFiles []tmsdb.MovieFile
	for _, file := range files {
		absPath := filepath.Join(tmsconfig.GlobalConfig.MoviePath, file.FilePath)
		if isTemp && strings.Contains(file.FilePath, "*") {
			matches, globErr := filepath.Glob(absPath)
			if globErr != nil {
				logutils.Log.WithError(globErr).Warnf("Error while globbing files by pattern: %s", absPath)
				continue
			}
			for _, match := range matches {
				rel, relErr := filepath.Rel(tmsconfig.GlobalConfig.MoviePath, match)
				if relErr != nil {
					logutils.Log.WithError(relErr).Warnf("Error while getting relative path for %s", match)
					continue
				}
				expandedFiles = append(expandedFiles, tmsdb.MovieFile{FilePath: rel})
			}
		} else {
			expandedFiles = append(expandedFiles, file)
		}
	}

	for _, file := range expandedFiles {
		logutils.Log.Debugf("Deleting file (relative): %s", file.FilePath)
	}

	if deleteErr := deleteFiles(expandedFiles); deleteErr != nil {
		logutils.Log.WithError(deleteErr).Error("Failed to delete files")
		return deleteErr
	}

	if isTemp {
		err = tmsdb.GlobalDB.RemoveTempFilesByMovieID(context.Background(), movieID)
	} else {
		err = tmsdb.GlobalDB.RemoveFilesByMovieID(context.Background(), movieID)
	}

	if err != nil {
		logutils.Log.WithError(err).Error("Failed to remove files from database")
		return tmsutils.LogAndReturnError("Failed to remove files from database", err)
	}

	logutils.Log.Infof("Files for movie ID %d deleted successfully", movieID)
	return nil
}

func deleteFiles(files []tmsdb.MovieFile) error {
	foldersToDelete := make(map[string]struct{})

	for _, file := range files {
		filePath := filepath.Join(tmsconfig.GlobalConfig.MoviePath, file.FilePath)
		folderPath := filepath.Dir(filePath)
		foldersToDelete[folderPath] = struct{}{}

		if err := deleteFileOrFolder(filePath); err != nil {
			logutils.Log.WithError(err).Warnf("Failed to delete file or folder: %s", filePath)
		}
	}

	for folderPath := range foldersToDelete {
		if IsEmptyDirectory(folderPath) {
			if err := os.Remove(folderPath); err != nil {
				logutils.Log.WithError(err).Warnf("Failed to delete folder %s", folderPath)
			} else {
				logutils.Log.Infof("Folder %s deleted successfully", folderPath)
			}
		}
	}

	return nil
}

func deleteFileOrFolder(path string) error {
	const (
		maxAttempts      = 5
		retrySleepMillis = 500
	)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		fileInfo, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				logutils.Log.Warnf("File %s does not exist, skipping", path)
				return nil
			}
			return err
		}

		canDelete := true

		if !fileInfo.IsDir() {
			f, openErr := os.OpenFile(path, os.O_RDWR|os.O_EXCL, 0)
			if openErr != nil {
				canDelete = false
			}
			if f != nil {
				f.Close()
			}
		}

		if canDelete {
			if fileInfo.IsDir() {
				return os.RemoveAll(path)
			}
			return os.Remove(path)
		}
		logutils.Log.Warnf("File %s is in use, attempt %d/%d, retrying...", path, attempt, maxAttempts)
		time.Sleep(time.Duration(retrySleepMillis) * time.Millisecond)
	}

	logutils.Log.Errorf("Failed to delete %s after %d attempts: file is in use", path, maxAttempts)
	return tmsutils.LogAndReturnError("File is in use and cannot be deleted", nil)
}

func GetAvailableSpaceGB(path string) (float64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		logutils.Log.WithError(err).Error("Failed to get filesystem stats")
		return 0, err
	}
	availableSpaceGB := float64(int64(stat.Bavail)*stat.Bsize) / (1024 * 1024 * 1024) // #nosec G115
	return availableSpaceGB, nil
}

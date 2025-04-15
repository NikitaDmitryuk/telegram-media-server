package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/database"

	"github.com/sirupsen/logrus"
)

func DeleteTempFilesByMovieID(movieID int) error {
	tempFiles, err := tmsdb.GlobalDB.GetTempFilesByMovieID(context.Background(), movieID)
	if err != nil {
		logrus.WithError(err).Errorf("Failed to get temp files for movie ID %d", movieID)
		return fmt.Errorf("failed to get temp files for movie ID %d: %w", movieID, err)
	}

	for _, tempFile := range tempFiles {
		filePath := filepath.Join(tmsconfig.GlobalConfig.MoviePath, tempFile.FilePath)

		if err := os.Remove(filePath); err != nil {
			logrus.WithError(err).Warnf("Failed to delete temp file %s", filePath)
		} else {
			logrus.Infof("Temp file %s deleted successfully", filePath)
		}
	}

	if err := tmsdb.GlobalDB.RemoveTempFilesByMovieID(context.Background(), movieID); err != nil {
		logrus.Errorf("Temp files for movie ID %d delete failed", movieID)
	} else {
		logrus.Infof("All temp files for movie ID %d deleted successfully", movieID)
	}

	return nil
}

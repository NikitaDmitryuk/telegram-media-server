package database

import (
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
)

func (s *SQLiteDatabase) MigrateToFileSize() error {
	logger.Log.Info("Starting migration: adding file_size field and removing download_histories table")

	if err := s.db.Exec("ALTER TABLE movies ADD COLUMN file_size INTEGER NOT NULL DEFAULT 0").Error; err != nil {
		if err.Error() != "duplicate column name: file_size" {
			logger.Log.WithError(err).Error("Failed to add file_size column")
			return err
		}
		logger.Log.Info("file_size column already exists")
	} else {
		logger.Log.Info("Successfully added file_size column to movies table")
	}

	if err := s.db.Migrator().DropTable("download_histories"); err != nil {
		logger.Log.WithError(err).Warn("Failed to drop download_histories table (it may not exist)")
	} else {
		logger.Log.Info("Successfully dropped download_histories table")
	}

	logger.Log.Info("Migration completed successfully")
	return nil
}

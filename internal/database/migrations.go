package database

import (
	"strings"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

func (s *SQLiteDatabase) MigrateToFileSize() error {
	logutils.Log.Info("Starting migration: adding file_size field and removing download_histories table")

	if err := s.db.Exec("ALTER TABLE movies ADD COLUMN file_size INTEGER NOT NULL DEFAULT 0").Error; err != nil {
		if err.Error() != "duplicate column name: file_size" {
			logutils.Log.WithError(err).Error("Failed to add file_size column")
			return err
		}
		logutils.Log.Info("file_size column already exists")
	} else {
		logutils.Log.Info("Successfully added file_size column to movies table")
	}

	if err := s.db.Migrator().DropTable("download_histories"); err != nil {
		logutils.Log.WithError(err).Warn("Failed to drop download_histories table (it may not exist)")
	} else {
		logutils.Log.Info("Successfully dropped download_histories table")
	}

	if err := s.db.Exec("ALTER TABLE movies ADD COLUMN qbittorrent_hash TEXT NOT NULL DEFAULT ''").Error; err != nil {
		if !strings.Contains(err.Error(), "duplicate column") {
			logutils.Log.WithError(err).Error("Failed to add qbittorrent_hash column")
			return err
		}
		logutils.Log.Info("qbittorrent_hash column already exists")
	} else {
		logutils.Log.Info("Successfully added qbittorrent_hash column to movies table")
	}

	logutils.Log.Info("Migration completed successfully")
	return nil
}

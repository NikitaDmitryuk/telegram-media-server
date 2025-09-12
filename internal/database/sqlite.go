package database

import (
	"fmt"
	"path/filepath"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type SQLiteDatabase struct {
	db *gorm.DB
}

func NewSQLiteDatabase() *SQLiteDatabase {
	return &SQLiteDatabase{}
}

func (s *SQLiteDatabase) Init(config *tmsconfig.Config) error {
	dbPath := filepath.Join(config.MoviePath, "movie.db")

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	s.db = db

	if err := s.runMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	logutils.Log.Info("Database initialized successfully")
	return nil
}

func (s *SQLiteDatabase) runMigrations() error {
	if err := s.db.AutoMigrate(&Movie{}, &MovieFile{}, &User{}, &TemporaryPassword{}); err != nil {
		return fmt.Errorf("auto migration failed: %w", err)
	}

	if err := s.MigrateToFileSize(); err != nil {
		return fmt.Errorf("file size migration failed: %w", err)
	}

	return nil
}

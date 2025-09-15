package database

import (
	"fmt"
	"path/filepath"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type SQLiteDatabase struct {
	db *gorm.DB
}

func NewSQLiteDatabase() *SQLiteDatabase {
	return &SQLiteDatabase{}
}

func (s *SQLiteDatabase) Init(config *domain.Config) error {
	dbPath := filepath.Join(config.MoviePath, "movie.db")

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	s.db = db

	if err := s.runMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	logger.Log.Info("Database initialized successfully")
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

// Close закрывает соединение с базой данных
func (s *SQLiteDatabase) Close() error {
	if s.db == nil {
		return nil
	}

	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("failed to close database connection: %w", err)
	}

	logger.Log.Info("Database connection closed successfully")
	return nil
}

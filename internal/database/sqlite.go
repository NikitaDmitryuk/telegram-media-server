package database

import (
	"fmt"
	"path/filepath"
	"time"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type SQLiteDatabase struct {
	db *gorm.DB
}

const (
	sqliteMaxIdleTime     = 5 * time.Minute
	sqliteConnMaxLifetime = 30 * time.Minute
)

func NewSQLiteDatabase() *SQLiteDatabase {
	return &SQLiteDatabase{}
}

func (s *SQLiteDatabase) Init(config *tmsconfig.Config) error {
	dbPath := filepath.Join(config.MoviePath, "movie.db")
	dsn := dbPath + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on"

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	s.db = db
	if err := s.configureConnectionPool(); err != nil {
		return err
	}

	if err := s.runMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	logutils.Log.Info("Database initialized successfully")
	return nil
}

func (s *SQLiteDatabase) configureConnectionPool() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("failed to configure database connection pool: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxIdleTime(sqliteMaxIdleTime)
	sqlDB.SetConnMaxLifetime(sqliteConnMaxLifetime)
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

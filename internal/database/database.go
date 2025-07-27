package database

import (
	"context"
	"time"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

type Database interface {
	Init(config *tmsconfig.Config) error
	AddMovie(ctx context.Context, name string, mainFiles, tempFiles []string) (uint, error)
	RemoveMovie(ctx context.Context, movieID uint) error
	GetMovieList(ctx context.Context) ([]Movie, error)
	GetTempFilesByMovieID(ctx context.Context, movieID uint) ([]MovieFile, error)
	UpdateDownloadedPercentage(ctx context.Context, movieID uint, percentage int) error
	SetLoaded(ctx context.Context, movieID uint) error
	GetMovieByID(ctx context.Context, movieID uint) (Movie, error)
	MovieExistsFiles(ctx context.Context, files []string) (bool, error)
	MovieExistsId(ctx context.Context, movieID uint) (bool, error)
	GetFilesByMovieID(ctx context.Context, movieID uint) ([]MovieFile, error)
	RemoveFilesByMovieID(ctx context.Context, movieID uint) error
	RemoveTempFilesByMovieID(ctx context.Context, movieID uint) error
	MovieExistsUploadedFile(ctx context.Context, fileName string) (bool, error)
	Login(ctx context.Context, password string, chatID int64, userName string, config *tmsconfig.Config) (bool, error)
	GetUserRole(ctx context.Context, chatID int64) (UserRole, error)
	IsUserAccessAllowed(ctx context.Context, chatID int64) (bool, UserRole, error)
	AssignTemporaryPassword(ctx context.Context, password string, chatID int64) error
	ExtendTemporaryUser(ctx context.Context, chatID int64, newExpiration time.Time) error
	GenerateTemporaryPassword(ctx context.Context, duration time.Duration) (string, error)
	AddDownloadHistory(ctx context.Context, userID, movieID uint) error
	GetUserByChatID(ctx context.Context, chatID int64) (User, error)
}

var GlobalDB Database

func InitDatabase(config *tmsconfig.Config) error {
	database := NewSQLiteDatabase()
	if err := database.Init(config); err != nil {
		logutils.Log.WithError(err).Error("Failed to initialize the database")
		return err
	}

	GlobalDB = database
	logutils.Log.Info("Database initialized successfully")
	return nil
}

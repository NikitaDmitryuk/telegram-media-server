package database

import (
	"context"
	"time"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/sirupsen/logrus"
)

type Database interface {
	Init(config *tmsconfig.Config) error
	AddMovie(ctx context.Context, name string, mainFiles []string, tempFiles []string) (int, error)
	RemoveMovie(ctx context.Context, movieID int) error
	GetMovieList(ctx context.Context) ([]Movie, error)
	GetTempFilesByMovieID(ctx context.Context, movieID int) ([]MovieFile, error)
	UpdateDownloadedPercentage(ctx context.Context, id int, percentage int) error
	SetLoaded(ctx context.Context, id int) error
	GetMovieByID(ctx context.Context, movieID int) (Movie, error)
	MovieExistsFiles(ctx context.Context, files []string) (bool, error)
	MovieExistsId(ctx context.Context, id int) (bool, error)
	GetFilesByMovieID(ctx context.Context, movieID int) ([]MovieFile, error)
	RemoveFilesByMovieID(ctx context.Context, movieID int) error
	RemoveTempFilesByMovieID(ctx context.Context, movieID int) error
	MovieExistsUploadedFile(ctx context.Context, fileName string) (bool, error)
	Login(ctx context.Context, password string, chatID int64, userName string) (bool, error)
	GetUserRole(ctx context.Context, chatID int64) (UserRole, error)
	IsUserAccessAllowed(ctx context.Context, chatID int64) (bool, UserRole, error)
	AssignTemporaryPassword(ctx context.Context, password string, chatID int64) error
	ExtendTemporaryUser(ctx context.Context, chatID int64, newExpiration time.Time) error
	GenerateTemporaryPassword(ctx context.Context, duration time.Duration) (string, error)
	AddDownloadHistory(ctx context.Context, userID uint, movieID uint) error
	GetUserByChatID(ctx context.Context, chatID int64) (User, error)
}

var GlobalDB Database

func init() {
	database := NewSQLiteDatabase()
	if err := database.Init(tmsconfig.GlobalConfig); err != nil {
		logrus.WithError(err).Fatal("Failed to initialize the database")
	} else {
		GlobalDB = database
		logrus.Info("Database initialized successfully")
	}
}

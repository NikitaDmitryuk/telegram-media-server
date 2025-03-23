package database

import (
	"context"
	"fmt"
	"path/filepath"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type SQLiteDatabase struct {
	db *gorm.DB
}

func NewSQLiteDatabase() *SQLiteDatabase {
	return &SQLiteDatabase{}
}

func (s *SQLiteDatabase) Init(config *tmsconfig.Config) error {
	dbPath := filepath.Join(config.MoviePath, "movie.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		logrus.Error("Failed to open the database: ", err)
		return fmt.Errorf("failed to open the database: %v", err)
	}
	s.db = db

	if err := s.db.AutoMigrate(&Movie{}, &MovieFile{}, &User{}); err != nil {
		logrus.Error("Failed to perform migration: ", err)
		return fmt.Errorf("failed to perform migration: %v", err)
	}
	return nil
}

func (s *SQLiteDatabase) AddMovie(ctx context.Context, name string, mainFile string, tempFiles []string) (int, error) {
	movie := Movie{Name: name}
	if err := s.db.WithContext(ctx).Create(&movie).Error; err != nil {
		logrus.Error("Failed to add movie: ", err)
		return 0, err
	}

	main_file := MovieFile{MovieID: movie.ID, FilePath: mainFile, TempFile: false}
	if err := s.db.WithContext(ctx).Create(&main_file).Error; err != nil {
		logrus.Error("Failed to add main file: ", err)
		return 0, err
	}

	for _, filePath := range tempFiles {
		movieFile := MovieFile{MovieID: movie.ID, FilePath: filePath, TempFile: true}
		if err := s.db.WithContext(ctx).Create(&movieFile).Error; err != nil {
			logrus.Error("Failed to add temp file: ", err)
			return 0, err
		}
	}
	logrus.Debug("Movie added successfully with ID: ", movie.ID)
	return int(movie.ID), nil
}

func (s *SQLiteDatabase) RemoveMovie(ctx context.Context, movieID int) error {
	if err := s.db.WithContext(ctx).Delete(&Movie{}, movieID).Error; err != nil {
		logrus.Error("Failed to remove movie: ", err)
		return err
	}
	logrus.Debug("Movie removed successfully with ID: ", movieID)
	return nil
}

func (s *SQLiteDatabase) GetMovieList(ctx context.Context) ([]Movie, error) {
	var movies []Movie
	if err := s.db.WithContext(ctx).Find(&movies).Error; err != nil {
		logrus.Error("Failed to fetch movie list: ", err)
		return nil, err
	}
	logrus.Debug("Movie list fetched successfully")
	return movies, nil
}

func (s *SQLiteDatabase) UpdateDownloadedPercentage(ctx context.Context, id int, percentage int) error {
	if err := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", id).Update("downloaded_percentage", percentage).Error; err != nil {
		logrus.Error("Failed to update downloaded percentage: ", err)
		return err
	}
	logrus.Debug("Downloaded percentage updated successfully for movie ID: ", id)
	return nil
}

func (s *SQLiteDatabase) SetLoaded(ctx context.Context, id int) error {
	if err := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", id).Update("downloaded_percentage", 100).Error; err != nil {
		logrus.Error("Failed to set movie as loaded: ", err)
		return err
	}
	logrus.Debug("Movie set as loaded successfully with ID: ", id)
	return nil
}

func (s *SQLiteDatabase) GetMovieByID(ctx context.Context, movieID int) (Movie, error) {
	var movie Movie
	if err := s.db.WithContext(ctx).First(&movie, movieID).Error; err != nil {
		logrus.Error("Failed to fetch movie by ID: ", err)
		return Movie{}, err
	}
	logrus.Debug("Movie fetched successfully with ID: ", movieID)
	return movie, nil
}

func (s *SQLiteDatabase) MovieExistsFiles(ctx context.Context, files []string) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&MovieFile{}).Where("file_path IN ?", files).Count(&count).Error; err != nil {
		logrus.Error("Failed to check if any of the files exist: ", err)
		return false, err
	}
	logrus.Debug("File existence check completed for provided files")
	return count > 0, nil
}

func (s *SQLiteDatabase) MovieExistsId(ctx context.Context, id int) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", id).Count(&count).Error; err != nil {
		logrus.Error("Failed to check if movie exists by ID: ", err)
		return false, err
	}
	logrus.Debug("Movie existence check completed for ID: ", id)
	return count > 0, nil
}

func (s *SQLiteDatabase) GetFilesByMovieID(ctx context.Context, movieID int) ([]MovieFile, error) {
	var files []MovieFile
	if err := s.db.WithContext(ctx).Where("movie_id = ?", movieID).Find(&files).Error; err != nil {
		logrus.Error("Failed to fetch files by movie ID: ", err)
		return nil, err
	}
	logrus.Debug("Files fetched successfully for movie ID: ", movieID)
	return files, nil
}

func (s *SQLiteDatabase) RemoveFilesByMovieID(ctx context.Context, movieID int) error {
	if err := s.db.WithContext(ctx).Where("movie_id = ?", movieID).Delete(&MovieFile{}).Error; err != nil {
		logrus.Error("Failed to remove files by movie ID: ", err)
		return err
	}
	logrus.Debug("Files removed successfully for movie ID: ", movieID)
	return nil
}

func (s *SQLiteDatabase) MovieExistsUploadedFile(ctx context.Context, fileName string) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&MovieFile{}).Where("file_path = ?", fileName).Count(&count).Error; err != nil {
		logrus.Error("Failed to check if uploaded file exists: ", err)
		return false, err
	}
	logrus.Debug("Uploaded file existence check completed for file: ", fileName)
	return count > 0, nil
}

func (s *SQLiteDatabase) Login(ctx context.Context, password string, chatID int64, userName string) (bool, error) {
	if password != tmsconfig.GlobalConfig.Password {
		logrus.Warn("Invalid password attempt")
		return false, nil
	}
	user := User{Name: userName, ChatID: chatID}
	if err := s.db.WithContext(ctx).Create(&user).Error; err != nil {
		logrus.Error("Failed to add user: ", err)
		return false, err
	}
	logrus.Debug("User logged in successfully with chat ID: ", chatID)
	return true, nil
}

func (s *SQLiteDatabase) CheckUser(ctx context.Context, chatID int64) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&User{}).Where("chat_id = ?", chatID).Count(&count).Error; err != nil {
		logrus.Error("Failed to check if user exists: ", err)
		return false, err
	}
	logrus.Debug("User existence check completed for chat ID: ", chatID)
	return count > 0, nil
}

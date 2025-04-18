package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"time"

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

	if err := s.db.AutoMigrate(&Movie{}, &MovieFile{}, &User{}, &DownloadHistory{}, &TemporaryPassword{}, &DownloadHistory{}); err != nil {
		logrus.Error("Failed to perform migration: ", err)
		return fmt.Errorf("failed to perform migration: %v", err)
	}
	return nil
}

func (s *SQLiteDatabase) AddMovie(ctx context.Context, name string, mainFiles []string, tempFiles []string) (int, error) {
	movie := Movie{Name: name}
	if err := s.db.WithContext(ctx).Create(&movie).Error; err != nil {
		logrus.Error("Failed to add movie: ", err)
		return 0, err
	}

	for _, mainFile := range mainFiles {
		mainFileRecord := MovieFile{MovieID: movie.ID, FilePath: mainFile, TempFile: false}
		if err := s.db.WithContext(ctx).Create(&mainFileRecord).Error; err != nil {
			logrus.Error("Failed to add main file: ", err)
			return 0, err
		}
	}

	for _, filePath := range tempFiles {
		tempFileRecord := MovieFile{MovieID: movie.ID, FilePath: filePath, TempFile: true}
		if err := s.db.WithContext(ctx).Create(&tempFileRecord).Error; err != nil {
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

func (s *SQLiteDatabase) RemoveTempFilesByMovieID(ctx context.Context, movieID int) error {
	if err := s.db.WithContext(ctx).Where("movie_id = ? AND temp_file = ?", movieID, true).Delete(&MovieFile{}).Error; err != nil {
		logrus.WithError(err).Errorf("Failed to get temp files for movie ID %d", movieID)
		return err
	}
	logrus.Debug("Temp files removed successfully for movie ID: ", movieID)
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

func (s *SQLiteDatabase) GetTempFilesByMovieID(ctx context.Context, movieID int) ([]MovieFile, error) {
	var tempFiles []MovieFile
	if err := s.db.WithContext(ctx).Where("movie_id = ? AND temp_file = ?", movieID, true).Find(&tempFiles).Error; err != nil {
		logrus.WithError(err).Errorf("Failed to get temp files for movie ID %d", movieID)
		return nil, err
	}
	return tempFiles, nil
}

func (s *SQLiteDatabase) Login(ctx context.Context, password string, chatID int64, userName string) (bool, error) {
	if password == tmsconfig.GlobalConfig.AdminPassword {
		user := User{
			Name:   userName,
			ChatID: chatID,
			Role:   AdminRole,
		}
		if err := s.db.WithContext(ctx).Where("chat_id = ?", chatID).FirstOrCreate(&user).Error; err != nil {
			logrus.WithError(err).Error("Failed to create or update admin user")
			return false, err
		}

		if err := s.db.WithContext(ctx).Model(&user).Updates(User{Name: userName, ChatID: chatID, Role: AdminRole}).Error; err != nil {
			logrus.WithError(err).Error("Failed to update admin user")
			return false, err
		}

		logrus.Infof("Admin user logged in successfully: %s", userName)
		return true, nil
	}

	if password == tmsconfig.GlobalConfig.RegularPassword {
		user := User{
			Name:   userName,
			ChatID: chatID,
			Role:   RegularRole,
		}
		if err := s.db.WithContext(ctx).Where("chat_id = ?", chatID).FirstOrCreate(&user).Error; err != nil {
			logrus.WithError(err).Error("Failed to create or update regular user")
			return false, err
		}

		if err := s.db.WithContext(ctx).Model(&user).Updates(User{Name: userName, ChatID: chatID, Role: RegularRole}).Error; err != nil {
			logrus.WithError(err).Error("Failed to update regular user")
			return false, err
		}

		logrus.Infof("Regular user logged in successfully: %s", userName)
		return true, nil
	}

	var tempPass TemporaryPassword
	if err := s.db.WithContext(ctx).Where("password = ?", password).First(&tempPass).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logrus.Warnf("Invalid password attempt by user: %s", userName)
			return false, nil
		}
		logrus.WithError(err).Error("Failed to check temporary password")
		return false, err
	}

	if time.Now().After(tempPass.ExpiresAt) {
		logrus.Warnf("Temporary password expired for user: %s", userName)
		return false, nil
	}

	var user User
	if err := s.db.WithContext(ctx).Where("chat_id = ?", chatID).FirstOrCreate(&user).Error; err != nil {
		logrus.WithError(err).Error("Failed to fetch or create user")
		return false, err
	}

	user.Role = TemporaryRole
	user.ExpiresAt = &tempPass.ExpiresAt
	if err := s.db.WithContext(ctx).Model(&user).Updates(User{Name: userName, ChatID: chatID, Role: TemporaryRole, ExpiresAt: &tempPass.ExpiresAt}).Error; err != nil {
		logrus.WithError(err).Error("Failed to update temporary user")
		return false, err
	}

	if err := s.db.Model(&user).Association("Passwords").Append(&tempPass); err != nil {
		logrus.WithError(err).Error("Failed to associate user with temporary password")
		return false, err
	}

	logrus.Infof("Temporary user logged in successfully: %s", userName)
	return true, nil
}

func (s *SQLiteDatabase) GetUserRole(ctx context.Context, chatID int64) (UserRole, error) {
	var user User
	if err := s.db.WithContext(ctx).Where("chat_id = ?", chatID).First(&user).Error; err != nil {
		logrus.WithError(err).Errorf("Failed to get user role for chat ID %d", chatID)
		return "", err
	}
	logrus.Debugf("User role fetched successfully for chat ID %d: %s", chatID, user.Role)
	return user.Role, nil
}

func (s *SQLiteDatabase) IsUserAccessAllowed(ctx context.Context, chatID int64) (bool, UserRole, error) {
	var user User
	if err := s.db.WithContext(ctx).Preload("Passwords").Where("chat_id = ?", chatID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logrus.Warnf("Access denied for unknown user with chat ID %d: user not found", chatID)
			return false, "", nil
		}
		logrus.WithError(err).Errorf("Failed to check access for chat ID %d", chatID)
		return false, "", err
	}

	if user.Role == TemporaryRole {
		for _, tempPass := range user.Passwords {
			if time.Now().Before(tempPass.ExpiresAt) {
				logrus.Debugf("Access allowed for temporary user with chat ID %d", chatID)
				return true, TemporaryRole, nil
			}
		}
		logrus.Warnf("Access denied for temporary user with chat ID %d: all passwords expired", chatID)
		return false, TemporaryRole, nil
	}

	logrus.Debugf("Access allowed for user with chat ID %d, role: %s", chatID, user.Role)
	return true, user.Role, nil
}

func (s *SQLiteDatabase) AssignTemporaryPassword(ctx context.Context, password string, chatID int64) error {
	var tempPass TemporaryPassword
	if err := s.db.WithContext(ctx).Where("password = ?", password).First(&tempPass).Error; err != nil {
		logrus.WithError(err).Errorf("Failed to find temporary password: %s", password)
		return err
	}

	if time.Now().After(tempPass.ExpiresAt) {
		logrus.Warnf("Cannot assign expired temporary password: %s", password)
		return fmt.Errorf("temporary password expired")
	}

	if err := s.db.WithContext(ctx).Model(&User{}).Where("chat_id = ?", chatID).Update("temp_pass_id", nil).Error; err != nil {
		logrus.WithError(err).Errorf("Failed to clear previous temporary password for user with chat ID %d", chatID)
		return err
	}

	if err := s.db.WithContext(ctx).Model(&User{}).Where("chat_id = ?", chatID).Updates(map[string]interface{}{
		"temp_pass_id": tempPass.ID,
		"expires_at":   tempPass.ExpiresAt,
		"role":         TemporaryRole,
	}).Error; err != nil {
		logrus.WithError(err).Errorf("Failed to assign new temporary password to user with chat ID %d", chatID)
		return err
	}

	logrus.Debugf("New temporary password assigned successfully to user with chat ID %d", chatID)
	return nil
}

func (s *SQLiteDatabase) ExtendTemporaryUser(ctx context.Context, chatID int64, newExpiration time.Time) error {
	if err := s.db.WithContext(ctx).Model(&User{}).Where("chat_id = ? AND role = ?", chatID, TemporaryRole).Update("expires_at", newExpiration).Error; err != nil {
		logrus.WithError(err).Errorf("Failed to extend expiration for temporary user with chat ID %d", chatID)
		return err
	}

	logrus.Debugf("Temporary user with chat ID %d extended until %s", chatID, newExpiration)
	return nil
}

func (s *SQLiteDatabase) GenerateTemporaryPassword(ctx context.Context, duration time.Duration) (string, error) {
	passwordBytes := make([]byte, 16)
	if _, err := rand.Read(passwordBytes); err != nil {
		logrus.WithError(err).Error("Failed to generate random password")
		return "", err
	}
	password := hex.EncodeToString(passwordBytes)

	expiresAt := time.Now().Add(duration)

	tempPass := TemporaryPassword{
		Password:  password,
		ExpiresAt: expiresAt,
	}

	if err := s.db.WithContext(ctx).Create(&tempPass).Error; err != nil {
		logrus.WithError(err).Error("Failed to save temporary password to database")
		return "", err
	}

	logrus.Debugf("Temporary password generated and saved: %s, expires at: %s", password, expiresAt)
	return password, nil
}

func (s *SQLiteDatabase) AddDownloadHistory(ctx context.Context, userID uint, movieID uint) error {
	downloadHistory := DownloadHistory{
		UserID:  userID,
		MovieID: movieID,
	}

	if err := s.db.WithContext(ctx).Create(&downloadHistory).Error; err != nil {
		logrus.WithError(err).Errorf("Failed to add download history for user ID %d and movie ID %d", userID, movieID)
		return err
	}

	logrus.Debugf("Download history added successfully for user ID %d and movie ID %d", userID, movieID)
	return nil
}

func (s *SQLiteDatabase) GetUserByChatID(ctx context.Context, chatID int64) (User, error) {
	var user User
	if err := s.db.WithContext(ctx).Where("chat_id = ?", chatID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logrus.Warnf("User with chat ID %d not found", chatID)
			return User{}, nil
		}
		logrus.WithError(err).Errorf("Failed to fetch user with chat ID %d", chatID)
		return User{}, err
	}

	logrus.Debugf("User fetched successfully with chat ID %d", chatID)
	return user, nil
}

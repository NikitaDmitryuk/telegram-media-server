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
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
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
		logutils.Log.WithError(err).Error("Failed to open the database")
		return fmt.Errorf("failed to open the database: %w", err)
	}
	s.db = db

	if err := s.db.AutoMigrate(&Movie{}, &MovieFile{}, &User{}, &DownloadHistory{}, &TemporaryPassword{}); err != nil {
		logutils.Log.WithError(err).Error("Failed to perform migration")
		return fmt.Errorf("failed to perform migration: %w", err)
	}
	logutils.Log.Info("Database initialized successfully")
	return nil
}

func (s *SQLiteDatabase) AddMovie(ctx context.Context, name string, mainFiles, tempFiles []string) (uint, error) {
	movie := Movie{Name: name}
	if err := s.db.WithContext(ctx).Create(&movie).Error; err != nil {
		logutils.Log.WithError(err).Error("Failed to add movie")
		return 0, err
	}

	if err := s.addFiles(ctx, movie.ID, mainFiles, false); err != nil {
		return 0, err
	}

	if err := s.addFiles(ctx, movie.ID, tempFiles, true); err != nil {
		return 0, err
	}

	logutils.Log.Debugf("Movie added successfully with ID: %d", movie.ID)
	return movie.ID, nil
}

func (s *SQLiteDatabase) addFiles(ctx context.Context, movieID uint, files []string, isTemp bool) error {
	for _, filePath := range files {
		fileRecord := MovieFile{MovieID: movieID, FilePath: filePath, TempFile: isTemp}
		if err := s.db.WithContext(ctx).Create(&fileRecord).Error; err != nil {
			logutils.Log.WithError(err).Errorf("Failed to add file: %s", filePath)
			return err
		}
	}
	return nil
}

func (s *SQLiteDatabase) RemoveMovie(ctx context.Context, movieID uint) error {
	if err := s.db.WithContext(ctx).Delete(&Movie{}, movieID).Error; err != nil {
		logutils.Log.WithError(err).Error("Failed to remove movie")
		return err
	}
	logutils.Log.Debugf("Movie removed successfully with ID: %d", movieID)
	return nil
}

func (s *SQLiteDatabase) GetMovieList(ctx context.Context) ([]Movie, error) {
	var movies []Movie
	if err := s.db.WithContext(ctx).Find(&movies).Error; err != nil {
		logutils.Log.WithError(err).Error("Failed to fetch movie list")
		return nil, err
	}
	logutils.Log.Debug("Movie list fetched successfully")
	return movies, nil
}

func (s *SQLiteDatabase) UpdateDownloadedPercentage(ctx context.Context, movieID uint, percentage int) error {
	if err := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", movieID).Update("downloaded_percentage", percentage).Error; err != nil {
		logutils.Log.WithError(err).Error("Failed to update downloaded percentage")
		return err
	}
	logutils.Log.Debugf("Downloaded percentage updated successfully for movie ID: %d", movieID)
	return nil
}

func (s *SQLiteDatabase) SetLoaded(ctx context.Context, movieID uint) error {
	const fullyLoaded = 100
	if err := s.UpdateDownloadedPercentage(ctx, movieID, fullyLoaded); err != nil {
		logutils.Log.WithError(err).Error("Failed to set movie as loaded")
		return err
	}
	logutils.Log.Debugf("Movie set as loaded successfully with ID: %d", movieID)
	return nil
}

func (s *SQLiteDatabase) GetMovieByID(ctx context.Context, movieID uint) (Movie, error) {
	var movie Movie
	if err := s.db.WithContext(ctx).First(&movie, movieID).Error; err != nil {
		logutils.Log.WithError(err).Error("Failed to fetch movie by ID")
		return Movie{}, err
	}
	logutils.Log.Debugf("Movie fetched successfully with ID: %d", movieID)
	return movie, nil
}

func (s *SQLiteDatabase) MovieExistsFiles(ctx context.Context, files []string) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&MovieFile{}).Where("file_path IN ?", files).Count(&count).Error; err != nil {
		logutils.Log.WithError(err).Error("Failed to check if any of the files exist")
		return false, err
	}
	logutils.Log.Debug("File existence check completed for provided files")
	return count > 0, nil
}

func (s *SQLiteDatabase) MovieExistsId(ctx context.Context, movieID uint) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", movieID).Count(&count).Error; err != nil {
		logutils.Log.WithError(err).Error("Failed to check if movie exists by ID")
		return false, err
	}
	logutils.Log.Debugf("Movie existence check completed for ID: %d", movieID)
	return count > 0, nil
}

func (s *SQLiteDatabase) GetFilesByMovieID(ctx context.Context, movieID uint) ([]MovieFile, error) {
	var files []MovieFile
	if err := s.db.WithContext(ctx).Where("movie_id = ?", movieID).Find(&files).Error; err != nil {
		logutils.Log.WithError(err).Error("Failed to fetch files by movie ID")
		return nil, err
	}
	logutils.Log.Debugf("Files fetched successfully for movie ID: %d", movieID)
	return files, nil
}

func (s *SQLiteDatabase) RemoveFilesByMovieID(ctx context.Context, movieID uint) error {
	if err := s.db.WithContext(ctx).Where("movie_id = ?", movieID).Delete(&MovieFile{}).Error; err != nil {
		logutils.Log.WithError(err).Error("Failed to remove files by movie ID")
		return err
	}
	logutils.Log.Debugf("Files removed successfully for movie ID: %d", movieID)
	return nil
}

func (s *SQLiteDatabase) RemoveTempFilesByMovieID(ctx context.Context, movieID uint) error {
	if err := s.db.WithContext(ctx).Where("movie_id = ? AND temp_file = ?", movieID, true).Delete(&MovieFile{}).Error; err != nil {
		logutils.Log.WithError(err).Errorf("Failed to get temp files for movie ID %d", movieID)
		return err
	}
	logutils.Log.Debugf("Temp files removed successfully for movie ID: %d", movieID)
	return nil
}

func (s *SQLiteDatabase) MovieExistsUploadedFile(ctx context.Context, fileName string) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&MovieFile{}).Where("file_path = ?", fileName).Count(&count).Error; err != nil {
		logutils.Log.WithError(err).Error("Failed to check if uploaded file exists")
		return false, err
	}
	logutils.Log.Debugf("Uploaded file existence check completed for file: %s", fileName)
	return count > 0, nil
}

func (s *SQLiteDatabase) GetTempFilesByMovieID(ctx context.Context, movieID uint) ([]MovieFile, error) {
	var tempFiles []MovieFile
	if err := s.db.WithContext(ctx).Where("movie_id = ? AND temp_file = ?", movieID, true).Find(&tempFiles).Error; err != nil {
		logutils.Log.WithError(err).Errorf("Failed to get temp files for movie ID %d", movieID)
		return nil, err
	}
	return tempFiles, nil
}

func (s *SQLiteDatabase) Login(ctx context.Context, password string, chatID int64, userName string) (bool, error) {
	var role UserRole
	switch password {
	case tmsconfig.GlobalConfig.AdminPassword:
		role = AdminRole
	case tmsconfig.GlobalConfig.RegularPassword:
		role = RegularRole
	default:
		return s.handleTemporaryPassword(ctx, password, chatID, userName)
	}

	return s.createOrUpdateUser(ctx, chatID, userName, role, nil)
}

func (s *SQLiteDatabase) handleTemporaryPassword(ctx context.Context, password string, chatID int64, userName string) (bool, error) {
	var tempPass TemporaryPassword
	if err := s.db.WithContext(ctx).Where("password = ?", password).First(&tempPass).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logutils.Log.Warnf("Invalid password attempt by user: %s", userName)
			return false, nil
		}
		logutils.Log.WithError(err).Error("Failed to check temporary password")
		return false, err
	}

	if time.Now().After(tempPass.ExpiresAt) {
		logutils.Log.Warnf("Temporary password expired for user: %s", userName)
		return false, nil
	}

	return s.createOrUpdateUser(ctx, chatID, userName, TemporaryRole, &tempPass.ExpiresAt)
}

func (s *SQLiteDatabase) createOrUpdateUser(
	ctx context.Context,
	chatID int64,
	userName string,
	role UserRole,
	expiresAt *time.Time,
) (bool, error) {
	user := User{
		Name:      userName,
		ChatID:    chatID,
		Role:      role,
		ExpiresAt: expiresAt,
	}

	if err := s.db.WithContext(ctx).Where("chat_id = ?", chatID).FirstOrCreate(&user).Error; err != nil {
		logutils.Log.WithError(err).Error("Failed to create or update user")
		return false, err
	}

	if err := s.db.WithContext(ctx).Model(&user).Updates(user).Error; err != nil {
		logutils.Log.WithError(err).Error("Failed to update user")
		return false, err
	}

	logutils.Log.Infof("%s user logged in successfully: %s", role, userName)
	return true, nil
}

func (s *SQLiteDatabase) GetUserRole(ctx context.Context, chatID int64) (UserRole, error) {
	var user User
	if err := s.db.WithContext(ctx).Where("chat_id = ?", chatID).First(&user).Error; err != nil {
		logutils.Log.WithError(err).Errorf("Failed to get user role for chat ID %d", chatID)
		return "", err
	}
	logutils.Log.Debugf("User role fetched successfully for chat ID %d: %s", chatID, user.Role)
	return user.Role, nil
}

func (s *SQLiteDatabase) IsUserAccessAllowed(ctx context.Context, chatID int64) (bool, UserRole, error) {
	var user User
	if err := s.db.WithContext(ctx).Preload("Passwords").Where("chat_id = ?", chatID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logutils.Log.Warnf("Access denied for unknown user with chat ID %d: user not found", chatID)
			return false, "", nil
		}
		logutils.Log.WithError(err).Errorf("Failed to check access for chat ID %d", chatID)
		return false, "", err
	}

	if user.Role == TemporaryRole && !isTemporaryPasswordValid(user.Passwords) {
		logutils.Log.Warnf("Access denied for temporary user with chat ID %d: all passwords expired", chatID)
		return false, TemporaryRole, nil
	}

	logutils.Log.Debugf("Access allowed for user with chat ID %d, role: %s", chatID, user.Role)
	return true, user.Role, nil
}

func isTemporaryPasswordValid(passwords []TemporaryPassword) bool {
	for _, tempPass := range passwords {
		if time.Now().Before(tempPass.ExpiresAt) {
			return true
		}
	}
	return false
}

func (s *SQLiteDatabase) AssignTemporaryPassword(ctx context.Context, password string, chatID int64) error {
	var tempPass TemporaryPassword
	if err := s.db.WithContext(ctx).Where("password = ?", password).First(&tempPass).Error; err != nil {
		logutils.Log.WithError(err).Errorf("Failed to find temporary password: %s", password)
		return err
	}

	if time.Now().After(tempPass.ExpiresAt) {
		logutils.Log.Warnf("Cannot assign expired temporary password: %s", password)
		return fmt.Errorf("temporary password expired")
	}

	if err := s.db.WithContext(ctx).Model(&User{}).Where("chat_id = ?", chatID).Update("temp_pass_id", nil).Error; err != nil {
		logutils.Log.WithError(err).Errorf("Failed to clear previous temporary password for user with chat ID %d", chatID)
		return err
	}

	if err := s.db.WithContext(ctx).Model(&User{}).Where("chat_id = ?", chatID).Updates(map[string]any{
		"temp_pass_id": tempPass.ID,
		"expires_at":   tempPass.ExpiresAt,
		"role":         TemporaryRole,
	}).Error; err != nil {
		logutils.Log.WithError(err).Errorf("Failed to assign new temporary password to user with chat ID %d", chatID)
		return err
	}

	logutils.Log.Debugf("New temporary password assigned successfully to user with chat ID %d", chatID)
	return nil
}

func (s *SQLiteDatabase) ExtendTemporaryUser(ctx context.Context, chatID int64, newExpiration time.Time) error {
	err := s.db.WithContext(ctx).
		Model(&User{}).
		Where("chat_id = ? AND role = ?", chatID, TemporaryRole).
		Update("expires_at", newExpiration).Error

	if err != nil {
		logutils.Log.WithError(err).Errorf(
			"Failed to extend expiration for temporary user with chat ID %d", chatID,
		)
		return err
	}

	logutils.Log.Debugf(
		"Temporary user with chat ID %d extended until %s", chatID, newExpiration,
	)
	return nil
}

func (s *SQLiteDatabase) GenerateTemporaryPassword(ctx context.Context, duration time.Duration) (string, error) {
	const passwordLength = 16
	passwordBytes := make([]byte, passwordLength)
	if _, err := rand.Read(passwordBytes); err != nil {
		logutils.Log.WithError(err).Error("Failed to generate random password")
		return "", err
	}
	password := hex.EncodeToString(passwordBytes)

	expiresAt := time.Now().Add(duration)

	tempPass := TemporaryPassword{
		Password:  password,
		ExpiresAt: expiresAt,
	}

	if err := s.db.WithContext(ctx).Create(&tempPass).Error; err != nil {
		logutils.Log.WithError(err).Error("Failed to save temporary password to database")
		return "", err
	}

	logutils.Log.Debugf("Temporary password generated and saved: %s, expires at: %s", password, expiresAt)
	return password, nil
}

func (s *SQLiteDatabase) AddDownloadHistory(ctx context.Context, userID, movieID uint) error {
	downloadHistory := DownloadHistory{
		UserID:  userID,
		MovieID: movieID,
	}

	if err := s.db.WithContext(ctx).Create(&downloadHistory).Error; err != nil {
		logutils.Log.WithError(err).Errorf("Failed to add download history for user ID %d and movie ID %d", userID, movieID)
		return err
	}

	logutils.Log.Debugf("Download history added successfully for user ID %d and movie ID %d", userID, movieID)
	return nil
}

func (s *SQLiteDatabase) GetUserByChatID(ctx context.Context, chatID int64) (User, error) {
	var user User
	if err := s.db.WithContext(ctx).Where("chat_id = ?", chatID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logutils.Log.Warnf("User with chat ID %d not found", chatID)
			return User{}, nil
		}
		logutils.Log.WithError(err).Errorf("Failed to fetch user with chat ID %d", chatID)
		return User{}, err
	}

	logutils.Log.Debugf("User fetched successfully with chat ID %d", chatID)
	return user, nil
}

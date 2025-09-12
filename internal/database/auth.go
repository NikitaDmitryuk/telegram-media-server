package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"gorm.io/gorm"
)

const (
	guestRole = "guest"
)

func (s *SQLiteDatabase) Login(
	ctx context.Context,
	password string,
	chatID int64,
	userName string,
	config *tmsconfig.Config,
) (bool, error) {
	if password == config.AdminPassword {
		_, err := s.createOrUpdateUser(ctx, chatID, userName, "admin", nil)
		return true, err
	}
	if password == config.RegularPassword {
		_, err := s.createOrUpdateUser(ctx, chatID, userName, "regular", nil)
		return true, err
	}
	return s.handleTemporaryPassword(ctx, password, chatID, userName)
}

func (s *SQLiteDatabase) handleTemporaryPassword(ctx context.Context, password string, chatID int64, userName string) (bool, error) {
	var passwords []TemporaryPassword
	result := s.db.WithContext(ctx).Where("password = ?", password).Find(&passwords)
	if result.Error != nil {
		return false, result.Error
	}

	if !isTemporaryPasswordValid(passwords) {
		return false, nil
	}

	tempPassword := passwords[0]
	expiresAt := tempPassword.ExpiresAt
	return s.createOrUpdateUser(ctx, chatID, userName, "temporary", &expiresAt)
}

func (s *SQLiteDatabase) createOrUpdateUser(
	ctx context.Context,
	chatID int64,
	userName string,
	role UserRole,
	expiresAt *time.Time,
) (bool, error) {
	var user User
	result := s.db.WithContext(ctx).Where("chat_id = ?", chatID).First(&user)

	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return false, result.Error
	}

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		user = User{
			Name:      userName,
			ChatID:    chatID,
			Role:      role,
			ExpiresAt: expiresAt,
		}
		result = s.db.WithContext(ctx).Create(&user)
	} else {
		user.Name = userName
		user.Role = role
		user.ExpiresAt = expiresAt
		result = s.db.WithContext(ctx).Save(&user)
	}

	return result.Error == nil, result.Error
}

func (s *SQLiteDatabase) GetUserRole(ctx context.Context, chatID int64) (UserRole, error) {
	var user User
	result := s.db.WithContext(ctx).Where("chat_id = ?", chatID).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return guestRole, nil
		}
		return guestRole, result.Error
	}

	if user.IsExpired() {
		return guestRole, nil
	}

	return user.Role, nil
}

func (s *SQLiteDatabase) IsUserAccessAllowed(ctx context.Context, chatID int64) (isAllowed bool, userRole UserRole, err error) {
	var user User
	result := s.db.WithContext(ctx).Where("chat_id = ?", chatID).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return false, guestRole, nil
		}
		return false, guestRole, result.Error
	}

	if user.IsExpired() {
		return false, guestRole, nil
	}

	return true, user.Role, nil
}

func isTemporaryPasswordValid(passwords []TemporaryPassword) bool {
	if len(passwords) == 0 {
		return false
	}
	tempPassword := passwords[0]
	return !tempPassword.IsExpired()
}

func (s *SQLiteDatabase) AssignTemporaryPassword(ctx context.Context, password string, chatID int64) error {
	var tempPassword TemporaryPassword
	result := s.db.WithContext(ctx).Where("password = ?", password).First(&tempPassword)
	if result.Error != nil {
		return result.Error
	}

	var user User
	result = s.db.WithContext(ctx).Where("chat_id = ?", chatID).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return fmt.Errorf("user not found")
		}
		return result.Error
	}

	return s.db.WithContext(ctx).Model(&tempPassword).Association("Users").Append(&user)
}

func (s *SQLiteDatabase) ExtendTemporaryUser(ctx context.Context, chatID int64, newExpiration time.Time) error {
	var user User
	result := s.db.WithContext(ctx).Where("chat_id = ?", chatID).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return fmt.Errorf("user not found")
		}
		return result.Error
	}

	if user.Role != "temporary" {
		return fmt.Errorf("user is not temporary")
	}

	user.ExpiresAt = &newExpiration
	result = s.db.WithContext(ctx).Save(&user)
	return result.Error
}

func (s *SQLiteDatabase) GenerateTemporaryPassword(ctx context.Context, duration time.Duration) (string, error) {
	const passwordLength = 8
	bytes := make([]byte, passwordLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	password := hex.EncodeToString(bytes)

	expiresAt := time.Now().Add(duration)
	tempPassword := TemporaryPassword{
		Password:  password,
		ExpiresAt: expiresAt,
	}

	result := s.db.WithContext(ctx).Create(&tempPassword)
	if result.Error != nil {
		return "", result.Error
	}

	return password, nil
}

func (s *SQLiteDatabase) GetUserByChatID(ctx context.Context, chatID int64) (User, error) {
	var user User
	result := s.db.WithContext(ctx).Where("chat_id = ?", chatID).First(&user)
	if result.Error != nil {
		return User{}, result.Error
	}
	return user, nil
}

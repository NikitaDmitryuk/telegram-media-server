package services

import (
	"context"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/utils"
)

// AuthService реализует бизнес-логику авторизации
type AuthService struct {
	db  database.Database
	cfg *domain.Config
}

// NewAuthService создает новый сервис авторизации
func NewAuthService(db database.Database, cfg *domain.Config) domain.AuthServiceInterface {
	return &AuthService{
		db:  db,
		cfg: cfg,
	}
}

// Login выполняет авторизацию пользователя
func (s *AuthService) Login(ctx context.Context, password string, chatID int64, userName string) (*domain.LoginResult, error) {
	if password == "" {
		return &domain.LoginResult{
			Success: false,
			Message: "password_required",
		}, nil
	}

	success, err := s.db.Login(ctx, password, chatID, userName, s.cfg)
	if err != nil {
		logger.Log.WithError(err).Error("Login failed due to database error")
		return nil, utils.WrapError(err, "login database operation failed", map[string]any{
			"chat_id":  chatID,
			"username": userName,
		})
	}

	if !success {
		logger.Log.WithField("username", userName).Warn("Login failed due to incorrect or expired password")
		return &domain.LoginResult{
			Success: false,
			Message: "wrong_password",
		}, nil
	}

	// Определяем роль пользователя
	role, err := s.db.GetUserRole(ctx, chatID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get user role after successful login")
		return nil, utils.WrapError(err, "failed to get user role", map[string]any{
			"chat_id": chatID,
		})
	}

	logger.Log.WithFields(map[string]any{
		"username": userName,
		"role":     role,
	}).Info("User logged in successfully")

	return &domain.LoginResult{
		Success: true,
		Role:    role,
		Message: "login_success",
	}, nil
}

// CheckAccess проверяет доступ пользователя
func (s *AuthService) CheckAccess(ctx context.Context, chatID int64) (hasAccess bool, role database.UserRole, err error) {
	allowed, role, err := s.db.IsUserAccessAllowed(ctx, chatID)
	if err != nil {
		return false, "", utils.WrapError(err, "failed to check user access", map[string]any{
			"chat_id": chatID,
		})
	}

	return allowed, role, nil
}

// GenerateTempPassword генерирует временный пароль
func (s *AuthService) GenerateTempPassword(ctx context.Context, duration time.Duration) (string, error) {
	password, err := s.db.GenerateTemporaryPassword(ctx, duration)
	if err != nil {
		return "", utils.WrapError(err, "failed to generate temporary password", map[string]any{
			"duration": duration,
		})
	}

	logger.Log.WithField("duration", duration).Info("Temporary password generated")
	return password, nil
}

// IsAdmin проверяет, является ли пользователь администратором
func (s *AuthService) IsAdmin(ctx context.Context, chatID int64) (bool, error) {
	role, err := s.db.GetUserRole(ctx, chatID)
	if err != nil {
		return false, utils.WrapError(err, "failed to get user role", map[string]any{
			"chat_id": chatID,
		})
	}

	return role == "admin", nil
}

// ValidatePassword проверяет валидность пароля
func (s *AuthService) ValidatePassword(password string) error {
	if len(password) < s.cfg.SecuritySettings.PasswordMinLength {
		return utils.NewAppError("password_too_short", "password is too short", map[string]any{
			"min_length": s.cfg.SecuritySettings.PasswordMinLength,
			"actual":     len(password),
		})
	}

	return nil
}

// ExtendTemporaryUser продлевает доступ временного пользователя
func (s *AuthService) ExtendTemporaryUser(ctx context.Context, chatID int64, newExpiration time.Time) error {
	err := s.db.ExtendTemporaryUser(ctx, chatID, newExpiration)
	if err != nil {
		return utils.WrapError(err, "failed to extend temporary user", map[string]any{
			"chat_id":        chatID,
			"new_expiration": newExpiration,
		})
	}

	logger.Log.WithFields(map[string]any{
		"chat_id":        chatID,
		"new_expiration": newExpiration,
	}).Info("Temporary user access extended")

	return nil
}

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/models"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MockDatabase implements database.Database for middleware testing.
// It embeds testutils.DatabaseStub for all no-op methods and overrides IsUserAccessAllowed.
type MockDatabase struct {
	testutils.DatabaseStub
	users             map[int64]*models.User
	shouldReturnError bool
	accessCheckError  error
	accessAllowed     bool
	userRole          models.UserRole
}

func NewMockDatabase() *MockDatabase {
	return &MockDatabase{
		users: make(map[int64]*models.User),
	}
}

func (m *MockDatabase) IsUserAccessAllowed(_ context.Context, chatID int64) (bool, models.UserRole, error) {
	if m.shouldReturnError {
		return false, "", m.accessCheckError
	}

	if user, exists := m.users[chatID]; exists {
		if user.IsExpired() {
			return false, models.UserRole("guest"), nil
		}
		return true, user.Role, nil
	}

	return m.accessAllowed, m.userRole, nil
}

func TestAuthMiddleware(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping middleware test in short mode - requires logger setup")
	}

	// Initialize logger for tests
	logutils.InitLogger("debug")
	tests := []struct {
		name         string
		update       *tgbotapi.Update
		mockSetup    func(*MockDatabase)
		expectedAuth bool
		expectedRole models.UserRole
		expectError  bool
	}{
		{
			name: "successful authentication with message",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 123},
					From: &tgbotapi.User{ID: 456},
				},
			},
			mockSetup: func(db *MockDatabase) {
				db.accessAllowed = true
				db.userRole = models.AdminRole
			},
			expectedAuth: true,
			expectedRole: models.AdminRole,
		},
		{
			name: "successful authentication with callback query",
			update: &tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Message: &tgbotapi.Message{
						Chat: &tgbotapi.Chat{ID: 123},
					},
					From: &tgbotapi.User{ID: 456},
				},
			},
			mockSetup: func(db *MockDatabase) {
				db.accessAllowed = true
				db.userRole = models.RegularRole
			},
			expectedAuth: true,
			expectedRole: models.RegularRole,
		},
		{
			name: "access denied",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 123},
					From: &tgbotapi.User{ID: 456},
				},
			},
			mockSetup: func(db *MockDatabase) {
				db.accessAllowed = false
				db.userRole = models.UserRole("guest")
			},
			expectedAuth: false,
			expectedRole: models.UserRole("guest"),
		},
		{
			name: "database error",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 123},
					From: &tgbotapi.User{ID: 456},
				},
			},
			mockSetup: func(db *MockDatabase) {
				db.shouldReturnError = true
				db.accessCheckError = errors.New("database connection failed")
			},
			expectedAuth: false,
			expectedRole: "",
		},
		{
			name:   "invalid update - no message or callback",
			update: &tgbotapi.Update{
				// No Message or CallbackQuery
			},
			mockSetup: func(_ *MockDatabase) {
				// No setup needed
			},
			expectedAuth: false,
			expectedRole: "",
		},
		{
			name:   "nil update",
			update: nil,
			mockSetup: func(_ *MockDatabase) {
				// No setup needed
			},
			expectedAuth: false,
			expectedRole: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := NewMockDatabase()
			if tt.mockSetup != nil {
				tt.mockSetup(db)
			}

			// Handle nil update case
			if tt.update == nil {
				// This should not panic and should return false
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("AuthMiddleware panicked with nil update: %v", r)
					}
				}()
				auth, role := AuthMiddleware(nil, db)
				if auth != false || role != "" {
					t.Errorf("AuthMiddleware with nil update = (%v, %v), want (false, \"\")", auth, role)
				}
				return
			}

			auth, role := AuthMiddleware(tt.update, db)

			if auth != tt.expectedAuth {
				t.Errorf("AuthMiddleware() auth = %v, want %v", auth, tt.expectedAuth)
			}

			if role != tt.expectedRole {
				t.Errorf("AuthMiddleware() role = %v, want %v", role, tt.expectedRole)
			}
		})
	}
}

func TestCheckAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping check access test in short mode - requires logger setup")
	}

	// Initialize logger for tests
	logutils.InitLogger("debug")
	tests := []struct {
		name         string
		update       *tgbotapi.Update
		mockSetup    func(*MockDatabase)
		expectedAuth bool
	}{
		{
			name: "access allowed",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 123},
					From: &tgbotapi.User{ID: 456},
				},
			},
			mockSetup: func(db *MockDatabase) {
				db.accessAllowed = true
				db.userRole = models.AdminRole
			},
			expectedAuth: true,
		},
		{
			name: "access denied",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 123},
					From: &tgbotapi.User{ID: 456},
				},
			},
			mockSetup: func(db *MockDatabase) {
				db.accessAllowed = false
			},
			expectedAuth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := NewMockDatabase()
			tt.mockSetup(db)

			result := CheckAccess(tt.update, db)

			if result != tt.expectedAuth {
				t.Errorf("CheckAccess() = %v, want %v", result, tt.expectedAuth)
			}
		})
	}
}

func TestCheckAccessWithRole(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping check access with role test in short mode - requires logger setup")
	}

	// Initialize logger for tests
	logutils.InitLogger("debug")
	tests := []struct {
		name         string
		update       *tgbotapi.Update
		allowedRoles []models.UserRole
		mockSetup    func(*MockDatabase)
		expectedAuth bool
	}{
		{
			name: "admin access to admin-only resource",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 123},
					From: &tgbotapi.User{ID: 456},
				},
			},
			allowedRoles: []models.UserRole{models.AdminRole},
			mockSetup: func(db *MockDatabase) {
				db.accessAllowed = true
				db.userRole = models.AdminRole
			},
			expectedAuth: true,
		},
		{
			name: "regular user access to admin-only resource",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 123},
					From: &tgbotapi.User{ID: 456},
				},
			},
			allowedRoles: []models.UserRole{models.AdminRole},
			mockSetup: func(db *MockDatabase) {
				db.accessAllowed = true
				db.userRole = models.RegularRole
			},
			expectedAuth: false,
		},
		{
			name: "multiple allowed roles - user has one of them",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 123},
					From: &tgbotapi.User{ID: 456},
				},
			},
			allowedRoles: []models.UserRole{models.AdminRole, models.RegularRole},
			mockSetup: func(db *MockDatabase) {
				db.accessAllowed = true
				db.userRole = models.RegularRole
			},
			expectedAuth: true,
		},
		{
			name: "user not authenticated",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 123},
					From: &tgbotapi.User{ID: 456},
				},
			},
			allowedRoles: []models.UserRole{models.AdminRole},
			mockSetup: func(db *MockDatabase) {
				db.accessAllowed = false
			},
			expectedAuth: false,
		},
		{
			name: "empty allowed roles",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 123},
					From: &tgbotapi.User{ID: 456},
				},
			},
			allowedRoles: []models.UserRole{},
			mockSetup: func(db *MockDatabase) {
				db.accessAllowed = true
				db.userRole = models.AdminRole
			},
			expectedAuth: false,
		},
		{
			name: "temporary user access to regular resource",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 123},
					From: &tgbotapi.User{ID: 456},
				},
			},
			allowedRoles: []models.UserRole{models.AdminRole, models.RegularRole, models.TemporaryRole},
			mockSetup: func(db *MockDatabase) {
				db.accessAllowed = true
				db.userRole = models.TemporaryRole
			},
			expectedAuth: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := NewMockDatabase()
			tt.mockSetup(db)

			result := CheckAccessWithRole(tt.update, tt.allowedRoles, db)

			if result != tt.expectedAuth {
				t.Errorf("CheckAccessWithRole() = %v, want %v", result, tt.expectedAuth)
			}
		})
	}
}

func TestLoggingMiddleware(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping logging middleware test in short mode - requires logger setup")
	}

	// Initialize logger for tests
	logutils.InitLogger("debug")
	tests := []struct {
		name   string
		update *tgbotapi.Update
	}{
		{
			name: "message update",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{
					From: &tgbotapi.User{UserName: "testuser"},
					Text: "test message",
				},
			},
		},
		{
			name: "callback query update",
			update: &tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					From: &tgbotapi.User{UserName: "testuser"},
					Data: "callback_data",
				},
			},
		},
		{
			name:   "nil update",
			update: nil,
		},
		{
			name:   "empty update",
			update: &tgbotapi.Update{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test mainly ensures LoggingMiddleware doesn't panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("LoggingMiddleware panicked: %v", r)
				}
			}()

			LoggingMiddleware(tt.update)
		})
	}
}

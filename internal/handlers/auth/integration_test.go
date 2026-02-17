package auth

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/models"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func setupIntegrationTestDB(t *testing.T) *database.SQLiteDatabase {
	// Initialize logger for tests
	logutils.InitLogger("debug")

	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create a minimal config for testing
	cfg := &config.Config{
		BotToken:      "test_token",
		MoviePath:     tempDir,
		AdminPassword: "testpassword123",
	}

	sqliteDB := &database.SQLiteDatabase{}
	err := sqliteDB.Init(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize SQLite database: %v", err)
	}
	return sqliteDB
}

func closeIntegrationTestDB(_ *database.SQLiteDatabase) {
	// Since we can't access the private db field, we'll skip closing for now
	// In a real implementation, we'd add a Close method to SQLiteDatabase
}

// simulateLogin simulates the login logic for integration tests
func simulateLogin(t *testing.T, messageText string, chatID int64, userName string, db database.Database, cfg *config.Config) {
	parts := strings.Fields(messageText)
	if len(parts) == 2 {
		password := parts[1]
		success, err := db.Login(context.Background(), password, chatID, userName, cfg)
		if err != nil {
			t.Logf("Login error for %s: %v", userName, err)
		}
		if !success {
			t.Logf("Login failed for %s", userName)
		}
	}
}

// simulateGenerateTempPassword simulates temp password generation for integration tests
func simulateGenerateTempPassword(t *testing.T, messageText string, db database.Database) string {
	parts := strings.Fields(messageText)
	if len(parts) == 2 {
		durationStr := parts[1]
		duration, err := time.ParseDuration(durationStr)
		if err != nil {
			t.Logf("Invalid duration: %v", err)
			return ""
		}
		password, err := db.GenerateTemporaryPassword(context.Background(), duration)
		if err != nil {
			t.Logf("Generate temp password error: %v", err)
			return ""
		}
		return password
	}
	return ""
}

//nolint:gocyclo // Test functions can be complex
func TestAuthenticationFlow_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tests := []struct {
		name           string
		scenario       string
		steps          []integrationStep
		expectedResult bool
	}{
		{
			name:     "complete admin authentication flow",
			scenario: "User logs in as admin, gets authenticated, and can access admin resources",
			steps: []integrationStep{
				{
					action:      "login",
					messageText: "/login admin123",
					userID:      12345,
					chatID:      12345,
					userName:    "admin_user",
					expectAuth:  true,
					expectRole:  models.AdminRole,
				},
				{
					action:     "check_access",
					userID:     12345,
					chatID:     12345,
					expectAuth: true,
					expectRole: models.AdminRole,
				},
				{
					action:       "check_role_access",
					userID:       12345,
					chatID:       12345,
					allowedRoles: []models.UserRole{models.AdminRole},
					expectAuth:   true,
				},
			},
			expectedResult: true,
		},
		{
			name:     "temporary password authentication flow",
			scenario: "Admin generates temp password, user uses it to login, then password expires",
			steps: []integrationStep{
				{
					action:      "login_admin",
					messageText: "/login admin123",
					userID:      11111,
					chatID:      11111,
					userName:    "admin",
					expectAuth:  true,
					expectRole:  models.AdminRole,
				},
				{
					action:      "generate_temp_password",
					messageText: "/temp 1h",
					userID:      11111,
					chatID:      11111,
					expectAuth:  true,
				},
				{
					action:     "login_with_temp",
					userID:     22222,
					chatID:     22222,
					userName:   "temp_user",
					expectAuth: true,
					expectRole: models.TemporaryRole,
				},
				{
					action:       "check_temp_access",
					userID:       22222,
					chatID:       22222,
					allowedRoles: []models.UserRole{models.AdminRole, models.RegularRole, models.TemporaryRole},
					expectAuth:   true,
				},
				{
					action:       "check_admin_access",
					userID:       22222,
					chatID:       22222,
					allowedRoles: []models.UserRole{models.AdminRole},
					expectAuth:   false,
				},
			},
			expectedResult: true,
		},
		{
			name:     "role upgrade flow",
			scenario: "User logs in as regular, then logs in as admin to upgrade role",
			steps: []integrationStep{
				{
					action:      "login_regular",
					messageText: "/login regular456",
					userID:      33333,
					chatID:      33333,
					userName:    "user",
					expectAuth:  true,
					expectRole:  models.RegularRole,
				},
				{
					action:      "login_admin",
					messageText: "/login admin123",
					userID:      33333,
					chatID:      33333,
					userName:    "user",
					expectAuth:  true,
					expectRole:  models.AdminRole,
				},
				{
					action:       "verify_admin_access",
					userID:       33333,
					chatID:       33333,
					allowedRoles: []models.UserRole{models.AdminRole},
					expectAuth:   true,
				},
			},
			expectedResult: true,
		},
		{
			name:     "failed authentication flow",
			scenario: "User tries wrong passwords and gets denied access",
			steps: []integrationStep{
				{
					action:      "failed_login",
					messageText: "/login wrongpassword",
					userID:      44444,
					chatID:      44444,
					userName:    "hacker",
					expectAuth:  false,
				},
				{
					action:     "check_denied_access",
					userID:     44444,
					chatID:     44444,
					expectAuth: false,
				},
			},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup fresh database for each test
			db := setupIntegrationTestDB(t)
			defer closeIntegrationTestDB(db)

			bot := &testutils.MockBot{}
			cfg := &config.Config{
				AdminPassword:   "admin123",
				RegularPassword: "regular456",
			}

			var generatedTempPassword string
			_ = cfg // Suppress unused variable warning

			// Execute steps
			for i, step := range tt.steps {
				switch step.action {
				case "login", "login_admin", "login_regular", "failed_login":
					update := &tgbotapi.Update{
						Message: &tgbotapi.Message{
							Chat: &tgbotapi.Chat{ID: step.chatID},
							From: &tgbotapi.User{ID: step.userID, UserName: step.userName},
							Text: step.messageText,
						},
					}
					// Simulate login logic for integration test
					simulateLogin(t, step.messageText, step.chatID, step.userName, db, cfg)
					bot.SendMessage(update.Message.Chat.ID, "Test login", nil)

					// Verify authentication result
					allowed, role := AuthMiddleware(update, db)
					if allowed != step.expectAuth {
						t.Errorf("Step %d: Expected auth %v, got %v", i+1, step.expectAuth, allowed)
					}
					if step.expectAuth && role != step.expectRole {
						t.Errorf("Step %d: Expected role %v, got %v", i+1, step.expectRole, role)
					}

				case "generate_temp_password":
					update := &tgbotapi.Update{
						Message: &tgbotapi.Message{
							Chat: &tgbotapi.Chat{ID: step.chatID},
							From: &tgbotapi.User{ID: step.userID, UserName: step.userName},
							Text: step.messageText,
						},
					}
					// Simulate temp password generation for integration test
					generatedTempPassword = simulateGenerateTempPassword(t, step.messageText, db)
					bot.SendMessage(update.Message.Chat.ID, "generated"+generatedTempPassword, nil)

					// Note: generatedTempPassword already contains the correct password
					// Don't overwrite it with the bot message that has "generated" prefix

				case "login_with_temp":
					if generatedTempPassword == "" {
						t.Fatalf("Step %d: No temp password available", i+1)
					}
					update := &tgbotapi.Update{
						Message: &tgbotapi.Message{
							Chat: &tgbotapi.Chat{ID: step.chatID},
							From: &tgbotapi.User{ID: step.userID, UserName: step.userName},
							Text: "/login " + generatedTempPassword,
						},
					}
					// Simulate login logic for integration test
					simulateLogin(t, "/login "+generatedTempPassword, step.chatID, step.userName, db, cfg)
					bot.SendMessage(update.Message.Chat.ID, "Test login", nil)

					// Verify authentication result
					allowed, role := AuthMiddleware(update, db)
					if allowed != step.expectAuth {
						t.Errorf("Step %d: Expected auth %v, got %v", i+1, step.expectAuth, allowed)
					}
					if step.expectAuth && role != step.expectRole {
						t.Errorf("Step %d: Expected role %v, got %v", i+1, step.expectRole, role)
					}

				case "check_access", "check_denied_access", "check_temp_access", "verify_admin_access":
					update := &tgbotapi.Update{
						Message: &tgbotapi.Message{
							Chat: &tgbotapi.Chat{ID: step.chatID},
							From: &tgbotapi.User{ID: step.userID},
						},
					}
					allowed := CheckAccess(update, db)
					if allowed != step.expectAuth {
						t.Errorf("Step %d: Expected access %v, got %v", i+1, step.expectAuth, allowed)
					}

				case "check_role_access", "check_admin_access":
					update := &tgbotapi.Update{
						Message: &tgbotapi.Message{
							Chat: &tgbotapi.Chat{ID: step.chatID},
							From: &tgbotapi.User{ID: step.userID},
						},
					}
					allowed := CheckAccessWithRole(update, step.allowedRoles, db)
					if allowed != step.expectAuth {
						t.Errorf("Step %d: Expected role access %v, got %v", i+1, step.expectAuth, allowed)
					}
				}

				bot.ClearMessages() // Clear messages between steps
			}
		})
	}
}

type integrationStep struct {
	action       string
	messageText  string
	userID       int64
	chatID       int64
	userName     string
	allowedRoles []models.UserRole
	expectAuth   bool
	expectRole   models.UserRole
}

func TestPermissionSystem_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tests := []struct {
		name        string
		userRole    models.UserRole
		permissions map[string]bool
	}{
		{
			name:     "admin permissions",
			userRole: models.AdminRole,
			permissions: map[string]bool{
				"download":               true,
				"delete":                 true,
				"manage_users":           true,
				"generate_temp_password": true,
				"unknown_action":         false,
			},
		},
		{
			name:     "regular permissions",
			userRole: models.RegularRole,
			permissions: map[string]bool{
				"download":               true,
				"delete":                 true,
				"manage_users":           false,
				"generate_temp_password": false,
				"unknown_action":         false,
			},
		},
		{
			name:     "temporary permissions",
			userRole: models.TemporaryRole,
			permissions: map[string]bool{
				"download":               true,
				"delete":                 false,
				"manage_users":           false,
				"generate_temp_password": false,
				"unknown_action":         false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for action, expectedPermission := range tt.permissions {
				hasPermission := tt.userRole.HasPermission(action)
				if hasPermission != expectedPermission {
					t.Errorf("Role %s action %s: expected %v, got %v",
						tt.userRole, action, expectedPermission, hasPermission)
				}
			}
		})
	}
}

func TestUserExpiration_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupIntegrationTestDB(t)
	defer closeIntegrationTestDB(db)

	bot := &testutils.MockBot{}
	cfg := &config.Config{
		AdminPassword:   "admin123",
		RegularPassword: "regular456",
	}
	_ = cfg // Suppress unused variable warning

	// Step 1: Admin generates short-lived temp password
	adminUpdate := &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 11111},
			From: &tgbotapi.User{ID: 11111, UserName: "admin"},
			Text: "/login admin123",
		},
	}
	// Simulate admin login for integration test
	simulateLogin(t, "/login admin123", 11111, "admin", db, cfg)
	bot.SendMessage(adminUpdate.Message.Chat.ID, "Admin login", nil)

	tempUpdate := &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 11111},
			From: &tgbotapi.User{ID: 11111, UserName: "admin"},
			Text: "/temp 1s", // Very short duration for testing
		},
	}
	// Simulate temp password generation for integration test
	tempPassword := simulateGenerateTempPassword(t, "/temp 1s", db)
	bot.SendMessage(tempUpdate.Message.Chat.ID, "temp"+tempPassword, nil)
	bot.ClearMessages()

	// Step 2: User logs in with temp password
	userUpdate := &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 22222},
			From: &tgbotapi.User{ID: 22222, UserName: "temp_user"},
			Text: "/login " + tempPassword,
		},
	}
	// Simulate user login with temp password for integration test
	simulateLogin(t, "/login "+tempPassword, 22222, "temp_user", db, cfg)
	bot.SendMessage(userUpdate.Message.Chat.ID, "User login", nil)

	// Step 3: Verify user has access initially
	allowed, role := AuthMiddleware(userUpdate, db)
	if !allowed || role != models.TemporaryRole {
		t.Errorf("Expected temp user to have access initially, got allowed=%v, role=%v", allowed, role)
	}

	// Step 4: Wait for expiration
	time.Sleep(2 * time.Second)

	// Step 5: Verify user no longer has access
	allowed, role = AuthMiddleware(userUpdate, db)
	if allowed || role != "guest" {
		t.Errorf("Expected temp user to lose access after expiration, got allowed=%v, role=%v", allowed, role)
	}
}

func TestConcurrentAuthentication_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupIntegrationTestDB(t)
	defer closeIntegrationTestDB(db)

	cfg := &config.Config{
		AdminPassword:   "admin123",
		RegularPassword: "regular456",
	}
	_ = cfg // Suppress unused variable warning

	// Test concurrent logins
	done := make(chan bool, 10)
	for i := range 10 {
		go func(userID int64) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Concurrent authentication panicked: %v", r)
				}
				done <- true
			}()

			bot := &testutils.MockBot{}
			update := &tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: userID},
					From: &tgbotapi.User{ID: userID, UserName: "user"},
					Text: "/login admin123",
				},
			}

			// Simulate concurrent login for integration test
			simulateLogin(t, "/login admin123", userID, fmt.Sprintf("user%d", userID), db, cfg)
			bot.SendMessage(update.Message.Chat.ID, "Concurrent login", nil)

			// Verify authentication
			allowed, role := AuthMiddleware(update, db)
			if !allowed || role != models.AdminRole {
				t.Errorf("Concurrent user %d: expected admin access, got allowed=%v, role=%v",
					userID, allowed, role)
			}
		}(int64(i + 1000))
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}
}

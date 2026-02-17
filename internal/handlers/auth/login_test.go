package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MockDatabaseLogin embeds DatabaseStub and overrides Login to record calls.
type MockDatabaseLogin struct {
	testutils.DatabaseStub
	mu             sync.Mutex
	loginResult    bool
	loginError     error
	loginCallCount int
	lastLoginCall  *LoginCall
}

type LoginCall struct {
	Password string
	ChatID   int64
	UserName string
	Config   *config.Config
}

func NewMockDatabaseLogin() *MockDatabaseLogin {
	return &MockDatabaseLogin{}
}

func (m *MockDatabaseLogin) Login(_ context.Context, password string, chatID int64, userName string, cfg *config.Config) (bool, error) {
	m.mu.Lock()
	m.loginCallCount++
	m.lastLoginCall = &LoginCall{
		Password: password,
		ChatID:   chatID,
		UserName: userName,
		Config:   cfg,
	}
	result, err := m.loginResult, m.loginError
	m.mu.Unlock()
	return result, err
}

// runLoginTests is a shared helper that calls the real LoginHandler.
func runLoginTests(t *testing.T, tests []struct {
	name                    string
	messageText             string
	mockSetup               func(*MockDatabaseLogin)
	expectedLoginCalls      int
	expectedSuccess         bool
	expectError             bool
	expectedMessageContains string
}) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot := &testutils.MockBot{}
			db := NewMockDatabaseLogin()
			cfg := &config.Config{
				AdminPassword:   "adminpass123",
				RegularPassword: "regularpass456",
			}

			if tt.mockSetup != nil {
				tt.mockSetup(db)
			}

			update := &tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 12345},
					From: &tgbotapi.User{
						ID:       67890,
						UserName: "testuser",
					},
					Text: tt.messageText,
				},
			}

			a := &app.App{Bot: bot, DB: db, Config: cfg}
			LoginHandler(a, update)

			if db.loginCallCount != tt.expectedLoginCalls {
				t.Errorf("Expected %d login calls, got %d", tt.expectedLoginCalls, db.loginCallCount)
			}

			if tt.expectedLoginCalls > 0 && db.lastLoginCall != nil {
				expectedPassword := ""
				parts := strings.Fields(tt.messageText)
				if len(parts) == 2 {
					expectedPassword = parts[1]
				}
				if db.lastLoginCall.Password != expectedPassword {
					t.Errorf("Expected password %q, got %q", expectedPassword, db.lastLoginCall.Password)
				}
				if db.lastLoginCall.ChatID != update.Message.Chat.ID {
					t.Errorf("Expected chat ID %d, got %d", update.Message.Chat.ID, db.lastLoginCall.ChatID)
				}
				if db.lastLoginCall.UserName != update.Message.From.UserName {
					t.Errorf("Expected username %q, got %q", update.Message.From.UserName, db.lastLoginCall.UserName)
				}
			}

			if len(bot.SentMessages) == 0 {
				t.Error("Expected bot to send a message, but no message was sent")
				return
			}

			first := bot.SentMessages[0]
			if first.ChatID != 12345 {
				t.Errorf("Expected message to chat 12345, got %d", first.ChatID)
			}
		})
	}
}

func TestLoginHandler_Success(t *testing.T) {
	tests := []struct {
		name                    string
		messageText             string
		mockSetup               func(*MockDatabaseLogin)
		expectedLoginCalls      int
		expectedSuccess         bool
		expectError             bool
		expectedMessageContains string
	}{
		{
			name:        "successful admin login",
			messageText: "/login adminpass123",
			mockSetup: func(db *MockDatabaseLogin) {
				db.loginResult = true
				db.loginError = nil
			},
			expectedLoginCalls: 1,
			expectedSuccess:    true,
		},
		{
			name:        "successful regular login",
			messageText: "/login regularpass456",
			mockSetup: func(db *MockDatabaseLogin) {
				db.loginResult = true
				db.loginError = nil
			},
			expectedLoginCalls: 1,
			expectedSuccess:    true,
		},
	}

	runLoginTests(t, tests)
}

func TestLoginHandler_Failures(t *testing.T) {
	tests := []struct {
		name                    string
		messageText             string
		mockSetup               func(*MockDatabaseLogin)
		expectedLoginCalls      int
		expectedSuccess         bool
		expectError             bool
		expectedMessageContains string
	}{
		{
			name:        "failed login - wrong password",
			messageText: "/login wrongpassword",
			mockSetup: func(db *MockDatabaseLogin) {
				db.loginResult = false
				db.loginError = nil
			},
			expectedLoginCalls: 1,
		},
		{
			name:        "database error during login",
			messageText: "/login somepassword",
			mockSetup: func(db *MockDatabaseLogin) {
				db.loginResult = false
				db.loginError = errors.New("database connection failed")
			},
			expectedLoginCalls: 1,
			expectError:        true,
		},
		{
			name:        "invalid command format - no password",
			messageText: "/login",
			mockSetup:   func(_ *MockDatabaseLogin) {},
		},
		{
			name:        "invalid command format - too many arguments",
			messageText: "/login password extra argument",
			mockSetup:   func(_ *MockDatabaseLogin) {},
		},
		{
			name:        "empty password",
			messageText: "/login ",
			mockSetup:   func(_ *MockDatabaseLogin) {},
		},
		{
			name:        "password with special characters",
			messageText: "/login p@ssw0rd!#$",
			mockSetup: func(db *MockDatabaseLogin) {
				db.loginResult = true
				db.loginError = nil
			},
			expectedLoginCalls: 1,
			expectedSuccess:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot := &testutils.MockBot{}
			db := NewMockDatabaseLogin()
			cfg := &config.Config{
				AdminPassword:   "adminpass123",
				RegularPassword: "regularpass456",
			}

			if tt.mockSetup != nil {
				tt.mockSetup(db)
			}

			update := &tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 12345},
					From: &tgbotapi.User{
						ID:       67890,
						UserName: "testuser",
					},
					Text: tt.messageText,
				},
			}

			a := &app.App{Bot: bot, DB: db, Config: cfg}
			LoginHandler(a, update)

			if db.loginCallCount != tt.expectedLoginCalls {
				t.Errorf("Expected %d login calls, got %d", tt.expectedLoginCalls, db.loginCallCount)
			}

			if tt.expectedLoginCalls > 0 && db.lastLoginCall != nil {
				expectedPassword := ""
				parts := strings.Fields(tt.messageText)
				if len(parts) == 2 {
					expectedPassword = parts[1]
				}

				if db.lastLoginCall.Password != expectedPassword {
					t.Errorf("Expected password %q, got %q", expectedPassword, db.lastLoginCall.Password)
				}
				if db.lastLoginCall.ChatID != 12345 {
					t.Errorf("Expected chatID 12345, got %d", db.lastLoginCall.ChatID)
				}
				if db.lastLoginCall.UserName != "testuser" {
					t.Errorf("Expected username 'testuser', got %q", db.lastLoginCall.UserName)
				}
			}

			if len(bot.SentMessages) == 0 {
				t.Error("Expected bot to send a message, but no message was sent")
				return
			}

			first := bot.SentMessages[0]
			if first.ChatID != 12345 {
				t.Errorf("Expected message to chat 12345, got %d", first.ChatID)
			}

			if tt.expectedLoginCalls == 0 {
				if first.Text == "" {
					t.Error("Expected error message for invalid format")
				}
			}
		})
	}
}

func TestLoginHandler_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		update      *tgbotapi.Update
		shouldPanic bool
	}{
		{name: "nil update", update: nil, shouldPanic: true},
		{name: "nil message", update: &tgbotapi.Update{Message: nil}, shouldPanic: true},
		{
			name: "nil chat",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{Chat: nil, From: &tgbotapi.User{UserName: "test"}, Text: "/login password"},
			},
			shouldPanic: true,
		},
		{
			name: "nil from user",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}, From: nil, Text: "/login password"},
			},
			shouldPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot := &testutils.MockBot{}
			db := NewMockDatabaseLogin()
			cfg := &config.Config{}
			a := &app.App{Bot: bot, DB: db, Config: cfg}

			panicked := false
			func() {
				defer func() {
					if r := recover(); r != nil {
						panicked = true
					}
				}()
				LoginHandler(a, tt.update)
			}()

			if tt.shouldPanic && !panicked {
				t.Error("Expected LoginHandler to panic, but it did not")
			}
			if !tt.shouldPanic && panicked {
				t.Error("LoginHandler panicked unexpectedly")
			}
		})
	}
}

func TestLoginHandler_ConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent access test in short mode")
	}

	db := NewMockDatabaseLogin()
	db.loginResult = true
	cfg := &config.Config{AdminPassword: "admin123"}

	update := &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 123},
			From: &tgbotapi.User{ID: 456, UserName: "testuser"},
			Text: "/login admin123",
		},
	}

	done := make(chan bool, 10)
	for range 10 {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Concurrent LoginHandler panicked: %v", r)
				}
				done <- true
			}()
			bot := &testutils.MockBot{}
			a := &app.App{Bot: bot, DB: db, Config: cfg}
			LoginHandler(a, update)
		}()
	}

	for range 10 {
		<-done
	}

	db.mu.Lock()
	count := db.loginCallCount
	db.mu.Unlock()
	if count != 10 {
		t.Errorf("Expected 10 login calls, got %d", count)
	}
}

package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MockDatabaseTempPassword embeds DatabaseStub and overrides GenerateTemporaryPassword.
type MockDatabaseTempPassword struct {
	testutils.DatabaseStub
	generateResult     string
	generateError      error
	generateCallCount  int
	lastGenerateCall   *GenerateCall
	generatedPasswords []string
}

type GenerateCall struct {
	Duration time.Duration
}

func NewMockDatabaseTempPassword() *MockDatabaseTempPassword {
	return &MockDatabaseTempPassword{
		generatedPasswords: make([]string, 0),
	}
}

func (m *MockDatabaseTempPassword) GenerateTemporaryPassword(_ context.Context, duration time.Duration) (string, error) {
	m.generateCallCount++
	m.lastGenerateCall = &GenerateCall{Duration: duration}

	if m.generateError != nil {
		return "", m.generateError
	}

	password := m.generateResult
	if password == "" {
		password = "generated123456"
	}

	m.generatedPasswords = append(m.generatedPasswords, password)
	return password, nil
}

//nolint:gocyclo // Test functions can be complex
func TestGenerateTempPasswordHandler(t *testing.T) {
	invalidFormatMsg := lang.Translate("error.commands.invalid_format", nil)
	invalidDurationMsg := lang.Translate("error.validation.invalid_duration", nil)
	tempPasswordErrorMsg := lang.Translate("error.security.temp_password_error", nil)

	tests := []struct {
		name                  string
		messageText           string
		mockSetup             func(*MockDatabaseTempPassword)
		expectedGenerateCalls int
		expectedSuccess       bool
		expectError           bool
		expectedDuration      time.Duration
	}{
		{
			name:        "successful generation with hours",
			messageText: "/temp 24h",
			mockSetup: func(db *MockDatabaseTempPassword) {
				db.generateResult = "abc123def456"
				db.generateError = nil
			},
			expectedGenerateCalls: 1,
			expectedSuccess:       true,
			expectError:           false,
			expectedDuration:      24 * time.Hour,
		},
		{
			name:        "successful generation with minutes",
			messageText: "/temp 30m",
			mockSetup: func(db *MockDatabaseTempPassword) {
				db.generateResult = "xyz789uvw012"
				db.generateError = nil
			},
			expectedGenerateCalls: 1,
			expectedSuccess:       true,
			expectError:           false,
			expectedDuration:      30 * time.Minute,
		},
		{
			name:        "successful generation with days",
			messageText: "/temp 7d",
			mockSetup: func(db *MockDatabaseTempPassword) {
				db.generateResult = "temp7days123"
				db.generateError = nil
			},
			expectedGenerateCalls: 1,
			expectedSuccess:       true,
			expectError:           false,
			expectedDuration:      7 * 24 * time.Hour,
		},
		{
			name:        "database error during generation",
			messageText: "/temp 1h",
			mockSetup: func(db *MockDatabaseTempPassword) {
				db.generateError = errors.New("database connection failed")
			},
			expectedGenerateCalls: 1,
			expectedSuccess:       false,
			expectError:           true,
			expectedDuration:      time.Hour,
		},
		{
			name:        "invalid command format - no duration",
			messageText: "/temp",
			mockSetup: func(_ *MockDatabaseTempPassword) {
				// No database call expected
			},
			expectedGenerateCalls: 0,
			expectedSuccess:       false,
			expectError:           false,
		},
		{
			name:        "invalid command format - too many arguments",
			messageText: "/temp 1h extra argument",
			mockSetup: func(_ *MockDatabaseTempPassword) {
				// No database call expected
			},
			expectedGenerateCalls: 0,
			expectedSuccess:       false,
			expectError:           false,
		},
		{
			name:        "invalid duration format",
			messageText: "/temp invalid_duration",
			mockSetup: func(_ *MockDatabaseTempPassword) {
				// No database call expected due to validation failure
			},
			expectedGenerateCalls: 0,
			expectedSuccess:       false,
			expectError:           false,
		},
		{
			name:        "zero duration",
			messageText: "/temp 0h",
			mockSetup: func(db *MockDatabaseTempPassword) {
				db.generateResult = "zero123"
				db.generateError = nil
			},
			expectedGenerateCalls: 1,
			expectedSuccess:       true,
			expectError:           false,
			expectedDuration:      0,
		},
		{
			name:        "negative duration",
			messageText: "/temp -1h",
			mockSetup: func(_ *MockDatabaseTempPassword) {
				// Should be rejected by ValidateDurationString
			},
			expectedGenerateCalls: 0,
			expectedSuccess:       false,
			expectError:           false,
		},
		{
			name:        "very large duration",
			messageText: "/temp 8760h",
			mockSetup: func(db *MockDatabaseTempPassword) {
				db.generateResult = "longterm123"
				db.generateError = nil
			},
			expectedGenerateCalls: 1,
			expectedSuccess:       true,
			expectError:           false,
			expectedDuration:      8760 * time.Hour,
		},
		{
			name:        "seconds not supported by ValidateDurationString",
			messageText: "/temp 3600s",
			mockSetup: func(_ *MockDatabaseTempPassword) {
				// ValidateDurationString only accepts h, m, d - not s
			},
			expectedGenerateCalls: 0,
			expectedSuccess:       false,
			expectError:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot := &testutils.MockBot{}
			db := NewMockDatabaseTempPassword()

			if tt.mockSetup != nil {
				tt.mockSetup(db)
			}

			update := &tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 12345},
					From: &tgbotapi.User{
						ID:       67890,
						UserName: "admin_user",
					},
					Text: tt.messageText,
				},
			}

			a := &app.App{Bot: bot, DB: db}
			GenerateTempPasswordHandler(a, update)

			if db.generateCallCount != tt.expectedGenerateCalls {
				t.Errorf("Expected %d generate calls, got %d", tt.expectedGenerateCalls, db.generateCallCount)
			}

			if tt.expectedGenerateCalls > 0 && db.lastGenerateCall != nil {
				if db.lastGenerateCall.Duration != tt.expectedDuration {
					t.Errorf("Expected duration %v, got %v", tt.expectedDuration, db.lastGenerateCall.Duration)
				}
			}

			lastMessage := bot.GetLastMessage()
			if lastMessage == nil {
				t.Error("Expected bot to send a message, but no message was sent")
				return
			}

			if lastMessage.ChatID != 12345 {
				t.Errorf("Expected message to chat 12345, got %d", lastMessage.ChatID)
			}

			if tt.expectedSuccess {
				if tt.mockSetup != nil && db.generateResult != "" {
					if lastMessage.Text != db.generateResult {
						t.Errorf("Expected message to contain password %q, got %q", db.generateResult, lastMessage.Text)
					}
				}
			} else {
				if tt.expectError {
					if lastMessage.Text != tempPasswordErrorMsg {
						t.Errorf("Expected temp_password_error message, got %q", lastMessage.Text)
					}
				} else if tt.expectedGenerateCalls == 0 {
					if lastMessage.Text != invalidFormatMsg && lastMessage.Text != invalidDurationMsg {
						t.Errorf("Expected invalid format or duration message, got %q", lastMessage.Text)
					}
				}
			}
		})
	}
}

func TestGenerateTempPasswordHandler_EdgeCases(t *testing.T) {
	invalidFormatMsg := lang.Translate("error.commands.invalid_format", nil)

	tests := []struct {
		name               string
		update             *tgbotapi.Update
		shouldPanic        bool
		expectMessage      bool
		expectedMsgContent string
	}{
		{name: "nil update", update: nil, shouldPanic: true},
		{name: "nil message", update: &tgbotapi.Update{Message: nil}, shouldPanic: true},
		{
			name: "nil chat",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{Chat: nil, From: &tgbotapi.User{UserName: "admin"}, Text: "/temp 1h"},
			},
			shouldPanic: true,
		},
		{
			name: "nil from user",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}, From: nil, Text: "/temp 1h"},
			},
			shouldPanic:        false,
			expectMessage:      true,
			expectedMsgContent: "generated123456",
		},
		{
			name: "empty message text",
			update: &tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 123},
					From: &tgbotapi.User{UserName: "admin"},
					Text: "",
				},
			},
			shouldPanic:        false,
			expectMessage:      true,
			expectedMsgContent: invalidFormatMsg,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot := &testutils.MockBot{}
			db := NewMockDatabaseTempPassword()
			a := &app.App{Bot: bot, DB: db}

			panicked := false
			func() {
				defer func() {
					if r := recover(); r != nil {
						panicked = true
					}
				}()
				GenerateTempPasswordHandler(a, tt.update)
			}()

			if tt.shouldPanic && !panicked {
				t.Error("Expected GenerateTempPasswordHandler to panic, but it did not")
			}
			if !tt.shouldPanic && panicked {
				t.Error("GenerateTempPasswordHandler panicked unexpectedly")
			}

			if !tt.shouldPanic && tt.expectMessage {
				lastMessage := bot.GetLastMessage()
				if lastMessage == nil {
					t.Error("Expected bot to send a message")
				} else if tt.expectedMsgContent != "" && lastMessage.Text != tt.expectedMsgContent {
					t.Errorf("Expected message %q, got %q", tt.expectedMsgContent, lastMessage.Text)
				}
			}
		})
	}
}

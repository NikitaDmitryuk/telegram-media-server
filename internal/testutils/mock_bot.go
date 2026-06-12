package testutils

import (
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const mockBotPollInterval = 10 * time.Millisecond

// MockMessage captures a single message sent by MockBot.
type MockMessage struct {
	ChatID   int64
	Text     string
	Keyboard any
}

// MockDocument captures a single document sent by MockBot.
type MockDocument struct {
	ChatID   int64
	FileName string
	Data     []byte
}

// MockBot implements bot.Service for testing.
// SentMessages collects every message sent via SendMessage.
// SentDocuments collects every document sent via SendDocument.
type MockBot struct {
	mu sync.RWMutex

	SentMessages  []MockMessage
	SentDocuments []MockDocument

	// SendDocumentError, if set, is returned by SendDocument.
	SendDocumentError error
}

func (m *MockBot) SendMessage(chatID int64, text string, keyboard any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SentMessages = append(m.SentMessages, MockMessage{
		ChatID:   chatID,
		Text:     text,
		Keyboard: keyboard,
	})
}

func (*MockBot) SendMessageReturningID(_ int64, _ string, _ any) (int, error) {
	return 0, nil
}

func (m *MockBot) SendDocument(chatID int64, fileName string, data []byte) error {
	if m.SendDocumentError != nil {
		return m.SendDocumentError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SentDocuments = append(m.SentDocuments, MockDocument{
		ChatID:   chatID,
		FileName: fileName,
		Data:     data,
	})
	return nil
}

func (*MockBot) DownloadFile(_, _ string) error { return nil }

func (*MockBot) AnswerCallbackQuery(_ tgbotapi.CallbackConfig) {}

func (*MockBot) DeleteMessage(_ int64, _ int) error { return nil }

func (*MockBot) SaveFile(_ string, _ []byte) error { return nil }

func (*MockBot) EditMessageTextAndMarkup(_ int64, _ int, _ string, _ tgbotapi.InlineKeyboardMarkup) error {
	return nil
}

// GetLastMessage returns the most recently sent message, or nil if none.
func (m *MockBot) GetLastMessage() *MockMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.SentMessages) == 0 {
		return nil
	}
	msg := m.SentMessages[len(m.SentMessages)-1]
	return &msg
}

// GetLastDocument returns the most recently sent document, or nil if none.
func (m *MockBot) GetLastDocument() *MockDocument {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.SentDocuments) == 0 {
		return nil
	}
	doc := m.SentDocuments[len(m.SentDocuments)-1]
	return &doc
}

// ClearMessages resets the captured messages.
func (m *MockBot) ClearMessages() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SentMessages = nil
	m.SentDocuments = nil
}

func (m *MockBot) MessageCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.SentMessages)
}

func (m *MockBot) DocumentCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.SentDocuments)
}

func (m *MockBot) WaitForMessageContains(substr string, timeout time.Duration) (*MockMessage, bool) {
	deadline := time.Now().Add(timeout)
	for {
		m.mu.RLock()
		for i := range m.SentMessages {
			if strings.Contains(m.SentMessages[i].Text, substr) {
				msg := m.SentMessages[i]
				m.mu.RUnlock()
				return &msg, true
			}
		}
		m.mu.RUnlock()
		if time.Now().After(deadline) {
			return nil, false
		}
		time.Sleep(mockBotPollInterval)
	}
}

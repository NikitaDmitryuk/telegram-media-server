package testutils

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

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
	SentMessages  []MockMessage
	SentDocuments []MockDocument

	// SendDocumentError, if set, is returned by SendDocument.
	SendDocumentError error
}

func (m *MockBot) SendMessage(chatID int64, text string, keyboard any) {
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
	if len(m.SentMessages) == 0 {
		return nil
	}
	return &m.SentMessages[len(m.SentMessages)-1]
}

// GetLastDocument returns the most recently sent document, or nil if none.
func (m *MockBot) GetLastDocument() *MockDocument {
	if len(m.SentDocuments) == 0 {
		return nil
	}
	return &m.SentDocuments[len(m.SentDocuments)-1]
}

// ClearMessages resets the captured messages.
func (m *MockBot) ClearMessages() {
	m.SentMessages = nil
	m.SentDocuments = nil
}

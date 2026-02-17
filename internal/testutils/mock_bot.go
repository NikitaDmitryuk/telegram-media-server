package testutils

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

// MockMessage captures a single message sent by MockBot.
type MockMessage struct {
	ChatID   int64
	Text     string
	Keyboard any
}

// MockBot implements bot.Service for testing.
// SentMessages collects every message sent via SendMessage.
type MockBot struct {
	SentMessages []MockMessage
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

// ClearMessages resets the captured messages.
func (m *MockBot) ClearMessages() {
	m.SentMessages = nil
}

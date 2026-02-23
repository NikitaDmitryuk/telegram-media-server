package admin

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func newLogsUpdate(chatID int64) *tgbotapi.Update {
	return &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: chatID},
			From: &tgbotapi.User{ID: 1, UserName: "admin"},
			Text: "/logs",
		},
	}
}

func TestLogsHandler_Success(t *testing.T) {
	originalRunCommand := RunCommand
	defer func() { RunCommand = originalRunCommand }()
	os.Setenv("RUNNING_IN_DOCKER", "")
	defer os.Unsetenv("RUNNING_IN_DOCKER")

	logOutput := []byte(
		"Feb 17 12:00:00 server telegram-media-server[1234]: started\nFeb 17 12:01:00 server telegram-media-server[1234]: processing\n",
	)
	RunCommand = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return logOutput, nil
	}

	bot := &testutils.MockBot{}
	a := &app.App{Bot: bot}
	update := newLogsUpdate(123)

	LogsHandler(a, update)

	if len(bot.SentDocuments) != 1 {
		t.Fatalf("expected 1 document sent, got %d", len(bot.SentDocuments))
	}
	doc := bot.SentDocuments[0]
	if doc.ChatID != 123 {
		t.Errorf("expected chatID 123, got %d", doc.ChatID)
	}
	if doc.FileName != "logs.txt" {
		t.Errorf("expected fileName 'logs.txt', got %q", doc.FileName)
	}
	if !bytes.Equal(doc.Data, logOutput) {
		t.Errorf("expected document data to match log output")
	}
	if len(bot.SentMessages) != 0 {
		t.Errorf("expected no text messages, got %d", len(bot.SentMessages))
	}
}

func TestLogsHandler_EmptyOutput(t *testing.T) {
	originalRunCommand := RunCommand
	defer func() { RunCommand = originalRunCommand }()
	os.Setenv("RUNNING_IN_DOCKER", "")
	defer os.Unsetenv("RUNNING_IN_DOCKER")

	RunCommand = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte{}, nil
	}

	bot := &testutils.MockBot{}
	a := &app.App{Bot: bot}
	update := newLogsUpdate(456)

	LogsHandler(a, update)

	if len(bot.SentDocuments) != 0 {
		t.Errorf("expected no documents sent, got %d", len(bot.SentDocuments))
	}
	if len(bot.SentMessages) != 1 {
		t.Fatalf("expected 1 text message, got %d", len(bot.SentMessages))
	}
	msg := bot.SentMessages[0]
	if msg.ChatID != 456 {
		t.Errorf("expected chatID 456, got %d", msg.ChatID)
	}
	if !strings.Contains(msg.Text, "logs") && !strings.Contains(msg.Text, "empty") && !strings.Contains(msg.Text, "No logs") {
		t.Errorf("expected empty-logs message, got %q", msg.Text)
	}
}

func TestLogsHandler_CommandError(t *testing.T) {
	originalRunCommand := RunCommand
	defer func() { RunCommand = originalRunCommand }()
	os.Setenv("RUNNING_IN_DOCKER", "")
	defer os.Unsetenv("RUNNING_IN_DOCKER")

	RunCommand = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, errors.New("journalctl: command not found")
	}

	bot := &testutils.MockBot{}
	a := &app.App{Bot: bot}
	update := newLogsUpdate(789)

	LogsHandler(a, update)

	if len(bot.SentDocuments) != 0 {
		t.Errorf("expected no documents sent, got %d", len(bot.SentDocuments))
	}
	if len(bot.SentMessages) != 1 {
		t.Fatalf("expected 1 error message, got %d", len(bot.SentMessages))
	}
	msg := bot.SentMessages[0]
	if msg.ChatID != 789 {
		t.Errorf("expected chatID 789, got %d", msg.ChatID)
	}
	if !strings.Contains(msg.Text, "journalctl") {
		t.Errorf("expected error message to contain 'journalctl', got %q", msg.Text)
	}
}

func TestLogsHandler_SendDocumentError(t *testing.T) {
	originalRunCommand := RunCommand
	defer func() { RunCommand = originalRunCommand }()
	os.Setenv("RUNNING_IN_DOCKER", "")
	defer os.Unsetenv("RUNNING_IN_DOCKER")

	RunCommand = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("some log data"), nil
	}

	bot := &testutils.MockBot{
		SendDocumentError: errors.New("telegram API error"),
	}
	a := &app.App{Bot: bot}
	update := newLogsUpdate(101)

	LogsHandler(a, update)

	// SendDocument failed, so no documents captured
	if len(bot.SentDocuments) != 0 {
		t.Errorf("expected no documents captured (SendDocument returned error), got %d", len(bot.SentDocuments))
	}
	// Fallback error message should be sent
	if len(bot.SentMessages) != 1 {
		t.Fatalf("expected 1 fallback error message, got %d", len(bot.SentMessages))
	}
	msg := bot.SentMessages[0]
	if msg.ChatID != 101 {
		t.Errorf("expected chatID 101, got %d", msg.ChatID)
	}
	if !strings.Contains(msg.Text, "telegram API error") {
		t.Errorf("expected error message to contain original error, got %q", msg.Text)
	}
}

func TestLogsHandler_DockerSkipsJournalctl(t *testing.T) {
	os.Setenv("RUNNING_IN_DOCKER", "true")
	defer os.Unsetenv("RUNNING_IN_DOCKER")

	runCommandCalled := false
	originalRunCommand := RunCommand
	RunCommand = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		runCommandCalled = true
		return nil, nil
	}
	defer func() { RunCommand = originalRunCommand }()

	bot := &testutils.MockBot{}
	a := &app.App{Bot: bot}
	update := newLogsUpdate(111)

	LogsHandler(a, update)

	if runCommandCalled {
		t.Error("expected RunCommand not to be called when RUNNING_IN_DOCKER=true")
	}
	if len(bot.SentDocuments) != 0 {
		t.Errorf("expected no document sent, got %d", len(bot.SentDocuments))
	}
	if len(bot.SentMessages) != 1 {
		t.Fatalf("expected 1 message (Docker hint), got %d", len(bot.SentMessages))
	}
	if !strings.Contains(bot.SentMessages[0].Text, "docker") && !strings.Contains(bot.SentMessages[0].Text, "Docker") {
		t.Errorf("expected Docker hint in message, got %q", bot.SentMessages[0].Text)
	}
}

func TestLogsHandler_CommandArguments(t *testing.T) {
	originalRunCommand := RunCommand
	defer func() { RunCommand = originalRunCommand }()
	os.Setenv("RUNNING_IN_DOCKER", "")
	defer os.Unsetenv("RUNNING_IN_DOCKER")

	var capturedName string
	var capturedArgs []string
	RunCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		capturedName = name
		capturedArgs = args
		return []byte("log line"), nil
	}

	bot := &testutils.MockBot{}
	a := &app.App{Bot: bot}
	update := newLogsUpdate(200)

	LogsHandler(a, update)

	if capturedName != "journalctl" {
		t.Errorf("expected command 'journalctl', got %q", capturedName)
	}

	expectedArgs := []string{"-u", "telegram-media-server", "--since", "1 day ago", "--no-pager"}
	if len(capturedArgs) != len(expectedArgs) {
		t.Fatalf("expected %d args, got %d: %v", len(expectedArgs), len(capturedArgs), capturedArgs)
	}
	for i, exp := range expectedArgs {
		if capturedArgs[i] != exp {
			t.Errorf("arg[%d]: expected %q, got %q", i, exp, capturedArgs[i])
		}
	}
}

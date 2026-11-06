package bot

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestSanitizeBotErrorRemovesTokenAndProxy(t *testing.T) {
	token := "123456:secret-token"
	proxy := "socks5://user:password@proxy.example:1080"
	err := errors.New("Post \"https://api.telegram.org/bot" + token + "/getMe\": proxy " + proxy + " failed")

	safeErr := sanitizeBotError(err, token, proxy)
	if safeErr == nil {
		t.Fatal("sanitizeBotError returned nil")
	}
	for _, secret := range []string{token, "secret-token", proxy, "user:password"} {
		if strings.Contains(safeErr.Error(), secret) {
			t.Fatalf("sanitized error contains %q: %s", secret, safeErr)
		}
	}
	if !strings.Contains(safeErr.Error(), "<redacted>") {
		t.Fatalf("sanitized error does not contain redaction marker: %s", safeErr)
	}
}

func TestSendMessageRepairsInvalidUTF8AndPreservesValidTextAndKeyboard(t *testing.T) {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Open", "open")),
	)
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "invalid UTF-8", text: "До\xffсвидания 👋", want: "До�свидания 👋"},
		{name: "valid Unicode unchanged", text: "Привет 👋", want: "Привет 👋"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotText string
			var gotChatID string
			var gotKeyboard tgbotapi.InlineKeyboardMarkup
			client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				body := `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test","username":"testbot"}}`
				if !strings.HasSuffix(r.URL.Path, "/getMe") {
					requestBody, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("read request body: %v", err)
					}
					form, err := url.ParseQuery(string(requestBody))
					if err != nil {
						t.Errorf("parse request form: %v", err)
					}
					gotText = form.Get("text")
					gotChatID = form.Get("chat_id")
					if err := json.Unmarshal([]byte(form.Get("reply_markup")), &gotKeyboard); err != nil {
						t.Errorf("decode reply_markup: %v", err)
					}
					body = `{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":123,"type":"private"},"text":"ok"}}`
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}

			api, err := tgbotapi.NewBotAPIWithClient("123:token", "https://telegram.test/bot%s/%s", client)
			if err != nil {
				t.Fatalf("NewBotAPIWithClient() error = %v", err)
			}
			(&Bot{Api: api}).SendMessage(123, tt.text, keyboard)

			if !utf8.ValidString(gotText) {
				t.Fatalf("sent text is invalid UTF-8: %q", gotText)
			}
			if gotText != tt.want {
				t.Errorf("sent text = %q, want %q", gotText, tt.want)
			}
			if gotChatID != "123" {
				t.Errorf("chat_id = %q, want %q", gotChatID, "123")
			}
			if !reflect.DeepEqual(gotKeyboard, keyboard) {
				t.Errorf("reply_markup = %+v, want %+v", gotKeyboard, keyboard)
			}
		})
	}
}

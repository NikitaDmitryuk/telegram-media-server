package session

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type searchHandlerBot struct{ messages []string }

func (b *searchHandlerBot) SendMessage(_ int64, text string, _ any) {
	b.messages = append(b.messages, text)
}
func (b *searchHandlerBot) SendMessageReturningID(_ int64, text string, _ any) (int, error) {
	b.messages = append(b.messages, text)
	return len(b.messages), nil
}
func (*searchHandlerBot) SendDocument(int64, string, []byte) error    { return nil }
func (*searchHandlerBot) DownloadFile(string, string) error           { return nil }
func (*searchHandlerBot) AnswerCallbackQuery(tgbotapi.CallbackConfig) {}
func (*searchHandlerBot) DeleteMessage(int64, int) error              { return nil }
func (*searchHandlerBot) SaveFile(string, []byte) error               { return nil }
func (*searchHandlerBot) EditMessageTextAndMarkup(int64, int, string, tgbotapi.InlineKeyboardMarkup) error {
	return nil
}

var _ tmsbot.Service = (*searchHandlerBot)(nil)

func TestHandleTorrentSearchQueryDistinguishesNoIndexersFromNoResults(t *testing.T) {
	localesPath, err := filepath.Abs("../../../locales")
	if err != nil {
		t.Fatal(err)
	}
	if err := lang.InitLocalizer(&config.Config{Lang: "en", LangPath: localesPath}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		searchCode  int
		indexers    string
		wantMessage string
	}{
		{
			name:        "no usable indexers",
			searchCode:  http.StatusOK,
			indexers:    `[]`,
			wantMessage: "general.torrent_search.no_indexers",
		},
		{
			name:        "genuine empty result",
			searchCode:  http.StatusOK,
			indexers:    `[{"id":1,"enable":true,"protocol":"torrent","supportsSearch":true}]`,
			wantMessage: "general.torrent_search.not_found",
		},
		{
			name:        "search error",
			searchCode:  http.StatusBadGateway,
			indexers:    `[]`,
			wantMessage: "general.torrent_search.failed",
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearTestCache()
			prowlarr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/v1/search" {
					w.WriteHeader(tt.searchCode)
					if tt.searchCode == http.StatusOK {
						_, _ = w.Write([]byte(`[]`))
					}
					return
				}
				_, _ = w.Write([]byte(tt.indexers))
			}))
			defer prowlarr.Close()

			bot := &searchHandlerBot{}
			chatID := int64(8100 + i)
			setSearchSession(chatID, &SearchSession{}, stageAwaitQuery)
			update := &tgbotapi.Update{Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: chatID},
				Text: "Matrix",
			}}
			HandleTorrentSearchQuery(&app.App{
				Bot:    bot,
				Config: &config.Config{ProwlarrURL: prowlarr.URL, ProwlarrAPIKey: "key"},
			}, update)

			want := lang.Translate(tt.wantMessage, nil)
			found := false
			for _, message := range bot.messages {
				if strings.Contains(message, want) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("messages %q do not contain expected %q", bot.messages, want)
			}
			if tt.wantMessage == "general.torrent_search.no_indexers" {
				if _, cached := getCachedResults("Matrix"); cached {
					t.Fatal("no-indexer failure must not be cached")
				}
			}
			DeleteSearchSession(chatID)
		})
	}
}

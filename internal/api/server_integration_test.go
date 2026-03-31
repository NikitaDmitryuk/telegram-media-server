//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

// TestAPI_AddDownload_201TorrentBase64 requires integration/fixtures/big-buck-bunny.torrent.
// Run: go test -tags=integration ./internal/api/ -run TestAPI_AddDownload_201TorrentBase64
func TestAPI_AddDownload_201TorrentBase64(t *testing.T) {
	torrentPath := filepath.Join("..", "..", "integration", "fixtures", "big-buck-bunny.torrent")
	raw, err := os.ReadFile(torrentPath)
	if err != nil {
		t.Fatalf("integration fixture: %v", err)
	}
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: "secret", MoviePath: t.TempDir()}
	dm := &mockDM{startReturn: 77}
	db := &testutils.DatabaseStub{}
	a := &app.App{Config: cfg, DownloadManager: dm, DB: db}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	body, _ := json.Marshal(AddDownloadRequest{
		TorrentBase64: base64.StdEncoding.EncodeToString(raw),
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/downloads", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("AddDownload torrent_base64: got status %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	var resp AddDownloadResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != 77 {
		t.Errorf("expected id 77, got %d", resp.ID)
	}
}

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	tmsdownloader "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/notifier"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

func TestMain(m *testing.M) {
	logutils.InitLogger("error")
	os.Exit(m.Run())
}

// mockDM implements manager.Service for API tests.
type mockDM struct {
	activeIDs   []uint
	queueItems  []map[string]any
	startErr    error
	stopErr     error
	startReturn uint
}

func (m *mockDM) StartDownload(
	_ tmsdownloader.Downloader,
	_ notifier.QueueNotifier,
) (id uint, progressChan chan float64, errChan chan error, err error) {
	if m.startErr != nil {
		return 0, nil, nil, m.startErr
	}
	id = m.startReturn
	if id == 0 {
		id = 1
	}
	return id, make(chan float64), make(chan error), nil
}

func (m *mockDM) StopDownload(_ uint) error     { return m.stopErr }
func (*mockDM) StopDownloadSilent(_ uint) error { return nil }
func (*mockDM) StopAllDownloads()               {}
func (m *mockDM) GetActiveDownloads() []uint {
	if m.activeIDs != nil {
		return m.activeIDs
	}
	return nil
}
func (m *mockDM) GetQueueItems() []map[string]any {
	if m.queueItems != nil {
		return m.queueItems
	}
	return nil
}

// mockDMCompletion is like mockDM but returns channels that are closed/sent after a short delay,
// so that app.RunCompletionLoop can drain them and exit (tests API download completion flow).
type mockDMCompletion struct {
	startReturn uint
}

// Ensure mocks implement the manager Service interface.
var (
	_ tmsdmanager.Service = (*mockDM)(nil)
	_ tmsdmanager.Service = (*mockDMCompletion)(nil)
)

func (m *mockDMCompletion) StartDownload(
	_ tmsdownloader.Downloader,
	_ notifier.QueueNotifier,
) (id uint, progressChan chan float64, errChan chan error, err error) {
	id = m.startReturn
	if id == 0 {
		id = 1
	}
	progressChan = make(chan float64)
	errChan = make(chan error, 1)
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(progressChan)
		errChan <- nil
	}()
	return id, progressChan, errChan, nil
}
func (*mockDMCompletion) StopDownload(_ uint) error       { return nil }
func (*mockDMCompletion) StopDownloadSilent(_ uint) error { return nil }
func (*mockDMCompletion) StopAllDownloads()               {}
func (*mockDMCompletion) GetActiveDownloads() []uint      { return nil }
func (*mockDMCompletion) GetQueueItems() []map[string]any { return nil }

// dbWithMovie returns a movie for GetMovieByID(1); other methods from stub.
type dbWithMovie struct {
	testutils.DatabaseStub
}

func (*dbWithMovie) GetMovieByID(_ context.Context, movieID uint) (database.Movie, error) {
	if movieID == 1 {
		return database.Movie{
			ID:                   1,
			Name:                 "Test Movie",
			DownloadedPercentage: 50,
			ConversionStatus:     "",
			ConversionPercentage: 0,
		}, nil
	}
	return database.Movie{}, nil
}

func TestAPI_NoKey_LocalhostAllowed(t *testing.T) {
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: ""}
	a := &app.App{Config: cfg}
	srv := NewServer(a, "127.0.0.1:0", "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", http.NoBody)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("localhost without API key: got status %d, want 200", rec.Code)
	}
}

func TestAPI_NoKey_NonLocalhostRejected(t *testing.T) {
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: ""}
	a := &app.App{Config: cfg}
	srv := NewServer(a, "127.0.0.1:0", "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", http.NoBody)
	req.RemoteAddr = "8.8.8.8:12345" // public IP, never allowed without API key
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("non-localhost without API key: got status %d, want 401", rec.Code)
	}
}

func TestAPI_NoKey_DockerPrivateIPAllowed(t *testing.T) {
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: ""}
	a := &app.App{Config: cfg}
	srv := NewServer(a, "127.0.0.1:0", "")

	os.Setenv("RUNNING_IN_DOCKER", "true")
	defer os.Unsetenv("RUNNING_IN_DOCKER")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", http.NoBody)
	req.RemoteAddr = "172.17.0.1:12345" // Docker host via port mapping
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Docker host (private IP) without API key: got status %d, want 200", rec.Code)
	}
}

func TestAPI_Health_401WithoutKey(t *testing.T) {
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: "secret"}
	a := &app.App{Config: cfg}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", http.NoBody)
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Health without key: got status %d, want 401", rec.Code)
	}
}

func TestAPI_Health_200WithKey(t *testing.T) {
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: "secret"}
	a := &app.App{Config: cfg}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", http.NoBody)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Health with Bearer: got status %d, want 200", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if len(body) == 0 {
		t.Error("Health response body empty")
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("X-Request-ID header missing")
	}
}

func TestAPI_Health_200WithXAPIKey(t *testing.T) {
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: "secret"}
	a := &app.App{Config: cfg}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", http.NoBody)
	req.Header.Set("X-API-Key", "secret")
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Health with X-API-Key: got status %d, want 200", rec.Code)
	}
}

func TestAPI_Health_401WrongKey(t *testing.T) {
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: "secret"}
	a := &app.App{Config: cfg}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", http.NoBody)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Health with wrong key: got status %d, want 401", rec.Code)
	}
}

func TestAPI_ListDownloads_200(t *testing.T) {
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: "secret"}
	db := &dbWithMovie{}
	dm := &mockDM{activeIDs: []uint{1}}
	a := &app.App{Config: cfg, DB: db, DownloadManager: dm}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/downloads", http.NoBody)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ListDownloads: got status %d, want 200", rec.Code)
	}
	var items []DownloadItem
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
	if len(items) > 0 {
		if items[0].ID != 1 || items[0].Title != "Test Movie" || items[0].Status != "downloading" {
			t.Errorf("unexpected item: %+v", items[0])
		}
	}
}

func TestAPI_ListDownloads_QueueItem(t *testing.T) {
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: "secret"}
	dm := &mockDM{queueItems: []map[string]any{
		{"movie_id": uint(2), "title": "Queued Movie", "position": 1},
	}}
	a := &app.App{Config: cfg, DB: &dbWithMovie{}, DownloadManager: dm}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/downloads", http.NoBody)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ListDownloads: got status %d, want 200", rec.Code)
	}
	var items []DownloadItem
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
	if len(items) > 0 && (items[0].Status != "queued" || items[0].Title != "Queued Movie") {
		t.Errorf("unexpected item: %+v", items[0])
	}
}

func TestAPI_DeleteDownload_204(t *testing.T) {
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: "secret"}
	dm := &mockDM{}
	a := &app.App{Config: cfg, DownloadManager: dm}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/downloads/1", http.NoBody)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("DeleteDownload: got status %d, want 204", rec.Code)
	}
}

func TestAPI_AddDownload_413BodyTooLarge(t *testing.T) {
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: "secret", MoviePath: t.TempDir()}
	a := &app.App{Config: cfg, DownloadManager: &mockDM{}}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	// Body larger than maxAddDownloadBodyBytes (1 MiB): valid JSON with a very long "url" value
	// so that the decoder hits MaxBytesReader limit and returns http.MaxBytesError.
	body := append(
		append([]byte(`{"url":"`), bytes.Repeat([]byte("x"), maxAddDownloadBodyBytes)...),
		[]byte(`"}`)...,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/downloads", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("AddDownload body too large: got status %d, want 413", rec.Code)
	}
}

func TestAPI_AddDownload_400EmptyURL(t *testing.T) {
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: "secret", MoviePath: t.TempDir()}
	a := &app.App{Config: cfg, DownloadManager: &mockDM{}}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	body, _ := json.Marshal(AddDownloadRequest{URL: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/downloads", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("AddDownload empty URL: got status %d, want 400", rec.Code)
	}
}

func TestAPI_AddDownload_201(t *testing.T) {
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: "secret", MoviePath: t.TempDir()}
	dm := &mockDM{startReturn: 42}
	db := &testutils.DatabaseStub{}
	a := &app.App{Config: cfg, DownloadManager: dm, DB: db}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	body, _ := json.Marshal(AddDownloadRequest{URL: "magnet:?xt=urn:btih:1234567890abcdef1234567890abcdef12345678&dn=Test+Movie"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/downloads", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("AddDownload: got status %d, want 201", rec.Code)
	}
	var resp AddDownloadResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != 42 {
		t.Errorf("expected id 42, got %d", resp.ID)
	}
}

// TestAPI_AddDownload_CompletionDrainsChannels verifies that when a download is added via API,
// the handler starts a completion goroutine that drains progressChan and errChan, so the manager
// (or mock) can complete and release resources. Uses mockDMCompletion which simulates completion.
func TestAPI_AddDownload_CompletionDrainsChannels(t *testing.T) {
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: "secret", MoviePath: t.TempDir()}
	dm := &mockDMCompletion{startReturn: 99}
	db := &testutils.DatabaseStub{}
	a := &app.App{Config: cfg, DownloadManager: dm, DB: db}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	body, _ := json.Marshal(AddDownloadRequest{URL: "magnet:?xt=urn:btih:ABCDEFGHIJKLMNOPQRSTUVWXYZ234567&dn=API+Movie"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/downloads", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("AddDownload: got status %d, want 201", rec.Code)
	}
	// Give completion goroutine time to drain channels and exit
	time.Sleep(100 * time.Millisecond)
}

func TestAPI_Search_503WhenNotConfigured(t *testing.T) {
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: "secret", ProwlarrURL: "", ProwlarrAPIKey: ""}
	a := &app.App{Config: cfg}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test", http.NoBody)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Search without Prowlarr: got status %d, want 503", rec.Code)
	}
}

func TestAPI_Search_400NoQuery(t *testing.T) {
	cfg := &config.Config{TMSAPIEnabled: true, TMSAPIKey: "secret", ProwlarrURL: "http://localhost:9696", ProwlarrAPIKey: "key"}
	a := &app.App{Config: cfg}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search", http.NoBody)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Search without q: got status %d, want 400", rec.Code)
	}
}

func TestAPI_OpenAPIYAML_200(t *testing.T) {
	a := &app.App{Config: &config.Config{}}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", http.NoBody)
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("openapi.yaml: got status %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-yaml" {
		t.Errorf("openapi.yaml: Content-Type %q, want application/x-yaml", ct)
	}
	body, _ := io.ReadAll(rec.Body)
	if len(body) == 0 || !bytes.Contains(body, []byte("openapi: 3.1.0")) {
		t.Error("openapi.yaml: body should contain openapi spec")
	}
}

func TestAPI_OpenAPILLMYAML_200(t *testing.T) {
	a := &app.App{Config: &config.Config{}}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi-llm.yaml", http.NoBody)
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("openapi-llm.yaml: got status %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-yaml" {
		t.Errorf("openapi-llm.yaml: Content-Type %q, want application/x-yaml", ct)
	}
	body, _ := io.ReadAll(rec.Body)
	if len(body) == 0 || !bytes.Contains(body, []byte("openapi: 3.1.0")) {
		t.Error("openapi-llm.yaml: body should contain openapi spec")
	}
}

func TestAPI_Docs_200(t *testing.T) {
	a := &app.App{Config: &config.Config{}}
	srv := NewServer(a, "127.0.0.1:0", "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs", http.NoBody)
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("docs: got status %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("docs: Content-Type %q, want text/html; charset=utf-8", ct)
	}
	body, _ := io.ReadAll(rec.Body)
	if len(body) == 0 || !bytes.Contains(body, []byte("swagger-ui")) {
		t.Error("docs: body should contain swagger-ui")
	}
}

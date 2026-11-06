package prowlarr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

const searchPath = "/api/v1/search"

func TestMain(m *testing.M) {
	logutils.InitLogger("error")
	os.Exit(m.Run())
}

func TestSearchTorrentsChecksIndexerUsabilityOnlyAfterEmptySearch(t *testing.T) {
	var searchCalls, indexerCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-key" {
			t.Errorf("X-Api-Key = %q, want test-key", r.Header.Get("X-Api-Key"))
		}
		switch r.URL.Path {
		case searchPath:
			searchCalls.Add(1)
			if r.URL.Query().Get("query") != "Matrix 1999" || r.URL.Query().Get("type") != "search" || r.URL.Query().Get("limit") != "40" {
				t.Errorf("unexpected search query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		case "/api/v1/indexer":
			indexerCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":7,"enable":true,"protocol":"torrent","supportsSearch":true}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	page, err := NewProwlarr(server.URL, "test-key").SearchTorrents("Matrix 1999", 0, 40, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Results == nil || len(page.Results) != 0 {
		t.Fatalf("empty search results = %#v, want non-nil empty slice", page.Results)
	}
	if searchCalls.Load() != 1 || indexerCalls.Load() != 1 {
		t.Fatalf("calls: search=%d indexer=%d, want 1 each", searchCalls.Load(), indexerCalls.Load())
	}
}

func TestSearchTorrentsPositiveResultSkipsIndexerRequest(t *testing.T) {
	var indexerCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/indexer" {
			indexerCalls.Add(1)
			http.Error(w, "unexpected indexer request", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"title":"The Matrix","size":123}]`))
	}))
	defer server.Close()

	page, err := NewProwlarr(server.URL, "key").SearchTorrents("Matrix", 0, 40, nil, nil)
	if err != nil || len(page.Results) != 1 {
		t.Fatalf("SearchTorrents() = (%d results, %v), want one result and nil error", len(page.Results), err)
	}
	if indexerCalls.Load() != 0 {
		t.Fatalf("indexer calls = %d, want 0", indexerCalls.Load())
	}
}

func TestSearchTorrentsReturnsNoUsableIndexers(t *testing.T) {
	tests := []struct {
		name     string
		indexers string
	}{
		{name: "none", indexers: `[]`},
		{name: "disabled", indexers: `[{"id":1,"enable":false,"protocol":"torrent","supportsSearch":true}]`},
		{name: "newznab", indexers: `[{"id":1,"enable":true,"protocol":"usenet","supportsSearch":true}]`},
		{name: "no search capability", indexers: `[{"id":1,"enable":true,"protocol":"torrent","supportsSearch":false}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == searchPath {
					_, _ = w.Write([]byte(`[]`))
					return
				}
				_, _ = w.Write([]byte(tt.indexers))
			}))
			defer server.Close()

			_, err := NewProwlarr(server.URL, "key").SearchTorrents("Matrix", 0, 40, nil, nil)
			if !errors.Is(err, ErrNoUsableIndexers) {
				t.Fatalf("error = %v, want ErrNoUsableIndexers", err)
			}
		})
	}
}

func TestSearchTorrentsHonorsSelectedIndexerIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == searchPath {
			if r.URL.Query().Get("indexerIds") != "7" {
				t.Errorf("indexerIds = %q, want 7", r.URL.Query().Get("indexerIds"))
			}
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`[{"id":8,"enable":true,"protocol":"torrent","supportsSearch":true}]`))
	}))
	defer server.Close()

	_, err := NewProwlarr(server.URL, "key").SearchTorrents("Matrix", 0, 40, []int{7}, nil)
	if !errors.Is(err, ErrNoUsableIndexers) {
		t.Fatalf("error = %v, want ErrNoUsableIndexers for unmatched selected ID", err)
	}
}

func TestSearchTorrentsIndexerErrorsRemainErrors(t *testing.T) {
	t.Run("unauthorized", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == searchPath {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}))
		defer server.Close()
		_, err := NewProwlarr(server.URL, "bad-key").SearchTorrents("Matrix", 0, 40, nil, nil)
		if err == nil || errors.Is(err, ErrNoUsableIndexers) {
			t.Fatalf("error = %v, want ordinary authorization error", err)
		}
	})

	t.Run("network", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == searchPath {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			http.Error(w, "unavailable", http.StatusBadGateway)
		}))
		defer server.Close()
		_, err := NewProwlarr(server.URL, "key").SearchTorrents("Matrix", 0, 40, nil, nil)
		if err == nil || errors.Is(err, ErrNoUsableIndexers) {
			t.Fatalf("error = %v, want ordinary upstream error", err)
		}
	})
}

func TestSearchTorrentsIndexerRequestUsesContext(t *testing.T) {
	indexerRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == searchPath {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		close(indexerRequest)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client := NewProwlarr(server.URL, "key")
	result := make(chan error, 1)
	go func() {
		_, err := client.SearchTorrentsWithContext(ctx, "Matrix", 0, 40, nil, nil)
		result <- err
	}()
	select {
	case <-indexerRequest:
	case <-time.After(time.Second):
		t.Fatal("search did not reach indexer request")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil || errors.Is(err, ErrNoUsableIndexers) || !strings.Contains(strings.ToLower(err.Error()), "context") {
			t.Fatalf("error = %v, want context cancellation error", err)
		}
	case <-time.After(time.Second):
		t.Fatal("search did not stop after context cancellation")
	}
}

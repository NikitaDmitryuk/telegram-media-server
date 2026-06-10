package qbittorrent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientCheckLoginSuccess(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v5.0.0"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "admin", "adminadmin")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	version, err := client.CheckLogin(context.Background())
	if err != nil {
		t.Fatalf("CheckLogin: %v", err)
	}
	if version != "v5.0.0" {
		t.Fatalf("version = %q, want v5.0.0", version)
	}
}

func TestClientLoginInvalidCredentials(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("Fails."))
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "admin", "bad")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := client.Login(context.Background()); err == nil {
		t.Fatal("Login succeeded, want invalid credentials error")
	}
}

func TestClientLoginForbidden(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "banned", http.StatusForbidden)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "admin", "adminadmin")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := client.Login(context.Background()); err == nil {
		t.Fatal("Login succeeded, want forbidden error")
	}
}

func TestClientLoginNonOK(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "broken", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "admin", "adminadmin")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := client.Login(context.Background()); err == nil {
		t.Fatal("Login succeeded, want non-OK error")
	}
}

func TestClientLoginConnectionRefused(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	client, err := NewClient(url, "admin", "adminadmin")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := client.Login(context.Background()); err == nil {
		t.Fatal("Login succeeded, want connection error")
	}
}

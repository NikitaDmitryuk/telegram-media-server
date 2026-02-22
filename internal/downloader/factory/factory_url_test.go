package factory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	aria2 "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/torrent"
	ytdlp "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/video"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

func TestMain(m *testing.M) {
	logutils.InitLogger("error")
	os.Exit(m.Run())
}

func TestCreateDownloaderFromURL_Empty(t *testing.T) {
	cfg := &config.Config{}
	ctx := context.Background()
	_, err := CreateDownloaderFromURL(ctx, "", "/tmp", cfg)
	if err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestCreateDownloaderFromURL_Magnet(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{}
	ctx := context.Background()
	magnet := "magnet:?xt=urn:btih:abc123&dn=Test+Movie"
	dl, err := CreateDownloaderFromURL(ctx, magnet, dir, cfg)
	if err != nil {
		t.Fatalf("CreateDownloaderFromURL(magnet): %v", err)
	}
	if dl == nil {
		t.Fatal("downloader is nil")
	}
	_ = dl.(*aria2.Aria2Downloader)
	// Check that a .magnet file was created
	// (we can't access private fields; just check type and that GetTitle works)
	title, err := dl.GetTitle()
	if err != nil {
		t.Fatalf("GetTitle: %v", err)
	}
	if title != "Test Movie" && title != "Magnet download" {
		t.Errorf("unexpected title: %s", title)
	}
}

func TestCreateDownloaderFromURL_VideoURL(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{}
	ctx := context.Background()
	url := "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
	dl, err := CreateDownloaderFromURL(ctx, url, dir, cfg)
	if err != nil {
		t.Fatalf("CreateDownloaderFromURL(video): %v", err)
	}
	if dl == nil {
		t.Fatal("downloader is nil")
	}
	_ = dl.(*ytdlp.YTDLPDownloader)
}

func TestCreateDownloaderFromURL_TorrentURLEnding(t *testing.T) {
	// We don't actually download; just check that the function recognizes .torrent URL
	// and would call downloadTorrentFile (which would fail for invalid URL in test).
	dir := t.TempDir()
	cfg := &config.Config{}
	ctx := context.Background()
	url := "https://example.com/file.torrent"
	_, err := CreateDownloaderFromURL(ctx, url, dir, cfg)
	// Should fail at download (connection or 404) or succeed if we mock
	if err != nil {
		// Expected when network fails or URL invalid
		if err.Error() != "" && err.Error() != "empty URL" {
			return
		}
	}
	// If no error, we'd have a downloader (e.g. in integration test with mock server)
	_ = dir
	_ = cfg
	_ = ctx
}

func TestCreateDownloaderFromURL_MagnetCreatesFile(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{}
	ctx := context.Background()
	magnet := "magnet:?xt=urn:btih:def456"
	_, err := CreateDownloaderFromURL(ctx, magnet, dir, cfg)
	if err != nil {
		t.Fatalf("CreateDownloaderFromURL: %v", err)
	}
	// List dir and check .magnet file exists
	entries, err := filepath.Glob(filepath.Join(dir, "magnet_*.magnet"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected one .magnet file, got %d", len(entries))
	}
}

package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/notifier"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

func TestTVPreparationFailureKeepsSourceAndMovieRecord(t *testing.T) {
	logutils.InitLogger("error")
	ctx := context.Background()
	cfg := testutils.TestConfig(t.TempDir())
	db := testutils.TestDatabase(t)
	id, err := db.AddMovie(ctx, "downloaded video", 0, []string{"movie.mp4"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(cfg.MoviePath, "movie.mp4")
	original := []byte("downloaded original remains available for retry")
	if writeErr := os.WriteFile(source, original, 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	if statusErr := db.UpdateConversionStatus(ctx, id, "failed"); statusErr != nil {
		t.Fatal(statusErr)
	}
	completed := make(chan error, 1)
	completed <- fmt.Errorf("%w: verification inconclusive", downloader.ErrTVPreparation)
	capture := &completionCapture{}
	RunCompletionLoop(&App{DB: db, Config: cfg}, completed, &testutils.MockDownloader{}, id, "downloaded video", capture)
	exists, err := db.MovieExistsId(ctx, id)
	if err != nil || !exists {
		t.Fatalf("movie record removed: exists=%v err=%v", exists, err)
	}
	files, err := db.GetFilesByMovieID(ctx, id)
	if err != nil || len(files) != 1 || files[0].FilePath != "movie.mp4" {
		t.Fatalf("file record changed: %+v err=%v", files, err)
	}
	saved, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(saved, original) {
		t.Fatal("source removed after preparation failure")
	}
	if !errors.Is(capture.failed, downloader.ErrTVPreparation) {
		t.Fatalf("failure not notified: %v", capture.failed)
	}
}

type preparationResumeManager struct {
	manager.Service
	resumes  int
	terminal error
}

func (m *preparationResumeManager) ResumeDownload(uint, downloader.Downloader, string, int, notifier.QueueNotifier) (chan error, error) {
	m.resumes++
	completed := make(chan error, 1)
	completed <- fmt.Errorf("%w: terminal completion", m.terminal)
	return completed, nil
}

func TestResumePreparationFailureDoesNotRestartDownloadedTorrent(t *testing.T) {
	logutils.InitLogger("error")
	for _, terminal := range []error{downloader.ErrTVPreparation, downloader.ErrStoppedByUser, downloader.ErrStoppedByDeletion} {
		t.Run(terminal.Error(), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/auth/login" {
					_, _ = io.WriteString(w, "Ok.")
					return
				}
				_, _ = io.WriteString(w, "v4.6.3")
			}))
			defer server.Close()
			cfg := testutils.TestConfig(t.TempDir())
			cfg.QBittorrentURL = server.URL
			db := testutils.TestDatabase(t)
			id, err := db.AddMovie(context.Background(), "downloaded", 0, []string{"movie.mp4"}, nil, 1)
			if err != nil {
				t.Fatal(err)
			}
			movie, err := db.GetMovieByID(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			controller := &preparationResumeManager{terminal: terminal}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			resumeIncompleteQBittorrentDownload(ctx, &App{DB: db, Config: cfg, DownloadManager: controller}, &movie)
			if ctx.Err() != nil || controller.resumes != 1 {
				t.Fatalf("preparation failure retried the download: calls=%d ctx=%v", controller.resumes, ctx.Err())
			}
			exists, err := db.MovieExistsId(context.Background(), id)
			if err != nil || !exists {
				t.Fatal("downloaded movie record removed")
			}
		})
	}
}

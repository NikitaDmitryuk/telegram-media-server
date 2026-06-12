package common

import (
	"context"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/models"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/notifier"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

type routerAccessDB struct {
	testutils.DatabaseStub
}

func (*routerAccessDB) IsUserAccessAllowed(_ context.Context, _ int64) (allowed bool, role database.UserRole, err error) {
	return true, models.RegularRole, nil
}

type routerDownloadManager struct {
	started chan struct{}
}

func newRouterDownloadManager() *routerDownloadManager {
	return &routerDownloadManager{started: make(chan struct{})}
}

func (m *routerDownloadManager) StartDownload(
	downloader.Downloader,
	notifier.QueueNotifier,
) (movieID uint, progressChan chan float64, outerErrChan chan error, err error) {
	close(m.started)
	errChan := make(chan error, 1)
	errChan <- downloader.ErrStoppedByDeletion
	return 1, make(chan float64), errChan, nil
}

func (*routerDownloadManager) ResumeDownload(uint, downloader.Downloader, string, int, notifier.QueueNotifier) (chan error, error) {
	return nil, nil
}

func (*routerDownloadManager) StopDownload(uint) error { return nil }

func (*routerDownloadManager) StopDownloadSilent(uint) error { return nil }

func (*routerDownloadManager) StopAllDownloads() {}

func (*routerDownloadManager) GetActiveDownloads() []uint { return nil }

func (*routerDownloadManager) GetQueueItems() []map[string]any { return nil }

func (*routerDownloadManager) RemoveQBittorrentTorrent(context.Context, uint) error { return nil }

func (*routerDownloadManager) ResumePendingTVConversions(context.Context) {}

func TestRouterExtractsLinkFromFreeFormMessage(t *testing.T) {
	logutils.InitLogger("debug")

	bot := &testutils.MockBot{}
	dm := newRouterDownloadManager()
	cfg := testutils.TestConfig(t.TempDir())
	if err := lang.InitLocalizer(cfg); err != nil {
		t.Fatalf("InitLocalizer: %v", err)
	}
	a := &app.App{
		Bot:             bot,
		DB:              &routerAccessDB{},
		Config:          cfg,
		DownloadManager: dm,
	}

	update := testutils.TextUpdate(
		123,
		456,
		"user",
		"Скачай вот это magnet:?xt=urn:btih:1234567890abcdef1234567890abcdef12345678&dn=RouterTest.",
	)

	Router(a, update)

	select {
	case <-dm.started:
	case <-time.After(time.Second):
		t.Fatal("Router did not start download for a free-form message containing a link")
	}
}

func TestRouterKeepsUnknownMessageWithoutLink(t *testing.T) {
	logutils.InitLogger("debug")

	cfg := testutils.TestConfig(t.TempDir())
	if err := lang.InitLocalizer(cfg); err != nil {
		t.Fatalf("InitLocalizer: %v", err)
	}
	bot := &testutils.MockBot{}
	a := &app.App{
		Bot:             bot,
		DB:              &routerAccessDB{},
		Config:          cfg,
		DownloadManager: newRouterDownloadManager(),
	}

	Router(a, testutils.TextUpdate(123, 456, "user", "скачай example.com/movie"))

	last := bot.GetLastMessage()
	if last == nil {
		t.Fatal("Router did not send a response for unknown text")
	}
	want := lang.Translate("error.commands.unknown_command", nil)
	if last.Text != want {
		t.Fatalf("Router response = %q, want %q", last.Text, want)
	}
}

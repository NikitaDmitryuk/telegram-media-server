package manager

import (
	"context"
	"testing"
	"time"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

// firstEpisodeMockDB implements database.Database for first_episode_ready test.
type firstEpisodeMockDB struct {
	updateEpisodesCalls []struct {
		movieID   uint
		completed int
	}
}

func (*firstEpisodeMockDB) Init(_ *tmsconfig.Config) error { return nil }
func (*firstEpisodeMockDB) AddMovie(_ context.Context, _ string, _ int64, _, _ []string, _ int) (uint, error) {
	return 1, nil
}
func (m *firstEpisodeMockDB) UpdateEpisodesProgress(_ context.Context, movieID uint, completed int) error {
	m.updateEpisodesCalls = append(m.updateEpisodesCalls, struct {
		movieID   uint
		completed int
	}{movieID, completed})
	return nil
}
func (*firstEpisodeMockDB) RemoveMovie(_ context.Context, _ uint) error { return nil }
func (*firstEpisodeMockDB) GetMovieList(_ context.Context) ([]database.Movie, error) {
	return nil, nil
}
func (*firstEpisodeMockDB) GetTempFilesByMovieID(_ context.Context, _ uint) ([]database.MovieFile, error) {
	return nil, nil
}
func (*firstEpisodeMockDB) UpdateDownloadedPercentage(_ context.Context, _ uint, _ int) error {
	return nil
}
func (*firstEpisodeMockDB) SetLoaded(_ context.Context, _ uint) error { return nil }
func (*firstEpisodeMockDB) UpdateConversionStatus(_ context.Context, _ uint, _ string) error {
	return nil
}
func (*firstEpisodeMockDB) UpdateConversionPercentage(_ context.Context, _ uint, _ int) error {
	return nil
}
func (*firstEpisodeMockDB) SetTvCompatibility(_ context.Context, _ uint, _ string) error { return nil }
func (*firstEpisodeMockDB) GetMovieByID(_ context.Context, _ uint) (database.Movie, error) {
	return database.Movie{}, nil
}
func (*firstEpisodeMockDB) MovieExistsFiles(_ context.Context, _ []string) (bool, error) {
	return false, nil
}
func (*firstEpisodeMockDB) MovieExistsId(_ context.Context, _ uint) (bool, error) {
	return true, nil
}
func (*firstEpisodeMockDB) GetFilesByMovieID(_ context.Context, _ uint) ([]database.MovieFile, error) {
	return nil, nil
}
func (*firstEpisodeMockDB) RemoveFilesByMovieID(_ context.Context, _ uint) error {
	return nil
}
func (*firstEpisodeMockDB) RemoveTempFilesByMovieID(_ context.Context, _ uint) error {
	return nil
}
func (*firstEpisodeMockDB) MovieExistsUploadedFile(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (*firstEpisodeMockDB) Login(_ context.Context, _ string, _ int64, _ string, _ *tmsconfig.Config) (bool, error) {
	return false, nil
}
func (*firstEpisodeMockDB) GetUserRole(_ context.Context, _ int64) (database.UserRole, error) {
	return "", nil
}
func (*firstEpisodeMockDB) IsUserAccessAllowed(_ context.Context, _ int64) (allowed bool, role database.UserRole, err error) {
	return false, "", nil
}
func (*firstEpisodeMockDB) AssignTemporaryPassword(_ context.Context, _ string, _ int64) error {
	return nil
}
func (*firstEpisodeMockDB) ExtendTemporaryUser(_ context.Context, _ int64, _ time.Time) error {
	return nil
}
func (*firstEpisodeMockDB) GenerateTemporaryPassword(_ context.Context, _ time.Duration) (string, error) {
	return "", nil
}
func (*firstEpisodeMockDB) GetUserByChatID(_ context.Context, _ int64) (database.User, error) {
	return database.User{}, nil
}

// firstEpisodeDownloader sends episode 1 first, then progress after delay so monitor sees first_episode_ready.
type firstEpisodeDownloader struct {
	title string
}

func (d *firstEpisodeDownloader) GetTitle() (string, error) { return d.title, nil }
func (*firstEpisodeDownloader) GetFiles() (mainFiles, tempFiles []string, err error) {
	return []string{"e01.mkv"}, []string{"t.torrent"}, nil
}
func (*firstEpisodeDownloader) GetFileSize() (int64, error) { return 1024, nil }
func (*firstEpisodeDownloader) TotalEpisodes() int          { return 8 }
func (*firstEpisodeDownloader) StoppedManually() bool       { return false }
func (*firstEpisodeDownloader) StopDownload() error         { return nil }

func (*firstEpisodeDownloader) StartDownload(
	_ context.Context,
) (progressChan chan float64, errChan chan error, episodesChan <-chan int, err error) {
	progressChan = make(chan float64, 1)
	errChan = make(chan error, 1)
	epCh := make(chan int, 1)
	epCh <- 1
	close(epCh)
	go func() {
		time.Sleep(100 * time.Millisecond) // let monitor read episodesChan first
		progressChan <- 100.0
		errChan <- nil
		close(progressChan)
		close(errChan)
	}()
	return progressChan, errChan, epCh, nil
}

func TestFirstEpisodeReadyNotification(t *testing.T) {
	logutils.InitLogger("debug")

	tempDir := testutils.TempDir(t)
	cfg := testutils.TestConfig(tempDir)
	mockDB := &firstEpisodeMockDB{}

	dm := NewDownloadManager(cfg, mockDB)
	const chatID int64 = 456
	const title = "Test Series S01"

	mockDl := &firstEpisodeDownloader{title: title}

	movieID, _, errCh, err := dm.StartDownload(mockDl, chatID)
	if err != nil {
		t.Fatalf("StartDownload: %v", err)
	}
	if movieID != 1 {
		t.Fatalf("expected movieID 1, got %d", movieID)
	}

	var notification QueueNotification
	select {
	case notification = <-dm.GetNotificationChan():
	case err := <-errCh:
		t.Fatalf("download failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for first_episode_ready notification")
	}

	if notification.Type != "first_episode_ready" {
		t.Errorf("notification type: want first_episode_ready, got %q", notification.Type)
	}
	if notification.ChatID != chatID {
		t.Errorf("notification ChatID: want %d, got %d", chatID, notification.ChatID)
	}
	if notification.Title != title {
		t.Errorf("notification Title: want %q, got %q", title, notification.Title)
	}
	if notification.MovieID != 1 {
		t.Errorf("notification MovieID: want 1, got %d", notification.MovieID)
	}

	if len(mockDB.updateEpisodesCalls) < 1 {
		t.Errorf("expected at least one UpdateEpisodesProgress call, got %d", len(mockDB.updateEpisodesCalls))
	} else {
		call := mockDB.updateEpisodesCalls[0]
		if call.movieID != 1 || call.completed != 1 {
			t.Errorf("UpdateEpisodesProgress: want movieID=1 completed=1, got movieID=%d completed=%d", call.movieID, call.completed)
		}
	}

	// Wait for download to finish (progress 100, errChan closed) so monitor exits
	<-errCh
}

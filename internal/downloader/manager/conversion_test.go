package manager

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/notifier"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

// conversionMockDB embeds DatabaseStub and records conversion-related DB calls.
type conversionMockDB struct {
	testutils.DatabaseStub
	mu          sync.Mutex
	loadedCalls int

	SetTvCompatibilityCalls []struct {
		MovieID uint
		Compat  string
	}
	UpdateConversionStatusCalls []struct {
		MovieID uint
		Status  string
	}
	UpdateConversionPctCalls []struct {
		MovieID uint
		Pct     int
	}
}

func (*conversionMockDB) GetMovieByID(_ context.Context, movieID uint) (database.Movie, error) {
	return database.Movie{ID: movieID, TvCompatibility: "green"}, nil
}

func (*conversionMockDB) MovieExistsId(_ context.Context, _ uint) (bool, error) {
	return true, nil
}

func (m *conversionMockDB) SetTvCompatibility(_ context.Context, movieID uint, compat string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SetTvCompatibilityCalls = append(m.SetTvCompatibilityCalls, struct {
		MovieID uint
		Compat  string
	}{movieID, compat})
	return nil
}

func (m *conversionMockDB) UpdateConversionStatus(_ context.Context, movieID uint, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.UpdateConversionStatusCalls = append(m.UpdateConversionStatusCalls, struct {
		MovieID uint
		Status  string
	}{movieID, status})
	return nil
}

func (m *conversionMockDB) UpdateConversionPercentage(_ context.Context, movieID uint, pct int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.UpdateConversionPctCalls = append(m.UpdateConversionPctCalls, struct {
		MovieID uint
		Pct     int
	}{movieID, pct})
	return nil
}

func TestEnqueueConversionIfNeeded_CompatibilityModeOff(t *testing.T) {
	cfg := testutils.TestConfig("/tmp")
	cfg.VideoSettings.CompatibilityMode = false
	mockDB := &conversionMockDB{}
	dm := NewDownloadManager(cfg, mockDB)
	ctx := context.Background()

	needWait, done, compatRed := dm.enqueueConversionIfNeeded(ctx, 1, "test")
	if needWait || done != nil || compatRed {
		t.Errorf(
			"compatibility mode off: want needWait=false, done=nil, compatRed=false; got needWait=%v done=%v compatRed=%v",
			needWait,
			done != nil,
			compatRed,
		)
	}

	mockDB.mu.Lock()
	defer mockDB.mu.Unlock()
	if len(mockDB.SetTvCompatibilityCalls) != 0 {
		t.Errorf("Compatibility mode off: expected no SetTvCompatibility calls, got %d", len(mockDB.SetTvCompatibilityCalls))
	}
	if len(mockDB.UpdateConversionStatusCalls) != 0 {
		t.Errorf("Compatibility mode off: expected no UpdateConversionStatus calls, got %d", len(mockDB.UpdateConversionStatusCalls))
	}
}

func TestEnqueueConversionIfNeeded_UnknownProbeStillAttemptsPreparation(t *testing.T) {
	cfg := testutils.TestConfig(t.TempDir())
	cfg.VideoSettings.CompatibilityMode = true
	mockDB := &conversionMockDB{}
	dm := &DownloadManager{cfg: cfg, db: mockDB, conversionQueue: make(chan conversionJob, 1)}
	needWait, done, compatRed := dm.enqueueConversionIfNeeded(context.Background(), 1, "test")
	if !needWait || done == nil || compatRed {
		t.Fatal("unknown probe bypassed preparation")
	}
	if len(mockDB.SetTvCompatibilityCalls) != 1 || mockDB.SetTvCompatibilityCalls[0].Compat != "yellow" {
		t.Fatal("unknown probe retained stale green flag")
	}
}

func TestEnqueueConversion_CompatibilityModeOff(t *testing.T) {
	t.Helper()
	cfg := testutils.TestConfig("/tmp")
	cfg.VideoSettings.CompatibilityMode = false
	dm := NewDownloadManager(cfg, &conversionMockDB{})

	_, _ = dm.EnqueueConversion(1, "test")
	// Should not block or panic; conversion queue is not sent to when mode is off
}

func TestConfigurationInheritance_CompatibilityMode(t *testing.T) {
	cfg := testutils.TestConfig("/tmp")
	cfg.VideoSettings.CompatibilityMode = true
	dm := NewDownloadManager(cfg, testutils.TestDatabase(t))
	if !dm.cfg.VideoSettings.CompatibilityMode {
		t.Error("expected CompatibilityMode true on manager config")
	}
	if dm.conversionQueue == nil {
		t.Error("expected conversionQueue to be initialized when CompatibilityMode is true")
	}
}

func TestConversionWorkerStaleGreenDoesNotBypassFailure(t *testing.T) {
	cfg := testutils.TestConfig(t.TempDir())
	cfg.VideoSettings.CompatibilityMode = true
	db := &conversionMockDB{}
	dm := &DownloadManager{cfg: cfg, db: db, conversionQueue: make(chan conversionJob, 1)}
	done := make(chan struct{})
	dm.conversionQueue <- conversionJob{MovieID: 1, Done: done}
	close(dm.conversionQueue)
	go dm.runConversionWorker()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not finish")
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if len(db.UpdateConversionStatusCalls) != 2 || db.UpdateConversionStatusCalls[1].Status != "failed" {
		t.Fatalf("stale green skipped preparation: %+v", db.UpdateConversionStatusCalls)
	}
	if len(db.UpdateConversionPctCalls) != 1 || db.UpdateConversionPctCalls[0].Pct != 0 {
		t.Fatal("failed conversion marked 100 percent")
	}
}

func TestTVPreparationResultRejectsFailedOrMissingConversion(t *testing.T) {
	cfg := testutils.TestConfig(t.TempDir())
	cfg.VideoSettings.CompatibilityMode = true
	dm := &DownloadManager{cfg: cfg, db: &conversionMockDB{}}
	if err := dm.tvPreparationResult(context.Background(), 1); err == nil {
		t.Fatal("failed/missing preparation announced ready")
	}
	cfg.VideoSettings.CompatibilityMode = false
	if err := dm.tvPreparationResult(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
}

func (m *conversionMockDB) SetLoaded(context.Context, uint, string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadedCalls++
	return nil
}

func TestFailedPreparationNeverMarksDownloadLoaded(t *testing.T) {
	cfg := testutils.TestConfig(t.TempDir())
	cfg.VideoSettings.CompatibilityMode = true
	db := &conversionMockDB{}
	dm := &DownloadManager{cfg: cfg, db: db, jobs: make(map[uint]*downloadJob),
		semaphore: make(chan struct{}, 1), conversionQueue: make(chan conversionJob, 1), downloadSettings: cfg.DownloadSettings}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	finished := make(chan error, 1)
	finished <- nil
	job := &downloadJob{downloader: &testutils.MockDownloader{}, ctx: ctx, cancel: cancel, errChan: finished,
		queueNotifier: notifier.Noop, startTime: time.Now()}
	dm.jobs[1] = job
	dm.semaphore <- struct{}{}
	go dm.runConversionWorker()
	go dm.monitorDownload(1, job, result)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("failed preparation reported successful completion")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("completion blocked")
	}
	close(dm.conversionQueue)
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.loadedCalls != 0 {
		t.Fatal("failed preparation marked download loaded")
	}
}

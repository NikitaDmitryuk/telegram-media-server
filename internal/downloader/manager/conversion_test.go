package manager

import (
	"context"
	"sync"
	"testing"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

// conversionMockDB embeds DatabaseStub and records conversion-related DB calls.
type conversionMockDB struct {
	testutils.DatabaseStub
	mu sync.Mutex

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
	return database.Movie{ID: movieID, TvCompatibility: "yellow"}, nil
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

	needWait, done, compatRed := dm.enqueueConversionIfNeeded(ctx, 1, 0, "test")
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

func TestEnqueueConversionIfNeeded_CompatibilityModeOn_NoFiles_KeepsEarlyEstimate(t *testing.T) {
	cfg := testutils.TestConfig("/tmp")
	cfg.VideoSettings.CompatibilityMode = true
	cfg.VideoSettings.TvH264Level = "4.1"
	mockDB := &conversionMockDB{}
	// GetFilesByMovieID returns empty -> ProbeTvCompatibility returns "" (unknown).
	// When the probe cannot determine compatibility (no files on disk, ffprobe missing, etc.),
	// we keep the early estimate (e.g. green from file extension) instead of overriding with red.
	dm := NewDownloadManager(cfg, mockDB)
	ctx := context.Background()

	needWait, done, compatRed := dm.enqueueConversionIfNeeded(ctx, 1, 0, "test")
	if needWait || done != nil || compatRed {
		t.Errorf(
			"unknown probe: want needWait=false, done=nil, compatRed=false; got needWait=%v done=%v compatRed=%v",
			needWait,
			done != nil,
			compatRed,
		)
	}

	mockDB.mu.Lock()
	defer mockDB.mu.Unlock()
	// When probe returns "" the early estimate is preserved — no DB calls should be made.
	if len(mockDB.SetTvCompatibilityCalls) != 0 {
		t.Errorf("expected 0 SetTvCompatibility calls when probe is unknown, got %d", len(mockDB.SetTvCompatibilityCalls))
	}
	if len(mockDB.UpdateConversionStatusCalls) != 0 {
		t.Errorf("expected 0 UpdateConversionStatus calls when probe is unknown, got %d", len(mockDB.UpdateConversionStatusCalls))
	}
	if len(mockDB.UpdateConversionPctCalls) != 0 {
		t.Errorf("expected 0 UpdateConversionPercentage calls when probe is unknown, got %d", len(mockDB.UpdateConversionPctCalls))
	}
}

func TestEnqueueConversion_CompatibilityModeOff(t *testing.T) {
	t.Helper()
	cfg := testutils.TestConfig("/tmp")
	cfg.VideoSettings.CompatibilityMode = false
	dm := NewDownloadManager(cfg, &conversionMockDB{})

	_, _ = dm.EnqueueConversion(1, 0, "test")
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

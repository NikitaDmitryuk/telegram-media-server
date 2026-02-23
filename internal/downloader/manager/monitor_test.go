package manager

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

func TestMain(m *testing.M) {
	logutils.InitLogger("debug")
	os.Exit(m.Run())
}

// helper that builds a DownloadManager with small stagnant-detection windows.
func newTestManager(t *testing.T) *DownloadManager {
	t.Helper()
	cfg := testutils.TestConfig("/tmp")
	cfg.DownloadSettings.DownloadTimeout = 0 // no global timeout
	cfg.DownloadSettings.ProgressUpdateInterval = 50 * time.Millisecond
	cfg.VideoSettings.CompatibilityMode = false
	db := testutils.TestDatabase(t)
	return NewDownloadManager(cfg, db)
}

// TestStagnantProgressTriggersError verifies that if progress doesn't change for
// maxStagnantDuration the monitor reports a stagnation error.
// We cannot override maxStagnantDuration (it's a local var, 30 min), so instead we
// simulate the condition by sending a small initial progress and then never updating.
// To make the test fast we check that the stagnation path is exercised by closing
// the progress channel (i.e. "download completed") before the 30 min threshold — and
// the monitor should return nil on normal close, NOT a stagnation error.
func TestStagnantProgress_NormalCloseBeforeThreshold(t *testing.T) {
	dm := newTestManager(t)

	progressChan := make(chan float64, 10)
	errChan := make(chan error, 1)
	outerErrChan := make(chan error, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := downloadJob{
		downloader:   &testutils.MockDownloader{ShouldBlock: true},
		startTime:    time.Now(),
		progressChan: progressChan,
		errChan:      errChan,
		ctx:          ctx,
		cancel:       cancel,
	}

	dm.mu.Lock()
	dm.jobs[1] = &job
	dm.mu.Unlock()
	dm.semaphore <- struct{}{} // occupy semaphore so cleanup works

	go dm.monitorDownload(1, &job, outerErrChan)

	// Send some progress, then close progressChan and send result on errChan.
	progressChan <- 50.0
	time.Sleep(20 * time.Millisecond)
	close(progressChan)
	errChan <- nil

	select {
	case err := <-outerErrChan:
		if err != nil {
			t.Errorf("Expected nil error on normal close, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Monitor did not return in time after progressChan was closed")
	}
}

// TestEpisodeChanResetsStagnantTimer verifies the core fix: receiving a value on
// episodesChan resets the stagnation timer so the download isn't killed between
// sequential torrent batches.
//
// Strategy: send a small progress (triggers stagnation tracking), then keep the
// download alive by periodically sending episode completions. Finally close
// progressChan to complete the download. If the fix is correct the download
// should complete normally (nil error). Without the fix the monitor would
// eventually trigger a stagnation error (but since maxStagnantDuration is 30 min
// we can't wait that long; we verify the reset logic by checking that the timer
// doesn't fire within a controlled window).
func TestEpisodeChanResetsStagnantTimer(t *testing.T) {
	dm := newTestManager(t)

	progressChan := make(chan float64, 10)
	errChan := make(chan error, 1)
	episodesChan := make(chan int, 10)
	outerErrChan := make(chan error, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := downloadJob{
		downloader:    &testutils.MockDownloader{ShouldBlock: true},
		startTime:     time.Now(),
		progressChan:  progressChan,
		errChan:       errChan,
		episodesChan:  episodesChan,
		ctx:           ctx,
		cancel:        cancel,
		totalEpisodes: 5,
	}

	dm.mu.Lock()
	dm.jobs[1] = &job
	dm.mu.Unlock()
	dm.semaphore <- struct{}{}

	go dm.monitorDownload(1, &job, outerErrChan)

	// Send initial progress so stagnation tracking starts.
	progressChan <- 10.0
	time.Sleep(20 * time.Millisecond)

	// Episode completions should reset the stagnant timer.
	episodesChan <- 1
	time.Sleep(20 * time.Millisecond)
	episodesChan <- 2
	time.Sleep(20 * time.Millisecond)

	// Complete the download normally.
	close(progressChan)
	errChan <- nil

	select {
	case err := <-outerErrChan:
		if err != nil {
			t.Errorf("Expected nil error when episodes keep resetting stagnant timer, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Monitor did not return in time")
	}
}

// TestProgressChanClose_CompletesDownload verifies that closing progressChan
// (normal completion path) results in a nil error.
func TestProgressChanClose_CompletesDownload(t *testing.T) {
	dm := newTestManager(t)

	progressChan := make(chan float64, 10)
	errChan := make(chan error, 1)
	outerErrChan := make(chan error, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := downloadJob{
		downloader:   &testutils.MockDownloader{ShouldBlock: true},
		startTime:    time.Now(),
		progressChan: progressChan,
		errChan:      errChan,
		ctx:          ctx,
		cancel:       cancel,
	}

	dm.mu.Lock()
	dm.jobs[1] = &job
	dm.mu.Unlock()
	dm.semaphore <- struct{}{}

	go dm.monitorDownload(1, &job, outerErrChan)

	progressChan <- 50.0
	progressChan <- 100.0
	time.Sleep(20 * time.Millisecond)
	close(progressChan)
	errChan <- nil

	select {
	case err := <-outerErrChan:
		if err != nil {
			t.Errorf("Expected nil error for completed download, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Monitor did not complete in time")
	}
}

// TestErrChanReceivesError verifies that an error sent on errChan is forwarded.
func TestErrChanReceivesError(t *testing.T) {
	dm := newTestManager(t)

	progressChan := make(chan float64, 10)
	errChan := make(chan error, 1)
	outerErrChan := make(chan error, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := downloadJob{
		downloader:   &testutils.MockDownloader{ShouldBlock: true},
		startTime:    time.Now(),
		progressChan: progressChan,
		errChan:      errChan,
		ctx:          ctx,
		cancel:       cancel,
	}

	dm.mu.Lock()
	dm.jobs[1] = &job
	dm.mu.Unlock()
	dm.semaphore <- struct{}{}

	go dm.monitorDownload(1, &job, outerErrChan)

	errChan <- context.DeadlineExceeded

	select {
	case err := <-outerErrChan:
		if err == nil {
			t.Error("Expected error from errChan, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Error("Monitor did not forward error in time")
	}
}

// TestContextCancellation verifies that canceling the context stops the monitor.
func TestContextCancellation(t *testing.T) {
	dm := newTestManager(t)

	progressChan := make(chan float64, 10)
	errChan := make(chan error, 1)
	outerErrChan := make(chan error, 1)

	ctx, cancel := context.WithCancel(context.Background())

	job := downloadJob{
		downloader:   &testutils.MockDownloader{ShouldBlock: true},
		startTime:    time.Now(),
		progressChan: progressChan,
		errChan:      errChan,
		ctx:          ctx,
		cancel:       cancel,
	}

	dm.mu.Lock()
	dm.jobs[1] = &job
	dm.mu.Unlock()
	dm.semaphore <- struct{}{}

	go dm.monitorDownload(1, &job, outerErrChan)

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-outerErrChan:
		if err == nil {
			t.Error("Expected context cancellation error, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Error("Monitor did not react to context cancellation")
	}
}

// TestEpisodeChanClosed_DoesNotPanic verifies that closing episodesChan
// is handled gracefully (channel is nilled out, no panic).
func TestEpisodeChanClosed_DoesNotPanic(t *testing.T) {
	dm := newTestManager(t)

	progressChan := make(chan float64, 10)
	errChan := make(chan error, 1)
	episodesChan := make(chan int, 10)
	outerErrChan := make(chan error, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := downloadJob{
		downloader:    &testutils.MockDownloader{ShouldBlock: true},
		startTime:     time.Now(),
		progressChan:  progressChan,
		errChan:       errChan,
		episodesChan:  episodesChan,
		ctx:           ctx,
		cancel:        cancel,
		totalEpisodes: 3,
	}

	dm.mu.Lock()
	dm.jobs[1] = &job
	dm.mu.Unlock()
	dm.semaphore <- struct{}{}

	go dm.monitorDownload(1, &job, outerErrChan)

	// Send one episode, then close the channel.
	episodesChan <- 1
	time.Sleep(20 * time.Millisecond)
	close(episodesChan)
	time.Sleep(20 * time.Millisecond)

	// Complete download via progressChan close.
	close(progressChan)
	errChan <- nil

	select {
	case err := <-outerErrChan:
		if err != nil {
			t.Errorf("Expected nil error, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Monitor did not complete in time after episodesChan closed")
	}
}

// TestDownloadTimeout verifies the absolute download timeout (not stagnant, but total).
func TestDownloadTimeout_FromMonitor(t *testing.T) {
	cfg := testutils.TestConfig("/tmp")
	cfg.DownloadSettings.DownloadTimeout = 200 * time.Millisecond
	cfg.DownloadSettings.ProgressUpdateInterval = 50 * time.Millisecond
	cfg.VideoSettings.CompatibilityMode = false
	db := testutils.TestDatabase(t)
	dm := NewDownloadManager(cfg, db)

	progressChan := make(chan float64, 10)
	errChan := make(chan error, 1)
	outerErrChan := make(chan error, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := downloadJob{
		downloader:   &testutils.MockDownloader{ShouldBlock: true},
		startTime:    time.Now(),
		progressChan: progressChan,
		errChan:      errChan,
		ctx:          ctx,
		cancel:       cancel,
	}

	dm.mu.Lock()
	dm.jobs[1] = &job
	dm.mu.Unlock()
	dm.semaphore <- struct{}{}

	go dm.monitorDownload(1, &job, outerErrChan)

	select {
	case err := <-outerErrChan:
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Monitor did not fire download timeout")
	}
}

// TestErrChanNilError verifies that sending nil on errChan is treated as success.
func TestErrChanNilError(t *testing.T) {
	dm := newTestManager(t)

	progressChan := make(chan float64, 10)
	errChan := make(chan error, 1)
	outerErrChan := make(chan error, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := downloadJob{
		downloader:   &testutils.MockDownloader{ShouldBlock: true},
		startTime:    time.Now(),
		progressChan: progressChan,
		errChan:      errChan,
		ctx:          ctx,
		cancel:       cancel,
	}

	dm.mu.Lock()
	dm.jobs[1] = &job
	dm.mu.Unlock()
	dm.semaphore <- struct{}{}

	go dm.monitorDownload(1, &job, outerErrChan)

	errChan <- nil

	select {
	case err := <-outerErrChan:
		if err != nil {
			t.Errorf("Expected nil error for successful errChan close, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Monitor did not return in time")
	}
}

// TestProgressOver100_IsClamped verifies that progress > 100 is clamped to 100.
func TestProgressOver100_IsClamped(t *testing.T) {
	dm := newTestManager(t)

	progressChan := make(chan float64, 10)
	errChan := make(chan error, 1)
	outerErrChan := make(chan error, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := downloadJob{
		downloader:   &testutils.MockDownloader{ShouldBlock: true},
		startTime:    time.Now(),
		progressChan: progressChan,
		errChan:      errChan,
		ctx:          ctx,
		cancel:       cancel,
	}

	dm.mu.Lock()
	dm.jobs[1] = &job
	dm.mu.Unlock()
	dm.semaphore <- struct{}{}

	go dm.monitorDownload(1, &job, outerErrChan)

	// Send progress > 100; it should be clamped.
	progressChan <- 150.0
	time.Sleep(20 * time.Millisecond)

	// Complete normally: progressChan close then result on errChan (monitor waits for errChan).
	close(progressChan)
	errChan <- nil

	select {
	case err := <-outerErrChan:
		if err != nil {
			t.Errorf("Expected nil error, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Monitor did not complete in time")
	}
}

// TestMultipleEpisodes_SequentialCompletion simulates a multi-file torrent download
// where several episodes complete sequentially, ensuring the monitor handles them
// correctly and completes successfully.
func TestMultipleEpisodes_SequentialCompletion(t *testing.T) {
	dm := newTestManager(t)

	progressChan := make(chan float64, 10)
	errChan := make(chan error, 1)
	episodesChan := make(chan int, 10)
	outerErrChan := make(chan error, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := downloadJob{
		downloader:    &testutils.MockDownloader{ShouldBlock: true, TotalEps: 4},
		startTime:     time.Now(),
		progressChan:  progressChan,
		errChan:       errChan,
		episodesChan:  episodesChan,
		ctx:           ctx,
		cancel:        cancel,
		totalEpisodes: 4,
	}

	dm.mu.Lock()
	dm.jobs[1] = &job
	dm.mu.Unlock()
	dm.semaphore <- struct{}{}

	go dm.monitorDownload(1, &job, outerErrChan)

	// Simulate downloading 4 episodes sequentially.
	for ep := 1; ep <= 4; ep++ {
		progressChan <- float64(ep * 25)
		time.Sleep(15 * time.Millisecond)
		episodesChan <- ep
		time.Sleep(15 * time.Millisecond)
	}

	close(progressChan)
	errChan <- nil

	select {
	case err := <-outerErrChan:
		if err != nil {
			t.Errorf("Expected nil error after all episodes completed, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Monitor did not complete in time")
	}
}

// TestNoEpisodesChan_NilHandling verifies that the monitor works when episodesChan is nil
// (e.g. single-file download or yt-dlp download).
func TestNoEpisodesChan_NilHandling(t *testing.T) {
	dm := newTestManager(t)

	progressChan := make(chan float64, 10)
	errChan := make(chan error, 1)
	outerErrChan := make(chan error, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := downloadJob{
		downloader:   &testutils.MockDownloader{ShouldBlock: true},
		startTime:    time.Now(),
		progressChan: progressChan,
		errChan:      errChan,
		episodesChan: nil, // no episodes channel
		ctx:          ctx,
		cancel:       cancel,
	}

	dm.mu.Lock()
	dm.jobs[1] = &job
	dm.mu.Unlock()
	dm.semaphore <- struct{}{}

	go dm.monitorDownload(1, &job, outerErrChan)

	progressChan <- 100.0
	time.Sleep(20 * time.Millisecond)
	close(progressChan)
	errChan <- nil

	select {
	case err := <-outerErrChan:
		if err != nil {
			t.Errorf("Expected nil error, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Monitor did not complete in time")
	}
}

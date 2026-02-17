package manager

import (
	"sync"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

// TestConcurrentDownloads_TwoMoviesRunSimultaneously verifies that two different movies
// can download simultaneously when MaxConcurrentDownloads >= 2.
// This test ensures that the semaphore allows multiple concurrent downloads.
func TestConcurrentDownloads_TwoMoviesRunSimultaneously(t *testing.T) {
	cfg := testutils.TestConfig("/tmp")
	cfg.DownloadSettings.MaxConcurrentDownloads = 3
	cfg.VideoSettings.CompatibilityMode = false
	db := testutils.TestDatabase(t)
	dm := NewDownloadManager(cfg, db)

	// Create two mock downloaders that will block until we signal them
	mock1 := &testutils.MockDownloader{
		ShouldBlock: true,
		Title:       "Movie 1",
		Files:       []string{"/tmp/movie1.mp4"},
	}
	mock2 := &testutils.MockDownloader{
		ShouldBlock: true,
		Title:       "Movie 2",
		Files:       []string{"/tmp/movie2.mp4"},
	}

	var wg sync.WaitGroup
	var movie1ID, movie2ID uint
	var err1, err2 error

	// Start first download
	wg.Add(1)
	go func() {
		defer wg.Done()
		movie1ID, _, _, err1 = dm.StartDownload(mock1, 100)
	}()

	// Wait a bit to ensure first download starts
	time.Sleep(50 * time.Millisecond)

	// Start second download
	wg.Add(1)
	go func() {
		defer wg.Done()
		movie2ID, _, _, err2 = dm.StartDownload(mock2, 200)
	}()

	// Wait for both StartDownload calls to complete
	wg.Wait()

	if err1 != nil {
		t.Fatalf("StartDownload for Movie 1 failed: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("StartDownload for Movie 2 failed: %v", err2)
	}

	// Both downloads should be active (not queued)
	activeCount := dm.GetDownloadCount()
	queueCount := dm.GetQueueCount()

	if activeCount != 2 {
		t.Errorf("Expected 2 active downloads, got %d (queue: %d)", activeCount, queueCount)
	}
	if queueCount != 0 {
		t.Errorf("Expected 0 queued downloads, got %d", queueCount)
	}

	// Verify both movie IDs are in active downloads
	activeDownloads := dm.GetActiveDownloads()
	found1, found2 := false, false
	for _, id := range activeDownloads {
		if id == movie1ID {
			found1 = true
		}
		if id == movie2ID {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Errorf("Expected both movies to be active, found1=%v, found2=%v, activeDownloads=%v",
			found1, found2, activeDownloads)
	}

	// Cleanup: stop downloads to trigger cleanup
	_ = dm.StopDownload(movie1ID)
	_ = dm.StopDownload(movie2ID)
}

// TestSemaphoreReleasedAfterCompletion verifies that the semaphore is released
// after a download completes, allowing new downloads to start.
func TestSemaphoreReleasedAfterCompletion(t *testing.T) {
	cfg := testutils.TestConfig("/tmp")
	cfg.DownloadSettings.MaxConcurrentDownloads = 1 // Only 1 concurrent download
	cfg.VideoSettings.CompatibilityMode = false
	db := testutils.TestDatabase(t)
	dm := NewDownloadManager(cfg, db)

	// First download - will complete quickly
	mock1 := &testutils.MockDownloader{
		ShouldBlock: false, // Will complete immediately
		Title:       "Movie 1",
		Files:       []string{"/tmp/movie1_sem.mp4"},
	}

	movieID1, _, errChan1, err := dm.StartDownload(mock1, 100)
	if err != nil {
		t.Fatalf("StartDownload for Movie 1 failed: %v", err)
	}

	// Wait for first download to complete
	select {
	case <-errChan1:
		// Download completed
	case <-time.After(2 * time.Second):
		t.Fatal("First download did not complete in time")
	}

	// Give time for semaphore to be released
	time.Sleep(100 * time.Millisecond)

	// Check that semaphore was released
	if dm.GetDownloadCount() != 0 {
		t.Errorf("Expected 0 active downloads after completion, got %d", dm.GetDownloadCount())
	}

	// Second download - should start immediately (not be queued)
	mock2 := &testutils.MockDownloader{
		ShouldBlock: true,
		Title:       "Movie 2",
		Files:       []string{"/tmp/movie2_sem.mp4"},
	}

	movieID2, _, _, err := dm.StartDownload(mock2, 200)
	if err != nil {
		t.Fatalf("StartDownload for Movie 2 failed: %v", err)
	}

	// Should be active, not queued
	activeCount := dm.GetDownloadCount()
	queueCount := dm.GetQueueCount()

	if activeCount != 1 {
		t.Errorf("Expected 1 active download, got %d", activeCount)
	}
	if queueCount != 0 {
		t.Errorf("Second download should not be queued, but queue count is %d", queueCount)
	}

	// Cleanup
	_ = dm.StopDownload(movieID1)
	_ = dm.StopDownload(movieID2)
}

// TestQueuedDownloadReceivesProgressAndCompletion verifies that when a download
// starts from the queue, the original caller receives progress updates and
// completion signal through the channels returned by StartDownload.
func TestQueuedDownloadReceivesProgressAndCompletion(t *testing.T) {
	cfg := testutils.TestConfig("/tmp")
	cfg.DownloadSettings.MaxConcurrentDownloads = 1 // Force queueing
	cfg.VideoSettings.CompatibilityMode = false
	db := testutils.TestDatabase(t)
	dm := NewDownloadManager(cfg, db)

	// First download - blocks the semaphore
	mock1 := &testutils.MockDownloader{
		ShouldBlock: true,
		Title:       "Blocking Movie",
		Files:       []string{"/tmp/blocking.mp4"},
	}

	movie1ID, _, _, err := dm.StartDownload(mock1, 100)
	if err != nil {
		t.Fatalf("StartDownload for blocking movie failed: %v", err)
	}

	// Verify first download is active
	if dm.GetDownloadCount() != 1 {
		t.Fatalf("Expected 1 active download, got %d", dm.GetDownloadCount())
	}

	// Second download - should be queued
	mock2 := &testutils.MockDownloader{
		ShouldBlock: false, // Will complete immediately when started
		Title:       "Queued Movie",
		Files:       []string{"/tmp/queued.mp4"},
	}

	movie2ID, progressChan2, errChan2, err := dm.StartDownload(mock2, 200)
	if err != nil {
		t.Fatalf("StartDownload for queued movie failed: %v", err)
	}

	// Verify second download is queued
	time.Sleep(50 * time.Millisecond)
	queueCount := dm.GetQueueCount()
	if queueCount != 1 {
		t.Fatalf("Expected 1 queued download, got %d", queueCount)
	}

	// Stop first download to release semaphore
	_ = dm.StopDownload(movie1ID)

	// Wait for queue to be processed and second download to complete
	progressReceived := false
	completionReceived := false

	timeout := time.After(5 * time.Second)
	for !completionReceived {
		select {
		case progress, ok := <-progressChan2:
			if ok {
				progressReceived = true
				t.Logf("Received progress: %.2f", progress)
			}
		case err, ok := <-errChan2:
			if !ok {
				// Channel closed without error means success
				completionReceived = true
			} else if err == nil {
				completionReceived = true
			} else {
				t.Fatalf("Queued download failed with error: %v", err)
			}
		case <-timeout:
			t.Fatalf("Queued download did not complete in time. progressReceived=%v", progressReceived)
		}
	}

	// Note: progressReceived may or may not be true depending on timing
	// The important thing is that completionReceived is true
	t.Logf("Queued download completed. progressReceived=%v", progressReceived)

	// Cleanup
	_ = dm.StopDownload(movie2ID)
}

// TestQueuedDownloadDoesNotCompleteImmediately verifies that adding a download
// to the queue does NOT immediately signal completion (the bug we fixed).
func TestQueuedDownloadDoesNotCompleteImmediately(t *testing.T) {
	cfg := testutils.TestConfig("/tmp")
	cfg.DownloadSettings.MaxConcurrentDownloads = 1 // Force queueing
	cfg.VideoSettings.CompatibilityMode = false
	db := testutils.TestDatabase(t)
	dm := NewDownloadManager(cfg, db)

	// First download - blocks the semaphore
	mock1 := &testutils.MockDownloader{
		ShouldBlock: true,
		Title:       "Blocking Movie",
		Files:       []string{"/tmp/blocking_imm.mp4"},
	}

	movie1ID, _, _, err := dm.StartDownload(mock1, 100)
	_ = movie1ID // Suppress unused warning; we only care that it starts
	if err != nil {
		t.Fatalf("StartDownload for blocking movie failed: %v", err)
	}

	// Second download - should be queued
	mock2 := &testutils.MockDownloader{
		ShouldBlock: true, // Will block when started
		Title:       "Queued Movie",
		Files:       []string{"/tmp/queued_imm.mp4"},
	}

	_, progressChan2, errChan2, err := dm.StartDownload(mock2, 200)
	if err != nil {
		t.Fatalf("StartDownload for queued movie failed: %v", err)
	}

	// Verify second download is queued
	time.Sleep(50 * time.Millisecond)
	queueCount := dm.GetQueueCount()
	if queueCount != 1 {
		t.Fatalf("Expected 1 queued download, got %d", queueCount)
	}

	// Wait a bit and check that channels have NOT been closed yet
	// This is the key test - previously the channels would close immediately
	time.Sleep(200 * time.Millisecond)

	select {
	case _, ok := <-progressChan2:
		if !ok {
			t.Error("progressChan2 was closed while download is still in queue - this is the bug!")
		}
		// If we received progress, that's fine (though unexpected while in queue)
	default:
		// No data on channel - expected behavior
	}

	select {
	case err, ok := <-errChan2:
		if !ok {
			t.Error("errChan2 was closed while download is still in queue - this is the bug!")
		} else if err != nil {
			t.Errorf("Received unexpected error while in queue: %v", err)
		} else {
			t.Error("Received nil error (completion) while download is still in queue - this is the bug!")
		}
	default:
		// No data on channel - expected behavior
		t.Log("Channels correctly remain open while download is in queue")
	}

	// Verify download is still in queue
	if dm.GetQueueCount() != 1 {
		t.Errorf("Download should still be in queue, but queue count is %d", dm.GetQueueCount())
	}
}

// TestMultipleDownloadsWithSeries verifies that when downloading a series (multi-file torrent),
// other downloads are not blocked and can run concurrently.
func TestMultipleDownloadsWithSeries(t *testing.T) {
	cfg := testutils.TestConfig("/tmp")
	cfg.DownloadSettings.MaxConcurrentDownloads = 3
	cfg.VideoSettings.CompatibilityMode = false
	db := testutils.TestDatabase(t)
	dm := NewDownloadManager(cfg, db)

	// Series download (multi-episode)
	seriesMock := &testutils.MockDownloader{
		ShouldBlock:  true,
		Title:        "TV Series S01",
		Files:        []string{"/tmp/series/S01E01.mp4", "/tmp/series/S01E02.mp4"},
		TotalEps:     2,
		EpisodesChan: make(chan int, 2),
	}

	// Regular movie
	movieMock := &testutils.MockDownloader{
		ShouldBlock: true,
		Title:       "Regular Movie",
		Files:       []string{"/tmp/movie.mp4"},
	}

	// Start series download
	seriesID, _, _, err := dm.StartDownload(seriesMock, 100)
	if err != nil {
		t.Fatalf("StartDownload for series failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Start movie download - should also be active (not queued)
	movieID, _, _, err := dm.StartDownload(movieMock, 200)
	if err != nil {
		t.Fatalf("StartDownload for movie failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Both should be active
	activeCount := dm.GetDownloadCount()
	queueCount := dm.GetQueueCount()

	if activeCount != 2 {
		t.Errorf("Expected 2 active downloads (series + movie), got %d (queue: %d)", activeCount, queueCount)
	}
	if queueCount != 0 {
		t.Errorf("Expected 0 queued downloads, got %d", queueCount)
	}

	// Verify both are in active downloads
	activeDownloads := dm.GetActiveDownloads()
	foundSeries, foundMovie := false, false
	for _, id := range activeDownloads {
		if id == seriesID {
			foundSeries = true
		}
		if id == movieID {
			foundMovie = true
		}
	}
	if !foundSeries || !foundMovie {
		t.Errorf("Expected both series and movie to be active, foundSeries=%v, foundMovie=%v",
			foundSeries, foundMovie)
	}

	// Cleanup
	_ = dm.StopDownload(seriesID)
	_ = dm.StopDownload(movieID)
}

// assertChannelsOpen checks that both progress and error channels have not been closed or received data.
func assertChannelsOpen(t *testing.T, progressChan chan float64, errChan chan error) {
	t.Helper()

	select {
	case _, ok := <-progressChan:
		if !ok {
			t.Fatal("progressChan closed while movie is still in queue")
		}
	default:
	}

	select {
	case e, ok := <-errChan:
		if !ok {
			t.Fatal("errChan closed while movie is still in queue")
		}
		t.Fatalf("Unexpected value on errChan while in queue: %v", e)
	default:
	}
}

// waitForCompletion drains progressChan/errChan and returns when the download completes or times out.
func waitForCompletion(t *testing.T, progressChan chan float64, errChan chan error, dm *DownloadManager) {
	t.Helper()

	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-progressChan:
			if !ok {
				progressChan = nil
			}
		case e, ok := <-errChan:
			if !ok || e == nil {
				return
			}
			t.Fatalf("Queued movie failed with error: %v", e)
		case <-timeout:
			t.Fatalf(
				"Queued movie did not complete after series finished. active=%d queue=%d",
				dm.GetDownloadCount(), dm.GetQueueCount(),
			)
		}
	}
}

// TestSeriesCompletionUnblocksQueuedDownload verifies that when a series occupies
// the only semaphore slot and completes, the queued download starts and completes
// properly through the original caller's channels.
// This is the key scenario for bug #2: "if a series is downloading first, other downloads don't start".
func TestSeriesCompletionUnblocksQueuedDownload(t *testing.T) {
	cfg := testutils.TestConfig("/tmp")
	cfg.DownloadSettings.MaxConcurrentDownloads = 1
	cfg.VideoSettings.CompatibilityMode = false
	db := testutils.TestDatabase(t)
	dm := NewDownloadManager(cfg, db)

	// Series download occupies the single slot
	seriesMock := &testutils.MockDownloader{
		ShouldBlock:  true,
		Title:        "TV Series S01",
		Files:        []string{"/tmp/series_q/S01E01.mp4", "/tmp/series_q/S01E02.mp4"},
		TotalEps:     2,
		EpisodesChan: make(chan int, 2),
	}

	seriesID, _, _, err := dm.StartDownload(seriesMock, 100)
	if err != nil {
		t.Fatalf("StartDownload for series failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if dm.GetDownloadCount() != 1 {
		t.Fatalf("Expected 1 active download (series), got %d", dm.GetDownloadCount())
	}

	// Movie goes to queue because slot is occupied by series
	movieMock := &testutils.MockDownloader{
		ShouldBlock: false,
		Title:       "Queued After Series",
		Files:       []string{"/tmp/queued_after_series.mp4"},
	}

	movieID, progressChan, errChan, err := dm.StartDownload(movieMock, 200)
	if err != nil {
		t.Fatalf("StartDownload for movie failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if dm.GetQueueCount() != 1 {
		t.Fatalf("Expected 1 queued download, got %d", dm.GetQueueCount())
	}

	assertChannelsOpen(t, progressChan, errChan)

	// Stop the series — this releases the semaphore
	_ = dm.StopDownload(seriesID)

	waitForCompletion(t, progressChan, errChan, dm)

	t.Logf("Movie completed successfully after series released the slot")

	// After everything completes, there should be no active or queued downloads
	time.Sleep(100 * time.Millisecond)
	if dm.GetDownloadCount() != 0 {
		t.Errorf("Expected 0 active downloads, got %d", dm.GetDownloadCount())
	}
	if dm.GetQueueCount() != 0 {
		t.Errorf("Expected 0 queued downloads, got %d", dm.GetQueueCount())
	}

	_ = dm.StopDownload(movieID)
}

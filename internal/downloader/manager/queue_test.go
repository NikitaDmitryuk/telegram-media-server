package manager

import (
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/notifier"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

func newQueueTestManager(t *testing.T) *DownloadManager {
	t.Helper()
	cfg := testutils.TestConfig("/tmp")
	cfg.DownloadSettings.MaxConcurrentDownloads = 2
	cfg.VideoSettings.CompatibilityMode = false
	db := testutils.TestDatabase(t)

	// Build the manager manually without starting the background processQueue
	// goroutine, because tests add queuedDownload items with nil downloader.
	return &DownloadManager{
		jobs:             make(map[uint]*downloadJob),
		queue:            make([]queuedDownload, 0),
		semaphore:        make(chan struct{}, cfg.GetDownloadSettings().MaxConcurrentDownloads),
		downloadSettings: cfg.GetDownloadSettings(),
		db:               db,
		cfg:              cfg,
		conversionQueue:  make(chan conversionJob, ConversionQueueSize),
	}
}

func TestCalculateEstimatedWaitTime(t *testing.T) {
	dm := newQueueTestManager(t)

	tests := []struct {
		name     string
		position int
		expected string
	}{
		{"Position zero", 0, "Starting soon"},
		{"Position negative", -1, "Starting soon"},
		{"Position 1 with 2 concurrent", 1, "~15 minutes"},
		{"Position 2 with 2 concurrent", 2, "~30 minutes"},
		{"Position 4 with 2 concurrent", 4, "~1 hours"},
		{"Position 5 with 2 concurrent", 5, "~1 hours 15 minutes"},
		{"Position 8 with 2 concurrent", 8, "~2 hours"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dm.calculateEstimatedWaitTime(tt.position)
			if result != tt.expected {
				t.Errorf("calculateEstimatedWaitTime(%d) = %q, want %q", tt.position, result, tt.expected)
			}
		})
	}
}

func TestGetQueueCount_Empty(t *testing.T) {
	dm := newQueueTestManager(t)

	if count := dm.GetQueueCount(); count != 0 {
		t.Errorf("GetQueueCount() = %d, want 0 for empty queue", count)
	}
}

func TestGetQueueCount_WithItems(t *testing.T) {
	dm := newQueueTestManager(t)

	dm.queueMutex.Lock()
	dm.queue = append(dm.queue,
		queuedDownload{movieID: 1, title: "Movie 1", addedAt: time.Now(), queueNotifier: notifier.Noop},
		queuedDownload{movieID: 2, title: "Movie 2", addedAt: time.Now(), queueNotifier: notifier.Noop},
		queuedDownload{movieID: 3, title: "Movie 3", addedAt: time.Now(), queueNotifier: notifier.Noop},
	)
	dm.queueMutex.Unlock()

	if count := dm.GetQueueCount(); count != 3 {
		t.Errorf("GetQueueCount() = %d, want 3", count)
	}
}

func TestGetQueueItems_Empty(t *testing.T) {
	dm := newQueueTestManager(t)

	items := dm.GetQueueItems()
	if len(items) != 0 {
		t.Errorf("GetQueueItems() returned %d items, want 0", len(items))
	}
}

func TestGetQueueItems_Content(t *testing.T) {
	dm := newQueueTestManager(t)

	now := time.Now()
	dm.queueMutex.Lock()
	dm.queue = append(dm.queue,
		queuedDownload{movieID: 10, title: "Alpha", addedAt: now, queueNotifier: notifier.Noop},
		queuedDownload{movieID: 20, title: "Beta", addedAt: now, queueNotifier: notifier.Noop},
	)
	dm.queueMutex.Unlock()

	items := dm.GetQueueItems()
	if len(items) != 2 {
		t.Fatalf("GetQueueItems() returned %d items, want 2", len(items))
	}

	if items[0]["movie_id"] != uint(10) {
		t.Errorf("First item movie_id = %v, want 10", items[0]["movie_id"])
	}
	if items[0]["title"] != "Alpha" {
		t.Errorf("First item title = %v, want 'Alpha'", items[0]["title"])
	}
	if items[0]["position"] != 1 {
		t.Errorf("First item position = %v, want 1", items[0]["position"])
	}
	if items[1]["movie_id"] != uint(20) {
		t.Errorf("Second item movie_id = %v, want 20", items[1]["movie_id"])
	}
	if items[1]["position"] != 2 {
		t.Errorf("Second item position = %v, want 2", items[1]["position"])
	}
}

func TestRemoveFromQueue_Exists(t *testing.T) {
	dm := newQueueTestManager(t)

	dm.queueMutex.Lock()
	dm.queue = append(dm.queue,
		queuedDownload{movieID: 1, title: "Movie 1", queueNotifier: notifier.Noop},
		queuedDownload{movieID: 2, title: "Movie 2", queueNotifier: notifier.Noop},
		queuedDownload{movieID: 3, title: "Movie 3", queueNotifier: notifier.Noop},
	)
	dm.queueMutex.Unlock()

	removed := dm.RemoveFromQueue(2)
	if !removed {
		t.Error("RemoveFromQueue(2) returned false, want true")
	}

	if count := dm.GetQueueCount(); count != 2 {
		t.Errorf("After removing, GetQueueCount() = %d, want 2", count)
	}

	// Verify the right item was removed.
	items := dm.GetQueueItems()
	for _, item := range items {
		if item["movie_id"] == uint(2) {
			t.Error("Movie 2 should have been removed from queue")
		}
	}
}

func TestRemoveFromQueue_NotExists(t *testing.T) {
	dm := newQueueTestManager(t)

	dm.queueMutex.Lock()
	dm.queue = append(dm.queue, queuedDownload{movieID: 1, title: "Movie 1", queueNotifier: notifier.Noop})
	dm.queueMutex.Unlock()

	removed := dm.RemoveFromQueue(999)
	if removed {
		t.Error("RemoveFromQueue(999) returned true for non-existent item")
	}

	if count := dm.GetQueueCount(); count != 1 {
		t.Errorf("Queue count should remain 1, got %d", count)
	}
}

func TestRemoveFromQueue_EmptyQueue(t *testing.T) {
	dm := newQueueTestManager(t)

	removed := dm.RemoveFromQueue(1)
	if removed {
		t.Error("RemoveFromQueue on empty queue should return false")
	}
}

func TestGetTotalDownloads(t *testing.T) {
	dm := newQueueTestManager(t)

	// No active downloads and no queue items.
	if total := dm.GetTotalDownloads(); total != 0 {
		t.Errorf("GetTotalDownloads() = %d, want 0", total)
	}

	// Add queue items.
	dm.queueMutex.Lock()
	dm.queue = append(dm.queue,
		queuedDownload{movieID: 1, queueNotifier: notifier.Noop},
		queuedDownload{movieID: 2, queueNotifier: notifier.Noop},
	)
	dm.queueMutex.Unlock()

	if total := dm.GetTotalDownloads(); total != 2 {
		t.Errorf("GetTotalDownloads() = %d, want 2 (0 active + 2 queued)", total)
	}
}

func TestGetDownloadCount(t *testing.T) {
	dm := newQueueTestManager(t)

	if count := dm.GetDownloadCount(); count != 0 {
		t.Errorf("GetDownloadCount() = %d, want 0", count)
	}
}

func TestGetActiveDownloads_Empty(t *testing.T) {
	dm := newQueueTestManager(t)

	downloads := dm.GetActiveDownloads()
	if len(downloads) != 0 {
		t.Errorf("GetActiveDownloads() returned %d items, want 0", len(downloads))
	}
}

func TestGetActiveDownloads_WithJobs(t *testing.T) {
	dm := newQueueTestManager(t)

	dm.mu.Lock()
	dm.jobs[10] = &downloadJob{title: "Movie A"}
	dm.jobs[20] = &downloadJob{title: "Movie B"}
	dm.mu.Unlock()

	downloads := dm.GetActiveDownloads()
	if len(downloads) != 2 {
		t.Fatalf("GetActiveDownloads() returned %d items, want 2", len(downloads))
	}

	found := map[uint]bool{}
	for _, id := range downloads {
		found[id] = true
	}
	if !found[10] || !found[20] {
		t.Errorf("Expected movie IDs 10 and 20, got %v", downloads)
	}
}

func TestGetDownloadCount_WithJobs(t *testing.T) {
	dm := newQueueTestManager(t)

	dm.mu.Lock()
	dm.jobs[1] = &downloadJob{}
	dm.jobs[2] = &downloadJob{}
	dm.jobs[3] = &downloadJob{}
	dm.mu.Unlock()

	if count := dm.GetDownloadCount(); count != 3 {
		t.Errorf("GetDownloadCount() = %d, want 3", count)
	}
}

func TestGetTotalDownloads_Mixed(t *testing.T) {
	dm := newQueueTestManager(t)

	// 2 active jobs.
	dm.mu.Lock()
	dm.jobs[1] = &downloadJob{}
	dm.jobs[2] = &downloadJob{}
	dm.mu.Unlock()

	// 3 queued.
	dm.queueMutex.Lock()
	dm.queue = append(dm.queue,
		queuedDownload{movieID: 10, queueNotifier: notifier.Noop},
		queuedDownload{movieID: 20, queueNotifier: notifier.Noop},
		queuedDownload{movieID: 30, queueNotifier: notifier.Noop},
	)
	dm.queueMutex.Unlock()

	if total := dm.GetTotalDownloads(); total != 5 {
		t.Errorf("GetTotalDownloads() = %d, want 5 (2 active + 3 queued)", total)
	}
}

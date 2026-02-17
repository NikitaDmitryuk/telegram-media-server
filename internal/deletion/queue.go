package deletion

import (
	"sync"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

const queueBufferSize = 100

// Queue enqueues movies for background stop and delete. Only logging is performed; no user notifications.
// IsPendingDeletion is true from Enqueue until the worker finishes (success or failure), so the movie can be hidden from the delete menu.
type Queue interface {
	Enqueue(movieID uint)
	IsPendingDeletion(movieID uint) bool
}

// queueImpl processes stop+delete in a single worker; success and failures are only logged.
type queueImpl struct {
	ch              chan uint
	moviePath       string
	db              database.Database
	downloadManager tmsdmanager.Service
	mu              sync.RWMutex
	pending         map[uint]struct{}
}

// NewQueue starts a background worker and returns a Queue that enqueues by movie ID.
func NewQueue(moviePath string, db database.Database, downloadManager tmsdmanager.Service) Queue {
	q := &queueImpl{
		ch:              make(chan uint, queueBufferSize),
		moviePath:       moviePath,
		db:              db,
		downloadManager: downloadManager,
		pending:         make(map[uint]struct{}),
	}
	go q.worker()
	return q
}

func (q *queueImpl) Enqueue(movieID uint) {
	q.mu.Lock()
	q.pending[movieID] = struct{}{}
	q.mu.Unlock()

	select {
	case q.ch <- movieID:
		logutils.Log.WithField("movie_id", movieID).Info("Movie enqueued for deletion")
	default:
		q.mu.Lock()
		delete(q.pending, movieID)
		q.mu.Unlock()
		logutils.Log.WithField("movie_id", movieID).Warn("Deletion queue full, movie not enqueued")
	}
}

func (q *queueImpl) IsPendingDeletion(movieID uint) bool {
	q.mu.RLock()
	_, ok := q.pending[movieID]
	q.mu.RUnlock()
	return ok
}

func (q *queueImpl) worker() {
	for movieID := range q.ch {
		if err := q.downloadManager.StopDownloadSilent(movieID); err != nil {
			logutils.Log.WithError(err).WithField("movie_id", movieID).Debug("Stop download for deletion (may already be completed)")
		}
		if err := filemanager.DeleteMovie(movieID, q.moviePath, q.db, q.downloadManager); err != nil {
			logutils.Log.WithError(err).WithField("movie_id", movieID).Error("Failed to delete movie in background")
		} else {
			logutils.Log.WithField("movie_id", movieID).Info("Movie deleted successfully in background")
		}
		q.mu.Lock()
		delete(q.pending, movieID)
		q.mu.Unlock()
	}
}

// NoopQueue is a Queue that does nothing. Use in tests when deletion is not under test.
type NoopQueue struct{}

func (NoopQueue) Enqueue(uint) {}

func (NoopQueue) IsPendingDeletion(uint) bool { return false }

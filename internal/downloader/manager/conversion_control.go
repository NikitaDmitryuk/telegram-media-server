package manager

import (
	"context"
	"sync"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

type conversionControl struct {
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	started bool // guarded by conversionMu
	once    sync.Once
	detach  func() bool
}

func (c *conversionControl) finish() {
	c.cancel()
	if c.detach != nil {
		c.detach()
	}
	c.once.Do(func() { close(c.done) })
}

func (dm *DownloadManager) finishConversion(movieID uint, c *conversionControl) {
	dm.conversionMu.Lock()
	if dm.conversions[movieID] == c {
		delete(dm.conversions, movieID)
	}
	dm.conversionMu.Unlock()
	c.finish()
}

func (dm *DownloadManager) allowConversion(movieID uint) {
	dm.conversionMu.Lock()
	delete(dm.blockedConversions, movieID)
	dm.conversionMu.Unlock()
}

// enqueueConversion registers queued and active work before exposing it to the
// worker. Context cancellation therefore also covers startup recovery jobs.
func (dm *DownloadManager) enqueueConversion(ctx context.Context, movieID uint, title string) (enqueued bool, done <-chan struct{}) {
	if !dm.cfg.VideoSettings.CompatibilityMode {
		return false, nil
	}
	dm.conversionMu.Lock()
	if dm.conversionsStopped || dm.blockedConversions[movieID] || ctx.Err() != nil {
		dm.conversionMu.Unlock()
		return false, nil
	}
	if current := dm.conversions[movieID]; current != nil {
		dm.conversionMu.Unlock()
		return true, current.done
	}
	if dm.conversions == nil {
		dm.conversions = make(map[uint]*conversionControl)
	}
	// #nosec G118 -- cancellation ownership transfers to conversionControl.finish and stop paths.
	jobCtx, cancel := context.WithCancel(ctx)
	c := &conversionControl{ctx: jobCtx, cancel: cancel, done: make(chan struct{})}
	dm.conversions[movieID] = c
	c.detach = context.AfterFunc(ctx, func() { dm.cancelSpecificConversion(movieID, c) })
	job := conversionJob{MovieID: movieID, Title: title, Done: c.done, control: c}
	select {
	case dm.conversionQueue <- job:
		dm.conversionMu.Unlock()
		logutils.Log.WithField("movie_id", movieID).Info("Movie enqueued for TV preparation")
		return true, c.done
	default:
		delete(dm.conversions, movieID)
		dm.conversionMu.Unlock()
		c.finish()
		_ = dm.db.UpdateConversionStatus(context.Background(), movieID, "failed")
		return false, nil
	}
}

func (dm *DownloadManager) cancelSpecificConversion(movieID uint, c *conversionControl) {
	dm.conversionMu.Lock()
	c.cancel()
	queued := !c.started
	if queued && dm.conversions[movieID] == c {
		delete(dm.conversions, movieID)
	}
	dm.conversionMu.Unlock()
	if queued {
		c.finish()
	}
}

// cancelConversionAndWait is the publication barrier used by deletion: a running
// converter cannot rename output after this method returns. No manager lock is
// held while waiting for the process and its cleanup to finish.
func (dm *DownloadManager) cancelConversionAndWait(movieID uint) {
	dm.conversionMu.Lock()
	if dm.blockedConversions == nil {
		dm.blockedConversions = make(map[uint]bool)
	}
	dm.blockedConversions[movieID] = true
	c := dm.conversions[movieID]
	dm.conversionMu.Unlock()
	if c != nil {
		dm.cancelSpecificConversion(movieID, c)
		<-c.done
	}
}

func (dm *DownloadManager) beginConversion(movieID uint, c *conversionControl) bool {
	dm.conversionMu.Lock()
	defer dm.conversionMu.Unlock()
	if c.ctx.Err() != nil || dm.blockedConversions[movieID] || dm.conversionsStopped {
		return false
	}
	c.started = true
	return true
}

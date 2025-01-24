package download_manager

import (
	"context"
	"sync"
)

type DownloadStatus int

const (
	InProgress DownloadStatus = iota
	Completed
	Cancelled
	Failed
)

type DownloadManager struct {
	mu   sync.RWMutex
	jobs map[int]*DownloadJob
}

type DownloadJob struct {
	MovieID    int
	Context    context.Context
	CancelFunc context.CancelFunc
	Status     DownloadStatus
	Error      error
}

func (dm *DownloadManager) StartDownload(movieID int, downloadFunc func(context.Context) error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	job := &DownloadJob{
		MovieID:    movieID,
		Context:    ctx,
		CancelFunc: cancel,
		Status:     InProgress,
	}
	dm.jobs[movieID] = job

	go func() {
		err := downloadFunc(ctx)
		dm.mu.Lock()
		defer dm.mu.Unlock()
		if err != nil {
			job.Status = Failed
			job.Error = err
		} else {
			job.Status = Completed
		}
		delete(dm.jobs, movieID)
	}()
}

func (dm *DownloadManager) StopDownload(movieID int) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if job, exists := dm.jobs[movieID]; exists {
		job.CancelFunc()
		job.Status = Cancelled
		delete(dm.jobs, movieID)
	}
}

func (dm *DownloadManager) StopAllDownloads() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	for movieID, job := range dm.jobs {
		job.CancelFunc()
		job.Status = Cancelled
		delete(dm.jobs, movieID)
	}
}

func NewDownloadManager() *DownloadManager {
	return &DownloadManager{
		jobs: make(map[int]*DownloadJob),
	}
}

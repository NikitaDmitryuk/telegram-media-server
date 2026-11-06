package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/notifier"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

const stopProbe = `{"streams":[{"codec_type":"video","codec_name":"h264","profile":"High","level":40,"width":1920,"height":1080,"pix_fmt":"yuv420p","field_order":"progressive","sample_aspect_ratio":"1:1","r_frame_rate":"90000/1","avg_frame_rate":"30000/1001"},{"codec_type":"audio","codec_name":"aac","profile":"LC","channels":2,"sample_rate":"44100"}],"format":{"format_name":"mov,mp4","duration":"1000"}}`

type conversionStopDownloader struct {
	testutils.MockDownloader
	manual atomic.Bool
}

func (d *conversionStopDownloader) StopDownload() error   { d.manual.Store(true); return nil }
func (d *conversionStopDownloader) StoppedManually() bool { return d.manual.Load() }

func blockingConversionManager(t *testing.T) (manager *DownloadManager, store database.Database, movieID uint, sourcePath, startedPath string) {
	t.Helper()
	root := t.TempDir()
	cfg := testutils.TestConfig(root)
	cfg.VideoSettings.CompatibilityMode = true
	db := testutils.TestDatabase(t)
	id, err := db.AddMovie(context.Background(), "downloaded", 0, []string{"movie.mp4"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "movie.mp4")
	if err := os.WriteFile(source, []byte("original media"), 0o600); err != nil {
		t.Fatal(err)
	}
	tools := t.TempDir()
	started := filepath.Join(root, "started")
	probe := `#!/bin/sh
case "$*" in *-show_packets*) printf '%s' '{"packets":[{"pts_time":"0"},{"pts_time":"0.033367"},{"pts_time":"0.066733"}]}' ;; *) printf '%s' "$TV_STOP_PROBE" ;; esac
`
	encoder := "#!/bin/sh\nprintf started > \"$TV_STOP_STARTED\"\nexec sleep 30\n"
	// #nosec G306 -- executable fake tool in t.TempDir.
	if err := os.WriteFile(filepath.Join(tools, "ffprobe"), []byte(probe), 0o700); err != nil {
		t.Fatal(err)
	}
	// #nosec G306 -- executable fake tool in t.TempDir.
	if err := os.WriteFile(filepath.Join(tools, "ffmpeg"), []byte(encoder), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools+":"+os.Getenv("PATH"))
	t.Setenv("TV_STOP_PROBE", stopProbe)
	t.Setenv("TV_STOP_STARTED", started)
	dm := &DownloadManager{cfg: cfg, db: db, jobs: make(map[uint]*downloadJob), semaphore: make(chan struct{}, 1),
		conversionQueue: make(chan conversionJob, 4), downloadSettings: cfg.DownloadSettings}
	return dm, db, id, source, started
}

func waitConversionCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("conversion condition timed out")
}

func assertStoppedSource(t *testing.T, source string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil || string(data) != "original media" {
		t.Fatalf("original lost: %q err=%v", data, err)
	}
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(source), ".tvcompat-*.mp4"))
	if err != nil || len(paths) != 0 {
		t.Fatalf("temporary conversion remains: %v err=%v", paths, err)
	}
}

//nolint:gocyclo // Verify independent cancellation, preservation, status, and deletion guarantees.
func TestStopDuringTVPreparationReleasesMonitor(t *testing.T) {
	for _, silent := range []bool{false, true} {
		t.Run(map[bool]string{false: "stop", true: "silent delete"}[silent], func(t *testing.T) {
			dm, db, id, source, started := blockingConversionManager(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			finished := make(chan error, 1)
			finished <- nil
			job := &downloadJob{downloader: &conversionStopDownloader{}, ctx: ctx, cancel: cancel, errChan: finished,
				queueNotifier: notifier.Noop, startTime: time.Now()}
			result := make(chan error, 1)
			dm.jobs[id] = job
			dm.semaphore <- struct{}{}
			go dm.runConversionWorker()
			go dm.monitorDownload(id, job, result)
			waitConversionCondition(t, func() bool { _, err := os.Stat(started); return err == nil })
			var err error
			if silent {
				err = dm.StopDownloadSilent(id)
			} else {
				err = dm.StopDownload(id)
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case err = <-result:
				expected := downloader.ErrStoppedByUser
				if silent {
					expected = downloader.ErrStoppedByDeletion
				}
				if !errors.Is(err, expected) {
					t.Fatalf("completion=%v want=%v", err, expected)
				}
			case <-time.After(time.Second):
				t.Fatal("monitor remained blocked after stop")
			}
			waitConversionCondition(t, func() bool { return len(dm.semaphore) == 0 })
			assertStoppedSource(t, source)
			movie, err := db.GetMovieByID(context.Background(), id)
			if err != nil || movie.ConversionStatus == "done" || movie.ConversionPercentage >= 100 {
				t.Fatalf("canceled conversion ready: %+v err=%v", movie, err)
			}
			if silent {
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
				if err := db.RemoveMovie(context.Background(), id); err != nil {
					t.Fatal(err)
				}
				if ok, _ := dm.EnqueueConversion(id, "deleted"); ok {
					t.Fatal("deleted conversion re-enqueued")
				}
				if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("deleted source resurrected")
				}
			}
			close(dm.conversionQueue)
		})
	}
}

func TestStopRecoveredTVConversionWithoutDownloadJob(t *testing.T) {
	for _, method := range []string{"stop", "stop all", "context"} {
		t.Run(method, func(t *testing.T) {
			dm, db, id, source, started := blockingConversionManager(t)
			if err := db.UpdateDownloadedPercentage(context.Background(), id, 100); err != nil {
				t.Fatal(err)
			}
			if err := db.UpdateConversionStatus(context.Background(), id, "pending"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go dm.runConversionWorker()
			dm.ResumePendingTVConversions(ctx)
			dm.conversionMu.Lock()
			control := dm.conversions[id]
			dm.conversionMu.Unlock()
			if control == nil {
				t.Fatal("startup conversion was not registered")
			}
			waitConversionCondition(t, func() bool { _, err := os.Stat(started); return err == nil })
			switch method {
			case "stop":
				if err := dm.StopDownloadSilent(id); err != nil {
					t.Fatal(err)
				}
			case "stop all":
				dm.StopAllDownloads()
			case "context":
				cancel()
			}
			select {
			case <-control.done:
			case <-time.After(time.Second):
				t.Fatal("recovery conversion ignored cancellation")
			}
			assertStoppedSource(t, source)
			if method != "context" {
				if ok, _ := dm.EnqueueConversion(id, "stopped"); ok {
					t.Fatal("stopped recovery work re-enqueued")
				}
			}
			close(dm.conversionQueue)
		})
	}
}

func TestQueuedTVConversionCancelsBeforeWorkerStarts(t *testing.T) {
	dm, _, id, source, started := blockingConversionManager(t)
	ok, done := dm.EnqueueConversion(id, "queued")
	if !ok {
		t.Fatal("conversion not queued")
	}
	if err := dm.StopDownloadSilent(id); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	default:
		t.Fatal("queued cancellation waited for the worker")
	}
	workerDone := make(chan struct{})
	close(dm.conversionQueue)
	go func() { dm.runConversionWorker(); close(workerDone) }()
	select {
	case <-workerDone:
	case <-time.After(time.Second):
		t.Fatal("canceled queued job blocked worker")
	}
	assertStoppedSource(t, source)
	if _, err := os.Stat(started); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canceled queued job started ffmpeg")
	}
}

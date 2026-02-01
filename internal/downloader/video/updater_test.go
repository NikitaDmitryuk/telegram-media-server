package ytdlp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

func TestMain(m *testing.M) {
	if logutils.Log == nil {
		logutils.InitLogger("error")
	}
	os.Exit(m.Run())
}

func TestStartPeriodicUpdater_ZeroInterval_ReturnsImmediately(t *testing.T) {
	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		StartPeriodicUpdater(ctx, 0)
		close(done)
	}()
	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("StartPeriodicUpdater(_, 0) did not return immediately")
	}
}

func TestStartPeriodicUpdater_NegativeInterval_ReturnsImmediately(t *testing.T) {
	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		StartPeriodicUpdater(ctx, -time.Hour)
		close(done)
	}()
	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("StartPeriodicUpdater(_, negative) did not return immediately")
	}
}

func TestStartPeriodicUpdater_StopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		StartPeriodicUpdater(ctx, 10*time.Millisecond)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("StartPeriodicUpdater did not stop after context cancel")
	}
}

func TestRunUpdate_WithCanceledContext_ReturnsWithoutPanic(_ *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	RunUpdate(ctx)
	// No panic, returns quickly (may log warning)
}

func TestRunUpdate_WithTimeoutContext_ReturnsWithoutPanic(_ *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	RunUpdate(ctx)
	// No panic; update will almost certainly not finish in 1ms
}

func TestRunUpdate_Success_WithFakeYtdlp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping fake yt-dlp script test on Windows")
	}
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "yt-dlp")
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		t.Fatalf("write fake yt-dlp: %v", err)
	}
	if err := os.Chmod(scriptPath, 0755); err != nil {
		t.Fatalf("chmod fake yt-dlp: %v", err)
	}
	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)
	os.Setenv("PATH", tmpDir+string(filepath.ListSeparator)+origPath)

	RunUpdate(context.Background())
	// No panic; fake script exits 0
}

func TestRunUpdate_ExitFailure_WithFakeYtdlp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping fake yt-dlp script test on Windows")
	}
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "yt-dlp")
	script := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		t.Fatalf("write fake yt-dlp: %v", err)
	}
	if err := os.Chmod(scriptPath, 0755); err != nil {
		t.Fatalf("chmod fake yt-dlp: %v", err)
	}
	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)
	os.Setenv("PATH", tmpDir+string(filepath.ListSeparator)+origPath)

	RunUpdate(context.Background())
	// No panic; RunUpdate logs failure but does not crash
}

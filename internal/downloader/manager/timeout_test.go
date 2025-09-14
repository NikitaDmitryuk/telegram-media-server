package manager

import (
	"context"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

func TestMonitorDownloadWithoutTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timeout test in short mode")
	}

	cfg := testutils.TestConfig("/tmp")
	// Устанавливаем таймаут в 0 (без таймаута)
	cfg.DownloadSettings.DownloadTimeout = 0
	cfg.DownloadSettings.ProgressUpdateInterval = 100 * time.Millisecond

	db := testutils.TestDatabase(t)

	manager := NewDownloadManager(cfg, db)

	// Создаем фейковый downloader, который не завершается
	fakeDownloader := &testutils.MockDownloader{
		ShouldBlock: true,
	}

	movieID := uint(1)
	progressChan := make(chan float64, 1)
	outerErrChan := make(chan error, 1)

	job := downloadJob{
		downloader:   fakeDownloader,
		startTime:    time.Now(),
		progressChan: progressChan,
		errChan:      make(chan error, 1),
		ctx:          context.Background(),
		cancel:       func() {},
	}

	// Запускаем мониторинг в горутине
	go manager.monitorDownload(movieID, &job, outerErrChan)

	// Проверяем, что за разумное время не происходит таймаут
	select {
	case err := <-outerErrChan:
		if err != nil && err.Error() == "download timeout after 0s" {
			t.Errorf("Unexpected timeout error when timeout is disabled: %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		// Ожидаемо - никакого таймаута не должно быть
	}
}

func TestMonitorDownloadWithTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timeout test in short mode")
	}

	cfg := testutils.TestConfig("/tmp")
	// Устанавливаем короткий таймаут
	cfg.DownloadSettings.DownloadTimeout = 200 * time.Millisecond
	cfg.DownloadSettings.ProgressUpdateInterval = 50 * time.Millisecond

	db := testutils.TestDatabase(t)

	manager := NewDownloadManager(cfg, db)

	// Создаем фейковый downloader, который будет блокироваться
	fakeDownloader := &testutils.MockDownloader{
		ShouldBlock: true,
	}

	movieID := uint(1)
	progressChan := make(chan float64, 1)
	outerErrChan := make(chan error, 1)

	job := downloadJob{
		downloader:   fakeDownloader,
		startTime:    time.Now(),
		progressChan: progressChan,
		errChan:      make(chan error, 1),
		ctx:          context.Background(),
		cancel:       func() {},
	}

	// Запускаем мониторинг
	go manager.monitorDownload(movieID, &job, outerErrChan)

	// Ожидаем таймаут
	select {
	case err := <-outerErrChan:
		if err == nil {
			t.Error("Expected timeout error, but got nil")
			return
		}
		expectedMsg := "download timeout after 200ms"
		if err.Error() != expectedMsg {
			t.Errorf("Expected timeout error message '%s', got '%s'", expectedMsg, err.Error())
		}
	case <-time.After(1 * time.Second):
		t.Error("Test timed out waiting for download timeout")
	}
}

func TestConfigurationTimeoutInheritance(t *testing.T) {
	// Проверяем, что конфигурация правильно передается в менеджер
	cfg := testutils.TestConfig("/tmp")
	cfg.DownloadSettings.DownloadTimeout = 42 * time.Minute

	db := testutils.TestDatabase(t)

	manager := NewDownloadManager(cfg, db)

	if manager.downloadSettings.DownloadTimeout != 42*time.Minute {
		t.Errorf("Expected timeout 42m, got %v", manager.downloadSettings.DownloadTimeout)
	}
}

func TestZeroTimeoutConfiguration(t *testing.T) {
	// Проверяем, что нулевой таймаут правильно обрабатывается
	cfg := testutils.TestConfig("/tmp")
	cfg.DownloadSettings.DownloadTimeout = 0

	db := testutils.TestDatabase(t)

	manager := NewDownloadManager(cfg, db)

	if manager.downloadSettings.DownloadTimeout != 0 {
		t.Errorf("Expected timeout 0 (no timeout), got %v", manager.downloadSettings.DownloadTimeout)
	}
}

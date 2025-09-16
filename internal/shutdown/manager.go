package shutdown

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
)

// Manager управляет graceful shutdown приложения
type Manager struct {
	services []domain.GracefulShutdownInterface
	timeout  time.Duration
	mu       sync.RWMutex
}

// NewManager создает новый менеджер shutdown
func NewManager(timeout time.Duration) *Manager {
	return &Manager{
		services: make([]domain.GracefulShutdownInterface, 0),
		timeout:  timeout,
	}
}

// Register регистрирует сервис для graceful shutdown
func (m *Manager) Register(service domain.GracefulShutdownInterface) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.services = append(m.services, service)
	logger.Log.WithField("service", service.Name()).Info("Service registered for graceful shutdown")
}

// WaitForShutdown ожидает сигнал завершения и выполняет graceful shutdown
func (m *Manager) WaitForShutdown() error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	sig := <-sigChan
	logger.Log.WithField("signal", sig.String()).Info("Received shutdown signal")

	return m.Shutdown()
}

// Shutdown выполняет graceful shutdown всех зарегистрированных сервисов
func (m *Manager) Shutdown() error {
	logger.Log.Info("Starting graceful shutdown")

	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	m.mu.RLock()
	services := make([]domain.GracefulShutdownInterface, len(m.services))
	copy(services, m.services)
	m.mu.RUnlock()

	// Канал для сбора ошибок
	errChan := make(chan error, len(services))
	var wg sync.WaitGroup

	// Запускаем shutdown всех сервисов параллельно
	for _, service := range services {
		wg.Add(1)
		go func(svc domain.GracefulShutdownInterface) {
			defer wg.Done()

			logger.Log.WithField("service", svc.Name()).Info("Shutting down service")

			if err := svc.Shutdown(ctx); err != nil {
				logger.Log.WithError(err).WithField("service", svc.Name()).Error("Error during service shutdown")
				errChan <- fmt.Errorf("service %s shutdown failed: %w", svc.Name(), err)
			} else {
				logger.Log.WithField("service", svc.Name()).Info("Service shutdown completed")
			}
		}(service)
	}

	// Ждем завершения всех сервисов или таймаута
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Log.Info("All services shutdown completed")
	case <-ctx.Done():
		logger.Log.Warn("Shutdown timeout exceeded, forcing shutdown")
		return fmt.Errorf("shutdown timeout exceeded")
	}

	// Собираем все ошибки
	close(errChan)
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		logger.Log.WithField("error_count", len(errors)).Error("Some services failed to shutdown gracefully")
		return fmt.Errorf("shutdown completed with %d errors", len(errors))
	}

	logger.Log.Info("Graceful shutdown completed successfully")
	return nil
}

// DatabaseShutdown реализует graceful shutdown для базы данных
type DatabaseShutdown struct {
	db domain.Database
}

// NewDatabaseShutdown создает новый shutdown handler для базы данных
func NewDatabaseShutdown(db domain.Database) *DatabaseShutdown {
	return &DatabaseShutdown{db: db}
}

// Shutdown выполняет shutdown базы данных
func (d *DatabaseShutdown) Shutdown(_ context.Context) error {
	if d.db != nil {
		if err := d.db.Close(); err != nil {
			logger.Log.WithError(err).Error("Failed to close database connections")
			return err
		}
	}
	logger.Log.Info("Database connections closed")
	return nil
}

// Name возвращает имя сервиса
func (*DatabaseShutdown) Name() string {
	return "database"
}

// DownloadManagerShutdown реализует graceful shutdown для менеджера загрузок
type DownloadManagerShutdown struct {
	dm domain.DownloadManagerInterface
}

// NewDownloadManagerShutdown создает новый shutdown handler для менеджера загрузок
func NewDownloadManagerShutdown(dm domain.DownloadManagerInterface) *DownloadManagerShutdown {
	return &DownloadManagerShutdown{dm: dm}
}

// Shutdown выполняет shutdown менеджера загрузок
func (d *DownloadManagerShutdown) Shutdown(_ context.Context) error {
	logger.Log.Info("Stopping all downloads")
	d.dm.StopAllDownloads()
	return nil
}

// Name возвращает имя сервиса
func (*DownloadManagerShutdown) Name() string {
	return "download_manager"
}

// HTTPServerShutdown реализует graceful shutdown для HTTP сервера
type HTTPServerShutdown struct {
	server HTTPServer
}

// HTTPServer интерфейс для HTTP сервера
type HTTPServer interface {
	Shutdown(ctx context.Context) error
}

// NewHTTPServerShutdown создает новый shutdown handler для HTTP сервера
func NewHTTPServerShutdown(server HTTPServer) *HTTPServerShutdown {
	return &HTTPServerShutdown{server: server}
}

// Shutdown выполняет shutdown HTTP сервера
func (h *HTTPServerShutdown) Shutdown(ctx context.Context) error {
	logger.Log.Info("Shutting down HTTP server")
	return h.server.Shutdown(ctx)
}

// Name возвращает имя сервиса
func (*HTTPServerShutdown) Name() string {
	return "http_server"
}

package services

import (
	"context"
	"path/filepath"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/errors"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/utils"
)

// DownloadService реализует бизнес-логику загрузок
type DownloadService struct {
	downloadManager     domain.DownloadManagerInterface
	downloaderFactory   domain.DownloaderFactory
	notificationService domain.NotificationService
	db                  database.Database
	cfg                 *domain.Config
	bot                 domain.BotInterface
}

// NewDownloadService создает новый сервис загрузок
func NewDownloadService(
	downloadManager domain.DownloadManagerInterface,
	downloaderFactory domain.DownloaderFactory,
	notificationService domain.NotificationService,
	db database.Database,
	cfg *domain.Config,
	bot domain.BotInterface,
) domain.DownloadServiceInterface {
	return &DownloadService{
		downloadManager:     downloadManager,
		downloaderFactory:   downloaderFactory,
		notificationService: notificationService,
		db:                  db,
		cfg:                 cfg,
		bot:                 bot,
	}
}

// HandleVideoLink обрабатывает ссылку на видео
func (s *DownloadService) HandleVideoLink(ctx context.Context, link string, chatID int64) error {
	// Валидация теперь происходит в фабрике

	logger.Log.WithFields(map[string]any{
		"link":    link,
		"chat_id": chatID,
	}).Info("Starting video download")

	// Создаем downloader через фабрику
	downloader, err := s.downloaderFactory.CreateVideoDownloader(ctx, link, s.cfg)
	if err != nil {
		return errors.WrapDomainError(err, errors.ErrorTypeValidation, "invalid_video_link", "failed to create video downloader")
	}

	movieID, progressChan, errChan, err := s.downloadManager.StartDownload(downloader, chatID)
	if err != nil {
		return errors.WrapDomainError(err, errors.ErrorTypeDownload, "start_download_failed", "failed to start video download").
			WithDetails(map[string]any{
				"link":    link,
				"chat_id": chatID,
			})
	}

	// Запускаем мониторинг загрузки в горутине (уведомление отправляется в monitorDownload)
	go s.monitorDownload(ctx, movieID, progressChan, errChan, chatID, "Video")

	logger.Log.WithFields(map[string]any{
		"movie_id": movieID,
		"link":     link,
		"chat_id":  chatID,
	}).Info("Video download started successfully")

	return nil
}

// HandleTorrentFile обрабатывает торрент файл
func (s *DownloadService) HandleTorrentFile(ctx context.Context, fileData []byte, fileName string, chatID int64) error {
	logger.Log.WithFields(map[string]any{
		"filename": fileName,
		"chat_id":  chatID,
		"size":     len(fileData),
	}).Info("Starting torrent download")

	// Сначала сохраняем торрент-файл на диск
	if err := s.bot.SaveFile(fileName, fileData); err != nil {
		return errors.WrapDomainError(err, errors.ErrorTypeValidation, "save_torrent_failed", "failed to save torrent file to disk").
			WithDetails(map[string]any{
				"filename": fileName,
				"chat_id":  chatID,
			})
	}

	// Создаем downloader через фабрику (теперь файл существует на диске)
	// Передаем полный путь к файлу для проверки существования
	fullPath := filepath.Join(s.cfg.MoviePath, fileName)
	downloader, err := s.downloaderFactory.CreateTorrentDownloader(ctx, fullPath, s.cfg.MoviePath, s.cfg)
	if err != nil {
		return errors.WrapDomainError(err, errors.ErrorTypeValidation, "invalid_torrent_file", "failed to create torrent downloader")
	}

	movieID, progressChan, errChan, err := s.downloadManager.StartDownload(downloader, chatID)
	if err != nil {
		return errors.WrapDomainError(err, errors.ErrorTypeDownload, "start_download_failed", "failed to start torrent download").
			WithDetails(map[string]any{
				"filename": fileName,
				"chat_id":  chatID,
			})
	}

	// Запускаем мониторинг загрузки в горутине (уведомление отправляется в monitorDownload)
	go s.monitorDownload(ctx, movieID, progressChan, errChan, chatID, fileName)

	logger.Log.WithFields(map[string]any{
		"movie_id": movieID,
		"filename": fileName,
		"chat_id":  chatID,
	}).Info("Torrent download started successfully")

	return nil
}

// GetDownloadStatus возвращает статус загрузки
func (s *DownloadService) GetDownloadStatus(ctx context.Context, movieID uint) (*domain.DownloadStatus, error) {
	movie, err := s.db.GetMovieByID(ctx, movieID)
	if err != nil {
		return nil, utils.WrapError(err, "failed to get movie from database", map[string]any{
			"movie_id": movieID,
		})
	}

	status := &domain.DownloadStatus{
		MovieID:   movie.ID,
		Title:     movie.Name,
		Progress:  float64(movie.DownloadedPercentage),
		FileSize:  movie.FileSize,
		StartTime: movie.CreatedAt,
	}

	// Определяем статус на основе процента загрузки
	const fullDownload = 100
	switch {
	case movie.DownloadedPercentage == fullDownload:
		status.Status = "completed"
	case movie.DownloadedPercentage > 0:
		status.Status = "downloading"
	default:
		status.Status = "pending"
	}

	return status, nil
}

// CancelDownload отменяет загрузку
func (s *DownloadService) CancelDownload(ctx context.Context, movieID uint) error {
	err := s.downloadManager.StopDownload(movieID)
	if err != nil {
		return utils.WrapError(err, "failed to stop download", map[string]any{
			"movie_id": movieID,
		})
	}

	// Удаляем запись из базы данных
	err = s.db.RemoveMovie(ctx, movieID)
	if err != nil {
		logger.Log.WithError(err).WithField("movie_id", movieID).Warn("Failed to remove movie from database after canceling download")
		// Не возвращаем ошибку, так как загрузка уже остановлена
	}

	logger.Log.WithField("movie_id", movieID).Info("Download canceled successfully")
	return nil
}

// monitorDownload мониторит процесс загрузки
func (s *DownloadService) monitorDownload(
	ctx context.Context,
	movieID uint,
	progressChan chan float64,
	errChan chan error,
	chatID int64,
	title string,
) {
	logger.Log.WithFields(map[string]any{
		"movie_id": movieID,
		"chat_id":  chatID,
		"title":    title,
	}).Info("Starting download monitoring")

	ticker := time.NewTicker(s.cfg.DownloadSettings.ProgressUpdateInterval)
	defer ticker.Stop()

	s.notifyDownloadStarted(ctx, chatID, title, movieID)

	for {
		select {
		case progress, ok := <-progressChan:
			if !ok {
				logger.Log.WithField("movie_id", movieID).Info("Progress channel closed - monitoring complete")
				s.handleProgressChannelClosed(movieID)
				return
			}
			logger.Log.WithFields(map[string]any{
				"movie_id": movieID,
				"progress": progress,
			}).Debug("Received progress update")
			s.handleProgressUpdate(ctx, movieID, progress, chatID, title)

		case err, ok := <-errChan:
			if !ok {
				logger.Log.WithField("movie_id", movieID).Info("Error channel closed - download completed")
				s.handleErrorChannelClosed(ctx, movieID, chatID, title)
				return
			}
			logger.Log.WithFields(map[string]any{
				"movie_id": movieID,
				"error":    err,
			}).Info("Received error result")
			s.handleDownloadResult(ctx, movieID, err, chatID, title)
			return

		case <-ctx.Done():
			logger.Log.WithField("movie_id", movieID).Info("Download monitoring canceled due to context cancellation")
			return

		case <-ticker.C:
			// Периодическая проверка статуса
			continue
		}
	}
}

// notifyDownloadStarted отправляет уведомление о начале загрузки
func (s *DownloadService) notifyDownloadStarted(ctx context.Context, chatID int64, title string, movieID uint) {
	logger.Log.WithFields(map[string]any{
		"movie_id": movieID,
		"chat_id":  chatID,
		"title":    title,
	}).Info("Sending download started notification")

	if err := s.notificationService.NotifyDownloadStarted(ctx, chatID, title); err != nil {
		logger.Log.WithError(err).WithField("movie_id", movieID).Warn("Failed to send download started notification")
	} else {
		logger.Log.WithField("movie_id", movieID).Info("Download started notification sent successfully")
	}
}

// handleProgressChannelClosed обрабатывает закрытие канала прогресса
func (*DownloadService) handleProgressChannelClosed(movieID uint) {
	logger.Log.WithField("movie_id", movieID).Debug("Progress channel closed")
}

// handleProgressUpdate обрабатывает обновление прогресса
func (s *DownloadService) handleProgressUpdate(ctx context.Context, movieID uint, progress float64, chatID int64, title string) {
	logger.Log.WithFields(map[string]any{
		"movie_id": movieID,
		"progress": progress,
		"chat_id":  chatID,
	}).Info("Handling progress update")

	if err := s.db.UpdateDownloadedPercentage(ctx, movieID, int(progress)); err != nil {
		logger.Log.WithError(err).WithField("movie_id", movieID).Error("Failed to update download progress")
	}

	if err := s.notificationService.NotifyDownloadProgress(ctx, chatID, title, int(progress)); err != nil {
		logger.Log.WithError(err).WithField("movie_id", movieID).Warn("Failed to send download progress notification")
	} else {
		logger.Log.WithFields(map[string]any{
			"movie_id": movieID,
			"progress": progress,
		}).Info("Progress notification sent successfully")
	}
}

// handleErrorChannelClosed обрабатывает закрытие канала ошибок
func (s *DownloadService) handleErrorChannelClosed(ctx context.Context, movieID uint, chatID int64, title string) {
	logger.Log.WithFields(map[string]any{
		"movie_id": movieID,
		"chat_id":  chatID,
		"title":    title,
	}).Info("Error channel closed - sending completion notification")

	if err := s.notificationService.NotifyDownloadCompleted(ctx, chatID, title); err != nil {
		logger.Log.WithError(err).WithField("movie_id", movieID).Warn("Failed to send download completed notification")
	} else {
		logger.Log.WithField("movie_id", movieID).Info("Download completed notification sent successfully")
	}
}

// handleDownloadResult обрабатывает результат загрузки
func (s *DownloadService) handleDownloadResult(ctx context.Context, movieID uint, err error, chatID int64, title string) {
	if err != nil {
		s.handleDownloadError(ctx, movieID, err, chatID, title)
	} else {
		s.handleDownloadSuccess(ctx, movieID, chatID, title)
	}
}

// handleDownloadError обрабатывает ошибку загрузки
func (s *DownloadService) handleDownloadError(ctx context.Context, movieID uint, err error, chatID int64, title string) {
	logger.Log.WithError(err).WithField("movie_id", movieID).Error("Download failed")
	if notifyErr := s.notificationService.NotifyDownloadFailed(ctx, chatID, title, err); notifyErr != nil {
		logger.Log.WithError(notifyErr).WithField("movie_id", movieID).Warn("Failed to send download failed notification")
	}
}

// handleDownloadSuccess обрабатывает успешную загрузку
func (s *DownloadService) handleDownloadSuccess(ctx context.Context, movieID uint, chatID int64, title string) {
	if err := s.db.SetLoaded(ctx, movieID); err != nil {
		logger.Log.WithError(err).WithField("movie_id", movieID).Error("Failed to mark movie as loaded")
	}
	logger.Log.WithField("movie_id", movieID).Info("Download completed successfully")
	if err := s.notificationService.NotifyDownloadCompleted(ctx, chatID, title); err != nil {
		logger.Log.WithError(err).WithField("movie_id", movieID).Warn("Failed to send download completed notification")
	}
}

package services

import (
	"context"
	"os"
	"path/filepath"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/utils"
)

// MovieService реализует бизнес-логику работы с фильмами
type MovieService struct {
	db              database.Database
	downloadManager domain.DownloadManagerInterface
	cfg             *domain.Config
}

// NewMovieService создает новый сервис фильмов
func NewMovieService(
	db database.Database,
	downloadManager domain.DownloadManagerInterface,
	cfg *domain.Config,
) domain.MovieServiceInterface {
	return &MovieService{
		db:              db,
		downloadManager: downloadManager,
		cfg:             cfg,
	}
}

// GetMovieList возвращает список фильмов
func (s *MovieService) GetMovieList(ctx context.Context) ([]database.Movie, error) {
	movies, err := s.db.GetMovieList(ctx)
	if err != nil {
		return nil, utils.WrapError(err, "failed to get movie list from database", nil)
	}

	logger.Log.WithField("count", len(movies)).Debug("Retrieved movie list")
	return movies, nil
}

// DeleteMovie удаляет фильм по ID
func (s *MovieService) DeleteMovie(ctx context.Context, movieID uint) error {
	// Проверяем существование фильма
	exists, err := s.db.MovieExistsId(ctx, movieID)
	if err != nil {
		return utils.WrapError(err, "failed to check movie existence", map[string]any{
			"movie_id": movieID,
		})
	}

	if !exists {
		return utils.NewAppError("movie_not_found", "movie not found", map[string]any{
			"movie_id": movieID,
		})
	}

	// Останавливаем загрузку если она активна
	err = s.downloadManager.StopDownload(movieID)
	if err != nil {
		logger.Log.WithError(err).WithField("movie_id", movieID).Warn("Failed to stop download, continuing with deletion")
	}

	// Получаем информацию о файлах перед удалением
	movie, err := s.db.GetMovieByID(ctx, movieID)
	if err != nil {
		return utils.WrapError(err, "failed to get movie details", map[string]any{
			"movie_id": movieID,
		})
	}

	// Удаляем файлы из файловой системы
	err = s.deleteMovieFiles(ctx, movieID)
	if err != nil {
		logger.Log.WithError(err).WithField("movie_id", movieID).Warn("Failed to delete movie files from filesystem")
		// Продолжаем удаление из базы данных даже если файлы не удалились
	}

	// Удаляем из базы данных
	err = s.db.RemoveMovie(ctx, movieID)
	if err != nil {
		return utils.WrapError(err, "failed to remove movie from database", map[string]any{
			"movie_id": movieID,
		})
	}

	logger.Log.WithFields(map[string]any{
		"movie_id": movieID,
		"title":    movie.Name,
	}).Info("Movie deleted successfully")

	return nil
}

// DeleteAllMovies удаляет все фильмы
func (s *MovieService) DeleteAllMovies(ctx context.Context) error {
	movies, err := s.db.GetMovieList(ctx)
	if err != nil {
		return utils.WrapError(err, "failed to get movie list for deletion", nil)
	}

	if len(movies) == 0 {
		logger.Log.Info("No movies to delete")
		return nil
	}

	// Останавливаем все активные загрузки
	s.downloadManager.StopAllDownloads()

	var deletionErrors []error
	deletedCount := 0

	for _, movie := range movies {
		err := s.DeleteMovie(ctx, movie.ID)
		if err != nil {
			logger.Log.WithError(err).WithField("movie_id", movie.ID).Error("Failed to delete movie")
			deletionErrors = append(deletionErrors, err)
		} else {
			deletedCount++
		}
	}

	logger.Log.WithFields(map[string]any{
		"total":   len(movies),
		"deleted": deletedCount,
		"errors":  len(deletionErrors),
	}).Info("Bulk movie deletion completed")

	if len(deletionErrors) > 0 {
		return utils.NewAppError("partial_deletion_failure", "some movies could not be deleted", map[string]any{
			"total_errors": len(deletionErrors),
			"deleted":      deletedCount,
			"total":        len(movies),
		})
	}

	return nil
}

// GetMovieByID возвращает фильм по ID
func (s *MovieService) GetMovieByID(ctx context.Context, movieID uint) (*database.Movie, error) {
	movie, err := s.db.GetMovieByID(ctx, movieID)
	if err != nil {
		return nil, utils.WrapError(err, "failed to get movie by ID", map[string]any{
			"movie_id": movieID,
		})
	}

	return &movie, nil
}

// deleteMovieFiles удаляет файлы фильма из файловой системы
func (s *MovieService) deleteMovieFiles(ctx context.Context, movieID uint) error {
	// Получаем основные файлы
	mainFiles, err := s.db.GetFilesByMovieID(ctx, movieID)
	if err != nil {
		return utils.WrapError(err, "failed to get main files", map[string]any{
			"movie_id": movieID,
		})
	}

	// Получаем временные файлы
	tempFiles, err := s.db.GetTempFilesByMovieID(ctx, movieID)
	if err != nil {
		return utils.WrapError(err, "failed to get temp files", map[string]any{
			"movie_id": movieID,
		})
	}

	// Удаляем основные файлы
	for _, file := range mainFiles {
		filePath := filepath.Join(s.cfg.MoviePath, file.FilePath)
		if deleteErr := s.deleteFile(filePath); deleteErr != nil {
			logger.Log.WithError(deleteErr).WithField("file_path", filePath).Warn("Failed to delete main file")
		}
	}

	// Удаляем временные файлы
	for _, file := range tempFiles {
		filePath := filepath.Join(s.cfg.MoviePath, file.FilePath)
		if deleteErr := s.deleteFile(filePath); deleteErr != nil {
			logger.Log.WithError(deleteErr).WithField("file_path", filePath).Warn("Failed to delete temp file")
		}
	}

	// Удаляем записи о файлах из базы данных
	err = s.db.RemoveFilesByMovieID(ctx, movieID)
	if err != nil {
		logger.Log.WithError(err).WithField("movie_id", movieID).Warn("Failed to remove main files from database")
	}

	err = s.db.RemoveTempFilesByMovieID(ctx, movieID)
	if err != nil {
		logger.Log.WithError(err).WithField("movie_id", movieID).Warn("Failed to remove temp files from database")
	}

	return nil
}

// deleteFile удаляет файл из файловой системы
func (*MovieService) deleteFile(filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		logger.Log.WithField("file_path", filePath).Debug("File does not exist, skipping deletion")
		return nil
	}

	err := os.Remove(filePath)
	if err != nil {
		return utils.WrapError(err, "failed to delete file", map[string]any{
			"file_path": filePath,
		})
	}

	logger.Log.WithField("file_path", filePath).Debug("File deleted successfully")
	return nil
}

// GetMovieStats возвращает статистику по фильмам
func (s *MovieService) GetMovieStats(ctx context.Context) (*MovieStats, error) {
	movies, err := s.db.GetMovieList(ctx)
	if err != nil {
		return nil, utils.WrapError(err, "failed to get movie list for stats", nil)
	}

	stats := &MovieStats{
		Total: len(movies),
	}

	var totalSize int64
	const fullDownload = 100
	for _, movie := range movies {
		totalSize += movie.FileSize

		switch movie.DownloadedPercentage {
		case fullDownload:
			stats.Completed++
		case 0:
			stats.Pending++
		default:
			stats.InProgress++
		}
	}

	stats.TotalSize = totalSize
	return stats, nil
}

// MovieStats представляет статистику по фильмам
type MovieStats struct {
	Total      int
	Completed  int
	InProgress int
	Pending    int
	TotalSize  int64
}

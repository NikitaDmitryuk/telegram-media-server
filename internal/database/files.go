package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

var errMainMovieFilePathConflict = errors.New("main movie file path changed or no longer exists")

const sqlitePragmaCleanupTimeout = 5 * time.Second

// ReplaceMainMovieFilePath updates one main file path only if it still matches
// the caller's expected path. It never creates or replaces file records.
func (s *SQLiteDatabase) ReplaceMainMovieFilePath(ctx context.Context, movieID, fileID uint, expectedPath, preparedPath string) error {
	if expectedPath == "" || preparedPath == "" {
		return errors.New("expected and prepared movie file paths must be non-empty")
	}

	return s.withRetry(ctx, "ReplaceMainMovieFilePath", func() error {
		return s.db.WithContext(ctx).Connection(func(connDB *gorm.DB) (operationErr error) {
			var previousSynchronous int
			if err := connDB.WithContext(ctx).Raw("PRAGMA synchronous").Row().Scan(&previousSynchronous); err != nil {
				return fmt.Errorf("read SQLite synchronous setting: %w", err)
			}
			if err := connDB.WithContext(ctx).Exec("PRAGMA synchronous = FULL").Error; err != nil {
				return fmt.Errorf("enable SQLite full synchronous mode: %w", err)
			}

			defer func() {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), sqlitePragmaCleanupTimeout)
				defer cancel()
				restoreErr := connDB.WithContext(cleanupCtx).Exec(fmt.Sprintf("PRAGMA synchronous = %d", previousSynchronous)).Error
				if restoreErr != nil {
					restoreErr = fmt.Errorf("restore SQLite synchronous setting: %w", restoreErr)
					if operationErr == nil {
						operationErr = restoreErr
					} else {
						operationErr = errors.Join(operationErr, restoreErr)
					}
				}
			}()

			operationErr = connDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				result := tx.Model(&MovieFile{}).
					Where("id = ? AND movie_id = ? AND file_path = ? AND temp_file = ?", fileID, movieID, expectedPath, false).
					Update("file_path", preparedPath)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return errMainMovieFilePathConflict
				}
				return nil
			})
			return operationErr
		})
	})
}

func addFilesWithDB(db *gorm.DB, movieID uint, files []string, isTemp bool) error {
	if len(files) == 0 {
		return nil
	}
	movieFiles := make([]MovieFile, 0, len(files))
	for _, file := range files {
		movieFiles = append(movieFiles, MovieFile{MovieID: movieID, FilePath: file, TempFile: isTemp})
	}
	return db.Create(&movieFiles).Error
}

func (s *SQLiteDatabase) GetFilesByMovieID(ctx context.Context, movieID uint) ([]MovieFile, error) {
	var files []MovieFile
	if err := s.withRetry(ctx, "GetFilesByMovieID", func() error {
		return s.db.WithContext(ctx).Where("movie_id = ? AND temp_file = ?", movieID, false).Find(&files).Error
	}); err != nil {
		return nil, err
	}
	return files, nil
}

func (s *SQLiteDatabase) RemoveFilesByMovieID(ctx context.Context, movieID uint) error {
	return s.withRetry(ctx, "RemoveFilesByMovieID", func() error {
		return s.db.WithContext(ctx).Where("movie_id = ? AND temp_file = ?", movieID, false).Delete(&MovieFile{}).Error
	})
}

// ReplaceMainMovieFiles replaces all main (non-temp) file records for the movie.
func (s *SQLiteDatabase) ReplaceMainMovieFiles(ctx context.Context, movieID uint, paths []string) error {
	return s.withRetry(ctx, "ReplaceMainMovieFiles", func() error {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("movie_id = ? AND temp_file = ?", movieID, false).Delete(&MovieFile{}).Error; err != nil {
				return err
			}
			return addFilesWithDB(tx, movieID, paths, false)
		})
	})
}

func (s *SQLiteDatabase) RemoveTempFilesByMovieID(ctx context.Context, movieID uint) error {
	return s.withRetry(ctx, "RemoveTempFilesByMovieID", func() error {
		return s.db.WithContext(ctx).Where("movie_id = ? AND temp_file = ?", movieID, true).Delete(&MovieFile{}).Error
	})
}

func (s *SQLiteDatabase) GetTempFilesByMovieID(ctx context.Context, movieID uint) ([]MovieFile, error) {
	var files []MovieFile
	if err := s.withRetry(ctx, "GetTempFilesByMovieID", func() error {
		return s.db.WithContext(ctx).Where("movie_id = ? AND temp_file = ?", movieID, true).Find(&files).Error
	}); err != nil {
		return nil, err
	}
	return files, nil
}

package database

import (
	"context"

	"gorm.io/gorm"
)

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

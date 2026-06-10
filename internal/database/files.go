package database

import (
	"context"

	"gorm.io/gorm"
)

func (s *SQLiteDatabase) addFiles(ctx context.Context, movieID uint, files []string, isTemp bool) error {
	for _, file := range files {
		movieFile := MovieFile{MovieID: movieID, FilePath: file, TempFile: isTemp}
		result := s.db.WithContext(ctx).Create(&movieFile)
		if result.Error != nil {
			return result.Error
		}
	}
	return nil
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
	result := s.db.WithContext(ctx).Where("movie_id = ? AND temp_file = ?", movieID, false).Delete(&MovieFile{})
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// ReplaceMainMovieFiles replaces all main (non-temp) file records for the movie.
func (s *SQLiteDatabase) ReplaceMainMovieFiles(ctx context.Context, movieID uint, paths []string) error {
	return s.withRetry(ctx, "ReplaceMainMovieFiles", func() error {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("movie_id = ? AND temp_file = ?", movieID, false).Delete(&MovieFile{}).Error; err != nil {
				return err
			}
			for _, file := range paths {
				movieFile := MovieFile{MovieID: movieID, FilePath: file, TempFile: false}
				if err := tx.Create(&movieFile).Error; err != nil {
					return err
				}
			}
			return nil
		})
	})
}

func (s *SQLiteDatabase) RemoveTempFilesByMovieID(ctx context.Context, movieID uint) error {
	result := s.db.WithContext(ctx).Where("movie_id = ? AND temp_file = ?", movieID, true).Delete(&MovieFile{})
	if result.Error != nil {
		return result.Error
	}
	return nil
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

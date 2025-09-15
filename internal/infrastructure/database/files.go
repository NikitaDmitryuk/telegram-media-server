package database

import (
	"context"
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
	result := s.db.WithContext(ctx).Where("movie_id = ? AND temp_file = ?", movieID, false).Find(&files)
	if result.Error != nil {
		return nil, result.Error
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

func (s *SQLiteDatabase) RemoveTempFilesByMovieID(ctx context.Context, movieID uint) error {
	result := s.db.WithContext(ctx).Where("movie_id = ? AND temp_file = ?", movieID, true).Delete(&MovieFile{})
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *SQLiteDatabase) GetTempFilesByMovieID(ctx context.Context, movieID uint) ([]MovieFile, error) {
	var files []MovieFile
	result := s.db.WithContext(ctx).Where("movie_id = ? AND temp_file = ?", movieID, true).Find(&files)
	if result.Error != nil {
		return nil, result.Error
	}
	return files, nil
}

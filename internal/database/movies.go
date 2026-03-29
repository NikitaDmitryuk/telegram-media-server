package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func (s *SQLiteDatabase) AddMovie(
	ctx context.Context,
	name string,
	fileSize int64,
	mainFiles, tempFiles []string,
	totalEpisodes int,
) (uint, error) {
	movie := Movie{
		Name:                 name,
		FileSize:             fileSize,
		DownloadedPercentage: 0,
		TotalEpisodes:        totalEpisodes,
	}

	result := s.db.WithContext(ctx).Create(&movie)
	if result.Error != nil {
		return 0, result.Error
	}

	if err := s.addFiles(ctx, movie.ID, mainFiles, false); err != nil {
		return 0, err
	}

	if err := s.addFiles(ctx, movie.ID, tempFiles, true); err != nil {
		return 0, err
	}

	return movie.ID, nil
}

func (s *SQLiteDatabase) UpdateMovieName(ctx context.Context, movieID uint, name string) error {
	result := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", movieID).Update("name", name)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *SQLiteDatabase) UpdateMovieFileSize(ctx context.Context, movieID uint, size int64) error {
	result := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", movieID).Update("file_size", size)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *SQLiteDatabase) UpdateMovieTotalEpisodes(ctx context.Context, movieID uint, total int) error {
	result := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", movieID).Update("total_episodes", total)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *SQLiteDatabase) RemoveMovie(ctx context.Context, movieID uint) error {
	result := s.db.WithContext(ctx).Delete(&Movie{}, movieID)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *SQLiteDatabase) GetMovieList(ctx context.Context) ([]Movie, error) {
	var movies []Movie
	result := s.db.WithContext(ctx).Find(&movies)
	if result.Error != nil {
		return nil, result.Error
	}
	return movies, nil
}

func (s *SQLiteDatabase) UpdateDownloadedPercentage(ctx context.Context, movieID uint, percentage int) error {
	result := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", movieID).Update("downloaded_percentage", percentage)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *SQLiteDatabase) UpdateEpisodesProgress(ctx context.Context, movieID uint, completedEpisodes int) error {
	result := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", movieID).Update("completed_episodes", completedEpisodes)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *SQLiteDatabase) SetLoaded(ctx context.Context, movieID uint, movieRoot string) error {
	const completePercentage = 100
	updates := map[string]any{"downloaded_percentage": completePercentage}
	if sum, err := s.sumMainFilesSizeOnDisk(ctx, movieID, movieRoot); err == nil && sum > 0 {
		updates["file_size"] = sum
	}
	result := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", movieID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// RefreshMovieFileSizeFromDisk updates file_size from actual main files on disk (e.g. magnet had unknown size at add time).
func (s *SQLiteDatabase) RefreshMovieFileSizeFromDisk(ctx context.Context, movieID uint, movieRoot string) (int64, error) {
	sum, err := s.sumMainFilesSizeOnDisk(ctx, movieID, movieRoot)
	if err != nil || sum <= 0 {
		return sum, err
	}
	result := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", movieID).Update("file_size", sum)
	if result.Error != nil {
		return 0, result.Error
	}
	return sum, nil
}

func (s *SQLiteDatabase) sumMainFilesSizeOnDisk(ctx context.Context, movieID uint, movieRoot string) (int64, error) {
	if movieRoot == "" {
		return 0, nil
	}
	files, err := s.GetFilesByMovieID(ctx, movieID)
	if err != nil {
		return 0, err
	}
	var sum int64
	for i := range files {
		p := filepath.Join(movieRoot, files[i].FilePath)
		fi, statErr := os.Stat(p)
		if statErr != nil {
			continue
		}
		sum += fi.Size()
	}
	return sum, nil
}

func (s *SQLiteDatabase) UpdateConversionStatus(ctx context.Context, movieID uint, status string) error {
	result := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", movieID).Update("conversion_status", status)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *SQLiteDatabase) UpdateConversionPercentage(ctx context.Context, movieID uint, percentage int) error {
	result := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", movieID).Update("conversion_percentage", percentage)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *SQLiteDatabase) SetTvCompatibility(ctx context.Context, movieID uint, compat string) error {
	result := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", movieID).Update("tv_compatibility", compat)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *SQLiteDatabase) SetQBittorrentHash(ctx context.Context, movieID uint, hash string) error {
	result := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", movieID).Update("qbittorrent_hash", hash)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *SQLiteDatabase) GetMovieByID(ctx context.Context, movieID uint) (Movie, error) {
	var movie Movie
	result := s.db.WithContext(ctx).First(&movie, movieID)
	if result.Error != nil {
		return Movie{}, result.Error
	}
	return movie, nil
}

func (s *SQLiteDatabase) GetIncompleteQBittorrentDownloads(ctx context.Context) ([]Movie, error) {
	var movies []Movie
	result := s.db.WithContext(ctx).
		Where("qbittorrent_hash != '' AND downloaded_percentage < 100").
		Find(&movies)
	if result.Error != nil {
		return nil, result.Error
	}
	return movies, nil
}

func (s *SQLiteDatabase) MovieExistsFiles(ctx context.Context, files []string) (bool, error) {
	for _, file := range files {
		var count int64
		result := s.db.WithContext(ctx).Model(&MovieFile{}).Where("file_path = ?", file).Count(&count)
		if result.Error != nil {
			return false, result.Error
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

func (s *SQLiteDatabase) MovieExistsId(ctx context.Context, movieID uint) (bool, error) {
	var count int64
	result := s.db.WithContext(ctx).Model(&Movie{}).Where("id = ?", movieID).Count(&count)
	if result.Error != nil {
		return false, result.Error
	}
	return count > 0, nil
}

func (s *SQLiteDatabase) MovieExistsUploadedFile(ctx context.Context, fileName string) (bool, error) {
	var count int64
	result := s.db.WithContext(ctx).Model(&MovieFile{}).Where("file_path LIKE ?", fmt.Sprintf("%%%s", fileName)).Count(&count)
	if result.Error != nil {
		return false, result.Error
	}
	return count > 0, nil
}

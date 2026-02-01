package database

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAddMovie_WithTotalEpisodes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	if migErr := db.AutoMigrate(&Movie{}, &MovieFile{}); migErr != nil {
		t.Fatalf("Failed to migrate: %v", migErr)
	}

	s := &SQLiteDatabase{db: db}
	ctx := context.Background()

	const totalEpisodes = 8
	movieID, err := s.AddMovie(ctx, "Test Series", 1024, []string{"s01e01.mkv"}, []string{"series.torrent"}, totalEpisodes)
	if err != nil {
		t.Fatalf("AddMovie: %v", err)
	}
	if movieID == 0 {
		t.Fatal("AddMovie returned 0 ID")
	}

	list, err := s.GetMovieList(ctx)
	if err != nil {
		t.Fatalf("GetMovieList: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 movie, got %d", len(list))
	}
	m := list[0]
	if m.TotalEpisodes != totalEpisodes {
		t.Errorf("TotalEpisodes: want %d, got %d", totalEpisodes, m.TotalEpisodes)
	}
	if m.CompletedEpisodes != 0 {
		t.Errorf("CompletedEpisodes: want 0, got %d", m.CompletedEpisodes)
	}
}

func TestUpdateEpisodesProgress(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	if migErr := db.AutoMigrate(&Movie{}, &MovieFile{}); migErr != nil {
		t.Fatalf("Failed to migrate: %v", migErr)
	}

	s := &SQLiteDatabase{db: db}
	ctx := context.Background()

	movieID, err := s.AddMovie(ctx, "Series", 2048, []string{"e01.mkv"}, []string{"t.torrent"}, 5)
	if err != nil {
		t.Fatalf("AddMovie: %v", err)
	}

	if updateErr := s.UpdateEpisodesProgress(ctx, movieID, 2); updateErr != nil {
		t.Fatalf("UpdateEpisodesProgress: %v", updateErr)
	}

	m, err := s.GetMovieByID(ctx, movieID)
	if err != nil {
		t.Fatalf("GetMovieByID: %v", err)
	}
	if m.CompletedEpisodes != 2 {
		t.Errorf("CompletedEpisodes: want 2, got %d", m.CompletedEpisodes)
	}
}

func TestUpdateConversionStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	if migErr := db.AutoMigrate(&Movie{}, &MovieFile{}); migErr != nil {
		t.Fatalf("Failed to migrate: %v", migErr)
	}

	s := &SQLiteDatabase{db: db}
	ctx := context.Background()

	movieID, err := s.AddMovie(ctx, "Movie", 1024, []string{"movie.mkv"}, []string{"movie.torrent"}, 0)
	if err != nil {
		t.Fatalf("AddMovie: %v", err)
	}

	if updateErr := s.UpdateConversionStatus(ctx, movieID, "pending"); updateErr != nil {
		t.Fatalf("UpdateConversionStatus: %v", updateErr)
	}
	m, getErr := s.GetMovieByID(ctx, movieID)
	if getErr != nil {
		t.Fatalf("GetMovieByID: %v", getErr)
	}
	if m.ConversionStatus != "pending" {
		t.Errorf("ConversionStatus: want pending, got %q", m.ConversionStatus)
	}

	if updateErr := s.UpdateConversionStatus(ctx, movieID, "done"); updateErr != nil {
		t.Fatalf("UpdateConversionStatus done: %v", updateErr)
	}
	m, _ = s.GetMovieByID(ctx, movieID)
	if m.ConversionStatus != "done" {
		t.Errorf("ConversionStatus: want done, got %q", m.ConversionStatus)
	}
}

func TestUpdateConversionPercentage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	if migErr := db.AutoMigrate(&Movie{}, &MovieFile{}); migErr != nil {
		t.Fatalf("Failed to migrate: %v", migErr)
	}

	s := &SQLiteDatabase{db: db}
	ctx := context.Background()

	movieID, err := s.AddMovie(ctx, "Movie", 1024, []string{"movie.mkv"}, []string{"movie.torrent"}, 0)
	if err != nil {
		t.Fatalf("AddMovie: %v", err)
	}

	if updateErr := s.UpdateConversionPercentage(ctx, movieID, 50); updateErr != nil {
		t.Fatalf("UpdateConversionPercentage: %v", updateErr)
	}
	m, getErr := s.GetMovieByID(ctx, movieID)
	if getErr != nil {
		t.Fatalf("GetMovieByID: %v", getErr)
	}
	if m.ConversionPercentage != 50 {
		t.Errorf("ConversionPercentage: want 50, got %d", m.ConversionPercentage)
	}
}

func TestSetTvCompatibility(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	if migErr := db.AutoMigrate(&Movie{}, &MovieFile{}); migErr != nil {
		t.Fatalf("Failed to migrate: %v", migErr)
	}

	s := &SQLiteDatabase{db: db}
	ctx := context.Background()

	movieID, err := s.AddMovie(ctx, "Movie", 1024, []string{"movie.mkv"}, []string{"movie.torrent"}, 0)
	if err != nil {
		t.Fatalf("AddMovie: %v", err)
	}

	if setErr := s.SetTvCompatibility(ctx, movieID, "green"); setErr != nil {
		t.Fatalf("SetTvCompatibility: %v", setErr)
	}
	m, getErr := s.GetMovieByID(ctx, movieID)
	if getErr != nil {
		t.Fatalf("GetMovieByID: %v", getErr)
	}
	if m.TvCompatibility != "green" {
		t.Errorf("TvCompatibility: want green, got %q", m.TvCompatibility)
	}
}

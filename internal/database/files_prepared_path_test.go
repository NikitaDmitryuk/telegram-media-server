package database

import (
	"context"
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newPreparedPathTestDB(t *testing.T) (sqliteDB *SQLiteDatabase, movieID uint, files []MovieFile) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:prepared_path_"+t.Name()+"?mode=memory&cache=shared&_foreign_keys=on"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if migrateErr := db.AutoMigrate(&Movie{}, &MovieFile{}); migrateErr != nil {
		t.Fatalf("migrate: %v", migrateErr)
	}
	movie := Movie{Name: "test movie"}
	if createMovieErr := db.Create(&movie).Error; createMovieErr != nil {
		t.Fatalf("create movie: %v", createMovieErr)
	}
	fileRows := []MovieFile{
		{MovieID: movie.ID, FilePath: "/movies/main.mkv"},
		{MovieID: movie.ID, FilePath: "/movies/sibling.mkv"},
		{MovieID: movie.ID, FilePath: "/movies/temp.mkv", TempFile: true},
	}
	if createFilesErr := db.Create(&fileRows).Error; createFilesErr != nil {
		t.Fatalf("create files: %v", createFilesErr)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &SQLiteDatabase{db: db}, movie.ID, fileRows
}

func TestReplaceMainMovieFilePath(t *testing.T) {
	s, movieID, files := newPreparedPathTestDB(t)
	ctx := context.Background()
	const prepared = "/movies/prepared.mkv"
	if err := s.ReplaceMainMovieFilePath(ctx, movieID, files[0].ID, files[0].FilePath, prepared); err != nil {
		t.Fatalf("ReplaceMainMovieFilePath: %v", err)
	}

	var got []MovieFile
	if err := s.db.WithContext(ctx).Order("id").Find(&got).Error; err != nil {
		t.Fatalf("read files: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("file row count = %d, want 3", len(got))
	}
	wantPaths := []string{prepared, files[1].FilePath, files[2].FilePath}
	for i, file := range got {
		if file.FilePath != wantPaths[i] {
			t.Errorf("file %d path = %q, want %q", file.ID, file.FilePath, wantPaths[i])
		}
		if file.TempFile != files[i].TempFile {
			t.Errorf("file %d temp_file = %v, want %v", file.ID, file.TempFile, files[i].TempFile)
		}
	}
}

func TestReplaceMainMovieFilePathRejectsNonMatchingRows(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*testing.T, *SQLiteDatabase, uint, []MovieFile) (uint, uint, string)
	}{
		{name: "wrong expected path", setup: func(_ *testing.T, _ *SQLiteDatabase, movieID uint, files []MovieFile) (uint, uint, string) {
			return movieID, files[0].ID, "/stale/path.mkv"
		}},
		{name: "wrong movie", setup: func(_ *testing.T, _ *SQLiteDatabase, movieID uint, files []MovieFile) (uint, uint, string) {
			return movieID + 100, files[0].ID, files[0].FilePath
		}},
		{name: "wrong file id", setup: func(_ *testing.T, _ *SQLiteDatabase, movieID uint, files []MovieFile) (uint, uint, string) {
			return movieID, files[0].ID + 100, files[0].FilePath
		}},
		{name: "temp file", setup: func(_ *testing.T, _ *SQLiteDatabase, movieID uint, files []MovieFile) (uint, uint, string) {
			return movieID, files[2].ID, files[2].FilePath
		}},
		{name: "deleted movie", setup: func(t *testing.T, s *SQLiteDatabase, movieID uint, files []MovieFile) (uint, uint, string) {
			if err := s.db.Where("movie_id = ?", movieID).Delete(&MovieFile{}).Error; err != nil {
				t.Fatalf("delete movie files: %v", err)
			}
			if err := s.db.Delete(&Movie{}, movieID).Error; err != nil {
				t.Fatalf("delete movie: %v", err)
			}
			return movieID, files[0].ID, files[0].FilePath
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, movieID, files := newPreparedPathTestDB(t)
			targetMovie, fileID, expected := tc.setup(t, s, movieID, files)
			if err := s.ReplaceMainMovieFilePath(context.Background(), targetMovie, fileID, expected, "/movies/prepared.mkv"); err == nil {
				t.Fatal("expected compare-and-swap conflict")
			}
			var count int64
			if err := s.db.Model(&MovieFile{}).Count(&count).Error; err != nil {
				t.Fatalf("count files: %v", err)
			}
			if count != 3 && tc.name != "deleted movie" {
				t.Fatalf("file row count = %d, want 3", count)
			}
			if tc.name != "deleted movie" {
				var target MovieFile
				if err := s.db.First(&target, files[0].ID).Error; err != nil {
					t.Fatalf("read target: %v", err)
				}
				if target.FilePath != files[0].FilePath {
					t.Fatalf("target path = %q, want unchanged %q", target.FilePath, files[0].FilePath)
				}
			}
		})
	}
}

func TestReplaceMainMovieFilePathRejectsEmptyPaths(t *testing.T) {
	s, movieID, files := newPreparedPathTestDB(t)
	for _, paths := range [][2]string{{"", "/prepared"}, {"/expected", ""}} {
		if err := s.ReplaceMainMovieFilePath(context.Background(), movieID, files[0].ID, paths[0], paths[1]); err == nil {
			t.Errorf("paths %q -> %q: expected validation error", paths[0], paths[1])
		}
	}
}

func TestReplaceMainMovieFilePathHonorsCanceledContext(t *testing.T) {
	s, movieID, files := newPreparedPathTestDB(t)
	setSynchronous(t, s.db, "NORMAL")
	ctx, cancel := context.WithCancel(context.Background())
	if registerErr := s.db.Callback().Update().Before("gorm:update").Register("test:cancel_prepared_path_update", func(*gorm.DB) {
		cancel()
	}); registerErr != nil {
		t.Fatalf("register cancel callback: %v", registerErr)
	}
	err := s.ReplaceMainMovieFilePath(ctx, movieID, files[0].ID, files[0].FilePath, "/movies/prepared.mkv")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	var file MovieFile
	if err := s.db.First(&file, files[0].ID).Error; err != nil {
		t.Fatalf("read target: %v", err)
	}
	if file.FilePath != files[0].FilePath {
		t.Fatalf("path = %q, want unchanged %q", file.FilePath, files[0].FilePath)
	}
	assertSynchronous(t, s.db, 1)
}

func TestReplaceMainMovieFilePathUsesFullSynchronousAndRestoresSetting(t *testing.T) {
	s, movieID, files := newPreparedPathTestDB(t)
	setSynchronous(t, s.db, "NORMAL")
	if err := s.db.Exec("CREATE TABLE synchronous_observations (value INTEGER NOT NULL)").Error; err != nil {
		t.Fatalf("create observation table: %v", err)
	}
	if err := s.db.Exec(`CREATE TRIGGER observe_synchronous BEFORE UPDATE ON movie_files
		BEGIN INSERT INTO synchronous_observations(value) SELECT synchronous FROM pragma_synchronous; END`).Error; err != nil {
		t.Fatalf("create observation trigger: %v", err)
	}
	if err := s.ReplaceMainMovieFilePath(context.Background(), movieID, files[0].ID, files[0].FilePath, "/movies/prepared.mkv"); err != nil {
		t.Fatalf("ReplaceMainMovieFilePath: %v", err)
	}
	var observed int
	if err := s.db.Raw("SELECT value FROM synchronous_observations").Row().Scan(&observed); err != nil {
		t.Fatalf("read observed synchronous setting: %v", err)
	}
	if observed != 2 {
		t.Fatalf("synchronous during update = %d, want FULL (2)", observed)
	}
	assertSynchronous(t, s.db, 1)

	if err := s.ReplaceMainMovieFilePath(context.Background(), movieID, files[1].ID, "/stale/path", "/prepared.mkv"); err == nil {
		t.Fatal("expected compare-and-swap conflict")
	}
	assertSynchronous(t, s.db, 1)
}

func setSynchronous(t *testing.T, db *gorm.DB, setting string) {
	t.Helper()
	if err := db.Exec("PRAGMA synchronous = " + setting).Error; err != nil {
		t.Fatalf("set synchronous %s: %v", setting, err)
	}
}

func assertSynchronous(t *testing.T, db *gorm.DB, want int) {
	t.Helper()
	var got int
	if err := db.Raw("PRAGMA synchronous").Row().Scan(&got); err != nil {
		t.Fatalf("read synchronous setting: %v", err)
	}
	if got != want {
		t.Fatalf("synchronous setting = %d, want %d", got, want)
	}
}

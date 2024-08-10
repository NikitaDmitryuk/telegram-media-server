package main

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func initDB() {
	dbPath := filepath.Join(GlobalConfig.MoviePath, "movie.db")
	db, _ = sql.Open("sqlite3", dbPath)
	if _, err := os.Stat(dbPath); err != nil {
		createTables()
	}
}

func createTables() {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS Movie (
			ID INTEGER PRIMARY KEY AUTOINCREMENT,
			NAME TEXT NOT NULL,
			UPLOADED_FILE TEXT NOT NULL,
			TORRENT_FILE TEXT NOT NULL,
			DOWNLOADED_PERCENTAGE INTEGER NOT NULL DEFAULT 0 CHECK (DOWNLOADED_PERCENTAGE BETWEEN 0 AND 100),
			DOWNLOADED BOOLEAN NOT NULL DEFAULT 0 CHECK (DOWNLOADED IN (0, 1))
		);
		CREATE TABLE IF NOT EXISTS User (
			ID INTEGER PRIMARY KEY AUTOINCREMENT,
			NAME TEXT NOT NULL,
			CHAT_ID INTEGER NOT NULL
		);
	`)
	if err != nil {
		log.Fatal("Error creating tables:", err)
	}
}

func addMovie(name, uploadedFile, torrentFile string) {
	_, err := db.Exec(`
		INSERT INTO Movie (NAME, UPLOADED_FILE, TORRENT_FILE)
		VALUES (?, ?, ?)
	`, name, uploadedFile, torrentFile)
	if err != nil {
		log.Println("Error adding movie:", err)
	}
}

// Other functions (setLoaded, getMovieByID, removeMovie, getMovieList, login, updateDownloadedPercentage, checkUser) go here...

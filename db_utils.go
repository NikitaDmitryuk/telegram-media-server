package main

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"time"

	"fmt"
	_ "modernc.org/sqlite"
)

var db *sql.DB

type Movie struct {
	ID                   int
	Name                 string
	Downloaded           bool
	DownloadedPercentage int
	TorrentFile          sql.NullString
}

type MovieFile struct {
	ID       int
	MovieID  int
	FilePath string
}

func initDB() {
	dbPath := filepath.Join(GlobalConfig.MoviePath, "movie.db")
	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		createTables()
	}
}

func createTables() {
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS Movie (
            ID INTEGER PRIMARY KEY AUTOINCREMENT,
            NAME TEXT NOT NULL,
            TORRENT_FILE TEXT,
            DOWNLOADED_PERCENTAGE INTEGER NOT NULL DEFAULT 0 CHECK (DOWNLOADED_PERCENTAGE BETWEEN 0 AND 100),
            DOWNLOADED BOOLEAN NOT NULL DEFAULT 0 CHECK (DOWNLOADED IN (0, 1))
        );

        CREATE TABLE IF NOT EXISTS MovieFiles (
            ID INTEGER PRIMARY KEY AUTOINCREMENT,
            MOVIE_ID INTEGER NOT NULL,
            FILE_PATH TEXT NOT NULL,
            FOREIGN KEY (MOVIE_ID) REFERENCES Movie(ID) ON DELETE CASCADE
        );

        CREATE TABLE IF NOT EXISTS User (
            ID INTEGER PRIMARY KEY AUTOINCREMENT,
            NAME TEXT NOT NULL,
            CHAT_ID INTEGER NOT NULL
        );
    `)
	if err != nil {
		log.Fatalf("Error creating tables: %v", err)
	}
}

func addMovie(name string, torrentFile *string, filePaths []string) int {
	result, err := executeWithRetry(`
        INSERT INTO Movie (NAME, TORRENT_FILE)
        VALUES (?, ?)
    `, name, torrentFile)
	if err != nil {
		log.Println("Error adding movie:", err)
		return 0
	}

	movieID, err := result.LastInsertId()
	if err != nil {
		log.Println("Error getting last insert ID:", err)
		return 0
	}

	for _, filePath := range filePaths {
		_, err := executeWithRetry(`
            INSERT INTO MovieFiles (MOVIE_ID, FILE_PATH)
            VALUES (?, ?)
        `, movieID, filePath)
		if err != nil {
			log.Printf("Error adding movie file (%s): %v", filePath, err)
		}
	}

	return int(movieID)
}

func removeMovie(movieID int) error {
	_, err := executeWithRetry("DELETE FROM Movie WHERE ID = ?", movieID)
	if err != nil {
		log.Printf("Error removing movie: %v", err)
	}
	return err
}

func getMovieList() ([]Movie, error) {
	rows, err := db.Query("SELECT ID, NAME, DOWNLOADED, DOWNLOADED_PERCENTAGE, TORRENT_FILE FROM Movie ORDER BY ID")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var movies []Movie
	for rows.Next() {
		var movie Movie
		err := rows.Scan(&movie.ID, &movie.Name, &movie.Downloaded, &movie.DownloadedPercentage, &movie.TorrentFile)
		if err != nil {
			return nil, err
		}
		movies = append(movies, movie)
	}
	return movies, nil
}

func updateDownloadedPercentage(id int, percentage int) error {
	log.Printf("%s %d", id, percentage)
	_, err := executeWithRetry(`
        UPDATE Movie
        SET DOWNLOADED_PERCENTAGE = ?
        WHERE ID = ?
    `, percentage, id)
	if err != nil {
		log.Printf("Failed to update download percentage: %v", err)
	}
	return err
}

func setLoaded(id int) error {
	_, err := executeWithRetry(`
        UPDATE Movie
        SET DOWNLOADED = 1
        WHERE ID = ?
    `, id)
	if err != nil {
		log.Printf("Failed to set movie as downloaded: %v", err)
	}
	return err
}

func getMovieByID(movieID int) (Movie, error) {
	row := db.QueryRow(`
        SELECT ID, NAME, DOWNLOADED, DOWNLOADED_PERCENTAGE, TORRENT_FILE
        FROM Movie
        WHERE ID = ?
    `, movieID)

	var movie Movie
	err := row.Scan(&movie.ID, &movie.Name, &movie.Downloaded, &movie.DownloadedPercentage, &movie.TorrentFile)
	if err != nil {
		return Movie{}, err
	}
	return movie, nil
}

func movieExistsTorrent(torrentFileName string) (bool, error) {
	row := db.QueryRow("SELECT COUNT(*) FROM Movie WHERE TORRENT_FILE = ?", torrentFileName)
	var count int
	err := row.Scan(&count)
	if err != nil {
		log.Printf("Error checking if movie exists: %v", err)
		return false, err
	}
	return count > 0, nil
}

func movieExistsId(id int) (bool, error) {
	row := db.QueryRow("SELECT COUNT(*) FROM Movie WHERE ID = ?", id)
	var count int
	err := row.Scan(&count)
	if err != nil {
		log.Printf("Error checking if movie exists: %v", err)
		return false, err
	}
	return count > 0, nil
}

func executeWithRetry(query string, args ...interface{}) (sql.Result, error) {
	const maxRetries = 5
	var result sql.Result
	var err error
	for i := 0; i < maxRetries; i++ {
		result, err = db.Exec(query, args...)
		if err == nil {
			return result, nil
		}
		time.Sleep(time.Millisecond * 100)
	}
	return nil, fmt.Errorf("max retries reached: %v", err)
}

func getFilesByMovieID(movieID int) ([]MovieFile, error) {
	rows, err := db.Query("SELECT FILE_PATH FROM MovieFiles WHERE MOVIE_ID = ?", movieID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []MovieFile
	for rows.Next() {
		var file MovieFile
		err := rows.Scan(&file.FilePath)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func removeFilesByMovieID(movieID int) error {
	_, err := db.Exec("DELETE FROM MovieFiles WHERE MOVIE_ID = ?", movieID)
	return err
}

func movieExistsUploadedFile(fileName string) (bool, error) {
	row := db.QueryRow(`
        SELECT COUNT(*)
        FROM MovieFiles
        WHERE FILE_PATH = ?
    `, fileName)

	var count int
	err := row.Scan(&count)
	if err != nil {
		log.Printf("Ошибка при проверке существования файла %s: %v", fileName, err)
		return false, err
	}

	return count > 0, nil
}

func login(password string, chatID int64, userName string) (bool, error) {
	if password == GlobalConfig.Password {
		_, err := executeWithRetry("INSERT INTO User (NAME, CHAT_ID) VALUES (?, ?)", userName, chatID)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func checkUser(chatID int64) (bool, error) {
	stmt, err := db.Prepare("SELECT * FROM User WHERE CHAT_ID = ?")
	if err != nil {
		return false, err
	}
	rows, err := stmt.Query(chatID)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	return rows.Next(), nil
}

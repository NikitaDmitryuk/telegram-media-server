package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"

	_ "modernc.org/sqlite"
)

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

func DBInit(config *tmsconfig.Config) (*sql.DB, error) {
	dbPath := filepath.Join(config.MoviePath, "movie.db")
	var err error
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		if err := dbCreateTables(db); err != nil {
			return nil, fmt.Errorf("failed to create tables: %v", err)
		}
	}
	return db, nil
}

func dbCreateTables(db *sql.DB) error {
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
		return fmt.Errorf("error creating tables: %v", err)
	}
	return nil
}

func DbAddMovie(bot *tmsbot.Bot, name string, torrentFile *string, filePaths []string) int {
	result, err := dbExecuteWithRetry(bot, `
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
		_, err := dbExecuteWithRetry(bot, `
            INSERT INTO MovieFiles (MOVIE_ID, FILE_PATH)
            VALUES (?, ?)
        `, movieID, filePath)
		if err != nil {
			log.Printf("Error adding movie file (%s): %v", filePath, err)
		}
	}

	return int(movieID)
}

func DbRemoveMovie(bot *tmsbot.Bot, movieID int) error {
	_, err := dbExecuteWithRetry(bot, "DELETE FROM Movie WHERE ID = ?", movieID)
	if err != nil {
		log.Printf("Error removing movie: %v", err)
	}
	return err
}

func DbGetMovieList(bot *tmsbot.Bot) ([]Movie, error) {
	rows, err := bot.GetDB().Query("SELECT ID, NAME, DOWNLOADED, DOWNLOADED_PERCENTAGE, TORRENT_FILE FROM Movie ORDER BY ID")
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

func DbUpdateDownloadedPercentage(bot *tmsbot.Bot, id int, percentage int) {
	_, err := dbExecuteWithRetry(bot, `
        UPDATE Movie
        SET DOWNLOADED_PERCENTAGE = ?
        WHERE ID = ?
    `, percentage, id)
	if err != nil {
		log.Printf("Failed to update download percentage: %v", err)
	}
}

func DbSetLoaded(bot *tmsbot.Bot, id int) {
	_, err := dbExecuteWithRetry(bot, `
        UPDATE Movie
        SET DOWNLOADED = 1
        WHERE ID = ?
    `, id)
	if err != nil {
		log.Printf("Failed to set movie as downloaded: %v", err)
	}
}

func DbGetMovieByID(bot *tmsbot.Bot, movieID int) (Movie, error) {
	row := bot.GetDB().QueryRow(`
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

func DbMovieExistsTorrent(bot *tmsbot.Bot, torrentFileName string) (bool, error) {
	row := bot.GetDB().QueryRow("SELECT COUNT(*) FROM Movie WHERE TORRENT_FILE = ?", torrentFileName)
	var count int
	err := row.Scan(&count)
	if err != nil {
		log.Printf("Error checking if movie exists: %v", err)
		return false, err
	}
	return count > 0, nil
}

func DbMovieExistsId(bot *tmsbot.Bot, id int) (bool, error) {
	row := bot.GetDB().QueryRow("SELECT COUNT(*) FROM Movie WHERE ID = ?", id)
	var count int
	err := row.Scan(&count)
	if err != nil {
		log.Printf("Error checking if movie exists: %v", err)
		return false, err
	}
	return count > 0, nil
}

func dbExecuteWithRetry(bot *tmsbot.Bot, query string, args ...interface{}) (sql.Result, error) {
	const maxRetries = 5
	var result sql.Result
	var err error
	for i := 0; i < maxRetries; i++ {
		result, err = bot.GetDB().Exec(query, args...)
		if err == nil {
			return result, nil
		}
		time.Sleep(time.Millisecond * 100)
	}
	return nil, fmt.Errorf("max retries reached: %v", err)
}

func DbGetFilesByMovieID(bot *tmsbot.Bot, movieID int) ([]MovieFile, error) {
	rows, err := bot.GetDB().Query("SELECT FILE_PATH FROM MovieFiles WHERE MOVIE_ID = ?", movieID)
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

func DbRemoveFilesByMovieID(bot *tmsbot.Bot, movieID int) error {
	_, err := bot.GetDB().Exec("DELETE FROM MovieFiles WHERE MOVIE_ID = ?", movieID)
	return err
}

func DbMovieExistsUploadedFile(bot *tmsbot.Bot, fileName string) (bool, error) {
	row := bot.GetDB().QueryRow(`
        SELECT COUNT(*)
        FROM MovieFiles
        WHERE FILE_PATH = ?
    `, fileName)

	var count int
	err := row.Scan(&count)
	if err != nil {
		log.Printf("Error checking file existence %s: %v", fileName, err)
		return false, err
	}

	return count > 0, nil
}

func DbLogin(bot *tmsbot.Bot, password string, chatID int64, userName string) (bool, error) {
	if password == bot.GetConfig().Password {
		_, err := dbExecuteWithRetry(bot, "INSERT INTO User (NAME, CHAT_ID) VALUES (?, ?)", userName, chatID)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func DbCheckUser(bot *tmsbot.Bot, chatID int64) (bool, error) {
	stmt, err := bot.GetDB().Prepare("SELECT * FROM User WHERE CHAT_ID = ?")
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

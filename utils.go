package main

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
)

func downloadFile(fileID, fileName string) error {
	file, err := GlobalBot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		log.Printf("Failed to get file: %v", err)
		return err
	}

	fileURL := file.Link(GlobalBot.Token)
	resp, err := http.Get(fileURL)
	if err != nil {
		log.Printf("Failed to download file: %v", err)
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath.Join(GlobalConfig.MoviePath, fileName))
	if err != nil {
		log.Printf("Failed to create file: %v", err)
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Printf("Failed to save file: %v", err)
		return err
	}

	log.Println("File downloaded successfully")
	return nil
}

func hasEnoughSpace(path string, requiredSpace int64) bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		log.Printf("Error getting filesystem stats: %v", err)
		return false
	}
	availableSpace := stat.Bavail * uint64(stat.Bsize)

	log.Printf("Required space: %d bytes", requiredSpace)
	log.Printf("Available space: %d bytes", availableSpace)

	return availableSpace >= uint64(requiredSpace)
}

func deleteMovie(id int) error {
	movie, err := dbGetMovieByID(id)
	if err != nil {
		return logAndReturnError(messages[lang].Error.MovieNotFound, err)
	}

	files, err := dbGetFilesByMovieID(id)
	if err != nil {
		return logAndReturnError(messages[lang].Error.GetFilesError, err)
	}

	var rootFolder string

	for _, file := range files {
		filePath := filepath.Join(GlobalConfig.MoviePath, file.FilePath)

		if rootFolder == "" {
			rootFolder = filepath.Dir(filePath)
		}

		err := os.Remove(filePath)
		if err != nil {
			log.Printf("Failed to delete file %s: %v", filePath, err)
		} else {
			log.Printf("File %s deleted successfully", filePath)
		}
	}

	if movie.TorrentFile.Valid && movie.TorrentFile.String != "" {
		torrentFilePath := filepath.Join(GlobalConfig.MoviePath, movie.TorrentFile.String)
		err := os.Remove(torrentFilePath)
		if err != nil {
			log.Printf("Failed to delete torrent file %s: %v", torrentFilePath, err)
		} else {
			log.Printf("Torrent file %s deleted successfully", torrentFilePath)
		}
	}

	err = dbRemoveMovie(id)
	if err != nil {
		return logAndReturnError(messages[lang].Error.DeleteMovieDBError, err)
	}

	err = dbRemoveFilesByMovieID(id)
	if err != nil {
		return logAndReturnError(messages[lang].Error.DeleteFilesDBError, err)
	}

	if rootFolder != "" && rootFolder != GlobalConfig.MoviePath && isEmptyDirectory(rootFolder) {
		err = os.Remove(rootFolder)
		if err != nil {
			log.Printf("Failed to delete root folder %s: %v", rootFolder, err)
		} else {
			log.Printf("Root folder %s deleted successfully", rootFolder)
		}
	}

	log.Printf("Movie with ID %d and all associated files deleted successfully", id)
	return nil
}

func isEmptyDirectory(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("Failed to read directory %s: %v", dir, err)
		return false
	}

	return len(entries) == 0
}

func sanitizeFileName(name string) string {
	re := regexp.MustCompile(`[^а-яА-Яa-zA-Z0-9]+`)
	return re.ReplaceAllString(name, "_")
}

func logAndReturnError(message string, err error) error {
	log.Printf("%s: %v\n", message, err)
	return fmt.Errorf("%s: %v", message, err)
}

func isValidLink(text string) bool {
	parsedURL, err := url.ParseRequestURI(text)
	if err != nil {
		return false
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false
	}

	re := regexp.MustCompile(`^[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !re.MatchString(parsedURL.Host) {
		return false
	}

	return true
}

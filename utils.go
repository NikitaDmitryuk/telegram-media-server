package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func downloadFile(fileID, fileName string) error {
	file, err := GlobalBot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		log.Printf(GetMessage(FailedToGetFileMsgID), err)
		return err
	}

	fileURL := file.Link(GlobalBot.Token)
	resp, err := http.Get(fileURL)
	if err != nil {
		log.Printf(GetMessage(FailedToDownloadFileMsgID), err)
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath.Join(GlobalConfig.MoviePath, fileName))
	if err != nil {
		log.Printf(GetMessage(FailedToCreateFileMsgID), err)
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Printf(GetMessage(FailedToSaveFileMsgID), err)
		return err
	}

	log.Println(GetMessage(FileDownloadedSuccessfullyMsgID))
	return nil
}

func hasEnoughSpace(path string, requiredSpace int64) bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		log.Printf(GetMessage(ErrorGettingFilesystemStatsMsgID), err)
		return false
	}
	availableSpace := stat.Bavail * uint64(stat.Bsize)

	log.Print(GetMessage(RequiredSpaceMsgID, requiredSpace))
	log.Print(GetMessage(AvailableSpaceMsgID, availableSpace))

	return availableSpace >= uint64(requiredSpace)
}

func deleteMovie(id int) error {
	movie, err := dbGetMovieByID(id)
	if err != nil {
		return logAndReturnError(GetMessage(MovieNotFoundMsgID), err)
	}

	files, err := dbGetFilesByMovieID(id)
	if err != nil {
		return logAndReturnError(GetMessage(GetFilesErrorMsgID), err)
	}

	var rootFolder string

	for _, file := range files {
		filePath := filepath.Join(GlobalConfig.MoviePath, file.FilePath)

		if rootFolder == "" {
			rootFolder = filepath.Dir(filePath)
		}

		err := os.Remove(filePath)
		if err != nil {
			log.Print(GetMessage(FailedToDeleteFileMsgID, filePath, err))
		} else {
			log.Print(GetMessage(FileDeletedSuccessfullyMsgID, filePath))
		}
	}

	if movie.TorrentFile.Valid && movie.TorrentFile.String != "" {
		torrentFilePath := filepath.Join(GlobalConfig.MoviePath, movie.TorrentFile.String)
		err := os.Remove(torrentFilePath)
		if err != nil {
			log.Print(GetMessage(FailedToDeleteTorrentFileMsgID, torrentFilePath, err))
		} else {
			log.Print(GetMessage(TorrentFileDeletedSuccessfullyMsgID, torrentFilePath))
		}
	}

	err = dbRemoveMovie(id)
	if err != nil {
		return logAndReturnError(GetMessage(DeleteMovieDBErrorMsgID), err)
	}

	err = dbRemoveFilesByMovieID(id)
	if err != nil {
		return logAndReturnError(GetMessage(DeleteFilesDBErrorMsgID), err)
	}

	if rootFolder != "" && rootFolder != GlobalConfig.MoviePath && isEmptyDirectory(rootFolder) {
		err = os.Remove(rootFolder)
		if err != nil {
			log.Print(GetMessage(FailedToDeleteRootFolderMsgID, rootFolder, err))
		} else {
			log.Print(GetMessage(RootFolderDeletedSuccessfullyMsgID, rootFolder))
		}
	}

	log.Print(GetMessage(MovieDeletedSuccessfullyMsgID, id))
	return nil
}

func isEmptyDirectory(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Print(GetMessage(FailedToReadDirectoryMsgID, dir, err))
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
	return re.MatchString(parsedURL.Host)
}

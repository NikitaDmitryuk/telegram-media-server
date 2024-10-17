package utils

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

	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func DownloadFile(fileID, fileName string) error {
	file, err := GlobalBot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		log.Printf(tmslang.GetMessage(tmslang.FailedToGetFileMsgID), err)
		return err
	}

	fileURL := file.Link(GlobalBot.Token)
	resp, err := http.Get(fileURL)
	if err != nil {
		log.Printf(tmslang.GetMessage(tmslang.FailedToDownloadFileMsgID), err)
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath.Join(GlobalConfig.MoviePath, fileName))
	if err != nil {
		log.Printf(tmslang.GetMessage(tmslang.FailedToCreateFileMsgID), err)
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Printf(tmslang.GetMessage(tmslang.FailedToSaveFileMsgID), err)
		return err
	}

	log.Println(tmslang.GetMessage(tmslang.FileDownloadedSuccessfullyMsgID))
	return nil
}

func HasEnoughSpace(path string, requiredSpace int64) bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		log.Printf(tmslang.GetMessage(tmslang.ErrorGettingFilesystemStatsMsgID), err)
		return false
	}
	availableSpace := stat.Bavail * uint64(stat.Bsize)

	log.Print(tmslang.GetMessage(tmslang.RequiredSpaceMsgID, requiredSpace))
	log.Print(tmslang.GetMessage(tmslang.AvailableSpaceMsgID, availableSpace))

	return availableSpace >= uint64(requiredSpace)
}

func DeleteMovie(id int) error {
	movie, err := dbGetMovieByID(id)
	if err != nil {
		return LogAndReturnError(tmslang.GetMessage(tmslang.MovieNotFoundMsgID), err)
	}

	files, err := dbGetFilesByMovieID(id)
	if err != nil {
		return LogAndReturnError(tmslang.GetMessage(tmslang.GetFilesErrorMsgID), err)
	}

	var rootFolder string

	for _, file := range files {
		filePath := filepath.Join(GlobalConfig.MoviePath, file.FilePath)

		if rootFolder == "" {
			rootFolder = filepath.Dir(filePath)
		}

		err := os.Remove(filePath)
		if err != nil {
			log.Print(tmslang.GetMessage(tmslang.FailedToDeleteFileMsgID, filePath, err))
		} else {
			log.Print(tmslang.GetMessage(tmslang.FileDeletedSuccessfullyMsgID, filePath))
		}
	}

	if movie.TorrentFile.Valid && movie.TorrentFile.String != "" {
		torrentFilePath := filepath.Join(GlobalConfig.MoviePath, movie.TorrentFile.String)
		err := os.Remove(torrentFilePath)
		if err != nil {
			log.Print(tmslang.GetMessage(tmslang.FailedToDeleteTorrentFileMsgID, torrentFilePath, err))
		} else {
			log.Print(tmslang.GetMessage(tmslang.TorrentFileDeletedSuccessfullyMsgID, torrentFilePath))
		}
	}

	err = dbRemoveMovie(id)
	if err != nil {
		return LogAndReturnError(tmslang.GetMessage(tmslang.DeleteMovieDBErrorMsgID), err)
	}

	err = dbRemoveFilesByMovieID(id)
	if err != nil {
		return LogAndReturnError(tmslang.GetMessage(tmslang.DeleteFilesDBErrorMsgID), err)
	}

	if rootFolder != "" && rootFolder != GlobalConfig.MoviePath && IsEmptyDirectory(rootFolder) {
		err = os.Remove(rootFolder)
		if err != nil {
			log.Print(tmslang.GetMessage(tmslang.FailedToDeleteRootFolderMsgID, rootFolder, err))
		} else {
			log.Print(tmslang.GetMessage(tmslang.RootFolderDeletedSuccessfullyMsgID, rootFolder))
		}
	}

	log.Print(tmslang.GetMessage(tmslang.MovieDeletedSuccessfullyMsgID, id))
	return nil
}

func IsEmptyDirectory(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Print(tmslang.GetMessage(tmslang.FailedToReadDirectoryMsgID, dir, err))
		return false
	}

	return len(entries) == 0
}

func SanitizeFileName(name string) string {
	re := regexp.MustCompile(`[^а-яА-Яa-zA-Z0-9]+`)
	return re.ReplaceAllString(name, "_")
}

func LogAndReturnError(message string, err error) error {
	log.Printf("%s: %v\n", message, err)
	return fmt.Errorf("%s: %v", message, err)
}

func IsValidLink(text string) bool {
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

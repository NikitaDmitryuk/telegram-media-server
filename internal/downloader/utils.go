package downloader

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/db"
	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tmsutils "github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func DownloadFile(bot *tmsbot.Bot, fileID, fileName string) error {
	file, err := bot.GetAPI().GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		log.Printf("Failed to get file: %v", err)
		return err
	}

	fileURL := file.Link(bot.GetAPI().Token)
	resp, err := http.Get(fileURL)
	if err != nil {
		log.Printf("Failed to download file: %v", err)
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath.Join(bot.GetConfig().MoviePath, fileName))
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

func DeleteMovie(bot *tmsbot.Bot, id int) error {
	movie, err := tmsdb.DbGetMovieByID(bot, id)
	if err != nil {
		return tmsutils.LogAndReturnError(tmslang.GetMessage(tmslang.MovieNotFoundMsgID), err)
	}

	bot.DownloadManager.StopDownload(id)

	files, err := tmsdb.DbGetFilesByMovieID(bot, id)
	if err != nil {
		return tmsutils.LogAndReturnError(tmslang.GetMessage(tmslang.GetFilesErrorMsgID), err)
	}

	var rootFolder string

	for _, file := range files {
		filePath := filepath.Join(bot.GetConfig().MoviePath, file.FilePath)

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
		torrentFilePath := filepath.Join(bot.GetConfig().MoviePath, movie.TorrentFile.String)
		err := os.Remove(torrentFilePath)
		if err != nil {
			log.Printf("Failed to delete torrent file %s: %v", torrentFilePath, err)
		} else {
			log.Printf("Torrent file %s deleted successfully", torrentFilePath)
		}
	}

	err = tmsdb.DbRemoveMovie(bot, id)
	if err != nil {
		return tmsutils.LogAndReturnError(tmslang.GetMessage(tmslang.DeleteMovieDBErrorMsgID), err)
	}

	err = tmsdb.DbRemoveFilesByMovieID(bot, id)
	if err != nil {
		return tmsutils.LogAndReturnError(tmslang.GetMessage(tmslang.DeleteFilesDBErrorMsgID), err)
	}

	if rootFolder != "" && rootFolder != bot.GetConfig().MoviePath && tmsutils.IsEmptyDirectory(rootFolder) {
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

func shouldUseProxy(bot *tmsbot.Bot, rawURL string) (bool, error) {

	parsedURL, err := url.Parse(rawURL)
	if err != nil {

		return false, errors.New("invalid URL")
	}

	proxy := bot.GetConfig().Proxy
	if proxy == "" {
		return false, nil
	}

	targetHosts := bot.GetConfig().ProxyHost

	if targetHosts == "" {
		return true, nil
	}

	for _, host := range strings.Split(targetHosts, ",") {
		if parsedURL.Host == strings.TrimSpace(host) {
			return true, nil
		}
	}

	return false, nil
}

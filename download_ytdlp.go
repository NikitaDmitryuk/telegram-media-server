package main

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
)

func downloadVideo(update tgbotapi.Update) {
	url := update.Message.Text
	log.Println(messages[lang].Info.StartVideoDownload, url)

	videoTitle, err := getVideoTitle(url)
	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, fmt.Sprintf(messages[lang].Error.VideoTitleError, err.Error()))
		return
	}

	finalFileName := generateFileName(videoTitle)
	log.Println(messages[lang].Info.FileSavedAs, finalFileName)

	isExists, err := dbMovieExistsUploadedFile(finalFileName)
	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, err.Error())
		return
	}

	if isExists {
		sendErrorMessage(update.Message.Chat.ID, messages[lang].Error.VideoExists)
		return
	}

	var torrentFile *string = nil
	filePaths := []string{finalFileName}
	videoId := dbAddMovie(videoTitle, torrentFile, filePaths)

	err = downloadWithYTDLP(url, finalFileName)
	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, fmt.Sprintf(messages[lang].Error.VideoDownloadError, err.Error()))
		deleteMovie(videoId)
		return
	}

	dbSetLoaded(videoId)
	dbUpdateDownloadedPercentage(videoId, 100)

	log.Println(messages[lang].Info.VideoSuccessfullyDownloaded, finalFileName)
	sendSuccessMessage(update.Message.Chat.ID, fmt.Sprintf(messages[lang].Info.VideoSuccessfullyDownloaded, videoTitle))
}

func generateFileName(title string) string {
	return sanitizeFileName(title) + ".mp4"
}

func getVideoTitle(url string) (string, error) {
	cmd := exec.Command("yt-dlp", "--get-title", url)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	videoTitle := strings.TrimSpace(string(output))
	return videoTitle, nil
}

func downloadWithYTDLP(url string, outputFileName string) error {
	cmd := exec.Command("yt-dlp", "-f", "bestvideo[vcodec=h264]+bestaudio[acodec=aac]/best", "-o", filepath.Join(GlobalConfig.MoviePath, outputFileName), url)

	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

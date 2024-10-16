package main

import (
	"log"
	"os/exec"
	"path/filepath"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func downloadVideo(update tgbotapi.Update) {
	url := update.Message.Text
	log.Print(GetMessage(StartVideoDownloadMsgID, url))

	videoTitle, err := getVideoTitle(url)
	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, GetMessage(VideoTitleErrorMsgID, err.Error()))
		return
	}

	finalFileName := generateFileName(videoTitle)
	log.Print(GetMessage(FileSavedAsMsgID, finalFileName))

	isExists, err := dbMovieExistsUploadedFile(finalFileName)
	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, err.Error())
		return
	}

	if isExists {
		sendErrorMessage(update.Message.Chat.ID, GetMessage(VideoExistsMsgID))
		return
	}

	var torrentFile *string = nil
	filePaths := []string{finalFileName}
	videoId := dbAddMovie(videoTitle, torrentFile, filePaths)

	err = downloadWithYTDLP(url, finalFileName)
	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, GetMessage(VideoDownloadErrorMsgID, err.Error()))
		deleteMovie(videoId)
		return
	}

	dbSetLoaded(videoId)
	dbUpdateDownloadedPercentage(videoId, 100)

	log.Print(GetMessage(VideoSuccessfullyDownloadedMsgID, finalFileName))
	sendSuccessMessage(update.Message.Chat.ID, GetMessage(VideoSuccessfullyDownloadedMsgID, videoTitle))
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

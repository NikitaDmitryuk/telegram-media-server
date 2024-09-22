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
	log.Println("Начало загрузки для URL:", url)

	videoTitle, err := getVideoTitle(url)
	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, "Ошибка получения названия видео: "+err.Error())
		return
	}

	finalFileName := generateFileName(videoTitle)
	log.Println("Файл будет сохранён как:", finalFileName)

	isExists, err := movieExistsUploadedFile(finalFileName)
	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, err.Error())
		return
	}

	if isExists {
		sendErrorMessage(update.Message.Chat.ID, "Видео уже существует или в процессе загрузки")
		return
	}

	var torrentFile *string = nil
	filePaths := []string{finalFileName}
	videoId := addMovie(videoTitle, torrentFile, filePaths)

	err = downloadWithYTDLP(url, finalFileName)
	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, "Ошибка загрузки видео: "+err.Error())
		deleteMovie(videoId)
		return
	}

	setLoaded(videoId)
	updateDownloadedPercentage(videoId, 100)

	log.Println("Видео успешно загружено:", finalFileName)
	sendSuccessMessage(update.Message.Chat.ID, "Видео успешно загружено: "+videoTitle)
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

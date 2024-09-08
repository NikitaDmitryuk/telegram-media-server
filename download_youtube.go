package main

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kkdai/youtube/v2"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
)

func downloadYouTubeVideo(update tgbotapi.Update) {
	url := update.Message.Text
	log.Println("Начало загрузки для URL:", url)

	videoID, err := extractVideoID(url)
	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, "Ошибка извлечения ID видео")
		return
	}
	log.Println("Извлечен ID видео:", videoID)

	client := youtube.Client{}
	video, err := client.GetVideo(videoID)
	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, "Ошибка получения информации о видео")
		return
	}
	log.Println("Получена информация о видео:", video.Title)

	videoFileName, audioFileName, finalFileName := generateFileNames(video.Title)
	log.Println("Файлы будут сохранены в:", videoFileName, "и", audioFileName)

	isExists, err := movieExistsUploadedFile(finalFileName)

	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, err.Error())
		return
	}

	if isExists {
		sendErrorMessage(update.Message.Chat.ID, "Видео уже существует или в процессе загрузки")
		return
	}
	videoId := addMovie(video.Title, finalFileName, "")

	bestVideoFormat, bestAudioFormat, err := findBestFormats(video.Formats)
	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, "Не удалось найти подходящий формат видео или аудио: "+video.Title)
		cleanupFiles(videoFileName, audioFileName)
		deleteMovie(videoId)
		return
	}

	videoStream, err := getStream(client, video, bestVideoFormat, "video")
	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, "Ошибка получения видеопотока: "+video.Title)
		cleanupFiles(videoFileName, audioFileName)
		deleteMovie(videoId)
		return
	}

	audioStream, err := getStream(client, video, bestAudioFormat, "audio")
	if err != nil {
		sendErrorMessage(update.Message.Chat.ID, "Ошибка получения аудиопотока: "+video.Title)
		cleanupFiles(videoFileName, audioFileName)
		deleteMovie(videoId)
		return
	}

	if err := saveStreamToFile(videoStream, videoFileName, "video"); err != nil {
		sendErrorMessage(update.Message.Chat.ID, "Ошибка сохранения видеопотока в файл: "+video.Title)
		cleanupFiles(videoFileName, audioFileName)
		deleteMovie(videoId)
		return
	}

	if err := saveStreamToFile(audioStream, audioFileName, "audio"); err != nil {
		sendErrorMessage(update.Message.Chat.ID, "Ошибка сохранения аудиопотока в файл: "+video.Title)
		cleanupFiles(videoFileName, audioFileName)
		deleteMovie(videoId)
		return
	}

	updateDownloadedPercentage(videoId, 50)

	if err := mergeVideoAndAudio(videoFileName, audioFileName, finalFileName); err != nil {
		sendErrorMessage(update.Message.Chat.ID, "Ошибка объединения видео и аудио: "+video.Title)
		cleanupFiles(videoFileName, audioFileName)
		deleteMovie(videoId)
		return
	}

	setLoaded(videoId)
	updateDownloadedPercentage(videoId, 100)

	if err := cleanupFiles(videoFileName, audioFileName); err != nil {
		sendErrorMessage(update.Message.Chat.ID, "Ошибка очистки файлов: "+video.Title)
		return
	}

	log.Println("Видео успешно загружено, объединено и очищено:", finalFileName)

	sendSuccessMessage(update.Message.Chat.ID, "Видео успешно загружено: "+video.Title)
}

func extractVideoID(url string) (string, error) {
	re := regexp.MustCompile(`(?:youtube\.com\/(?:[^\/\n\s]+\/\S+\/|(?:v|e(?:mbed)?)\/|\S*?[?&]v=)|youtu\.be\/|youtube\.com\/shorts\/|youtube\.com\/embed\/)([a-zA-Z0-9_-]{11})`)
	match := re.FindStringSubmatch(url)
	if len(match) < 2 {
		return "", fmt.Errorf("invalid YouTube URL")
	}
	return match[1], nil
}

func cleanupFiles(videoFileName, audioFileName string) error {
	if err := os.Remove(filepath.Join(GlobalConfig.MoviePath, videoFileName)); err != nil {
		return logAndReturnError("Error removing video file", err)
	}
	if err := os.Remove(filepath.Join(GlobalConfig.MoviePath, audioFileName)); err != nil {
		return logAndReturnError("Error removing audio file", err)
	}
	return nil
}

func generateFileNames(title string) (string, string, string) {
	sanitizedTitle := sanitizeFileName(title)
	videoFileName := sanitizedTitle + "_video.mp4"
	audioFileName := sanitizedTitle + "_audio.mp4"
	finalFileName := sanitizedTitle + ".mp4"
	return videoFileName, audioFileName, finalFileName
}

func findBestFormats(formats []youtube.Format) (*youtube.Format, *youtube.Format, error) {
	var bestVideoFormat, bestAudioFormat *youtube.Format
	for _, format := range formats {
		if format.QualityLabel != "" && (bestVideoFormat == nil || format.QualityLabel > bestVideoFormat.QualityLabel) {
			bestVideoFormat = &format
		}
		if format.AudioQuality != "" && (bestAudioFormat == nil || format.AudioQuality > bestAudioFormat.AudioQuality) {
			bestAudioFormat = &format
		}
	}
	if bestVideoFormat == nil || bestAudioFormat == nil {
		return nil, nil, fmt.Errorf("could not find suitable video or audio format")
	}
	return bestVideoFormat, bestAudioFormat, nil
}

func getStream(client youtube.Client, video *youtube.Video, format *youtube.Format, streamType string) (io.ReadCloser, error) {
	stream, _, err := client.GetStream(video, format)
	if err != nil {
		return nil, logAndReturnError(fmt.Sprintf("Error getting %s stream", streamType), err)
	}
	return stream, nil
}

func saveStreamToFile(stream io.ReadCloser, fileName, fileType string) error {
	filePath := filepath.Join(GlobalConfig.MoviePath, fileName)
	file, err := os.Create(filePath)
	if err != nil {
		return logAndReturnError(fmt.Sprintf("Error creating %s file", fileType), err)
	}
	defer func() {
		if err != nil {
			os.Remove(filePath)
		}
		file.Close()
	}()
	_, err = io.Copy(file, stream)
	if err != nil {
		return logAndReturnError(fmt.Sprintf("Error writing to %s file", fileType), err)
	}
	return nil
}

func mergeVideoAndAudio(videoFileName, audioFileName, finalFileName string) error {
	videoFilePath := filepath.Join(GlobalConfig.MoviePath, videoFileName)
	audioFilePath := filepath.Join(GlobalConfig.MoviePath, audioFileName)
	finalFilePath := filepath.Join(GlobalConfig.MoviePath, finalFileName)
	cmd := exec.Command("ffmpeg", "-i", videoFilePath, "-i", audioFilePath, "-c:v", "copy", "-c:a", "aac", finalFilePath)
	err := cmd.Run()
	if err != nil {
		return logAndReturnError("Error merging video and audio", err)
	}
	return nil
}

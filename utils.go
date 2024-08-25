package main

import (
	"fmt"
	"github.com/anacrolix/torrent"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kkdai/youtube/v2"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
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

func downloadTorrent(torrentFileName string, update tgbotapi.Update) error {
	clientConfig := torrent.NewDefaultClientConfig()
	clientConfig.DataDir = GlobalConfig.MoviePath
	client, err := torrent.NewClient(clientConfig)
	if err != nil {
		log.Fatalf("Failed to create torrent client: %v", err)
		return err
	}

	t, err := client.AddTorrentFromFile(filepath.Join(GlobalConfig.MoviePath, torrentFileName))
	if err != nil {
		log.Fatalf("Failed to add torrent: %v", err)
		return err
	}

	<-t.GotInfo()

	requiredSpace := t.Info().TotalLength()
	if !hasEnoughSpace(GlobalConfig.MoviePath, requiredSpace) {
		text := fmt.Sprintf("Ошибка: недостаточно места для загрузки фильма %s.", t.Name())
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
		GlobalBot.Send(msg)
		client.Close()
		return fmt.Errorf("недостаточно места для загрузки фильма %s", t.Name())
	}

	t.DownloadAll()
	movieName := t.Name()
	movieID := addMovie(movieName, movieName, torrentFileName)

	log.Printf("Start download - %s", movieName)

	go monitorDownload(t, movieID, client, update)
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

func monitorDownload(t *torrent.Torrent, movieID int, client *torrent.Client, update tgbotapi.Update) {
	var lastPercentage int = 0
	startTime := time.Now()

	for {
		select {
		case <-time.After(time.Duration(GlobalConfig.UpdateIntervalSeconds) * time.Second):
			percentage := int(t.BytesCompleted() * 100 / t.Info().TotalLength())
			elapsedTime := time.Since(startTime).Minutes()

			movieName, _, _, err := getMovieByID(movieID)
			if err != nil {
				log.Printf("Error getting movie by ID: %v", err)
				return
			}

			updateDownloadedPercentage(movieID, percentage)

			if percentage >= lastPercentage+GlobalConfig.UpdatePercentageStep {
				lastPercentage = percentage

				text := fmt.Sprintf("Загрузка %s: %d%%", movieName, percentage)
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
				GlobalBot.Send(msg)
			}

			if t.Complete.Bool() {
				setLoaded(movieID)
				text := fmt.Sprintf("Загрузка фильма %s завершена!", movieName)
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
				GlobalBot.Send(msg)
				client.Close()
				return
			}

			if elapsedTime >= float64(GlobalConfig.MaxWaitTimeMinutes) && percentage < GlobalConfig.MinDownloadPercentage {
				os.Remove(filepath.Join(GlobalConfig.MoviePath, t.InfoHash().HexString()+".torrent"))
				deleteMovie(movieID)
				text := fmt.Sprintf("Загрузка фильма %s остановлена из-за низкой скорости загрузки.", movieName)
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
				GlobalBot.Send(msg)
				client.Close()
				return
			}
		}
	}
}

func deleteMovie(id int) string {
	_, uploadedFile, torrentFile, err := getMovieByID(id)
	if err != nil {
		return "Фильм не найден"
	}
	os.Remove(filepath.Join(GlobalConfig.MoviePath, uploadedFile))
	if torrentFile != "" {
		os.Remove(filepath.Join(GlobalConfig.MoviePath, torrentFile))
	}
	removeMovie(id)
	return "Фильм успешно удален"
}

func downloadYouTubeVideo(url string) error {
	log.Println("Starting download for URL:", url)

	videoID, err := extractVideoID(url)
	if err != nil {
		log.Printf("Error extracting video ID: %v\n", err)
		return fmt.Errorf("error extracting video ID: %v", err)
	}
	log.Println("Extracted video ID:", videoID)

	client := youtube.Client{}
	video, err := client.GetVideo(videoID)
	if err != nil {
		log.Printf("Error getting video info: %v\n", err)
		return fmt.Errorf("error getting video info: %v", err)
	}
	log.Println("Retrieved video info:", video.Title)

	// Replace problematic characters and spaces
	sanitizedTitle := strings.ReplaceAll(video.Title, "//", "")
	sanitizedTitle = strings.ReplaceAll(sanitizedTitle, " ", "_")
	videoFullName := sanitizedTitle + ".mp4"
	filePath := filepath.Join(GlobalConfig.MoviePath, videoFullName)
	log.Println("File will be saved to:", filePath)

	var bestFormat *youtube.Format
	for _, format := range video.Formats {
		if format.AudioQuality != "" && (bestFormat == nil || format.QualityLabel > bestFormat.QualityLabel) {
			bestFormat = &format
		}
	}
	if bestFormat == nil {
		bestFormat = &video.Formats[0]
	}

	stream, _, err := client.GetStream(video, bestFormat)
	if err != nil {
		log.Printf("Error getting video stream: %v\n", err)
		return fmt.Errorf("error getting video stream: %v", err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		log.Printf("Error creating file: %v\n", err)
		return fmt.Errorf("error creating file: %v", err)
	}
	defer file.Close()

	_, err = file.ReadFrom(stream)
	if err != nil {
		log.Printf("Error writing to file: %v\n", err)
		return fmt.Errorf("error writing to file: %v", err)
	}

	log.Println("Video downloaded successfully:", videoFullName)

	id := addMovie(video.Title, videoFullName, "")
	setLoaded(id)
	updateDownloadedPercentage(id, 100)

	return nil
}

func extractVideoID(url string) (string, error) {
	re := regexp.MustCompile(`(?:youtube\.com\/(?:[^\/\n\s]+\/\S+\/|(?:v|e(?:mbed)?)\/|\S*?[?&]v=)|youtu\.be\/)([a-zA-Z0-9_-]{11})`)
	match := re.FindStringSubmatch(url)
	if len(match) < 2 {
		return "", fmt.Errorf("invalid YouTube URL")
	}
	return match[1], nil
}

func isYouTubeVideoLink(text string) bool {
	re := regexp.MustCompile(`^(https?\:\/\/)?(www\.youtube\.com|youtu\.?be)\/.+$`)
	return re.MatchString(text)
}

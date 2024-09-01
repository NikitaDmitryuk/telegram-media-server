package main

import (
	"fmt"
	"github.com/anacrolix/torrent"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kkdai/youtube/v2"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
)

var stopDownload = make(chan bool)

func stopTorrentDownload() {
	movies, err := getMovieList()
	if err != nil {
		log.Printf("failed get movie: %v", err)
	}
	for _, movie := range movies {
		if !movie.Downloaded {
			stopDownload <- true
		}
	}
}

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

	clientConfig.ListenPort = 42000 + rand.Intn(100)

	client, err := torrent.NewClient(clientConfig)
	if err != nil {
		log.Printf("Failed to create torrent client: %v", err)
		return err
	}

	t, err := client.AddTorrentFromFile(filepath.Join(GlobalConfig.MoviePath, torrentFileName))
	if err != nil {
		log.Printf("Failed to add torrent: %v", err)
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

			movie, err := getMovieByID(movieID)
			if err != nil {
				log.Printf("Error getting movie by ID: %v", err)
				return
			}

			updateDownloadedPercentage(movieID, percentage)

			if percentage >= lastPercentage+GlobalConfig.UpdatePercentageStep {
				lastPercentage = percentage

				text := fmt.Sprintf("Загрузка %s: %d%%", movie.Name, percentage)
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
				GlobalBot.Send(msg)
			}

			if t.Complete.Bool() {
				setLoaded(movieID)
				text := fmt.Sprintf("Загрузка фильма %s завершена!", movie.Name)
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
				GlobalBot.Send(msg)
				client.Close()
				return
			}

			if elapsedTime >= float64(GlobalConfig.MaxWaitTimeMinutes) && percentage < GlobalConfig.MinDownloadPercentage {
				os.Remove(filepath.Join(GlobalConfig.MoviePath, t.InfoHash().HexString()+".torrent"))
				setLoaded(movieID)
				deleteMovie(movieID)
				text := fmt.Sprintf("Загрузка фильма %s остановлена из-за низкой скорости загрузки.", movie.Name)
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
				GlobalBot.Send(msg)
				client.Close()
				return
			}
		case <-stopDownload:
			text := fmt.Sprintf("Загрузка фильма %s остановлена по запросу.", t.Name())
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
			GlobalBot.Send(msg)
			client.Close()
			setLoaded(movieID)
			deleteMovie(movieID)
			return
		}
	}
}

func deleteMovie(id int) string {
	movie, err := getMovieByID(id)
	if err != nil {
		return "Фильм не найден"
	}
	if movie.Downloaded {
		os.Remove(filepath.Join(GlobalConfig.MoviePath, movie.UploadedFile))
		if movie.TorrentFile != "" {
			os.Remove(filepath.Join(GlobalConfig.MoviePath, movie.TorrentFile))
		}
		removeMovie(id)
		return "Фильм успешно удален"
	}
	return "Фильм еще не загружен, поэтому не может быть удален"
}

func sanitizeFileName(name string) string {
	re := regexp.MustCompile(`[^а-яА-Яa-zA-Z0-9]+`)
	return re.ReplaceAllString(name, "_")
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

	sanitizedTitle := sanitizeFileName(video.Title)
	videoFileName := sanitizedTitle + "_video.mp4"
	audioFileName := sanitizedTitle + "_audio.mp4"
	finalFileName := sanitizedTitle + ".mp4"
	videoFilePath := filepath.Join(GlobalConfig.MoviePath, videoFileName)
	audioFilePath := filepath.Join(GlobalConfig.MoviePath, audioFileName)
	finalFilePath := filepath.Join(GlobalConfig.MoviePath, finalFileName)
	log.Println("Files will be saved to:", videoFilePath, "and", audioFilePath)

	var bestVideoFormat, bestAudioFormat *youtube.Format
	for _, format := range video.Formats {
		if format.QualityLabel != "" && (bestVideoFormat == nil || format.QualityLabel > bestVideoFormat.QualityLabel) {
			bestVideoFormat = &format
		}
		if format.AudioQuality != "" && (bestAudioFormat == nil || format.AudioQuality > bestAudioFormat.AudioQuality) {
			bestAudioFormat = &format
		}
	}

	if bestVideoFormat == nil || bestAudioFormat == nil {
		return fmt.Errorf("could not find suitable video or audio format")
	}

	videoStream, _, err := client.GetStream(video, bestVideoFormat)
	if err != nil {
		log.Printf("Error getting video stream: %v\n", err)
		return fmt.Errorf("error getting video stream: %v", err)
	}

	audioStream, _, err := client.GetStream(video, bestAudioFormat)
	if err != nil {
		log.Printf("Error getting audio stream: %v\n", err)
		return fmt.Errorf("error getting audio stream: %v", err)
	}

	videoFile, err := os.Create(videoFilePath)
	if err != nil {
		log.Printf("Error creating video file: %v\n", err)
		return fmt.Errorf("error creating video file: %v", err)
	}
	defer func() {
		if err != nil {
			os.Remove(videoFilePath)
		}
		videoFile.Close()
	}()

	audioFile, err := os.Create(audioFilePath)
	if err != nil {
		log.Printf("Error creating audio file: %v\n", err)
		return fmt.Errorf("error creating audio file: %v", err)
	}
	defer func() {
		if err != nil {
			os.Remove(audioFilePath)
		}
		audioFile.Close()
	}()

	_, err = io.Copy(videoFile, videoStream)
	if err != nil {
		log.Printf("Error writing to video file: %v\n", err)
		return fmt.Errorf("error writing to video file: %v", err)
	}

	_, err = io.Copy(audioFile, audioStream)
	if err != nil {
		log.Printf("Error writing to audio file: %v\n", err)
		return fmt.Errorf("error writing to audio file: %v", err)
	}

	// Use ffmpeg to merge video and audio
	cmd := exec.Command("ffmpeg", "-i", videoFilePath, "-i", audioFilePath, "-c:v", "copy", "-c:a", "aac", finalFilePath)
	err = cmd.Run()
	if err != nil {
		log.Printf("Error merging video and audio: %v\n", err)
		return fmt.Errorf("error merging video and audio: %v", err)
	}

	// Remove the source video and audio files
	err = os.Remove(videoFilePath)
	if err != nil {
		log.Printf("Error removing video file: %v\n", err)
		return fmt.Errorf("error removing video file: %v", err)
	}

	err = os.Remove(audioFilePath)
	if err != nil {
		log.Printf("Error removing audio file: %v\n", err)
		return fmt.Errorf("error removing audio file: %v", err)
	}

	log.Println("Video downloaded, merged, and cleaned up successfully:", finalFileName)

	id := addMovie(video.Title, finalFileName, "")
	setLoaded(id)
	updateDownloadedPercentage(id, 100)

	return nil
}

func extractVideoID(url string) (string, error) {
	re := regexp.MustCompile(`(?:youtube\.com\/(?:[^\/\n\s]+\/\S+\/|(?:v|e(?:mbed)?)\/|\S*?[?&]v=)|youtu\.be\/|youtube\.com\/shorts\/|youtube\.com\/embed\/)([a-zA-Z0-9_-]{11})`)
	match := re.FindStringSubmatch(url)
	if len(match) < 2 {
		return "", fmt.Errorf("invalid YouTube URL")
	}
	return match[1], nil
}

func isYouTubeVideoLink(text string) bool {
	re := regexp.MustCompile(`^(https?\:\/\/)?(www\.)?(youtube\.com|youtu\.?be)\/(watch\?v=|embed\/|v\/|.+\?v=)?([^&=%\?]{11})`)
	return re.MatchString(text)
}

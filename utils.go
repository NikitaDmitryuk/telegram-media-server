package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/anacrolix/torrent"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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

	if !hasEnoughSpace(t.Info().TotalLength()) {
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

func hasEnoughSpace(requiredSpace int64) bool {
	var stat syscall.Statfs_t
	wd, err := os.Getwd()
	if err != nil {
		log.Printf("Error getting current directory: %v", err)
		return false
	}
	syscall.Statfs(wd, &stat)
	availableSpace := stat.Bavail * uint64(stat.Bsize)
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

			if percentage >= lastPercentage+GlobalConfig.UpdatePercentageStep {
				lastPercentage = percentage
				updateDownloadedPercentage(movieName, percentage)
				text := fmt.Sprintf("Загрузка %s: %d%%", movieName, percentage)
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
				GlobalBot.Send(msg)
			}

			if t.Complete.Bool() {
				setLoaded(movieName)
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

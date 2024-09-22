package main

import (
	"fmt"
	"github.com/anacrolix/torrent"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

var stopDownload = make(chan bool)

func stopTorrentDownload() {
	movies, err := getMovieList()
	if err != nil {
		log.Printf("failed to get movie list: %v", err)
	}
	for _, movie := range movies {
		if !movie.Downloaded {
			stopDownload <- true
		}
	}
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
		message := fmt.Sprintf("Недостаточно места для загрузки фильма %s.", t.Name())
		sendErrorMessage(update.Message.Chat.ID, message)
		client.Close()
		return fmt.Errorf("недостаточно места для загрузки фильма %s", t.Name())
	}

	t.DownloadAll()

	movieName := t.Name()
	var filePaths []string

	for _, file := range t.Files() {
		fullFilePath := file.Path()
		filePaths = append(filePaths, fullFilePath)
	}

	movieID := addMovie(movieName, &torrentFileName, filePaths)

	log.Printf("Start download - %s", movieName)

	go monitorDownload(t, movieID, client, update)
	return nil
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
				sendSuccessMessage(update.Message.Chat.ID, text)
			}

			if t.Complete.Bool() {
				setLoaded(movieID)

				text := fmt.Sprintf("Загрузка фильма %s завершена!", movie.Name)
				sendSuccessMessage(update.Message.Chat.ID, text)

				client.Close()
				return
			}

			if elapsedTime >= float64(GlobalConfig.MaxWaitTimeMinutes) && percentage < GlobalConfig.MinDownloadPercentage {
				os.Remove(filepath.Join(GlobalConfig.MoviePath, t.InfoHash().HexString()+".torrent"))
				deleteMovie(movieID)
				text := fmt.Sprintf("Загрузка фильма %s остановлена из-за низкой скорости загрузки.", movie.Name)
				sendSuccessMessage(update.Message.Chat.ID, text)
				client.Close()
				return
			}
		case <-stopDownload:
			text := fmt.Sprintf("Загрузка фильма %s остановлена по запросу.", t.Name())
			sendSuccessMessage(update.Message.Chat.ID, text)
			client.Close()
			deleteMovie(movieID)
			return
		}
	}
}

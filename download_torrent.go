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

func stopTorrentDownload() error {
	movies, err := dbGetMovieList()
	if err != nil {
		log.Printf("failed to get movie list: %v", err)
		return logAndReturnError("failed to get movie list", err)
	}
	for _, movie := range movies {
		if !movie.Downloaded {
			stopDownload <- true
		}
	}
	return nil
}

func downloadTorrent(torrentFileName string, update tgbotapi.Update) error {
	clientConfig := torrent.NewDefaultClientConfig()
	clientConfig.DataDir = GlobalConfig.MoviePath

	clientConfig.ListenPort = 42000 + rand.Intn(100)

	client, err := torrent.NewClient(clientConfig)
	if err != nil {
		log.Printf(messages[lang].Error.TorrentClientError, err)
		return err
	}

	t, err := client.AddTorrentFromFile(filepath.Join(GlobalConfig.MoviePath, torrentFileName))
	if err != nil {
		log.Printf(messages[lang].Error.AddTorrentError, err)
		return err
	}

	<-t.GotInfo()

	requiredSpace := t.Info().TotalLength()
	if !hasEnoughSpace(GlobalConfig.MoviePath, requiredSpace) {
		message := fmt.Sprintf(messages[lang].Error.NotEnoughSpace, t.Name())
		sendErrorMessage(update.Message.Chat.ID, message)
		client.Close()
		return fmt.Errorf(messages[lang].Error.NotEnoughSpace, t.Name())
	}

	t.DownloadAll()

	movieName := t.Name()
	var filePaths []string

	for _, file := range t.Files() {
		fullFilePath := file.Path()
		filePaths = append(filePaths, fullFilePath)
	}

	movieID := dbAddMovie(movieName, &torrentFileName, filePaths)

	log.Printf(messages[lang].Info.StartDownload, movieName)

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

			movie, err := dbGetMovieByID(movieID)
			if err != nil {
				log.Printf(messages[lang].Error.GetMovieError, err)
				return
			}

			dbUpdateDownloadedPercentage(movieID, percentage)

			if percentage >= lastPercentage+GlobalConfig.UpdatePercentageStep {
				lastPercentage = percentage

				text := fmt.Sprintf(messages[lang].Info.DownloadProgress, movie.Name, percentage)
				sendSuccessMessage(update.Message.Chat.ID, text)
			}

			if t.Complete.Bool() {
				dbSetLoaded(movieID)

				text := fmt.Sprintf(messages[lang].Info.DownloadComplete, movie.Name)
				sendSuccessMessage(update.Message.Chat.ID, text)

				client.Close()
				return
			}

			if elapsedTime >= float64(GlobalConfig.MaxWaitTimeMinutes) && percentage < GlobalConfig.MinDownloadPercentage {
				os.Remove(filepath.Join(GlobalConfig.MoviePath, t.InfoHash().HexString()+".torrent"))
				deleteMovie(movieID)
				text := fmt.Sprintf(messages[lang].Error.DownloadStoppedLowSpeed, movie.Name)
				sendErrorMessage(update.Message.Chat.ID, text)
				client.Close()
				return
			}
		case <-stopDownload:
			text := fmt.Sprintf(messages[lang].Info.DownloadStopped, t.Name())
			sendSuccessMessage(update.Message.Chat.ID, text)
			client.Close()
			deleteMovie(movieID)
			return
		}
	}
}

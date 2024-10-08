package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/anacrolix/torrent"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var stopDownload = make(chan bool)

func stopTorrentDownload() error {
	movies, err := dbGetMovieList()
	if err != nil {
		log.Printf(GetMessage(GetMovieListErrorMsgID), err)
		return fmt.Errorf(GetMessage(GetMovieListErrorMsgID), err)
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
		log.Printf(GetMessage(TorrentClientErrorMsgID), err)
		return err
	}

	t, err := client.AddTorrentFromFile(filepath.Join(GlobalConfig.MoviePath, torrentFileName))
	if err != nil {
		log.Printf(GetMessage(AddTorrentErrorMsgID), err)
		return err
	}

	<-t.GotInfo()

	requiredSpace := t.Info().TotalLength()
	if !hasEnoughSpace(GlobalConfig.MoviePath, requiredSpace) {
		message := GetMessage(NotEnoughSpaceMsgID, t.Name())
		sendErrorMessage(update.Message.Chat.ID, message)
		client.Close()
		return fmt.Errorf(message)
	}

	t.DownloadAll()

	movieName := t.Name()
	var filePaths []string

	for _, file := range t.Files() {
		fullFilePath := file.Path()
		filePaths = append(filePaths, fullFilePath)
	}

	movieID := dbAddMovie(movieName, &torrentFileName, filePaths)

	log.Printf(GetMessage(StartDownloadMsgID), movieName)

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
				log.Printf(GetMessage(GetMovieErrorMsgID), err)
				return
			}

			dbUpdateDownloadedPercentage(movieID, percentage)

			if percentage >= lastPercentage+GlobalConfig.UpdatePercentageStep {
				lastPercentage = percentage

				text := GetMessage(DownloadProgressMsgID, movie.Name, percentage)
				sendSuccessMessage(update.Message.Chat.ID, text)
			}

			if t.Complete.Bool() {
				dbSetLoaded(movieID)

				text := GetMessage(DownloadCompleteMsgID, movie.Name)
				sendSuccessMessage(update.Message.Chat.ID, text)

				client.Close()
				return
			}

			if elapsedTime >= float64(GlobalConfig.MaxWaitTimeMinutes) && percentage < GlobalConfig.MinDownloadPercentage {
				os.Remove(filepath.Join(GlobalConfig.MoviePath, t.InfoHash().HexString()+".torrent"))
				deleteMovie(movieID)
				text := GetMessage(DownloadStoppedLowSpeedMsgID, movie.Name)
				sendErrorMessage(update.Message.Chat.ID, text)
				client.Close()
				return
			}
		case <-stopDownload:
			text := GetMessage(DownloadStoppedMsgID, t.Name())
			sendSuccessMessage(update.Message.Chat.ID, text)
			client.Close()
			deleteMovie(movieID)
			return
		}
	}
}

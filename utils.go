package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/anacrolix/torrent"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func downloadFile(fileID, fileName string) error {
	file, err := GlobalBot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		log.Printf("failed to get file: %v", err)
		return err
	}

	fileURL := file.Link(GlobalBot.Token)
	resp, err := http.Get(fileURL)
	if err != nil {
		log.Printf("failed to download file: %v", err)
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(fileName)
	if err != nil {
		log.Printf("failed to create file: %v", err)
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Printf("failed to save file: %v", err)
		return err
	}

	log.Println("File downloaded successfully")
	return nil
}

func downloadTorrent(torrentFileName string, update tgbotapi.Update) {
	clientConfig := torrent.NewDefaultClientConfig()
	client, err := torrent.NewClient(clientConfig)
	if err != nil {
		log.Fatalf("failed to create torrent client: %v", err)
	}
	defer client.Close()

	t, err := client.AddTorrentFromFile(filepath.Join(GlobalConfig.MoviePath, torrentFileName))
	if err != nil {
		log.Fatalf("failed to add torrent: %v", err)
	}

	<-t.GotInfo()
	t.DownloadAll()
	movieName := t.Name()
	addMovie(movieName, movieName, torrentFileName)

	go func() {
		for {
			select {
			case <-time.After(10 * time.Second):
				percentage := int(t.BytesCompleted() * 100 / t.Info().TotalLength())
				updateDownloadedPercentage(movieName, percentage)
				text := fmt.Sprintf("download percentage: %s", percentage)
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
				GlobalBot.Send(msg)
				if t.Complete.Bool() {
					setLoaded(movieName)
					if err != nil {
						log.Printf("failed to update download status: %v", err)
					}
					text := fmt.Sprintf("download seccess")
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
					GlobalBot.Send(msg)
					return
				}
			}
		}
	}()
}

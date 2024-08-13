package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"github.com/anacrolix/torrent"
	"time"

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

func downloadTorrent(filePath string, movieID int) {
	clientConfig := torrent.NewDefaultClientConfig()
	client, err := torrent.NewClient(clientConfig)
	if err != nil {
		log.Fatalf("failed to create torrent client: %v", err)
	}
	defer client.Close()

	t, err := client.AddTorrentFromFile(filePath)
	if err != nil {
		log.Fatalf("failed to add torrent: %v", err)
	}

	<-t.GotInfo()
	t.DownloadAll()

	go func() {
		for {
			select {
			case <-time.After(10 * time.Second):
				percentage := int(t.BytesCompleted() * 100 / t.Info().TotalLength())
				updateDownloadPercentage(movieName, percentage)
				if percentage == 100 {
					setLoaded(movieName)
					if err != nil {
						log.Printf("failed to update download status: %v", err)
					}
					return
				}
			}
		}
	}()
}

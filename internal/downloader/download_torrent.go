package downloader

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/db"
	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tmsutils "github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
	"github.com/anacrolix/torrent"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var stopDownload = make(chan bool)

func StopTorrentDownload(bot *tmsbot.Bot) error {
	movies, err := tmsdb.DbGetMovieList(bot)
	if err != nil {
		log.Printf(tmslang.GetMessage(tmslang.GetMovieListErrorMsgID), err)
		return fmt.Errorf(tmslang.GetMessage(tmslang.GetMovieListErrorMsgID), err)
	}
	for _, movie := range movies {
		if !movie.Downloaded {
			stopDownload <- true
		}
	}
	return nil
}

func DownloadTorrent(bot *tmsbot.Bot, torrentFileName string, update tgbotapi.Update) error {
	clientConfig := torrent.NewDefaultClientConfig()
	clientConfig.DataDir = bot.GetConfig().MoviePath

	clientConfig.ListenPort = 42000 + rand.Intn(100)

	client, err := torrent.NewClient(clientConfig)
	if err != nil {
		log.Printf(tmslang.GetMessage(tmslang.TorrentClientErrorMsgID), err)
		return err
	}

	t, err := client.AddTorrentFromFile(filepath.Join(bot.GetConfig().MoviePath, torrentFileName))
	if err != nil {
		log.Printf(tmslang.GetMessage(tmslang.AddTorrentErrorMsgID), err)
		return err
	}

	<-t.GotInfo()

	requiredSpace := t.Info().TotalLength()
	if !tmsutils.HasEnoughSpace(bot.GetConfig().MoviePath, requiredSpace) {
		message := tmslang.GetMessage(tmslang.NotEnoughSpaceMsgID, t.Name())
		bot.SendErrorMessage(update.Message.Chat.ID, message)
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

	movieID := tmsdb.DbAddMovie(bot, movieName, &torrentFileName, filePaths)

	log.Print(tmslang.GetMessage(tmslang.StartDownloadMsgID, movieName))

	go monitorDownload(bot, t, movieID, client, update)
	return nil
}

func monitorDownload(bot *tmsbot.Bot, t *torrent.Torrent, movieID int, client *torrent.Client, update tgbotapi.Update) {
	var lastPercentage int = 0
	startTime := time.Now()

	for {
		select {
		case <-time.After(time.Duration(bot.GetConfig().UpdateIntervalSeconds) * time.Second):
			percentage := int(t.BytesCompleted() * 100 / t.Info().TotalLength())
			elapsedTime := time.Since(startTime).Minutes()

			movie, err := tmsdb.DbGetMovieByID(bot, movieID)
			if err != nil {
				log.Print(tmslang.GetMessage(tmslang.GetMovieErrorMsgID, err))
				return
			}

			tmsdb.DbUpdateDownloadedPercentage(bot, movieID, percentage)

			if percentage >= lastPercentage+bot.GetConfig().UpdatePercentageStep {
				lastPercentage = percentage

				text := tmslang.GetMessage(tmslang.DownloadProgressMsgID, movie.Name, percentage)
				bot.SendSuccessMessage(update.Message.Chat.ID, text)
			}

			if t.Complete.Bool() {
				tmsdb.DbSetLoaded(bot, movieID)

				text := tmslang.GetMessage(tmslang.DownloadCompleteMsgID, movie.Name)
				bot.SendSuccessMessage(update.Message.Chat.ID, text)

				client.Close()
				return
			}

			if elapsedTime >= float64(bot.GetConfig().MaxWaitTimeMinutes) && percentage < bot.GetConfig().MinDownloadPercentage {
				os.Remove(filepath.Join(bot.GetConfig().MoviePath, t.InfoHash().HexString()+".torrent"))
				tmsutils.DeleteMovie(bot, movieID)
				text := tmslang.GetMessage(tmslang.DownloadStoppedLowSpeedMsgID, movie.Name)
				bot.SendErrorMessage(update.Message.Chat.ID, text)
				client.Close()
				return
			}
		case <-stopDownload:
			text := tmslang.GetMessage(tmslang.DownloadStoppedMsgID, t.Name())
			bot.SendSuccessMessage(update.Message.Chat.ID, text)
			client.Close()
			tmsutils.DeleteMovie(bot, movieID)
			return
		}
	}
}

package downloader

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/db"
	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tmsutils "github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func DownloadVideo(ctx context.Context, bot *tmsbot.Bot, update tgbotapi.Update) error {
	url := update.Message.Text
	log.Printf("Starting download for URL: %s", url)

	videoTitle, err := getVideoTitle(url)
	if err != nil {
		bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.VideoTitleErrorMsgID, err.Error()))
		return err
	}

	finalFileName := generateFileName(videoTitle)
	log.Printf("File will be saved as: %s", finalFileName)

	isExists, err := tmsdb.DbMovieExistsUploadedFile(bot, finalFileName)
	if err != nil {
		bot.SendErrorMessage(update.Message.Chat.ID, err.Error())
		return err
	}

	if isExists {
		bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.VideoExistsMsgID))
		return fmt.Errorf("video already exists")
	}

	var torrentFile *string = nil
	filePaths := []string{
		finalFileName,
		finalFileName + ".part",
	}
	videoId := tmsdb.DbAddMovie(bot, videoTitle, torrentFile, filePaths)

	downloadFunc := func(ctx context.Context) error {
		err := downloadWithYTDLP(ctx, bot, url, finalFileName)
		if err != nil {
			if ctx.Err() == context.Canceled {
				bot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.DownloadCancelledMsgID, videoTitle))
				log.Print("Download cancelled, cleaning up temporary files")
				if delErr := DeleteMovie(bot, videoId); delErr != nil {
					log.Printf("Delete error: %v", delErr)
				}
				return nil
			}
			bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.VideoDownloadErrorMsgID, err.Error()))
			if delErr := DeleteMovie(bot, videoId); delErr != nil {
				log.Printf("Delete error: %v", delErr)
			}
			return err
		}

		tmsdb.DbSetLoaded(bot, videoId)
		tmsdb.DbUpdateDownloadedPercentage(bot, videoId, 100)
		log.Printf("Video successfully downloaded: %s", finalFileName)
		bot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.VideoSuccessfullyDownloadedMsgID, videoTitle))
		return nil
	}

	bot.DownloadManager.StartDownload(videoId, downloadFunc)
	return nil
}

func generateFileName(title string) string {
	return tmsutils.SanitizeFileName(title) + ".mp4"
}

func getVideoTitle(url string) (string, error) {
	cmd := exec.Command("yt-dlp", "--get-title", url)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	videoTitle := strings.TrimSpace(string(output))
	return videoTitle, nil
}

func downloadWithYTDLP(ctx context.Context, bot *tmsbot.Bot, url string, outputFileName string) error {
	outputPath := filepath.Join(bot.GetConfig().MoviePath, outputFileName)
	cmd := exec.CommandContext(ctx, "yt-dlp", "-f", "bestvideo[vcodec=h264]+bestaudio[acodec=aac]/best", "-o", outputPath, url)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("yt-dlp error: %v, output: %s", err, string(output))
	}
	return nil
}

package downloader

import (
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

func DownloadVideo(bot *tmsbot.Bot, update tgbotapi.Update) {
	url := update.Message.Text
	log.Print(tmslang.GetMessage(tmslang.StartVideoDownloadMsgID, url))

	videoTitle, err := getVideoTitle(url)
	if err != nil {
		bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.VideoTitleErrorMsgID, err.Error()))
		return
	}

	finalFileName := generateFileName(videoTitle)
	log.Print(tmslang.GetMessage(tmslang.FileSavedAsMsgID, finalFileName))

	isExists, err := tmsdb.DbMovieExistsUploadedFile(bot, finalFileName)
	if err != nil {
		bot.SendErrorMessage(update.Message.Chat.ID, err.Error())
		return
	}

	if isExists {
		bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.VideoExistsMsgID))
		return
	}

	var torrentFile *string = nil
	filePaths := []string{finalFileName}
	videoId := tmsdb.DbAddMovie(bot, videoTitle, torrentFile, filePaths)

	err = downloadWithYTDLP(bot, url, finalFileName)
	if err != nil {
		bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.VideoDownloadErrorMsgID, err.Error()))
		if err := tmsutils.DeleteMovie(bot, videoId); err != nil {
			log.Printf("Delete error: %v", err)
		}
		return
	}

	tmsdb.DbSetLoaded(bot, videoId)
	tmsdb.DbUpdateDownloadedPercentage(bot, videoId, 100)

	log.Print(tmslang.GetMessage(tmslang.VideoSuccessfullyDownloadedMsgID, finalFileName))
	bot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.VideoSuccessfullyDownloadedMsgID, videoTitle))
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

func downloadWithYTDLP(bot *tmsbot.Bot, url string, outputFileName string) error {
	cmd := exec.Command("yt-dlp", "-f", "bestvideo[vcodec=h264]+bestaudio[acodec=aac]/best", "-o", filepath.Join(bot.GetConfig().MoviePath, outputFileName), url)

	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

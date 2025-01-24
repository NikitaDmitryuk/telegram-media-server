package handlers

import (
	"context"
	"log"
	"strconv"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/db"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	tmsdownloader "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tmsutils "github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func HandleUpdates(bot *tmsbot.Bot) {

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetAPI().GetUpdatesChan(u)

	for update := range updates {
		updateCopy := update
		go func(u tgbotapi.Update) {
			if u.Message == nil {
				return
			}

			log.Printf("[%s] %s", u.Message.From.UserName, u.Message.Text)

			if userExists, err := tmsdb.DbCheckUser(bot, u.Message.From.ID); err != nil {
				bot.SendErrorMessage(u.Message.Chat.ID, tmslang.GetMessage(tmslang.CheckUserErrorMsgID))
			} else if !userExists {
				handleUnknownUser(bot, u)
			} else {
				handleKnownUser(bot, u)
			}
		}(updateCopy)
	}
}

func handleUnknownUser(bot *tmsbot.Bot, update tgbotapi.Update) {
	if update.Message.IsCommand() {
		switch update.Message.Command() {
		case "login":
			loginHandler(bot, update)
		default:
			bot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.UnknownUserMsgID))
		}
	} else {
		bot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.UnknownUserMsgID))
	}
}

func handleKnownUser(bot *tmsbot.Bot, update tgbotapi.Update) {
	message := update.Message
	chatID := message.Chat.ID

	if message.IsCommand() {
		switch message.Command() {
		case "start":
			bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.StartCommandMsgID))
		case "ls":
			listHandler(bot, update)
		case "rm":
			deleteHandler(bot, update)
		case "stop":
			stopHandler(bot, update)
		case "vpn":
			vpnHandler(bot, update)
		default:
			bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.UnknownCommandMsgID))
		}
		return
	}

	if tmsutils.IsValidLink(message.Text) {
		tmsdownloader.DownloadVideo(context.Background(), bot, update)
		bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.VideoDownloadingMsgID))
		return
	}

	if doc := message.Document; doc != nil && strings.HasSuffix(doc.FileName, ".torrent") {
		fileName := doc.FileName

		if err := downloader.DownloadFile(bot, doc.FileID, fileName); err != nil {
			bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.DownloadDocumentErrorMsgID))
			return
		}

		exists, err := tmsdb.DbMovieExistsTorrent(bot, fileName)
		if err != nil {
			bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.TorrentFileExistsErrorMsgID))
			return
		}
		if exists {
			bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.FileAlreadyExistsMsgID))
			return
		}

		if err := tmsdownloader.DownloadTorrent(context.Background(), bot, fileName, update); err != nil {
			bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.TorrentFileDownloadErrorMsgID))
			return
		}

		bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.VideoDownloadingMsgID))
		return
	}

	bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.UnknownCommandMsgID))
}

func loginHandler(bot *tmsbot.Bot, update tgbotapi.Update) {
	textFields := strings.Fields(update.Message.Text)

	if len(textFields) != 2 {
		bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.InvalidCommandFormatMsgID))
	} else {
		success, err := tmsdb.DbLogin(bot, textFields[1], update.Message.Chat.ID, update.Message.From.UserName)
		if err != nil {
			bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.LoginErrorMsgID))
		} else if success {
			bot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.LoginSuccessMsgID))
		} else {
			bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.WrongPasswordMsgID))
		}
	}
}

func listHandler(bot *tmsbot.Bot, update tgbotapi.Update) {
	movies, err := tmsdb.DbGetMovieList(bot)
	if err != nil {
		bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.GetMovieListErrorMsgID))
		return
	}

	var msg string
	for _, movie := range movies {
		if movie.Downloaded {
			msg += tmslang.GetMessage(tmslang.MovieDownloadedMsgID, movie.ID, movie.Name)
		} else {
			msg += tmslang.GetMessage(tmslang.MovieDownloadingMsgID, movie.ID, movie.Name, movie.DownloadedPercentage)
		}
	}

	if len(msg) == 0 {
		msg = tmslang.GetMessage(tmslang.NoMoviesMsgID)
	}

	bot.SendSuccessMessage(update.Message.Chat.ID, msg)
}

func deleteHandler(bot *tmsbot.Bot, update tgbotapi.Update) {
	args := strings.Fields(update.Message.Text)
	if len(args) < 2 {
		bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.InvalidCommandFormatMsgID))
		return
	}

	if args[1] == "all" {
		movies, err := tmsdb.DbGetMovieList(bot)
		if err != nil {
			bot.SendErrorMessage(update.Message.Chat.ID, err.Error())
			return
		}
		for _, movie := range movies {
			err := downloader.DeleteMovie(bot, movie.ID)
			if err != nil {
				bot.SendErrorMessage(update.Message.Chat.ID, err.Error())
				return
			}
		}
		bot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.AllMoviesDeletedMsgID))
	} else {
		var invalidIDs []string
		var deletedIDs []string

		for _, arg := range args[1:] {
			id, err := strconv.Atoi(arg)
			if err != nil {
				invalidIDs = append(invalidIDs, arg)
				continue
			}
			exists, err := tmsdb.DbMovieExistsId(bot, id)
			if err != nil {
				bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.MovieCheckErrorMsgID, err))
				return
			}
			if exists {
				err := downloader.DeleteMovie(bot, id)
				if err != nil {
					bot.SendErrorMessage(update.Message.Chat.ID, err.Error())
					return
				}
				deletedIDs = append(deletedIDs, strconv.Itoa(id))
			} else {
				invalidIDs = append(invalidIDs, arg)
			}
		}

		if len(invalidIDs) > 0 {
			invalidMsg := tmslang.GetMessage(tmslang.InvalidIDsMsgID, strings.Join(invalidIDs, ", "))
			bot.SendErrorMessage(update.Message.Chat.ID, invalidMsg)
		}
		if len(deletedIDs) > 0 {
			deletedMsg := tmslang.GetMessage(tmslang.DeletedMoviesMsgID, strings.Join(deletedIDs, ", "))
			bot.SendSuccessMessage(update.Message.Chat.ID, deletedMsg)
		} else if len(invalidIDs) == 0 {
			bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.NoValidIDsMsgID))
		}
	}
}

func stopHandler(bot *tmsbot.Bot, update tgbotapi.Update) {
	args := strings.Fields(update.Message.Text)
	if len(args) < 2 {
		bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.StopNoArgumentMsgID))
		return
	}

	if args[1] == "all" {
		bot.DownloadManager.StopAllDownloads()
		bot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.TorrentDownloadsStoppedMsgID))
	} else {
		var invalidIDs []string
		var stoppedIDs []string

		for _, arg := range args[1:] {
			movieID, err := strconv.ParseInt(arg, 10, 64)
			if err != nil {
				invalidIDs = append(invalidIDs, arg)
				continue
			}
			bot.DownloadManager.StopDownload(int(movieID))
			stoppedIDs = append(stoppedIDs, strconv.FormatInt(movieID, 10))
		}

		if len(invalidIDs) > 0 {
			invalidMsg := tmslang.GetMessage(tmslang.InvalidIDsMsgID, strings.Join(invalidIDs, ", "))
			bot.SendErrorMessage(update.Message.Chat.ID, invalidMsg)
		}
		if len(stoppedIDs) > 0 {
			stoppedMsg := tmslang.GetMessage(tmslang.StoppedDownloadsMsgID, strings.Join(stoppedIDs, ", "))
			bot.SendSuccessMessage(update.Message.Chat.ID, stoppedMsg)
		} else if len(invalidIDs) == 0 {
			bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.NoValidIDsMsgID))
		}
	}
}

func vpnHandler(bot *tmsbot.Bot, update tgbotapi.Update) {
	args := strings.Fields(update.Message.Text)
	chatID := update.Message.Chat.ID

	if len(args) < 2 {
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.VPNUsageMsgID))
		return
	}

	action := strings.ToLower(args[1])
	switch action {
	case "on":
		setVPNOn(bot, chatID)
	case "off":
		setVPNOff(bot, chatID)
	case "status":
		sendVPNStatus(bot, chatID)
	default:
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.VPNInvalidActionMsgID))
	}
}

func setVPNOn(bot *tmsbot.Bot, chatID int64) {
	isActive, err := tmsutils.GetVPNState()
	if err != nil {
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.VPNCheckErrorMsgID, err))
		return
	}
	if isActive {
		bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.VPNAlreadyEnabledMsgID))
		return
	}

	if err := tmsutils.ManageVPN(true); err != nil {
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.VPNChangeErrorMsgID, err))
		return
	}
	bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.VPNEnabledMsgID))
}

func setVPNOff(bot *tmsbot.Bot, chatID int64) {
	isActive, err := tmsutils.GetVPNState()
	if err != nil {
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.VPNCheckErrorMsgID, err))
		return
	}
	if !isActive {
		bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.VPNAlreadyDisabledMsgID))
		return
	}

	if err := tmsutils.ManageVPN(false); err != nil {
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.VPNChangeErrorMsgID, err))
		return
	}
	bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.VPNDisabledMsgID))
}

func sendVPNStatus(bot *tmsbot.Bot, chatID int64) {
	state, err := tmsutils.GetVPNState()
	if err != nil {
		bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.VPNCheckErrorMsgID, err))
		return
	}
	status := "off"
	if state {
		status = "on"
	}
	bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.VPNStatusMsgID, status))
}

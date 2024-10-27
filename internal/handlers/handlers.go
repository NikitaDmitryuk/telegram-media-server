package handlers

import (
	"log"
	"strconv"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/db"
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

		if update.Message == nil {
			return
		}

		log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

		if userExists, err := tmsdb.DbCheckUser(bot, update.Message.From.ID); err != nil {
			bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.CheckUserErrorMsgID))
		} else if !userExists {
			handleUnknownUser(bot, update)
		} else {
			handleKnownUser(bot, update)
		}
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
		default:
			bot.SendErrorMessage(chatID, tmslang.GetMessage(tmslang.UnknownCommandMsgID))
		}
		return
	}

	if tmsutils.IsValidLink(message.Text) {
		go tmsdownloader.DownloadVideo(bot, update)
		bot.SendSuccessMessage(chatID, tmslang.GetMessage(tmslang.VideoDownloadingMsgID))
		return
	}

	if doc := message.Document; doc != nil && strings.HasSuffix(doc.FileName, ".torrent") {
		fileName := doc.FileName

		if err := tmsutils.DownloadFile(bot, doc.FileID, fileName); err != nil {
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

		if err := tmsdownloader.DownloadTorrent(bot, fileName, update); err != nil {
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
	commandID := strings.Fields(update.Message.Text)

	if len(commandID) != 2 {
		bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.InvalidCommandFormatMsgID))
		return
	}

	if commandID[1] == "all" {
		movies, _ := tmsdb.DbGetMovieList(bot)
		for _, movie := range movies {
			err := tmsutils.DeleteMovie(bot, movie.ID)
			if err != nil {
				bot.SendErrorMessage(update.Message.Chat.ID, err.Error())
				return
			}
		}
		bot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.AllMoviesDeletedMsgID))
	} else {
		id, err := strconv.Atoi(commandID[1])
		if err != nil {
			bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.InvalidIDMsgID))
			return
		}

		if exists, err := tmsdb.DbMovieExistsId(bot, id); err != nil {
			bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.MovieCheckErrorMsgID))
		} else if exists {
			err := tmsutils.DeleteMovie(bot, id)
			if err != nil {
				bot.SendErrorMessage(update.Message.Chat.ID, err.Error())
			} else {
				bot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.MovieDeletedMsgID))
			}
		} else {
			bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.InvalidIDMsgID))
		}
	}
}

func stopHandler(bot *tmsbot.Bot, update tgbotapi.Update) {
	go tmsdownloader.StopTorrentDownload(bot)
	bot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.TorrentDownloadsStoppedMsgID))
}

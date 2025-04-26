package handlers

import (
	"context"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tmsutils "github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

func LoginHandler(bot *tmsbot.Bot, update tgbotapi.Update) {
	textFields := strings.Fields(update.Message.Text)

	if len(textFields) != 2 {
		logrus.Warn("Invalid login command format")
		bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.InvalidCommandFormatMsgID))
		return
	}

	password := textFields[1]
	chatID := update.Message.Chat.ID
	userName := update.Message.From.UserName

	success, err := database.GlobalDB.Login(context.Background(), password, chatID, userName)
	if err != nil {
		logrus.WithError(err).Error("Login failed due to an error")
		bot.SendErrorMessage(chatID, lang.GetMessage(lang.LoginErrorMsgID))
		return
	}

	if success {
		logrus.WithField("username", userName).Info("User logged in successfully")
		bot.SendSuccessMessage(chatID, lang.GetMessage(lang.LoginSuccessMsgID))
	} else {
		logrus.WithField("username", userName).Warn("Login failed due to incorrect or expired password")
		bot.SendErrorMessage(chatID, lang.GetMessage(lang.WrongPasswordMsgID))
	}
}

func StartHandler(bot *tmsbot.Bot, update tgbotapi.Update) {
	message := lang.GetMessage(lang.StartCommandMsgID)
	SendMainMenu(bot, update.Message.Chat.ID, message)
}

func GenerateTempPasswordHandler(bot *tmsbot.Bot, update tgbotapi.Update) {
	args := strings.Fields(update.Message.Text)
	if len(args) != 2 {
		logrus.Warn("Invalid /temp command format")
		bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.InvalidCommandFormatMsgID))
		return
	}

	durationStr := args[1]
	duration, err := tmsutils.ValidateDurationString(durationStr)
	if err != nil {
		logrus.WithError(err).Warn("Invalid duration string for /temp command")
		bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.InvalidDurationMsgID))
		return
	}

	password, err := database.GlobalDB.GenerateTemporaryPassword(context.Background(), duration)
	if err != nil {
		logrus.WithError(err).Error("Failed to generate temporary password")
		bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.TempPasswordGenerationErrorMsgID))
		return
	}

	bot.SendSuccessMessage(update.Message.Chat.ID, password)
}

func handleCommand(bot *tmsbot.Bot, update tgbotapi.Update) {
	command := strings.ToLower(update.Message.Command())
	switch command {
	case "login":
		LoginHandler(bot, update)
	case "start", "ls":
		if checkAccess(bot, update) {
			if command == "start" {
				StartHandler(bot, update)
			} else {
				ListMoviesHandler(bot, update)
			}
		} else {
			bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.UnknownUserMsgID))
		}
	case "rm":
		if checkAccessWithRole(bot, update, []database.UserRole{database.AdminRole, database.RegularRole}) {
			DeleteMoviesHandler(bot, update)
		} else {
			bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.AccessDeniedMsgID))
		}
	case "temp":
		if checkAccessWithRole(bot, update, []database.UserRole{database.AdminRole}) {
			GenerateTempPasswordHandler(bot, update)
		} else {
			bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.AccessDeniedMsgID))
		}
	default:
		logrus.Warnf("Unknown command: %s", command)
		bot.SendErrorMessage(update.Message.Chat.ID, lang.GetMessage(lang.UnknownCommandMsgID))
	}
}

func handleMessage(bot *tmsbot.Bot, update tgbotapi.Update) {
	chatID := update.Message.Chat.ID
	text := update.Message.Text

	switch text {
	case lang.GetMessage(lang.ListMoviesMsgID):
		ListMoviesHandler(bot, update)
	case lang.GetMessage(lang.DeleteMovieMsgID):
		movies, err := database.GlobalDB.GetMovieList(context.Background())
		if err != nil {
			bot.SendErrorMessage(chatID, lang.GetMessage(lang.GetMovieListErrorMsgID))
			return
		}

		if len(movies) == 0 {
			bot.SendSuccessMessage(chatID, lang.GetMessage(lang.NoMoviesToDeleteMsgID))
			return
		}

		SendDeleteMovieMenu(bot, chatID, movies)
	default:
		if tmsutils.IsValidLink(text) {
			HandleDownloadLink(bot, update)
		} else if doc := update.Message.Document; doc != nil {
			if strings.HasSuffix(doc.FileName, ".torrent") {
				HandleTorrentFile(bot, update)
			} else {
				logrus.Warnf("Unsupported document type: %s", doc.FileName)
				bot.SendErrorMessage(chatID, lang.GetMessage(lang.UnsupportedFileTypeMsgID))
			}
		} else {
			logrus.Warnf("Unknown command or message: %s", text)
			bot.SendErrorMessage(chatID, lang.GetMessage(lang.UnknownCommandMsgID))
		}
	}
}

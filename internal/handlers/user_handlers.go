package handlers

import (
	"context"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	tmslang "github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

func LoginHandler(bot *tmsbot.Bot, update tgbotapi.Update) {
	textFields := strings.Fields(update.Message.Text)

	if len(textFields) != 2 {
		logrus.Warn("Invalid login command format")
		bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.InvalidCommandFormatMsgID))
		return
	}

	success, err := tmsdb.GlobalDB.Login(context.Background(), textFields[1], update.Message.Chat.ID, update.Message.From.UserName)
	if err != nil {
		logrus.WithError(err).Error("Login failed due to an error")
		bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.LoginErrorMsgID))
		return
	}

	if success {
		logrus.WithField("username", update.Message.From.UserName).Info("User logged in successfully")
		bot.SendSuccessMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.LoginSuccessMsgID))
	} else {
		logrus.WithField("username", update.Message.From.UserName).Warn("Login failed due to incorrect password")
		bot.SendErrorMessage(update.Message.Chat.ID, tmslang.GetMessage(tmslang.WrongPasswordMsgID))
	}
}

func StartHandler(bot *tmsbot.Bot, update tgbotapi.Update) {
	message := tmslang.GetMessage(tmslang.StartCommandMsgID)
	SendMainMenu(bot, update.Message.Chat.ID, message)
}

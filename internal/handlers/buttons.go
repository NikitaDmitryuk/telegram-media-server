package handlers

import (
	"strconv"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsdb "github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func SendMainMenu(bot *tmsbot.Bot, chatID int64, message string) {
	buttons := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(lang.GetMessage(lang.ListMoviesMsgID), "list_movies"),
			tgbotapi.NewInlineKeyboardButtonData(lang.GetMessage(lang.DeleteMovieMsgID), "delete_movie_menu"),
		),
	)

	msg := tgbotapi.NewMessage(chatID, message)
	msg.ReplyMarkup = buttons
	bot.Send(msg)
}

func SendDeleteMovieMenu(bot *tmsbot.Bot, chatID int64, movies []tmsdb.Movie) {
	if len(movies) == 0 {
		bot.SendSuccessMessage(chatID, lang.GetMessage(lang.NoMoviesToDeleteMsgID))
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, movie := range movies {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(movie.Name, "delete_movie:"+strconv.Itoa(int(movie.ID))),
		))
	}

	buttons := tgbotapi.NewInlineKeyboardMarkup(rows...)
	msg := tgbotapi.NewMessage(chatID, lang.GetMessage(lang.MessageToDeleteMsgID))
	msg.ReplyMarkup = buttons
	bot.Send(msg)
}

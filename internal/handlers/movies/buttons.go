package movies

import (
	"strconv"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func CreateDeleteMovieMenuMarkup(movies []database.Movie) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, movie := range movies {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(movie.Name, "delete_movie:"+strconv.FormatUint(uint64(movie.ID), 10)),
		))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(lang.Translate("general.interface.cancel", nil), "cancel_delete_menu"),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func SendDeleteMovieMenu(bot *tmsbot.Bot, chatID int64, movies []database.Movie) {
	if len(movies) == 0 {
		bot.SendMessage(chatID, lang.Translate("general.user_prompts.no_movies_to_delete", nil), nil)
		return
	}

	buttons := CreateDeleteMovieMenuMarkup(movies)
	msg := tgbotapi.NewMessage(chatID, lang.Translate("general.user_prompts.delete_prompt", nil))
	msg.ReplyMarkup = buttons
	bot.SendMessage(chatID, msg.Text, msg.ReplyMarkup)
}

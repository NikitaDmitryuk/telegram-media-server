package movies

import (
	"context"
	"strconv"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func CreateDeleteMovieMenuMarkup(movies []database.Movie) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for i := range movies {
		m := &movies[i]
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(m.Name, "delete_movie:"+strconv.FormatUint(uint64(m.ID), 10)),
		))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(lang.Translate("general.interface.cancel", nil), "cancel_delete_menu"),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func SendDeleteMovieMenu(bot tmsbot.Service, chatID int64, movies []database.Movie) {
	if len(movies) == 0 {
		bot.SendMessage(chatID, lang.Translate("general.user_prompts.no_movies_to_delete", nil), nil)
		return
	}

	buttons := CreateDeleteMovieMenuMarkup(movies)
	bot.SendMessage(chatID, lang.Translate("general.user_prompts.delete_prompt", nil), buttons)
}

// SendDeleteMovieMenuFromDB fetches the movie list from DB and sends the delete menu.
func SendDeleteMovieMenuFromDB(a *app.App, chatID int64) {
	movieList, err := a.DB.GetMovieList(context.Background())
	if err != nil {
		a.Bot.SendMessage(chatID, lang.Translate("error.movies.fetch_error", nil), nil)
		return
	}
	SendDeleteMovieMenu(a.Bot, chatID, movieList)
}

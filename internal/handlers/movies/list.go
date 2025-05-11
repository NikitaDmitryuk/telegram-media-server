package movies

import (
	"context"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func ListMoviesHandler(bot *tmsbot.Bot, update *tgbotapi.Update) {
	chatID := update.Message.Chat.ID

	movies, err := database.GlobalDB.GetMovieList(context.Background())
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to retrieve movie list")
		bot.SendErrorMessage(chatID, lang.Translate("error.movies.fetch_error", nil))
		return
	}

	if len(movies) == 0 {
		bot.SendSuccessMessage(chatID, lang.Translate("general.status_messages.empty_list", nil))
		return
	}

	var messages []string
	for _, movie := range movies {
		messages = append(messages, lang.Translate("general.downloaded_list", map[string]any{
			"ID":       movie.ID,
			"Name":     movie.Name,
			"Progress": movie.DownloadedPercentage,
		}))
	}

	message := strings.Join(messages, "\n")
	bot.SendSuccessMessage(chatID, message)
}

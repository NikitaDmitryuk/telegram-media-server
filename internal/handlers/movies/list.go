package movies

import (
	"context"
	"fmt"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
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

	moviePath := tmsconfig.GlobalConfig.MoviePath
	availableSpaceGB, err := filemanager.GetAvailableSpaceGB(moviePath)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get available disk space")
		availableSpaceGB = 0
	}

	formattedSpace := fmt.Sprintf("%.2f", availableSpaceGB)

	messages = append(messages, lang.Translate("general.disk_space_info", map[string]any{
		"AvailableSpaceGB": formattedSpace,
	}))

	message := strings.Join(messages, "\n")
	bot.SendSuccessMessage(chatID, message)
}

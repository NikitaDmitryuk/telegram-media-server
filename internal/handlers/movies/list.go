package movies

import (
	"context"
	"fmt"
	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/ui"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func ListMoviesHandler(bot *tmsbot.Bot, update *tgbotapi.Update, db database.Database, config *tmsconfig.Config) {
	chatID := update.Message.Chat.ID

	movies, err := db.GetMovieList(context.Background())
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to retrieve movie list")
		bot.SendMessage(chatID, lang.Translate("error.movies.fetch_error", nil), ui.GetMainMenuKeyboard())
		return
	}

	if len(movies) == 0 {
		bot.SendMessage(chatID, lang.Translate("general.status_messages.empty_list", nil), ui.GetMainMenuKeyboard())
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

	availableSpaceGB, err := filemanager.GetAvailableSpaceGB(config.MoviePath)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get available disk space")
		availableSpaceGB = 0
	}

	formattedSpace := fmt.Sprintf("%.2f", availableSpaceGB)

	messages = append(messages, lang.Translate("general.disk_space_info", map[string]any{
		"AvailableSpaceGB": formattedSpace,
	}))

	message := strings.Join(messages, "\n")
	bot.SendMessage(chatID, message, ui.GetMainMenuKeyboard())
}

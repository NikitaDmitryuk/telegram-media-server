package movies

import (
	"context"
	"fmt"
	"strings"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/ui"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func ListMoviesHandler(a *app.App, update *tgbotapi.Update) {
	chatID := update.Message.Chat.ID

	movies, err := a.DB.GetMovieList(context.Background())
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to retrieve movie list")
		a.Bot.SendMessage(chatID, lang.Translate("error.movies.fetch_error", nil), ui.GetMainMenuKeyboard())
		return
	}

	movies = FilterOutPendingDeletion(movies, a.DeleteQueue)

	if len(movies) == 0 {
		a.Bot.SendMessage(chatID, lang.Translate("general.status_messages.empty_list", nil), ui.GetMainMenuKeyboard())
		return
	}

	compatMode := a.Config.VideoSettings.CompatibilityMode
	var messages []string
	for i := range movies {
		movie := &movies[i]
		var formattedSize string
		if movie.FileSize == 0 {
			formattedSize = "—" // unknown size (e.g. magnet before metadata)
		} else {
			sizeGB := float64(movie.FileSize) / (1024 * 1024 * 1024) // #nosec G115
			unit := lang.Translate("general.unit_gb", nil)
			formattedSize = fmt.Sprintf("%.2f %s", sizeGB, unit)
		}
		episodes := ""
		if movie.TotalEpisodes > 1 {
			episodes = fmt.Sprintf("%d/%d ", movie.CompletedEpisodes, movie.TotalEpisodes)
		}
		progressStr := fmt.Sprintf("%d%%", movie.DownloadedPercentage)
		sticker := ""
		if compatMode {
			progressStr = fmt.Sprintf("%d/%d", movie.DownloadedPercentage, movie.ConversionPercentage)
			switch movie.TvCompatibility {
			case "green":
				sticker = "🟢 "
			case "yellow":
				sticker = "🟡 "
			case "red":
				sticker = "🔴 "
			}
		}
		messages = append(messages, lang.Translate("general.downloaded_list", map[string]any{
			"ID":       movie.ID,
			"Name":     movie.Name,
			"Progress": progressStr,
			"Sticker":  sticker,
			"Episodes": episodes,
			"SizeGB":   formattedSize,
		}))
	}

	availableSpaceGB, err := filemanager.GetAvailableSpaceGB(a.Config.MoviePath)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to get available disk space")
		availableSpaceGB = 0
	}

	formattedSpace := fmt.Sprintf("%.2f", availableSpaceGB)

	messages = append(messages, lang.Translate("general.disk_space_info", map[string]any{
		"AvailableSpaceGB": formattedSpace,
	}))

	message := strings.Join(messages, "\n")
	a.Bot.SendMessage(chatID, message, ui.GetMainMenuKeyboard())
}

package movies

import (
	"context"
	"fmt"
	"strings"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/ui"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func ListMoviesHandler(a *app.App, update *tgbotapi.Update) {
	chatID := update.Message.Chat.ID

	ctx := context.Background()
	movies, err := a.DB.GetMovieList(ctx)
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
		messages = append(messages, buildMovieListLine(&movies[i], compatMode))
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

func buildMovieListLine(movie *database.Movie, compatMode bool) string {
	formattedSize := formatListMovieSizeGB(movie)
	episodes := formatListEpisodesPrefix(movie)
	progressStr, sticker := formatListProgressAndSticker(movie, compatMode)
	return lang.Translate("general.downloaded_list", map[string]any{
		"ID":       movie.ID,
		"Name":     movie.Name,
		"Progress": progressStr,
		"Sticker":  sticker,
		"Episodes": episodes,
		"SizeGB":   formattedSize,
	})
}

func formatListMovieSizeGB(movie *database.Movie) string {
	if movie.FileSize == 0 {
		return "—" // unknown size (for example, magnet before qBittorrent metadata)
	}
	sizeGB := float64(movie.FileSize) / (1024 * 1024 * 1024) // #nosec G115
	unit := lang.Translate("general.unit_gb", nil)
	return fmt.Sprintf("%.2f %s", sizeGB, unit)
}

func formatListEpisodesPrefix(movie *database.Movie) string {
	if movie.TotalEpisodes <= 1 {
		return ""
	}
	return fmt.Sprintf("%d/%d ", movie.CompletedEpisodes, movie.TotalEpisodes)
}

func formatListProgressAndSticker(movie *database.Movie, compatMode bool) (progressStr, sticker string) {
	if !compatMode {
		return fmt.Sprintf("DL %d%%", movie.DownloadedPercentage), ""
	}
	convPct := movie.ConversionPercentage
	if movie.DownloadedPercentage >= 100 && (movie.ConversionStatus == "done" || movie.ConversionStatus == "skipped") &&
		convPct < 100 {
		convPct = 100
	}
	convStatus := movie.ConversionStatus
	if convStatus == "" {
		convStatus = "waiting"
	}
	tvStatus := movie.TvCompatibility
	if tvStatus == "" {
		tvStatus = "unknown"
	}
	progressStr = fmt.Sprintf("DL %d%% | CV %s %d%% | TV %s", movie.DownloadedPercentage, convStatus, convPct, tvStatus)
	switch movie.TvCompatibility {
	case "green":
		sticker = "🟢 "
	case "yellow":
		sticker = "🟡 "
	case "red":
		sticker = "🔴 "
	default:
		sticker = "⚪ "
	}
	return progressStr, sticker
}

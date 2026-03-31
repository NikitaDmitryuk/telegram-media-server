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
		messages = append(messages, buildMovieListLine(ctx, a, &movies[i], compatMode))
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

func buildMovieListLine(ctx context.Context, a *app.App, movie *database.Movie, compatMode bool) string {
	refreshMovieFileSizeFromDiskIfComplete(ctx, a, movie)
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

func refreshMovieFileSizeFromDiskIfComplete(ctx context.Context, a *app.App, movie *database.Movie) {
	if movie.FileSize != 0 || movie.DownloadedPercentage < 100 {
		return
	}
	sum, err := a.DB.RefreshMovieFileSizeFromDisk(ctx, movie.ID, a.Config.MoviePath)
	if err == nil && sum > 0 {
		movie.FileSize = sum
	}
}

func formatListMovieSizeGB(movie *database.Movie) string {
	if movie.FileSize == 0 {
		return "—" // unknown size (e.g. magnet before metadata)
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
		return fmt.Sprintf("%d%%", movie.DownloadedPercentage), ""
	}
	convPct := movie.ConversionPercentage
	if movie.DownloadedPercentage >= 100 && (movie.ConversionStatus == "done" || movie.ConversionStatus == "skipped") &&
		convPct < 100 {
		convPct = 100
	}
	progressStr = fmt.Sprintf("%d/%d", movie.DownloadedPercentage, convPct)
	switch movie.TvCompatibility {
	case "green":
		sticker = "🟢 "
	case "yellow":
		sticker = "🟡 "
	case "red":
		sticker = "🔴 "
	}
	return progressStr, sticker
}

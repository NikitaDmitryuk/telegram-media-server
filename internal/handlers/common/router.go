package common

import (
	"context"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/auth"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/callbacks"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/downloads"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/movies"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/ui"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func Router(bot *tmsbot.Bot, update *tgbotapi.Update) {
	if update.CallbackQuery != nil {
		callbacks.HandleCallbackQuery(bot, update)
		return
	}

	if update.Message == nil {
		return
	}

	auth.LoggingMiddleware(update)

	if update.Message.IsCommand() {
		if !auth.CheckAccess(update) && update.Message.Command() != "login" {
			bot.SendErrorMessage(update.Message.Chat.ID, lang.Translate("general.user_prompts.unknown_user", nil))
			return
		}
		handleCommand(bot, update)
		return
	}

	if !auth.CheckAccess(update) {
		bot.SendErrorMessage(update.Message.Chat.ID, lang.Translate("general.user_prompts.unknown_user", nil))
		return
	}

	handleMessage(bot, update)
}

func handleCommand(bot *tmsbot.Bot, update *tgbotapi.Update) {
	command := update.Message.Command()
	switch command {
	case "login":
		auth.LoginHandler(bot, update)
	case "start":
		ui.SendMainMenu(bot, update.Message.Chat.ID, lang.Translate("general.commands.start", nil))
	case "ls":
		movies.ListMoviesHandler(bot, update)
	case "rm":
		movies.DeleteMoviesHandler(bot, update)
	case "temp":
		auth.GenerateTempPasswordHandler(bot, update)
	default:
		bot.SendErrorMessage(update.Message.Chat.ID, lang.Translate("error.commands.unknown_command", nil))
	}
}

func handleMessage(bot *tmsbot.Bot, update *tgbotapi.Update) {
	text := update.Message.Text
	chatID := update.Message.Chat.ID

	switch text {
	case lang.Translate("general.interface.list_movies", nil):
		movies.ListMoviesHandler(bot, update)
	case lang.Translate("general.interface.delete_movie", nil):
		movieList, err := database.GlobalDB.GetMovieList(context.Background())
		if err != nil {
			bot.SendErrorMessage(chatID, lang.Translate("error.movies.fetch_error", nil))
			return
		}
		movies.SendDeleteMovieMenu(bot, chatID, movieList)
	default:
		handleUnknownMessage(bot, update, text, chatID)
	}
}

func handleUnknownMessage(bot *tmsbot.Bot, update *tgbotapi.Update, text string, chatID int64) {
	if IsValidLink(text) {
		downloads.HandleDownloadLink(bot, update)
	} else if doc := update.Message.Document; doc != nil && IsTorrentFile(doc.FileName) {
		downloads.HandleTorrentFile(bot, update)
	} else {
		bot.SendErrorMessage(chatID, lang.Translate("error.commands.unknown_command", nil))
	}
}

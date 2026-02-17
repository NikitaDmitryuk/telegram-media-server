package common

import (
	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/auth"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/callbacks"
	tmsdownloads "github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/downloads"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/movies"
	tmssession "github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/session"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/ui"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func Router(
	a *app.App,
	update *tgbotapi.Update,
) {
	if update.CallbackQuery != nil {
		if tmssession.HandleTorrentSearchCallback(a, update) {
			return
		}
		callbacks.HandleCallbackQuery(a, update)
		return
	}

	if update.Message == nil {
		return
	}

	auth.LoggingMiddleware(update)

	if update.Message.IsCommand() {
		if !auth.CheckAccess(update, a.DB) && update.Message.Command() != "login" {
			a.Bot.SendMessage(update.Message.Chat.ID, lang.Translate("general.user_prompts.unknown_user", nil), nil)
			return
		}
		handleCommand(a, update)
		return
	}

	if !auth.CheckAccess(update, a.DB) {
		a.Bot.SendMessage(update.Message.Chat.ID, lang.Translate("general.user_prompts.unknown_user", nil), nil)
		return
	}

	handleMessage(a, update)
}

func handleCommand(
	a *app.App,
	update *tgbotapi.Update,
) {
	command := update.Message.Command()
	switch command {
	case "login":
		auth.LoginHandler(a, update)
	case "start":
		ui.SendMainMenu(a.Bot, update.Message.Chat.ID, lang.Translate("general.commands.start", nil))
	case "ls":
		movies.ListMoviesHandler(a, update)
	case "rm":
		movies.DeleteMoviesHandler(a, update)
	case "temp":
		auth.GenerateTempPasswordHandler(a, update)
	default:
		a.Bot.SendMessage(update.Message.Chat.ID, lang.Translate("error.commands.unknown_command", nil), nil)
	}
}

func handleMessage(
	a *app.App,
	update *tgbotapi.Update,
) {
	text := update.Message.Text
	chatID := update.Message.Chat.ID

	switch text {
	case lang.Translate("general.interface.list_movies", nil):
		movies.ListMoviesHandler(a, update)
	case lang.Translate("general.interface.delete_movie", nil):
		movies.SendDeleteMovieMenuFromDB(a, chatID)
	case lang.Translate("general.interface.search_torrents", nil):
		tmssession.StartTorrentSearch(a.Bot, chatID)
	default:
		s, sess := tmssession.GetSearchSession(chatID)
		if s != nil && sess != nil {
			switch sess.Stage {
			case "await_query":
				tmssession.HandleTorrentSearchQuery(a, update)
				return
			case "show_results":
				switch text {
				case lang.Translate("general.torrent_search.more", nil):
					tmssession.HandleTorrentMore(a.Bot, chatID)
					return
				case lang.Translate("general.torrent_search.back", nil):
					tmssession.HandleTorrentBack(a.Bot, chatID)
					return
				case lang.Translate("general.torrent_search.cancel", nil):
					tmssession.HandleTorrentCancel(a.Bot, chatID)
					return
				}
			}
		}
		handleUnknownMessage(a, update, text, chatID)
	}
}

func handleUnknownMessage(
	a *app.App,
	update *tgbotapi.Update,
	text string,
	chatID int64,
) {
	if IsValidLink(text) {
		tmsdownloads.HandleDownloadLink(a, update)
	} else if doc := update.Message.Document; doc != nil && IsTorrentFile(doc.FileName) {
		tmsdownloads.HandleTorrentFile(a, update)
	} else {
		a.Bot.SendMessage(chatID, lang.Translate("error.commands.unknown_command", nil), nil)
	}
}

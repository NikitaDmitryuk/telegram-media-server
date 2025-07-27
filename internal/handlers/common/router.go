package common

import (
	"context"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
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
	bot *tmsbot.Bot,
	update *tgbotapi.Update,
	db database.Database,
	cfg *config.Config,
	downloadManager *tmsdmanager.DownloadManager,
) {
	if update.CallbackQuery != nil {
		if tmssession.HandleTorrentSearchCallback(bot, update, cfg, db, downloadManager) {
			return
		}
		callbacks.HandleCallbackQuery(bot, update, db, cfg, downloadManager)
		return
	}

	if update.Message == nil {
		return
	}

	auth.LoggingMiddleware(update)

	if update.Message.IsCommand() {
		if !auth.CheckAccess(update, db) && update.Message.Command() != "login" {
			bot.SendMessage(update.Message.Chat.ID, lang.Translate("general.user_prompts.unknown_user", nil), nil)
			return
		}
		handleCommand(bot, update, db, cfg, downloadManager)
		return
	}

	if !auth.CheckAccess(update, db) {
		bot.SendMessage(update.Message.Chat.ID, lang.Translate("general.user_prompts.unknown_user", nil), nil)
		return
	}

	handleMessage(bot, update, db, cfg, downloadManager)
}

func handleCommand(
	bot *tmsbot.Bot,
	update *tgbotapi.Update,
	db database.Database,
	cfg *config.Config,
	downloadManager *tmsdmanager.DownloadManager,
) {
	command := update.Message.Command()
	switch command {
	case "login":
		auth.LoginHandler(bot, update, db, cfg)
	case "start":
		ui.SendMainMenu(bot, update.Message.Chat.ID, lang.Translate("general.commands.start", nil))
	case "ls":
		movies.ListMoviesHandler(bot, update, db, cfg)
	case "rm":
		movies.DeleteMoviesHandler(bot, update, db, cfg.MoviePath, downloadManager)
	case "temp":
		auth.GenerateTempPasswordHandler(bot, update, db)
	default:
		bot.SendMessage(update.Message.Chat.ID, lang.Translate("error.commands.unknown_command", nil), nil)
	}
}

func handleMessage(
	bot *tmsbot.Bot,
	update *tgbotapi.Update,
	db database.Database,
	cfg *config.Config,
	downloadManager *tmsdmanager.DownloadManager,
) {
	text := update.Message.Text
	chatID := update.Message.Chat.ID

	switch text {
	case lang.Translate("general.interface.list_movies", nil):
		movies.ListMoviesHandler(bot, update, db, cfg)
	case lang.Translate("general.interface.delete_movie", nil):
		movieList, err := db.GetMovieList(context.Background())
		if err != nil {
			bot.SendMessage(chatID, lang.Translate("error.movies.fetch_error", nil), nil)
			return
		}
		movies.SendDeleteMovieMenu(bot, chatID, movieList)
	case lang.Translate("general.interface.search_torrents", nil):
		tmssession.StartTorrentSearch(bot, chatID)
	default:
		s, sess := tmssession.GetSearchSession(chatID)
		if s != nil && sess != nil {
			switch sess.Stage {
			case "await_query":
				tmssession.HandleTorrentSearchQuery(bot, update, cfg)
				return
			case "show_results":
				switch text {
				case lang.Translate("general.torrent_search.more", nil):
					s.Offset += 5
					tmssession.ShowTorrentSearchResults(bot, chatID)
					return
				case lang.Translate("general.torrent_search.cancel", nil):
					for _, msgID := range s.MessageIDs {
						_ = bot.DeleteMessage(chatID, msgID)
					}
					tmssession.DeleteSearchSession(chatID)
					ui.SendMainMenu(bot, chatID, lang.Translate("general.commands.start", nil))
					return
				}
			}
		}
		handleUnknownMessage(bot, update, text, chatID, cfg, db, downloadManager)
	}
}

func handleUnknownMessage(
	bot *tmsbot.Bot,
	update *tgbotapi.Update,
	text string,
	chatID int64,
	cfg *config.Config,
	db database.Database,
	downloadManager *tmsdmanager.DownloadManager,
) {
	if IsValidLink(text) {
		tmsdownloads.HandleDownloadLink(bot, update, cfg, db, downloadManager)
	} else if doc := update.Message.Document; doc != nil && IsTorrentFile(doc.FileName) {
		tmsdownloads.HandleTorrentFile(bot, update, cfg, db, downloadManager)
	} else {
		bot.SendMessage(chatID, lang.Translate("error.commands.unknown_command", nil), nil)
	}
}

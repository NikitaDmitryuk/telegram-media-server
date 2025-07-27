package session

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
	torrent "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/torrent"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/downloads"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/ui"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/prowlarr"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

const (
	searchPageSize  = 40
	displayPageSize = 4
	gbDivisor       = 1024.0 * 1024.0 * 1024.0
)

type SearchSession struct {
	Query      string
	Results    []prowlarr.TorrentSearchResult
	Offset     int
	MessageIDs []int
}

var searchSessionManager = NewSessionManager()

func GetSearchSession(chatID int64) (*SearchSession, *Session) {
	sess := searchSessionManager.Get(chatID)
	if sess == nil {
		return nil, nil
	}
	ss, ok := sess.Data["torrent_search"].(*SearchSession)
	if !ok {
		return nil, sess
	}
	return ss, sess
}

func setSearchSession(chatID int64, ss *SearchSession, stage string) {
	sess := searchSessionManager.Get(chatID)
	if sess == nil {
		sess = &Session{
			ChatID: chatID,
			Data:   make(map[string]any),
		}
	}
	sess.Data["torrent_search"] = ss
	sess.Stage = stage
	sess.LastActive = time.Now()
	searchSessionManager.Set(chatID, sess)
}

func DeleteSearchSession(chatID int64) {
	searchSessionManager.Delete(chatID)
}

func StartTorrentSearch(bot *tmsbot.Bot, chatID int64) {
	ss := &SearchSession{}
	setSearchSession(chatID, ss, "await_query")
	msg := tgbotapi.NewMessage(chatID, lang.Translate("general.torrent_search.enter_query", nil))
	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
	bot.SendMessage(chatID, lang.Translate("general.torrent_search.enter_query", nil), ui.GetTorrentSearchKeyboard(false))
}

func HandleTorrentSearchQuery(bot *tmsbot.Bot, update *tgbotapi.Update, config *tmsconfig.Config) {
	chatID := update.Message.Chat.ID
	ss, sess := GetSearchSession(chatID)
	if sess == nil || sess.Stage != "await_query" {
		return
	}
	query := update.Message.Text
	if query == lang.Translate("general.torrent_search.cancel", nil) {
		DeleteSearchSession(chatID)
		ui.SendMainMenuNoText(bot, chatID)
		return
	}
	if query == "" {
		bot.SendMessage(chatID, lang.Translate("general.torrent_search.empty_query", nil), ui.GetEmptyKeyboard())
		return
	}

	if config.ProwlarrURL == "" || config.ProwlarrAPIKey == "" {
		bot.SendMessage(chatID, lang.Translate("general.torrent_search.not_configured", nil), ui.GetEmptyKeyboard())
		DeleteSearchSession(chatID)
		ui.SendMainMenuNoText(bot, chatID)
		return
	}
	client := prowlarr.NewProwlarr(config.ProwlarrURL, config.ProwlarrAPIKey)
	page, err := client.SearchTorrents(query, 0, searchPageSize, nil, nil)
	if err != nil {
		logutils.Log.WithError(err).Error("Prowlarr search failed")
		bot.SendMessage(chatID, lang.Translate("general.torrent_search.failed", nil), ui.GetEmptyKeyboard())
		DeleteSearchSession(chatID)
		ui.SendMainMenuNoText(bot, chatID)
		return
	}
	if len(page.Results) == 0 {
		bot.SendMessage(chatID, lang.Translate("general.torrent_search.not_found", nil), ui.GetEmptyKeyboard())
		DeleteSearchSession(chatID)
		ui.SendMainMenuNoText(bot, chatID)
		return
	}

	sort.Slice(page.Results, func(i, j int) bool {
		return page.Results[i].Peers > page.Results[j].Peers
	})
	ss.Query = query
	ss.Results = page.Results
	ss.Offset = 0
	ss.MessageIDs = nil
	setSearchSession(chatID, ss, "show_results")
	ShowTorrentSearchResults(bot, chatID)
}

func ShowTorrentSearchResults(bot *tmsbot.Bot, chatID int64) {
	ss, sess := GetSearchSession(chatID)
	if sess == nil || sess.Stage != "show_results" {
		return
	}

	for _, msgID := range ss.MessageIDs {
		_ = bot.DeleteMessage(chatID, msgID)
	}
	ss.MessageIDs = nil
	results := ss.Results
	from := ss.Offset
	to := from + displayPageSize
	if to > len(results) {
		to = len(results)
	}
	for i := from; i < to; i++ {
		t := results[i]
		text := fmt.Sprintf("%s\nSize: %.2f GB",
			t.Title,
			float64(t.Size)/gbDivisor,
		)
		btn := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					lang.Translate("general.torrent_search.download", nil),
					fmt.Sprintf("torrent_search_download:%d", i),
				),
			),
		)
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ReplyMarkup = btn
		m, _ := bot.Api.Send(msg)
		ss.MessageIDs = append(ss.MessageIDs, m.MessageID)
	}

	hasMore := to < len(results)
	bot.SendMessage(chatID, lang.Translate("general.torrent_search.choose", nil), ui.GetTorrentSearchKeyboard(hasMore))

	setSearchSession(chatID, ss, "show_results")
}

func handleTorrentDownloadCallback(bot *tmsbot.Bot, chatID int64, data string, config *tmsconfig.Config) (string, error) {
	ss, _ := GetSearchSession(chatID)
	if ss == nil {
		return "", errors.New(lang.Translate("general.torrent_search.session_expired", nil))
	}
	var idx int
	_, err := fmt.Sscanf(data, "torrent_search_download:%d", &idx)
	if err != nil || idx < 0 || idx >= len(ss.Results) {
		return "", errors.New(lang.Translate("general.torrent_search.invalid_choice", nil))
	}
	candidate := ss.Results[idx]
	client := prowlarr.NewProwlarr(config.ProwlarrURL, config.ProwlarrAPIKey)
	fileBytes, err := client.GetTorrentFile(candidate.TorrentURL)
	if err != nil {
		return "", errors.New(lang.Translate("general.torrent_search.download_failed", nil))
	}

	fileName := uuid.New().String() + ".torrent"
	if err := bot.SaveFile(fileName, fileBytes); err != nil {
		return "", errors.New(lang.Translate("general.torrent_search.save_failed", nil))
	}
	return fileName, nil
}

func handleTorrentCancelCallback(bot *tmsbot.Bot, chatID int64) bool {
	ss, _ := GetSearchSession(chatID)
	if ss != nil {
		for _, msgID := range ss.MessageIDs {
			_ = bot.DeleteMessage(chatID, msgID)
		}
		DeleteSearchSession(chatID)
	}
	ui.SendMainMenuNoText(bot, chatID)
	return true
}

func handleTorrentMoreCallback(bot *tmsbot.Bot, chatID int64) bool {
	ss, _ := GetSearchSession(chatID)
	if ss != nil {
		ss.Offset += 5
		setSearchSession(chatID, ss, "show_results")
		ShowTorrentSearchResults(bot, chatID)
	}
	return true
}

func HandleTorrentSearchCallback(
	bot *tmsbot.Bot,
	update *tgbotapi.Update,
	config *tmsconfig.Config,
	db database.Database,
	downloadManager *tmsdmanager.DownloadManager,
) bool {
	if update.CallbackQuery == nil {
		return false
	}
	data := update.CallbackQuery.Data
	chatID := update.CallbackQuery.Message.Chat.ID

	if strings.HasPrefix(data, "torrent_search_download:") {
		fileName, err := handleTorrentDownloadCallback(bot, chatID, data, config)
		if err != nil {
			bot.SendMessage(chatID, err.Error(), ui.GetEmptyKeyboard())
			if update.CallbackQuery != nil {
				bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
			}
			return true
		}
		ss, _ := GetSearchSession(chatID)
		if ss != nil {
			for _, msgID := range ss.MessageIDs {
				_ = bot.DeleteMessage(chatID, msgID)
			}
		}
		downloaderInstance := torrent.NewAria2Downloader(bot, fileName, config.MoviePath)
		if downloaderInstance == nil {
			bot.SendMessage(chatID, lang.Translate("error.file_management.unsupported_type", nil), ui.GetEmptyKeyboard())
			if update.CallbackQuery != nil {
				bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
			}
			return true
		}
		DeleteSearchSession(chatID)
		downloads.HandleDownload(bot, chatID, downloaderInstance, config, db, downloadManager)
		ui.SendMainMenuNoText(bot, chatID)
		if update.CallbackQuery != nil {
			bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
		}
		return true
	}
	if data == "torrent_search_cancel" {
		return handleTorrentCancelCallback(bot, chatID)
	}
	if data == "torrent_search_more" {
		return handleTorrentMoreCallback(bot, chatID)
	}
	return false
}

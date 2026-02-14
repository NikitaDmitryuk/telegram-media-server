package session

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
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
	searchCacheTTL  = 60 * time.Minute

	stageAwaitQuery  = "await_query"
	stageShowResults = "show_results"
)

type SearchSession struct {
	Query      string
	Results    []prowlarr.TorrentSearchResult
	Offset     int
	MessageIDs []int
}

// searchCacheEntry stores sorted Prowlarr search results with a creation timestamp.
type searchCacheEntry struct {
	results []prowlarr.TorrentSearchResult
	created time.Time
}

var (
	searchSessionManager = NewSessionManager()
	searchCache          = make(map[string]*searchCacheEntry)
	searchCacheMu        sync.Mutex
)

func getCachedResults(query string) ([]prowlarr.TorrentSearchResult, bool) {
	key := strings.TrimSpace(strings.ToLower(query))
	searchCacheMu.Lock()
	defer searchCacheMu.Unlock()

	if entry, ok := searchCache[key]; ok && time.Since(entry.created) < searchCacheTTL {
		logutils.Log.WithField("query", query).Debug("Prowlarr search cache hit")
		// Return a copy so callers don't mutate the cache.
		cp := make([]prowlarr.TorrentSearchResult, len(entry.results))
		copy(cp, entry.results)
		return cp, true
	}
	return nil, false
}

func setCachedResults(query string, results []prowlarr.TorrentSearchResult) {
	key := strings.TrimSpace(strings.ToLower(query))
	searchCacheMu.Lock()
	defer searchCacheMu.Unlock()

	// Evict expired entries.
	now := time.Now()
	for k, v := range searchCache {
		if now.Sub(v.created) > searchCacheTTL {
			delete(searchCache, k)
		}
	}

	searchCache[key] = &searchCacheEntry{
		results: results,
		created: now,
	}
}

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
	setSearchSession(chatID, ss, stageAwaitQuery)
	bot.SendMessage(chatID, lang.Translate("general.torrent_search.enter_query", nil), ui.GetTorrentSearchKeyboard(false, false))
}

func HandleTorrentSearchQuery(bot *tmsbot.Bot, update *tgbotapi.Update, config *tmsconfig.Config) {
	chatID := update.Message.Chat.ID
	ss, sess := GetSearchSession(chatID)
	if sess == nil || sess.Stage != stageAwaitQuery {
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

	// Try cache first.
	if cached, ok := getCachedResults(query); ok {
		ss.Query = query
		ss.Results = cached
		ss.Offset = 0
		ss.MessageIDs = nil
		setSearchSession(chatID, ss, stageShowResults)
		ShowTorrentSearchResults(bot, chatID)
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

	// Cache sorted results.
	setCachedResults(query, page.Results)

	ss.Query = query
	ss.Results = page.Results
	ss.Offset = 0
	ss.MessageIDs = nil
	setSearchSession(chatID, ss, stageShowResults)
	ShowTorrentSearchResults(bot, chatID)
}

func ShowTorrentSearchResults(bot *tmsbot.Bot, chatID int64) {
	ss, sess := GetSearchSession(chatID)
	if sess == nil || sess.Stage != stageShowResults {
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
	hasBack := true // always show Back: first page → new query, later pages → previous page
	bot.SendMessage(chatID, lang.Translate("general.torrent_search.choose", nil), ui.GetTorrentSearchKeyboard(hasMore, hasBack))

	setSearchSession(chatID, ss, stageShowResults)
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

// HandleTorrentMore advances to the next page of search results.
func HandleTorrentMore(bot *tmsbot.Bot, chatID int64) {
	ss, _ := GetSearchSession(chatID)
	if ss == nil {
		return
	}
	ss.Offset += displayPageSize
	setSearchSession(chatID, ss, stageShowResults)
	ShowTorrentSearchResults(bot, chatID)
}

// HandleTorrentBack navigates to the previous page, or back to query input on the first page.
func HandleTorrentBack(bot *tmsbot.Bot, chatID int64) {
	ss, _ := GetSearchSession(chatID)
	if ss == nil {
		return
	}
	if ss.Offset > 0 {
		ss.Offset -= displayPageSize
		if ss.Offset < 0 {
			ss.Offset = 0
		}
		setSearchSession(chatID, ss, stageShowResults)
		ShowTorrentSearchResults(bot, chatID)
	} else {
		// First page: go back to query input so user can search again.
		for _, msgID := range ss.MessageIDs {
			_ = bot.DeleteMessage(chatID, msgID)
		}
		ss.MessageIDs = nil
		setSearchSession(chatID, ss, stageAwaitQuery)
		bot.SendMessage(chatID, lang.Translate("general.torrent_search.enter_query", nil), ui.GetTorrentSearchKeyboard(false, false))
	}
}

// HandleTorrentCancel cleans up the search session and returns to the main menu.
func HandleTorrentCancel(bot *tmsbot.Bot, chatID int64) {
	ss, _ := GetSearchSession(chatID)
	if ss != nil {
		for _, msgID := range ss.MessageIDs {
			_ = bot.DeleteMessage(chatID, msgID)
		}
		DeleteSearchSession(chatID)
	}
	ui.SendMainMenuNoText(bot, chatID)
}

func handleTorrentMoreCallback(bot *tmsbot.Bot, chatID int64) bool {
	HandleTorrentMore(bot, chatID)
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
		downloaderInstance := torrent.NewAria2Downloader(bot, fileName, config.MoviePath, config)
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
	if data == "torrent_search_back" {
		HandleTorrentBack(bot, chatID)
		bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
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

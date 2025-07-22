package downloads

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"strings"

	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	torrent "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/torrent"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/handlers/ui"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/prowlarr"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

const (
	searchPageSize  = 50
	displayPageSize = 5
	mbDivisor       = 1024.0 * 1024.0
	gbDivisor       = 1024.0 * 1024.0 * 1024.0
)

type SearchSession struct {
	ChatID     int64
	Query      string
	Results    []prowlarr.TorrentSearchResult
	Offset     int
	MessageIDs []int
	Stage      string
	LastActive time.Time
}

type SearchSessionManager struct {
	mu       sync.Mutex
	sessions map[int64]*SearchSession
}

var searchSessionManager = &SearchSessionManager{
	sessions: make(map[int64]*SearchSession),
}

func (m *SearchSessionManager) Get(chatID int64) *SearchSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[chatID]
}

func (m *SearchSessionManager) Set(chatID int64, s *SearchSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[chatID] = s
}

func (m *SearchSessionManager) Delete(chatID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, chatID)
}

func (m *SearchSessionManager) Cleanup(expire time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for k, v := range m.sessions {
		if now.Sub(v.LastActive) > expire {
			delete(m.sessions, k)
		}
	}
}

func GetSearchSession(chatID int64) *SearchSession {
	return searchSessionManager.Get(chatID)
}

func DeleteSearchSession(chatID int64) {
	searchSessionManager.Delete(chatID)
}

func StartTorrentSearch(bot *tmsbot.Bot, chatID int64) {
	session := &SearchSession{
		ChatID:     chatID,
		Stage:      "await_query",
		LastActive: time.Now(),
	}
	searchSessionManager.Set(chatID, session)
	msg := tgbotapi.NewMessage(chatID, lang.Translate("general.torrent_search.enter_query", nil))
	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
	bot.Send(msg)
}

func HandleTorrentSearchQuery(bot *tmsbot.Bot, update *tgbotapi.Update) {
	chatID := update.Message.Chat.ID
	session := searchSessionManager.Get(chatID)
	if session == nil || session.Stage != "await_query" {
		return
	}
	query := update.Message.Text
	if query == "" {
		bot.SendErrorMessage(chatID, lang.Translate("general.torrent_search.empty_query", nil))
		return
	}

	if tmsconfig.GlobalConfig.ProwlarrURL == "" || tmsconfig.GlobalConfig.ProwlarrAPIKey == "" {
		bot.SendErrorMessage(chatID, lang.Translate("general.torrent_search.not_configured", nil))
		searchSessionManager.Delete(chatID)
		ui.SendMainMenuNoText(bot, chatID)
		return
	}
	client := prowlarr.NewProwlarr(tmsconfig.GlobalConfig.ProwlarrURL, tmsconfig.GlobalConfig.ProwlarrAPIKey)
	page, err := client.SearchTorrents(query, 0, searchPageSize, nil, nil)
	if err != nil {
		logutils.Log.WithError(err).Error("Prowlarr search failed")
		bot.SendErrorMessage(chatID, lang.Translate("general.torrent_search.failed", nil))
		searchSessionManager.Delete(chatID)
		ui.SendMainMenuNoText(bot, chatID)
		return
	}
	if len(page.Results) == 0 {
		bot.SendErrorMessage(chatID, lang.Translate("general.torrent_search.not_found", nil))
		searchSessionManager.Delete(chatID)
		ui.SendMainMenuNoText(bot, chatID)
		return
	}

	sort.Slice(page.Results, func(i, j int) bool {
		return page.Results[i].Peers > page.Results[j].Peers
	})
	session.Query = query
	session.Results = page.Results
	session.Offset = 0
	session.Stage = "show_results"
	session.LastActive = time.Now()
	searchSessionManager.Set(chatID, session)
	ShowTorrentSearchResults(bot, chatID)
}

func ShowTorrentSearchResults(bot *tmsbot.Bot, chatID int64) {
	session := searchSessionManager.Get(chatID)
	if session == nil || session.Stage != "show_results" {
		return
	}

	for _, msgID := range session.MessageIDs {
		_ = bot.DeleteMessage(chatID, msgID)
	}
	session.MessageIDs = nil
	results := session.Results
	from := session.Offset
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
		session.MessageIDs = append(session.MessageIDs, m.MessageID)
	}

	var menuBtns []tgbotapi.KeyboardButton
	if to < len(results) {
		menuBtns = append(menuBtns, tgbotapi.NewKeyboardButton(lang.Translate("general.torrent_search.more", nil)))
	}
	menuBtns = append(menuBtns, tgbotapi.NewKeyboardButton(lang.Translate("general.torrent_search.cancel", nil)))
	menu := tgbotapi.NewReplyKeyboard(menuBtns)
	menu.OneTimeKeyboard = true
	menu.ResizeKeyboard = true

	replyMarkupConfig := tgbotapi.NewMessage(chatID, lang.Translate("general.torrent_search.choose", nil))
	replyMarkupConfig.ReplyMarkup = menu
	bot.Send(replyMarkupConfig)

	searchSessionManager.Set(chatID, session)
}

func handleTorrentDownloadCallback(bot *tmsbot.Bot, chatID int64, data string) bool {
	session := GetSearchSession(chatID)
	if session == nil {
		bot.SendErrorMessage(chatID, lang.Translate("general.torrent_search.session_expired", nil))
		return true
	}
	var idx int
	_, err := fmt.Sscanf(data, "torrent_search_download:%d", &idx)
	if err != nil || idx < 0 || idx >= len(session.Results) {
		bot.SendErrorMessage(chatID, lang.Translate("general.torrent_search.invalid_choice", nil))
		return true
	}
	candidate := session.Results[idx]
	client := prowlarr.NewProwlarr(tmsconfig.GlobalConfig.ProwlarrURL, tmsconfig.GlobalConfig.ProwlarrAPIKey)
	fileBytes, err := client.GetTorrentFile(candidate.TorrentURL)
	if err != nil {
		bot.SendErrorMessage(chatID, lang.Translate("general.torrent_search.download_failed", nil))
		return true
	}

	fileName := uuid.New().String() + ".torrent"
	if err := tmsbot.SaveFile(fileName, fileBytes); err != nil {
		bot.SendErrorMessage(chatID, lang.Translate("general.torrent_search.save_failed", nil))
		return true
	}
	downloaderInstance := torrent.NewAria2Downloader(bot, fileName)
	if downloaderInstance == nil {
		bot.SendErrorMessage(chatID, lang.Translate("error.file_management.unsupported_type", nil))
		return true
	}
	handleDownload(bot, chatID, downloaderInstance)
	for _, msgID := range session.MessageIDs {
		_ = bot.DeleteMessage(chatID, msgID)
	}
	DeleteSearchSession(chatID)
	ui.SendMainMenuNoText(bot, chatID)
	return true
}

func handleTorrentCancelCallback(bot *tmsbot.Bot, chatID int64) bool {
	session := GetSearchSession(chatID)
	if session != nil {
		for _, msgID := range session.MessageIDs {
			_ = bot.DeleteMessage(chatID, msgID)
		}
		DeleteSearchSession(chatID)
	}
	ui.SendMainMenuNoText(bot, chatID)
	return true
}

func handleTorrentMoreCallback(bot *tmsbot.Bot, chatID int64) bool {
	session := GetSearchSession(chatID)
	if session != nil {
		session.Offset += 5
		ShowTorrentSearchResults(bot, chatID)
	}
	return true
}

func HandleTorrentSearchCallback(bot *tmsbot.Bot, update *tgbotapi.Update) bool {
	if update.CallbackQuery == nil {
		return false
	}
	data := update.CallbackQuery.Data
	chatID := update.CallbackQuery.Message.Chat.ID

	if strings.HasPrefix(data, "torrent_search_download:") {
		return handleTorrentDownloadCallback(bot, chatID, data)
	}
	if data == "torrent_search_cancel" {
		return handleTorrentCancelCallback(bot, chatID)
	}
	if data == "torrent_search_more" {
		return handleTorrentMoreCallback(bot, chatID)
	}
	return false
}

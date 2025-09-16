package router

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/container"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Router реализует простую маршрутизацию сообщений
type Router struct {
	container *container.Container
}

// NewRouter создает новый роутер
func NewRouter(appContainer *container.Container) *Router {
	return &Router{
		container: appContainer,
	}
}

// HandleUpdate обрабатывает обновления от Telegram
func (r *Router) HandleUpdate(ctx context.Context, update *tgbotapi.Update) {
	// Обрабатываем callback queries (нажатия на inline кнопки)
	if update.CallbackQuery != nil {
		r.handleCallbackQuery(ctx, update)
		return
	}

	// Обрабатываем обычные сообщения
	if update.Message == nil {
		return
	}

	logger.Log.WithFields(map[string]any{
		"user_id":  update.Message.From.ID,
		"username": update.Message.From.UserName,
		"chat_id":  update.Message.Chat.ID,
		"text":     update.Message.Text,
	}).Info("Received message")

	// Обрабатываем команды (включая логин)
	if update.Message.IsCommand() {
		r.handleCommand(ctx, update)
		return
	}

	// Проверяем доступ для не-команд
	if !r.checkAccess(update) {
		r.container.GetBot().SendMessage(
			update.Message.Chat.ID,
			lang.Translate("general.user_prompts.unknown_user", nil),
			nil,
		)
		return
	}

	// Обрабатываем торрент-файлы
	if update.Message.Document != nil && r.isTorrentFile(update.Message.Document.FileName) {
		r.handleTorrentFile(ctx, update)
		return
	}

	// Обрабатываем ссылки для загрузки
	if r.isValidLink(update.Message.Text) {
		r.handleDownloadLink(ctx, update)
		return
	}

	// Обрабатываем сообщения с кнопок клавиатуры
	if r.handleKeyboardMessage(update) {
		return
	}

	// Обрабатываем обычные сообщения
	r.container.GetBot().SendMessage(
		update.Message.Chat.ID,
		lang.Translate("error.commands.unknown_command", nil),
		nil,
	)
}

// handleKeyboardMessage обрабатывает сообщения с кнопок клавиатуры
func (r *Router) handleKeyboardMessage(update *tgbotapi.Update) bool {
	if update.Message == nil || update.Message.Text == "" {
		return false
	}

	messageText := update.Message.Text

	// Маршрутизируем по тексту кнопки
	switch messageText {
	case lang.Translate("general.interface.list_movies", nil):
		// Обрабатываем как команду /ls
		r.handleListMovies(update)
		return true
	case lang.Translate("general.interface.delete_movie", nil):
		// Обработка удаления фильма
		r.handleDeleteMovie(update)
		return true
	case lang.Translate("general.interface.search_torrents", nil):
		// Обработка поиска торрентов
		r.container.GetBot().SendMessage(
			update.Message.Chat.ID,
			lang.Translate("general.torrent_search.enter_query", nil),
			nil,
		)
		return true
	}

	return false
}

const (
	// FullDownloadPercentage процент полной загрузки
	FullDownloadPercentage = 100
)

// handleListMovies обрабатывает запрос списка фильмов
func (r *Router) handleListMovies(update *tgbotapi.Update) {
	movies, err := r.container.GetDatabase().GetMovieList(context.Background())
	if err != nil {
		r.container.GetBot().SendMessage(
			update.Message.Chat.ID,
			lang.Translate("error.movies.fetch_error", nil),
			nil,
		)
		return
	}

	if len(movies) == 0 {
		r.container.GetBot().SendMessage(
			update.Message.Chat.ID,
			lang.Translate("general.status_messages.empty_list", nil),
			nil,
		)
		return
	}

	var messages []string
	for _, movie := range movies {
		sizeGB := float64(movie.FileSize) / (1024 * 1024 * 1024)
		formattedSize := fmt.Sprintf("%.2f", sizeGB)
		messages = append(messages, lang.Translate("general.downloaded_list", map[string]any{
			"ID":       movie.ID,
			"Name":     movie.Name,
			"Progress": movie.DownloadedPercentage,
			"SizeGB":   formattedSize,
		}))
	}

	message := strings.Join(messages, "")

	r.container.GetBot().SendMessage(update.Message.Chat.ID, message, nil)
}

// handleDeleteMovie обрабатывает запрос удаления фильмов
func (r *Router) handleDeleteMovie(update *tgbotapi.Update) {
	movies, err := r.container.GetDatabase().GetMovieList(context.Background())
	if err != nil {
		r.container.GetBot().SendMessage(
			update.Message.Chat.ID,
			lang.Translate("error.movies.fetch_error", nil),
			nil,
		)
		return
	}

	if len(movies) == 0 {
		r.container.GetBot().SendMessage(
			update.Message.Chat.ID,
			lang.Translate("general.user_prompts.no_movies_to_delete", nil),
			nil,
		)
		return
	}

	// Создаем inline клавиатуру с кнопками для удаления каждого фильма
	markup := r.createDeleteMovieMenuMarkup(movies)

	// Отправляем сообщение с inline кнопками
	if err := r.container.GetBot().SendMessageWithMarkup(update.Message.Chat.ID, lang.Translate("general.user_prompts.delete_prompt", nil), markup); err != nil {
		logger.Log.WithError(err).Error("Failed to send delete movie menu")
	}
}

// handleCommand обрабатывает команды
func (r *Router) handleCommand(ctx context.Context, update *tgbotapi.Update) {
	command := update.Message.Command()
	chatID := update.Message.Chat.ID

	switch command {
	case "start":
		r.handleStartCommand(ctx, update)
	case "login":
		r.handleLoginCommand(ctx, update)
	case "ls":
		// Проверяем доступ
		if !r.checkAccess(update) {
			r.container.GetBot().SendMessage(chatID, lang.Translate("general.user_prompts.unknown_user", nil), nil)
			return
		}
		r.handleListMovies(update)
	case "rm":
		// Проверяем доступ
		if !r.checkAccess(update) {
			r.container.GetBot().SendMessage(chatID, lang.Translate("general.user_prompts.unknown_user", nil), nil)
			return
		}
		r.handleDeleteMovieCommand(ctx, update)
	case "temp":
		// Проверяем доступ и права администратора
		if !r.checkAccess(update) {
			r.container.GetBot().SendMessage(chatID, lang.Translate("general.user_prompts.unknown_user", nil), nil)
			return
		}
		if !r.isAdmin(update) {
			r.container.GetBot().SendMessage(chatID, lang.Translate("general.user_prompts.no_permission", nil), nil)
			return
		}
		r.handleTempPasswordCommand(ctx, update)
	default:
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.commands.unknown_command", nil), nil)
	}
}

// handleStartCommand обрабатывает команду /start
func (r *Router) handleStartCommand(ctx context.Context, update *tgbotapi.Update) {
	chatID := update.Message.Chat.ID

	// Создаем меню с кнопками
	keyboard := r.createMainMenuKeyboard()
	r.container.GetBot().SendMessage(chatID, lang.Translate("general.commands.start", nil), keyboard)
}

// handleLoginCommand обрабатывает команду /login
func (r *Router) handleLoginCommand(ctx context.Context, update *tgbotapi.Update) {
	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID
	userName := update.Message.From.UserName

	// Извлекаем пароль из команды
	args := strings.Fields(update.Message.Text)
	if len(args) < 2 {
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.auth.password_required", nil), nil)
		return
	}

	password := args[1]

	// Выполняем авторизацию через сервис
	authService := r.container.GetAuthService()
	result, err := authService.Login(ctx, password, userID, userName)
	if err != nil {
		logger.Log.WithError(err).Error("Login failed")
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.auth.login_failed", nil), nil)
		return
	}

	if !result.Success {
		if result.Message == "wrong_password" {
			r.container.GetBot().SendMessage(chatID, lang.Translate("error.auth.wrong_password", nil), nil)
		} else {
			r.container.GetBot().SendMessage(chatID, lang.Translate("error.auth.login_failed", nil), nil)
		}
		return
	}

	// Успешная авторизация
	keyboard := r.createMainMenuKeyboard()
	message := lang.Translate("general.auth.login_success", map[string]any{
		"Role": string(result.Role),
	})
	r.container.GetBot().SendMessage(chatID, message, keyboard)
}

// handleDeleteMovieCommand обрабатывает команду /rm
func (r *Router) handleDeleteMovieCommand(ctx context.Context, update *tgbotapi.Update) {
	chatID := update.Message.Chat.ID

	args := strings.Fields(update.Message.Text)
	if len(args) < 2 {
		// Показываем список фильмов для выбора
		r.handleDeleteMovie(update)
		return
	}

	movieService := r.container.GetMovieService()

	if args[1] == "all" {
		// Удаляем все фильмы
		err := movieService.DeleteAllMovies(ctx)
		if err != nil {
			logger.Log.WithError(err).Error("Failed to delete all movies")
			r.container.GetBot().SendMessage(chatID, lang.Translate("error.movies.delete_failed", nil), nil)
			return
		}
		r.container.GetBot().SendMessage(chatID, lang.Translate("general.movies.all_deleted", nil), nil)
		return
	}

	// Удаляем конкретный фильм по ID
	movieID, err := strconv.ParseUint(args[1], 10, 32)
	if err != nil {
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.movies.invalid_id", nil), nil)
		return
	}

	err = movieService.DeleteMovie(ctx, uint(movieID))
	if err != nil {
		logger.Log.WithError(err).Error("Failed to delete movie")
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.movies.delete_failed", nil), nil)
		return
	}

	r.container.GetBot().SendMessage(chatID, lang.Translate("general.movies.deleted", nil), nil)
}

// handleTempPasswordCommand обрабатывает команду /temp
func (r *Router) handleTempPasswordCommand(ctx context.Context, update *tgbotapi.Update) {
	chatID := update.Message.Chat.ID

	args := strings.Fields(update.Message.Text)
	if len(args) < 2 {
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.auth.duration_required", nil), nil)
		return
	}

	durationStr := args[1]
	duration, err := parseDuration(durationStr)
	if err != nil {
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.auth.invalid_duration", nil), nil)
		return
	}

	authService := r.container.GetAuthService()
	tempPassword, err := authService.GenerateTempPassword(ctx, duration)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to generate temporary password")
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.auth.temp_password_failed", nil), nil)
		return
	}

	message := lang.Translate("general.auth.temp_password_created", map[string]any{
		"Password": tempPassword,
		"Duration": durationStr,
	})
	r.container.GetBot().SendMessage(chatID, message, nil)
}

// handleDownloadLink обрабатывает ссылки для загрузки
func (r *Router) handleDownloadLink(ctx context.Context, update *tgbotapi.Update) {
	chatID := update.Message.Chat.ID
	link := update.Message.Text

	downloadService := r.container.GetDownloadService()
	err := downloadService.HandleVideoLink(ctx, link, chatID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to handle video link")
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.downloads.video_failed", nil), nil)
		return
	}

	r.container.GetBot().SendMessage(chatID, lang.Translate("general.downloads.video_started", nil), nil)
}

// handleTorrentFile обрабатывает торрент-файлы
func (r *Router) handleTorrentFile(ctx context.Context, update *tgbotapi.Update) {
	chatID := update.Message.Chat.ID
	document := update.Message.Document

	// Получаем прямую ссылку на файл
	fileURL, err := r.container.GetBot().GetFileDirectURL(document.FileID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get torrent file URL")
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.downloads.torrent_download_failed", nil), nil)
		return
	}

	// Загружаем файл через HTTP клиент
	httpClient := r.container.GetHTTPClient()
	response, err := httpClient.Get(fileURL)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to download torrent file")
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.downloads.torrent_download_failed", nil), nil)
		return
	}

	// Проверяем успешность HTTP запроса
	if response.IsError {
		logger.Log.WithField("status_code", response.StatusCode).Error("HTTP request failed for torrent file")
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.downloads.torrent_download_failed", nil), nil)
		return
	}

	downloadService := r.container.GetDownloadService()
	err = downloadService.HandleTorrentFile(ctx, response.Body, document.FileName, chatID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to handle torrent file")
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.downloads.torrent_failed", nil), nil)
		return
	}

	// Уведомление отправляется в DownloadService, не дублируем здесь
}

// handleCallbackQuery обрабатывает callback queries
func (r *Router) handleCallbackQuery(ctx context.Context, update *tgbotapi.Update) {
	callbackData := update.CallbackQuery.Data
	chatID := update.CallbackQuery.Message.Chat.ID
	userID := update.CallbackQuery.From.ID

	logger.Log.WithFields(map[string]any{
		"callback_data": callbackData,
		"chat_id":       chatID,
		"user_id":       userID,
	}).Info("Received callback query")

	// Проверяем доступ пользователя
	allowed, role, err := r.container.GetDatabase().IsUserAccessAllowed(ctx, userID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to check user access")
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.authentication.access_check_failed", nil), nil)
		r.container.GetBot().AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
		return
	}
	if !allowed {
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.authentication.access_denied", nil), nil)
		r.container.GetBot().AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
		return
	}

	// Маршрутизация по типу callback
	switch {
	case strings.HasPrefix(callbackData, "delete_movie:"):
		r.handleDeleteMovieCallback(ctx, update, chatID, string(role), callbackData)

	case callbackData == "cancel_delete_menu":
		r.handleCancelDeleteCallback(ctx, update)

	case callbackData == "list_movies":
		r.handleListMovies(update)

	case strings.HasPrefix(callbackData, "torrent_search_download:"):
		r.handleTorrentSearchDownloadCallback(ctx, update, callbackData)

	case callbackData == "torrent_search_cancel":
		r.handleTorrentSearchCancelCallback(ctx, update)

	case callbackData == "torrent_search_more":
		r.handleTorrentSearchMoreCallback(ctx, update)

	default:
		logger.Log.Warnf("Unknown callback data: %s", callbackData)
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.commands.unknown_command", nil), nil)
	}

	// Подтверждаем получение callback
	r.container.GetBot().AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
}

// handleDeleteMovieCallback обрабатывает callback удаления фильма
func (r *Router) handleDeleteMovieCallback(ctx context.Context, update *tgbotapi.Update, chatID int64, role string, callbackData string) {
	// Проверяем права доступа для удаления
	if role != "admin" && role != "regular" {
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.authentication.access_denied", nil), nil)
		return
	}

	movieIDStr := strings.TrimPrefix(callbackData, "delete_movie:")
	movieID, err := strconv.ParseUint(movieIDStr, 10, 32)
	if err != nil {
		logger.Log.WithError(err).Errorf("Invalid movie ID: %s", movieIDStr)
		return
	}

	logger.Log.WithFields(map[string]any{
		"callback_data": callbackData,
		"chat_id":       chatID,
		"movie_id":      movieID,
		"action":        "delete_callback_started",
	}).Info("Starting delete from callback")

	// Запускаем удаление асинхронно
	go func() {
		err := r.container.GetMovieService().DeleteMovie(ctx, uint(movieID))
		if err != nil {
			logger.Log.WithError(err).Error("Failed to delete movie")
			r.container.GetBot().SendMessage(chatID, lang.Translate("error.movies.delete_failed", nil), nil)
			return
		}

		// Обновляем меню удаления с оставшимися фильмами
		movieList, err := r.container.GetMovieService().GetMovieList(ctx)
		if err != nil {
			logger.Log.WithError(err).Error("Failed to get movie list for menu update")
			return
		}

		r.updateDeleteMenu(chatID, update.CallbackQuery.Message.MessageID, movieList)
	}()
}

// handleCancelDeleteCallback обрабатывает отмену удаления
func (r *Router) handleCancelDeleteCallback(ctx context.Context, update *tgbotapi.Update) {
	chatID := update.CallbackQuery.Message.Chat.ID
	messageID := update.CallbackQuery.Message.MessageID

	err := r.container.GetBot().DeleteMessage(chatID, messageID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to delete message")
	}
}

// handleTorrentSearchDownloadCallback обрабатывает загрузку торрента из поиска
func (r *Router) handleTorrentSearchDownloadCallback(ctx context.Context, update *tgbotapi.Update, callbackData string) {
	// TODO: Реализовать после добавления сессий торрент-поиска
	chatID := update.CallbackQuery.Message.Chat.ID
	r.container.GetBot().SendMessage(chatID, lang.Translate("error.general.not_implemented", nil), nil)
}

// handleTorrentSearchCancelCallback обрабатывает отмену поиска торрентов
func (r *Router) handleTorrentSearchCancelCallback(ctx context.Context, update *tgbotapi.Update) {
	// TODO: Реализовать после добавления сессий торрент-поиска
	chatID := update.CallbackQuery.Message.Chat.ID
	r.container.GetBot().SendMessage(chatID, lang.Translate("error.general.not_implemented", nil), nil)
}

// handleTorrentSearchMoreCallback обрабатывает запрос дополнительных результатов поиска
func (r *Router) handleTorrentSearchMoreCallback(ctx context.Context, update *tgbotapi.Update) {
	// TODO: Реализовать после добавления сессий торрент-поиска
	chatID := update.CallbackQuery.Message.Chat.ID
	r.container.GetBot().SendMessage(chatID, lang.Translate("error.general.not_implemented", nil), nil)
}

// updateDeleteMenu обновляет меню удаления фильмов
func (r *Router) updateDeleteMenu(chatID int64, messageID int, movieList []domain.Movie) {
	if len(movieList) == 0 {
		// Если фильмов больше нет, удаляем сообщение
		err := r.container.GetBot().DeleteMessage(chatID, messageID)
		if err != nil {
			logger.Log.WithError(err).Error("Failed to delete message")
		}
		r.container.GetBot().SendMessage(chatID, lang.Translate("error.movies.all_deleted", nil), nil)
		return
	}

	// Создаем новую разметку с кнопками
	markup := r.createDeleteMovieMenuMarkup(movieList)

	// Обновляем сообщение через SendMessageWithMarkup - более простой способ
	err := r.container.GetBot().DeleteMessage(chatID, messageID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to delete original message")
	}

	// Отправляем новое сообщение
	err = r.container.GetBot().SendMessageWithMarkup(chatID, lang.Translate("general.user_prompts.delete_prompt", nil), markup)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send updated delete menu")
	}
}

// createDeleteMovieMenuMarkup создает разметку с кнопками для удаления фильмов
func (r *Router) createDeleteMovieMenuMarkup(movieList []domain.Movie) tgbotapi.InlineKeyboardMarkup {
	var buttons [][]tgbotapi.InlineKeyboardButton

	for _, movie := range movieList {
		button := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("🗑 %s", movie.Name),
			fmt.Sprintf("delete_movie:%d", movie.ID),
		)
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{button})
	}

	// Добавляем кнопку отмены
	cancelButton := tgbotapi.NewInlineKeyboardButtonData(
		lang.Translate("general.interface.cancel", nil),
		"cancel_delete_menu",
	)
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{cancelButton})

	return tgbotapi.NewInlineKeyboardMarkup(buttons...)
}

// createMainMenuKeyboard создает основное меню с кнопками
func (r *Router) createMainMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(lang.Translate("general.interface.list_movies", nil)),
			tgbotapi.NewKeyboardButton(lang.Translate("general.interface.delete_movie", nil)),
			tgbotapi.NewKeyboardButton(lang.Translate("general.interface.search_torrents", nil)),
		),
	)
}

// checkAccess проверяет доступ пользователя
func (r *Router) checkAccess(update *tgbotapi.Update) bool {
	if update.Message == nil {
		return false
	}

	userID := update.Message.From.ID
	allowed, _, err := r.container.GetDatabase().IsUserAccessAllowed(context.Background(), userID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to check user access")
		return false
	}

	return allowed
}

// isAdmin проверяет, является ли пользователь администратором
func (r *Router) isAdmin(update *tgbotapi.Update) bool {
	if update.Message == nil {
		return false
	}

	userID := update.Message.From.ID
	_, role, err := r.container.GetDatabase().IsUserAccessAllowed(context.Background(), userID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to check user role")
		return false
	}

	return role == "admin"
}

// isValidLink проверяет, является ли текст валидной ссылкой
func (r *Router) isValidLink(text string) bool {
	if text == "" {
		return false
	}
	return strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") || strings.HasPrefix(text, "magnet:")
}

// isTorrentFile проверяет, является ли файл торрентом
func (r *Router) isTorrentFile(fileName string) bool {
	return strings.HasSuffix(strings.ToLower(fileName), ".torrent")
}

// parseDuration парсит строку длительности в time.Duration
func parseDuration(s string) (time.Duration, error) {
	switch s {
	case "30m":
		return 30 * time.Minute, nil
	case "1h":
		return time.Hour, nil
	case "3h":
		return 3 * time.Hour, nil
	case "6h":
		return 6 * time.Hour, nil
	case "12h":
		return 12 * time.Hour, nil
	case "1d":
		return 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported duration: %s", s)
	}
}

package router

import (
	"context"
	"fmt"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/container"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/transport/handlers"
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
func (r *Router) HandleUpdate(_ context.Context, update *tgbotapi.Update) {
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

	// Проверяем доступ
	if !r.checkAccess(update) && update.Message.Command() != "login" {
		r.container.GetBot().SendMessage(
			update.Message.Chat.ID,
			lang.Translate("general.user_prompts.unknown_user", nil),
			nil,
		)
		return
	}

	// Обрабатываем команды
	if update.Message.IsCommand() {
		handlers.HandleCommand(
			r.container.GetBot(),
			update,
			r.container.GetDatabase(),
			r.container.GetConfig(),
		)
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

	message := lang.Translate("general.movies.list_header", nil)
	for i, movie := range movies {
		status := lang.Translate("general.movies.status_pending", nil)
		if movie.DownloadedPercentage == FullDownloadPercentage {
			status = lang.Translate("general.movies.status_completed", nil)
		} else if movie.DownloadedPercentage > 0 {
			status = lang.Translate("general.movies.status_downloading", nil)
		}
		message += fmt.Sprintf("%d. %s %s (%d%%)\n", i+1, status, movie.Name, movie.DownloadedPercentage)
	}

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
			lang.Translate("general.status_messages.empty_list", nil),
			nil,
		)
		return
	}

	// Если есть фильмы, показываем список для выбора
	r.container.GetBot().SendMessage(
		update.Message.Chat.ID,
		lang.Translate("general.user_prompts.delete_prompt", nil),
		nil,
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

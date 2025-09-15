package router

import (
	"context"

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
	if update.Message == nil {
		return
	}

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

	// Обрабатываем обычные сообщения
	r.container.GetBot().SendMessage(
		update.Message.Chat.ID,
		lang.Translate("error.commands.unknown_command", nil),
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

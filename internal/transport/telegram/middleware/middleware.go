package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/errors"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// UpdateContext содержит контекст обработки обновления
type UpdateContext struct {
	Context   context.Context
	Update    *tgbotapi.Update
	Bot       domain.BotInterface
	Container domain.ServiceContainerInterface
	UserID    int64
	ChatID    int64
	Username  string
	UserRole  database.UserRole
	StartTime time.Time
}

// MiddlewareFunc представляет функцию middleware
type MiddlewareFunc func(*UpdateContext) error

// Chain представляет цепочку middleware
type Chain struct {
	middlewares []MiddlewareFunc
}

// NewChain создает новую цепочку middleware
func NewChain(middlewares ...MiddlewareFunc) *Chain {
	return &Chain{
		middlewares: middlewares,
	}
}

// Use добавляет middleware в цепочку
func (c *Chain) Use(middleware MiddlewareFunc) *Chain {
	c.middlewares = append(c.middlewares, middleware)
	return c
}

// Execute выполняет всю цепочку middleware
func (c *Chain) Execute(ctx *UpdateContext) error {
	for _, middleware := range c.middlewares {
		if err := middleware(ctx); err != nil {
			return err
		}
	}
	return nil
}

// LoggingMiddleware логирует входящие обновления
func LoggingMiddleware(ctx *UpdateContext) error {
	ctx.StartTime = time.Now()

	if ctx.Update == nil {
		logger.Log.Error("Update is nil in LoggingMiddleware")
		return errors.NewDomainError(errors.ErrorTypeValidation, "nil_update", "update is nil")
	}

	if ctx.Update.Message != nil {
		logger.Log.WithFields(map[string]any{
			"user_id":  ctx.Update.Message.From.ID,
			"username": ctx.Update.Message.From.UserName,
			"chat_id":  ctx.Update.Message.Chat.ID,
			"text":     ctx.Update.Message.Text,
		}).Info("Received message")

		ctx.UserID = ctx.Update.Message.From.ID
		ctx.ChatID = ctx.Update.Message.Chat.ID
		ctx.Username = ctx.Update.Message.From.UserName
	} else if ctx.Update.CallbackQuery != nil {
		logger.Log.WithFields(map[string]any{
			"user_id":       ctx.Update.CallbackQuery.From.ID,
			"username":      ctx.Update.CallbackQuery.From.UserName,
			"callback_data": ctx.Update.CallbackQuery.Data,
		}).Info("Received callback query")

		ctx.UserID = ctx.Update.CallbackQuery.From.ID
		ctx.ChatID = ctx.Update.CallbackQuery.Message.Chat.ID
		ctx.Username = ctx.Update.CallbackQuery.From.UserName
	}

	return nil
}

// AuthMiddleware проверяет авторизацию пользователя
func AuthMiddleware(ctx *UpdateContext) error {
	if ctx.ChatID == 0 {
		return errors.NewDomainError(errors.ErrorTypeValidation, "invalid_chat_id", "chat ID not found")
	}

	authService := ctx.Container.GetAuthService()
	allowed, role, err := authService.CheckAccess(ctx.Context, ctx.ChatID)
	if err != nil {
		logger.Log.WithError(err).WithField("chat_id", ctx.ChatID).Error("Failed to check user access")
		return errors.WrapDomainError(err, errors.ErrorTypeAuth, "access_check_failed", "failed to check user access")
	}

	if !allowed {
		logger.Log.WithField("chat_id", ctx.ChatID).Info("Access denied")
		return errors.ErrUnauthorized.WithDetails(map[string]any{
			"chat_id": ctx.ChatID,
		})
	}

	ctx.UserRole = role
	logger.Log.WithFields(map[string]any{
		"chat_id": ctx.ChatID,
		"role":    role,
	}).Debug("Access granted")

	return nil
}

// RateLimitMiddleware ограничивает частоту запросов
func RateLimitMiddleware(limiter domain.RateLimiterInterface) MiddlewareFunc {
	return func(ctx *UpdateContext) error {
		if limiter == nil {
			return nil // Rate limiting отключен
		}

		allowed := limiter.Allow(ctx.UserID)
		if !allowed {
			logger.Log.WithField("user_id", ctx.UserID).Warn("Rate limit exceeded")
			return errors.NewDomainError(errors.ErrorTypeBusiness, "rate_limit_exceeded", "rate limit exceeded").
				WithUserMessage("error.general.rate_limit_exceeded")
		}

		return nil
	}
}

// MetricsMiddleware собирает метрики
func MetricsMiddleware(metrics domain.MetricsInterface) MiddlewareFunc {
	return func(ctx *UpdateContext) error {
		if metrics == nil {
			return nil // Метрики отключены
		}

		// Увеличиваем счетчик обновлений
		metrics.IncrementCounter("telegram_updates_total", map[string]string{
			"type":      getUpdateType(ctx.Update),
			"user_role": string(ctx.UserRole),
		})

		return nil
	}
}

// RecoveryMiddleware восстанавливается после паники
func RecoveryMiddleware(ctx *UpdateContext) error {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.WithFields(map[string]any{
				"panic":   r,
				"chat_id": ctx.ChatID,
				"user_id": ctx.UserID,
			}).Error("Panic recovered in middleware")

			// Отправляем пользователю сообщение об ошибке
			errorMessage := lang.Translate("error.general.internal_error", nil)
			ctx.Bot.SendMessage(ctx.ChatID, errorMessage, nil)
		}
	}()

	return nil
}

// TimingMiddleware измеряет время обработки
func TimingMiddleware(metrics domain.MetricsInterface) MiddlewareFunc {
	return func(ctx *UpdateContext) error {
		if metrics == nil {
			return nil
		}

		// Записываем время в конце обработки
		defer func() {
			duration := time.Since(ctx.StartTime)
			metrics.RecordDuration("telegram_update_duration", duration, map[string]string{
				"type":      getUpdateType(ctx.Update),
				"user_role": string(ctx.UserRole),
			})

			logger.Log.WithFields(map[string]any{
				"duration_ms": duration.Milliseconds(),
				"chat_id":     ctx.ChatID,
				"type":        getUpdateType(ctx.Update),
			}).Debug("Update processed")
		}()

		return nil
	}
}

// ValidationMiddleware валидирует входящие данные
func ValidationMiddleware(ctx *UpdateContext) error {
	if ctx.Update == nil {
		return errors.NewDomainError(errors.ErrorTypeValidation, "nil_update", "update cannot be nil")
	}

	// Проверяем, что у нас есть либо сообщение, либо callback query
	if ctx.Update.Message == nil && ctx.Update.CallbackQuery == nil {
		return errors.NewDomainError(errors.ErrorTypeValidation, "invalid_update", "update must contain message or callback query")
	}

	// Дополнительные валидации
	if err := validateMessage(ctx.Update.Message); err != nil {
		return err
	}

	if err := validateCallbackQuery(ctx.Update.CallbackQuery); err != nil {
		return err
	}

	return nil
}

// validateMessage проверяет валидность сообщения
func validateMessage(message *tgbotapi.Message) error {
	if message == nil {
		return nil // Сообщение может отсутствовать
	}

	// Проверяем размер текста сообщения
	const maxMessageLength = 4096
	if len(message.Text) > maxMessageLength {
		return errors.NewDomainError(errors.ErrorTypeValidation, "message_too_long",
			fmt.Sprintf("message text exceeds maximum length of %d characters", maxMessageLength))
	}

	// Проверяем валидность chat ID
	if message.Chat.ID == 0 {
		return errors.NewDomainError(errors.ErrorTypeValidation, "invalid_chat_id", "chat ID cannot be zero")
	}

	// Проверяем валидность user ID (если есть пользователь)
	if message.From != nil && message.From.ID == 0 {
		return errors.NewDomainError(errors.ErrorTypeValidation, "invalid_user_id", "user ID cannot be zero")
	}

	return nil
}

// validateCallbackQuery проверяет валидность callback query
func validateCallbackQuery(callbackQuery *tgbotapi.CallbackQuery) error {
	if callbackQuery == nil {
		return nil // Callback query может отсутствовать
	}

	// Проверяем наличие ID
	if callbackQuery.ID == "" {
		return errors.NewDomainError(errors.ErrorTypeValidation, "invalid_callback_id", "callback query ID cannot be empty")
	}

	// Проверяем размер данных callback query
	const maxCallbackDataLength = 64
	if len(callbackQuery.Data) > maxCallbackDataLength {
		return errors.NewDomainError(errors.ErrorTypeValidation, "callback_data_too_long",
			fmt.Sprintf("callback data exceeds maximum length of %d characters", maxCallbackDataLength))
	}

	// Проверяем валидность пользователя
	if callbackQuery.From.ID == 0 {
		return errors.NewDomainError(errors.ErrorTypeValidation, "invalid_callback_user_id", "callback query user ID cannot be zero")
	}

	return nil
}

// getUpdateType определяет тип обновления для метрик
func getUpdateType(update *tgbotapi.Update) string {
	if update.Message != nil {
		if update.Message.IsCommand() {
			return "command"
		}
		if update.Message.Document != nil {
			return "document"
		}
		return "message"
	}
	if update.CallbackQuery != nil {
		return "callback"
	}
	return "unknown"
}

// DefaultMiddlewareChain создает стандартную цепочку middleware
func DefaultMiddlewareChain(
	_ domain.ServiceContainerInterface,
	metrics domain.MetricsInterface,
	rateLimiter domain.RateLimiterInterface,
) *Chain {
	return NewChain(
		RecoveryMiddleware,
		LoggingMiddleware,
		ValidationMiddleware,
		TimingMiddleware(metrics),
		MetricsMiddleware(metrics),
		RateLimitMiddleware(rateLimiter),
		// AuthMiddleware добавляется отдельно для команд, которые требуют авторизации
	)
}

// AuthRequiredChain создает цепочку с обязательной авторизацией
func AuthRequiredChain(
	_ domain.ServiceContainerInterface,
	metrics domain.MetricsInterface,
	rateLimiter domain.RateLimiterInterface,
) *Chain {
	return DefaultMiddlewareChain(nil, metrics, rateLimiter).Use(AuthMiddleware)
}

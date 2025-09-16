package context

import (
	"context"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/infrastructure/database"
)

// ContextKey представляет ключ для контекста
type ContextKey string

const (
	// Ключи для контекста
	UserIDKey    ContextKey = "user_id"
	ChatIDKey    ContextKey = "chat_id"
	UsernameKey  ContextKey = "username"
	UserRoleKey  ContextKey = "user_role"
	RequestIDKey ContextKey = "request_id"
	StartTimeKey ContextKey = "start_time"
	TraceIDKey   ContextKey = "trace_id"
)

// WithUserID добавляет ID пользователя в контекст
func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// GetUserID извлекает ID пользователя из контекста
func GetUserID(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(UserIDKey).(int64)
	return userID, ok
}

// WithChatID добавляет ID чата в контекст
func WithChatID(ctx context.Context, chatID int64) context.Context {
	return context.WithValue(ctx, ChatIDKey, chatID)
}

// GetChatID извлекает ID чата из контекста
func GetChatID(ctx context.Context) (int64, bool) {
	chatID, ok := ctx.Value(ChatIDKey).(int64)
	return chatID, ok
}

// WithUsername добавляет имя пользователя в контекст
func WithUsername(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, UsernameKey, username)
}

// GetUsername извлекает имя пользователя из контекста
func GetUsername(ctx context.Context) (string, bool) {
	username, ok := ctx.Value(UsernameKey).(string)
	return username, ok
}

// WithUserRole добавляет роль пользователя в контекст
func WithUserRole(ctx context.Context, role database.UserRole) context.Context {
	return context.WithValue(ctx, UserRoleKey, role)
}

// GetUserRole извлекает роль пользователя из контекста
func GetUserRole(ctx context.Context) (database.UserRole, bool) {
	role, ok := ctx.Value(UserRoleKey).(database.UserRole)
	return role, ok
}

// WithRequestID добавляет ID запроса в контекст
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetRequestID извлекает ID запроса из контекста
func GetRequestID(ctx context.Context) (string, bool) {
	requestID, ok := ctx.Value(RequestIDKey).(string)
	return requestID, ok
}

// WithStartTime добавляет время начала в контекст
func WithStartTime(ctx context.Context, startTime time.Time) context.Context {
	return context.WithValue(ctx, StartTimeKey, startTime)
}

// GetStartTime извлекает время начала из контекста
func GetStartTime(ctx context.Context) (time.Time, bool) {
	startTime, ok := ctx.Value(StartTimeKey).(time.Time)
	return startTime, ok
}

// WithTraceID добавляет ID трассировки в контекст
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// GetTraceID извлекает ID трассировки из контекста
func GetTraceID(ctx context.Context) (string, bool) {
	traceID, ok := ctx.Value(TraceIDKey).(string)
	return traceID, ok
}

// EnrichContext добавляет все необходимые данные в контекст
func EnrichContext(ctx context.Context, userID, chatID int64, username string, role database.UserRole, requestID string) context.Context {
	ctx = WithUserID(ctx, userID)
	ctx = WithChatID(ctx, chatID)
	ctx = WithUsername(ctx, username)
	ctx = WithUserRole(ctx, role)
	ctx = WithRequestID(ctx, requestID)
	ctx = WithStartTime(ctx, time.Now())
	return ctx
}

// NewRequestContext создает новый контекст для запроса с таймаутом
func NewRequestContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// NewBackgroundContext создает новый фоновый контекст
func NewBackgroundContext() context.Context {
	return context.Background()
}

// IsContextCancelled проверяет, был ли контекст отменен
func IsContextCancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// GetContextFields извлекает все поля из контекста для логирования
func GetContextFields(ctx context.Context) map[string]any {
	fields := make(map[string]any)

	if userID, ok := GetUserID(ctx); ok {
		fields["user_id"] = userID
	}
	if chatID, ok := GetChatID(ctx); ok {
		fields["chat_id"] = chatID
	}
	if username, ok := GetUsername(ctx); ok {
		fields["username"] = username
	}
	if role, ok := GetUserRole(ctx); ok {
		fields["user_role"] = string(role)
	}
	if requestID, ok := GetRequestID(ctx); ok {
		fields["request_id"] = requestID
	}
	if traceID, ok := GetTraceID(ctx); ok {
		fields["trace_id"] = traceID
	}
	if startTime, ok := GetStartTime(ctx); ok {
		fields["duration_ms"] = time.Since(startTime).Milliseconds()
	}

	return fields
}

package ratelimit

import (
	"sync"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
)

// TokenBucketLimiter реализует алгоритм token bucket для rate limiting
type TokenBucketLimiter struct {
	buckets      map[int64]*bucket
	defaultLimit int
	refillRate   time.Duration
	mu           sync.RWMutex
}

// bucket представляет bucket для конкретного пользователя
type bucket struct {
	tokens     int
	limit      int
	lastRefill time.Time
	mu         sync.Mutex
}

// NewTokenBucketLimiter создает новый rate limiter
func NewTokenBucketLimiter(defaultLimit int, refillRate time.Duration) domain.RateLimiterInterface {
	limiter := &TokenBucketLimiter{
		buckets:      make(map[int64]*bucket),
		defaultLimit: defaultLimit,
		refillRate:   refillRate,
	}

	// Запускаем горутину для очистки старых buckets
	go limiter.cleanup()

	return limiter
}

// Allow проверяет, разрешен ли запрос для пользователя
func (tbl *TokenBucketLimiter) Allow(userID int64) bool {
	tbl.mu.RLock()
	b, exists := tbl.buckets[userID]
	tbl.mu.RUnlock()

	if !exists {
		tbl.mu.Lock()
		// Double-check после получения write lock
		if b, exists = tbl.buckets[userID]; !exists {
			b = &bucket{
				tokens:     tbl.defaultLimit,
				limit:      tbl.defaultLimit,
				lastRefill: time.Now(),
			}
			tbl.buckets[userID] = b
		}
		tbl.mu.Unlock()
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Пополняем токены
	now := time.Now()
	if now.Sub(b.lastRefill) >= tbl.refillRate {
		tokensToAdd := int(now.Sub(b.lastRefill) / tbl.refillRate)
		b.tokens = minInt(b.limit, b.tokens+tokensToAdd)
		b.lastRefill = now
	}

	// Проверяем доступность токена
	if b.tokens > 0 {
		b.tokens--
		return true
	}

	logger.Log.WithField("user_id", userID).Debug("Rate limit exceeded")
	return false
}

// Reset сбрасывает лимит для пользователя
func (tbl *TokenBucketLimiter) Reset(userID int64) {
	tbl.mu.Lock()
	defer tbl.mu.Unlock()

	if b, exists := tbl.buckets[userID]; exists {
		b.mu.Lock()
		b.tokens = b.limit
		b.lastRefill = time.Now()
		b.mu.Unlock()
	}

	logger.Log.WithField("user_id", userID).Debug("Rate limit reset")
}

// GetLimit возвращает лимит для пользователя
func (tbl *TokenBucketLimiter) GetLimit(userID int64) int {
	tbl.mu.RLock()
	defer tbl.mu.RUnlock()

	if b, exists := tbl.buckets[userID]; exists {
		return b.limit
	}
	return tbl.defaultLimit
}

// GetRemaining возвращает количество оставшихся токенов
func (tbl *TokenBucketLimiter) GetRemaining(userID int64) int {
	tbl.mu.RLock()
	b, exists := tbl.buckets[userID]
	tbl.mu.RUnlock()

	if !exists {
		return tbl.defaultLimit
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Пополняем токены перед возвратом
	now := time.Now()
	if now.Sub(b.lastRefill) >= tbl.refillRate {
		tokensToAdd := int(now.Sub(b.lastRefill) / tbl.refillRate)
		b.tokens = minInt(b.limit, b.tokens+tokensToAdd)
		b.lastRefill = now
	}

	return b.tokens
}

// SetLimit устанавливает индивидуальный лимит для пользователя
func (tbl *TokenBucketLimiter) SetLimit(userID int64, limit int) {
	tbl.mu.Lock()
	defer tbl.mu.Unlock()

	b, exists := tbl.buckets[userID]
	if !exists {
		b = &bucket{
			tokens:     limit,
			limit:      limit,
			lastRefill: time.Now(),
		}
		tbl.buckets[userID] = b
	} else {
		b.mu.Lock()
		b.limit = limit
		if b.tokens > limit {
			b.tokens = limit
		}
		b.mu.Unlock()
	}

	logger.Log.WithFields(map[string]any{
		"user_id": userID,
		"limit":   limit,
	}).Debug("Rate limit set for user")
}

// cleanup периодически очищает неиспользуемые buckets
func (tbl *TokenBucketLimiter) cleanup() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		tbl.mu.Lock()
		now := time.Now()
		for userID, b := range tbl.buckets {
			b.mu.Lock()
			// Удаляем buckets, которые не использовались более 24 часов
			if now.Sub(b.lastRefill) > 24*time.Hour {
				delete(tbl.buckets, userID)
			}
			b.mu.Unlock()
		}
		tbl.mu.Unlock()

		logger.Log.Debug("Rate limiter cleanup completed")
	}
}

// NoOpRateLimiter реализует интерфейс без ограничений
type NoOpRateLimiter struct{}

// NewNoOpRateLimiter создает no-op rate limiter
func NewNoOpRateLimiter() domain.RateLimiterInterface {
	return &NoOpRateLimiter{}
}

// Allow всегда возвращает true
func (*NoOpRateLimiter) Allow(_ int64) bool {
	return true
}

// Reset no-op реализация
func (*NoOpRateLimiter) Reset(_ int64) {}

// GetLimit возвращает максимальное значение
func (*NoOpRateLimiter) GetLimit(_ int64) int {
	return int(^uint(0) >> 1) // max int
}

// GetRemaining возвращает максимальное значение
func (*NoOpRateLimiter) GetRemaining(_ int64) int {
	return int(^uint(0) >> 1) // max int
}

// min возвращает минимальное из двух чисел
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

package timeutil

import (
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
)

// SystemTimeProvider реализует интерфейс времени через стандартную библиотеку
type SystemTimeProvider struct{}

// NewSystemTimeProvider создает новый провайдер времени
func NewSystemTimeProvider() domain.TimeProvider {
	return &SystemTimeProvider{}
}

// Now возвращает текущее время
func (*SystemTimeProvider) Now() time.Time {
	return time.Now()
}

// Sleep приостанавливает выполнение на указанное время
func (*SystemTimeProvider) Sleep(duration time.Duration) {
	time.Sleep(duration)
}

// After возвращает канал, который получит значение через указанное время
func (*SystemTimeProvider) After(duration time.Duration) <-chan time.Time {
	return time.After(duration)
}

// NewTimer создает новый таймер
func (*SystemTimeProvider) NewTimer(duration time.Duration) domain.Timer {
	return &SystemTimer{timer: time.NewTimer(duration)}
}

// NewTicker создает новый тикер
func (*SystemTimeProvider) NewTicker(duration time.Duration) domain.Ticker {
	return &SystemTicker{ticker: time.NewTicker(duration)}
}

// SystemTimer обертка над time.Timer
type SystemTimer struct {
	timer *time.Timer
}

// C возвращает канал таймера
func (t *SystemTimer) C() <-chan time.Time {
	return t.timer.C
}

// Stop останавливает таймер
func (t *SystemTimer) Stop() bool {
	return t.timer.Stop()
}

// Reset сбрасывает таймер
func (t *SystemTimer) Reset(duration time.Duration) bool {
	return t.timer.Reset(duration)
}

// SystemTicker обертка над time.Ticker
type SystemTicker struct {
	ticker *time.Ticker
}

// C возвращает канал тикера
func (t *SystemTicker) C() <-chan time.Time {
	return t.ticker.C
}

// Stop останавливает тикер
func (t *SystemTicker) Stop() {
	t.ticker.Stop()
}

// Reset сбрасывает тикер
func (t *SystemTicker) Reset(duration time.Duration) {
	t.ticker.Reset(duration)
}

// MockTimeProvider для тестирования
type MockTimeProvider struct {
	currentTime time.Time
	timers      []*MockTimer
	tickers     []*MockTicker
}

// NewMockTimeProvider создает mock провайдер времени
func NewMockTimeProvider(startTime time.Time) *MockTimeProvider {
	return &MockTimeProvider{
		currentTime: startTime,
		timers:      make([]*MockTimer, 0),
		tickers:     make([]*MockTicker, 0),
	}
}

// SetTime устанавливает текущее время
func (m *MockTimeProvider) SetTime(t time.Time) {
	m.currentTime = t
}

// AdvanceTime продвигает время вперед
func (m *MockTimeProvider) AdvanceTime(duration time.Duration) {
	m.currentTime = m.currentTime.Add(duration)

	// Уведомляем таймеры и тикеры
	for _, timer := range m.timers {
		timer.checkTime(m.currentTime)
	}
	for _, ticker := range m.tickers {
		ticker.checkTime(m.currentTime)
	}
}

// Now возвращает текущее mock время
func (m *MockTimeProvider) Now() time.Time {
	return m.currentTime
}

// Sleep в mock версии ничего не делает
func (*MockTimeProvider) Sleep(_ time.Duration) {
	// В тестах мы не хотим реально ждать
}

// After возвращает канал, который сработает при продвижении времени
func (m *MockTimeProvider) After(duration time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	targetTime := m.currentTime.Add(duration)

	// Если время уже прошло, отправляем сразу
	if !targetTime.After(m.currentTime) {
		ch <- m.currentTime
	}

	return ch
}

// NewTimer создает mock таймер
func (m *MockTimeProvider) NewTimer(duration time.Duration) domain.Timer {
	timer := &MockTimer{
		duration:   duration,
		targetTime: m.currentTime.Add(duration),
		c:          make(chan time.Time, 1),
		stopped:    false,
	}
	m.timers = append(m.timers, timer)
	return timer
}

// NewTicker создает mock тикер
func (m *MockTimeProvider) NewTicker(duration time.Duration) domain.Ticker {
	ticker := &MockTicker{
		duration:   duration,
		targetTime: m.currentTime.Add(duration),
		c:          make(chan time.Time, 1),
		stopped:    false,
	}
	m.tickers = append(m.tickers, ticker)
	return ticker
}

// MockTimer mock реализация таймера
type MockTimer struct {
	duration   time.Duration
	targetTime time.Time
	c          chan time.Time
	stopped    bool
}

// C возвращает канал таймера
func (t *MockTimer) C() <-chan time.Time {
	return t.c
}

// Stop останавливает таймер
func (t *MockTimer) Stop() bool {
	wasActive := !t.stopped
	t.stopped = true
	return wasActive
}

// Reset сбрасывает таймер
func (t *MockTimer) Reset(duration time.Duration) bool {
	wasActive := !t.stopped
	t.duration = duration
	t.stopped = false
	return wasActive
}

// checkTime проверяет, нужно ли сработать таймеру
func (t *MockTimer) checkTime(currentTime time.Time) {
	if !t.stopped && !currentTime.Before(t.targetTime) {
		select {
		case t.c <- currentTime:
		default:
		}
		t.stopped = true
	}
}

// MockTicker mock реализация тикера
type MockTicker struct {
	duration   time.Duration
	targetTime time.Time
	c          chan time.Time
	stopped    bool
}

// C возвращает канал тикера
func (t *MockTicker) C() <-chan time.Time {
	return t.c
}

// Stop останавливает тикер
func (t *MockTicker) Stop() {
	t.stopped = true
}

// Reset сбрасывает тикер
func (t *MockTicker) Reset(duration time.Duration) {
	t.duration = duration
	t.stopped = false
}

// checkTime проверяет, нужно ли сработать тикеру
func (t *MockTicker) checkTime(currentTime time.Time) {
	if !t.stopped && !currentTime.Before(t.targetTime) {
		select {
		case t.c <- currentTime:
		default:
		}
		// Устанавливаем следующий тик
		t.targetTime = t.targetTime.Add(t.duration)
	}
}

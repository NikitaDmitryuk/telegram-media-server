package session

import (
	"sync"
	"time"
)

type Session struct {
	ChatID     int64
	Data       map[string]any
	Stage      string
	LastActive time.Time
}

type SessionManager struct {
	mu       sync.Mutex
	sessions map[int64]*Session
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[int64]*Session),
	}
}

func (m *SessionManager) Get(chatID int64) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[chatID]
}

func (m *SessionManager) Set(chatID int64, s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[chatID] = s
}

func (m *SessionManager) Delete(chatID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, chatID)
}

func (m *SessionManager) Cleanup(expire time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for k, v := range m.sessions {
		if now.Sub(v.LastActive) > expire {
			delete(m.sessions, k)
		}
	}
}

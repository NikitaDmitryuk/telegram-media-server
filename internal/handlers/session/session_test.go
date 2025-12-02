package session

import (
	"sync"
	"testing"
	"time"
)

func TestNewSessionManager(t *testing.T) {
	manager := NewSessionManager()

	if manager == nil {
		t.Fatal("NewSessionManager returned nil")
		return
	}

	if manager.sessions == nil {
		t.Error("sessions map is nil")
	}

	if len(manager.sessions) != 0 {
		t.Errorf("Expected empty sessions map, got %d items", len(manager.sessions))
	}
}

func TestSessionManager_SetAndGet(t *testing.T) {
	manager := NewSessionManager()
	chatID := int64(123)

	// Test getting non-existent session
	session := manager.Get(chatID)
	if session != nil {
		t.Errorf("Expected nil session for non-existent chat ID, got %+v", session)
	}

	// Test setting and getting session
	testSession := &Session{
		ChatID:     chatID,
		Data:       map[string]any{"key": "value"},
		Stage:      "test_stage",
		LastActive: time.Now(),
	}

	manager.Set(chatID, testSession)

	retrievedSession := manager.Get(chatID)
	if retrievedSession == nil {
		t.Fatal("Retrieved session is nil")
		return
	}

	if retrievedSession.ChatID != chatID {
		t.Errorf("Expected chat ID %d, got %d", chatID, retrievedSession.ChatID)
	}

	if retrievedSession.Stage != "test_stage" {
		t.Errorf("Expected stage 'test_stage', got '%s'", retrievedSession.Stage)
	}

	if retrievedSession.Data["key"] != "value" {
		t.Errorf("Expected data key 'value', got '%v'", retrievedSession.Data["key"])
	}
}

func TestSessionManager_Delete(t *testing.T) {
	manager := NewSessionManager()
	chatID := int64(123)

	// Set a session
	testSession := &Session{
		ChatID: chatID,
		Data:   map[string]any{},
		Stage:  "test",
	}
	manager.Set(chatID, testSession)

	// Verify it exists
	if manager.Get(chatID) == nil {
		t.Fatal("Session should exist before deletion")
	}

	// Delete it
	manager.Delete(chatID)

	// Verify it's gone
	if manager.Get(chatID) != nil {
		t.Error("Session should be nil after deletion")
	}

	// Test deleting non-existent session (should not panic)
	manager.Delete(int64(999))
}

func TestSessionManager_Cleanup(t *testing.T) {
	manager := NewSessionManager()
	now := time.Now()

	// Add fresh session (should not be cleaned up)
	freshSession := &Session{
		ChatID:     123,
		Data:       map[string]any{},
		Stage:      "fresh",
		LastActive: now,
	}
	manager.Set(123, freshSession)

	// Add old session (should be cleaned up)
	oldSession := &Session{
		ChatID:     456,
		Data:       map[string]any{},
		Stage:      "old",
		LastActive: now.Add(-2 * time.Hour),
	}
	manager.Set(456, oldSession)

	// Add session just at the threshold (should not be cleaned up)
	thresholdSession := &Session{
		ChatID:     789,
		Data:       map[string]any{},
		Stage:      "threshold",
		LastActive: now.Add(-59 * time.Minute),
	}
	manager.Set(789, thresholdSession)

	// Cleanup sessions older than 1 hour
	manager.Cleanup(1 * time.Hour)

	// Check results
	if manager.Get(123) == nil {
		t.Error("Fresh session should not be cleaned up")
	}

	if manager.Get(456) != nil {
		t.Error("Old session should be cleaned up")
	}

	if manager.Get(789) == nil {
		t.Error("Threshold session should not be cleaned up")
	}
}

func TestSessionManager_ConcurrentAccess(_ *testing.T) {
	manager := NewSessionManager()
	numGoroutines := 100
	numOperations := 1000

	var wg sync.WaitGroup

	// Test concurrent writes
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range numOperations {
				chatID := int64(id*numOperations + j)
				session := &Session{
					ChatID:     chatID,
					Data:       map[string]any{"test": j},
					Stage:      "concurrent",
					LastActive: time.Now(),
				}
				manager.Set(chatID, session)
			}
		}(i)
	}

	// Test concurrent reads
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range numOperations {
				chatID := int64(id*numOperations + j)
				manager.Get(chatID)
			}
		}(i)
	}

	// Test concurrent deletes
	for i := range numGoroutines / 2 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range numOperations / 2 {
				chatID := int64(id*numOperations + j)
				manager.Delete(chatID)
			}
		}(i)
	}

	wg.Wait()

	// No assertions needed - if we get here without deadlock, the test passes
}

func TestSessionManager_CleanupWithExpiredSessions(t *testing.T) {
	manager := NewSessionManager()

	// Add multiple expired sessions
	for i := range 10 {
		session := &Session{
			ChatID:     int64(i),
			Data:       map[string]any{"id": i},
			Stage:      "expired",
			LastActive: time.Now().Add(-2 * time.Hour),
		}
		manager.Set(int64(i), session)
	}

	// Verify all sessions exist
	manager.mu.Lock()
	initialCount := len(manager.sessions)
	manager.mu.Unlock()

	if initialCount != 10 {
		t.Errorf("Expected 10 sessions before cleanup, got %d", initialCount)
	}

	// Cleanup with 1 hour threshold
	manager.Cleanup(1 * time.Hour)

	// Verify all sessions are cleaned up
	manager.mu.Lock()
	finalCount := len(manager.sessions)
	manager.mu.Unlock()

	if finalCount != 0 {
		t.Errorf("Expected 0 sessions after cleanup, got %d", finalCount)
	}
}

func BenchmarkSessionManager_Set(b *testing.B) {
	manager := NewSessionManager()
	session := &Session{
		ChatID:     123,
		Data:       map[string]any{"key": "value"},
		Stage:      "benchmark",
		LastActive: time.Now(),
	}

	b.ResetTimer()
	for i := range b.N {
		manager.Set(int64(i), session)
	}
}

func BenchmarkSessionManager_Get(b *testing.B) {
	manager := NewSessionManager()
	session := &Session{
		ChatID:     123,
		Data:       map[string]any{"key": "value"},
		Stage:      "benchmark",
		LastActive: time.Now(),
	}

	// Pre-populate with some sessions
	for i := range 1000 {
		manager.Set(int64(i), session)
	}

	b.ResetTimer()
	for i := range b.N {
		manager.Get(int64(i % 1000))
	}
}

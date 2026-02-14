package session

import (
	"testing"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/prowlarr"
)

// setupSearchSession creates a search session with the given results and offset,
// and returns the chatID used.
func setupSearchSession(t *testing.T, numResults, offset int) int64 {
	t.Helper()
	chatID := int64(42)

	results := make([]prowlarr.TorrentSearchResult, numResults)
	for i := range numResults {
		results[i] = prowlarr.TorrentSearchResult{
			Title: "Result",
			Peers: numResults - i,
		}
	}

	ss := &SearchSession{
		Query:   "test query",
		Results: results,
		Offset:  offset,
	}
	setSearchSession(chatID, ss, stageShowResults)
	return chatID
}

func TestSearchSession_SetGetDelete(t *testing.T) {
	chatID := int64(100)

	// Initially no session.
	ss, sess := GetSearchSession(chatID)
	if ss != nil || sess != nil {
		t.Error("Expected no session initially")
	}

	// Set session.
	setSearchSession(chatID, &SearchSession{Query: "hello"}, stageAwaitQuery)
	ss, sess = GetSearchSession(chatID)
	if ss == nil {
		t.Fatal("Expected non-nil SearchSession")
	}
	if sess == nil {
		t.Fatal("Expected non-nil Session")
	}
	if ss.Query != "hello" {
		t.Errorf("Expected query 'hello', got %q", ss.Query)
	}
	if sess.Stage != stageAwaitQuery {
		t.Errorf("Expected stage 'await_query', got %q", sess.Stage)
	}

	// Delete session.
	DeleteSearchSession(chatID)
	ss, sess = GetSearchSession(chatID)
	if ss != nil || sess != nil {
		t.Error("Expected no session after delete")
	}
}

func TestSearchSession_StageTransitions(t *testing.T) {
	chatID := int64(200)

	// Start at await_query.
	setSearchSession(chatID, &SearchSession{}, stageAwaitQuery)
	_, sess := GetSearchSession(chatID)
	if sess.Stage != stageAwaitQuery {
		t.Fatalf("Expected stage 'await_query', got %q", sess.Stage)
	}

	// Transition to show_results.
	ss, _ := GetSearchSession(chatID)
	ss.Query = "inception"
	ss.Results = sampleResults(10)
	ss.Offset = 0
	setSearchSession(chatID, ss, stageShowResults)
	_, sess = GetSearchSession(chatID)
	if sess.Stage != stageShowResults {
		t.Errorf("Expected stage 'show_results', got %q", sess.Stage)
	}

	// Transition back to await_query (simulates "Back" on first page).
	setSearchSession(chatID, ss, stageAwaitQuery)
	_, sess = GetSearchSession(chatID)
	if sess.Stage != stageAwaitQuery {
		t.Errorf("Expected stage 'await_query', got %q", sess.Stage)
	}

	// Clean up.
	DeleteSearchSession(chatID)
}

func TestSearchSession_OffsetMoreIncrement(t *testing.T) {
	// Simulates pressing "More" — offset should increase by displayPageSize.
	chatID := setupSearchSession(t, 20, 0)
	defer DeleteSearchSession(chatID)

	ss, _ := GetSearchSession(chatID)

	// First "More".
	ss.Offset += displayPageSize
	if ss.Offset != displayPageSize {
		t.Errorf("After first 'More': expected offset %d, got %d", displayPageSize, ss.Offset)
	}

	// Second "More".
	ss.Offset += displayPageSize
	if ss.Offset != displayPageSize*2 {
		t.Errorf("After second 'More': expected offset %d, got %d", displayPageSize*2, ss.Offset)
	}
}

func TestSearchSession_OffsetBackDecrement(t *testing.T) {
	// Simulates pressing "Back" — offset should decrease by displayPageSize.
	chatID := setupSearchSession(t, 20, displayPageSize*2)
	defer DeleteSearchSession(chatID)

	ss, _ := GetSearchSession(chatID)

	// First "Back".
	ss.Offset -= displayPageSize
	if ss.Offset != displayPageSize {
		t.Errorf("After first 'Back': expected offset %d, got %d", displayPageSize, ss.Offset)
	}

	// Second "Back".
	ss.Offset -= displayPageSize
	if ss.Offset != 0 {
		t.Errorf("After second 'Back': expected offset 0, got %d", ss.Offset)
	}
}

func TestSearchSession_OffsetBackFloorAtZero(t *testing.T) {
	// If offset would go below 0, clamp to 0.
	chatID := setupSearchSession(t, 10, 2)
	defer DeleteSearchSession(chatID)

	ss, _ := GetSearchSession(chatID)

	ss.Offset -= displayPageSize
	if ss.Offset < 0 {
		ss.Offset = 0
	}
	if ss.Offset != 0 {
		t.Errorf("Expected offset 0 after clamping, got %d", ss.Offset)
	}
}

func TestSearchSession_PaginationBounds(t *testing.T) {
	// Verify page boundaries with displayPageSize=4.
	numResults := 10
	chatID := setupSearchSession(t, numResults, 0)
	defer DeleteSearchSession(chatID)

	ss, _ := GetSearchSession(chatID)

	// Page 1: [0..3], hasMore=true.
	from := ss.Offset
	to := from + displayPageSize
	if to > numResults {
		to = numResults
	}
	if from != 0 || to != 4 {
		t.Errorf("Page 1: expected [0, 4), got [%d, %d)", from, to)
	}
	if to >= numResults {
		t.Error("Page 1 should have more results")
	}

	// Page 2: [4..7], hasMore=true.
	ss.Offset += displayPageSize
	from = ss.Offset
	to = from + displayPageSize
	if to > numResults {
		to = numResults
	}
	if from != 4 || to != 8 {
		t.Errorf("Page 2: expected [4, 8), got [%d, %d)", from, to)
	}
	if to >= numResults {
		t.Error("Page 2 should have more results")
	}

	// Page 3: [8..9], hasMore=false.
	ss.Offset += displayPageSize
	from = ss.Offset
	to = from + displayPageSize
	if to > numResults {
		to = numResults
	}
	if from != 8 || to != 10 {
		t.Errorf("Page 3: expected [8, 10), got [%d, %d)", from, to)
	}
	if to < numResults {
		t.Error("Page 3 should NOT have more results")
	}
}

func TestSearchSession_DisplayPageSizeConstant(t *testing.T) {
	// Guard against accidental changes to the constant.
	if displayPageSize != 4 {
		t.Errorf("displayPageSize should be 4, got %d", displayPageSize)
	}
}

func TestSearchSession_MultipleChatIDs(t *testing.T) {
	// Sessions for different chats should be independent.
	chatA := int64(1000)
	chatB := int64(2000)

	setSearchSession(chatA, &SearchSession{Query: "query A", Offset: 0}, stageShowResults)
	setSearchSession(chatB, &SearchSession{Query: "query B", Offset: 8}, stageAwaitQuery)

	defer DeleteSearchSession(chatA)
	defer DeleteSearchSession(chatB)

	ssA, sessA := GetSearchSession(chatA)
	ssB, sessB := GetSearchSession(chatB)

	if ssA.Query != "query A" || sessA.Stage != stageShowResults {
		t.Errorf("Chat A: unexpected state: query=%q stage=%q", ssA.Query, sessA.Stage)
	}
	if ssB.Query != "query B" || sessB.Stage != stageAwaitQuery {
		t.Errorf("Chat B: unexpected state: query=%q stage=%q", ssB.Query, sessB.Stage)
	}
	if ssA.Offset != 0 {
		t.Errorf("Chat A: expected offset 0, got %d", ssA.Offset)
	}
	if ssB.Offset != 8 {
		t.Errorf("Chat B: expected offset 8, got %d", ssB.Offset)
	}
}

func TestSearchSession_BackOnFirstPageChangesStage(t *testing.T) {
	// Simulates "Back" on the first page: stage should go to stageAwaitQuery.
	chatID := setupSearchSession(t, 10, 0)
	defer DeleteSearchSession(chatID)

	ss, _ := GetSearchSession(chatID)

	// On first page (offset==0), "Back" should transition to await_query.
	if ss.Offset == 0 {
		ss.MessageIDs = nil
		setSearchSession(chatID, ss, stageAwaitQuery)
	}

	_, sess := GetSearchSession(chatID)
	if sess.Stage != stageAwaitQuery {
		t.Errorf("Expected stage 'await_query' after Back on first page, got %q", sess.Stage)
	}
}

func TestSearchSession_BackOnLaterPageStaysInResults(t *testing.T) {
	// Simulates "Back" on page 2+: stage stays stageShowResults, offset decreases.
	chatID := setupSearchSession(t, 20, displayPageSize)
	defer DeleteSearchSession(chatID)

	ss, _ := GetSearchSession(chatID)

	if ss.Offset > 0 {
		ss.Offset -= displayPageSize
		if ss.Offset < 0 {
			ss.Offset = 0
		}
		setSearchSession(chatID, ss, stageShowResults)
	}

	ss, sess := GetSearchSession(chatID)
	if sess.Stage != stageShowResults {
		t.Errorf("Expected stage 'show_results' after Back on page 2, got %q", sess.Stage)
	}
	if ss.Offset != 0 {
		t.Errorf("Expected offset 0 after Back from page 2, got %d", ss.Offset)
	}
}

func TestSearchSession_FullNavigationCycle(t *testing.T) {
	// Simulate a full cycle: page 1 → page 2 → page 3 → back → back → back (to query).
	numResults := 10
	chatID := setupSearchSession(t, numResults, 0)
	defer DeleteSearchSession(chatID)

	ss, _ := GetSearchSession(chatID)

	// → page 2
	ss.Offset += displayPageSize
	if ss.Offset != 4 {
		t.Fatalf("Expected offset 4, got %d", ss.Offset)
	}

	// → page 3
	ss.Offset += displayPageSize
	if ss.Offset != 8 {
		t.Fatalf("Expected offset 8, got %d", ss.Offset)
	}

	// ← page 2
	ss.Offset -= displayPageSize
	if ss.Offset != 4 {
		t.Fatalf("Expected offset 4, got %d", ss.Offset)
	}

	// ← page 1
	ss.Offset -= displayPageSize
	if ss.Offset != 0 {
		t.Fatalf("Expected offset 0, got %d", ss.Offset)
	}

	// ← back to query input
	setSearchSession(chatID, ss, stageAwaitQuery)
	_, sess := GetSearchSession(chatID)
	if sess.Stage != stageAwaitQuery {
		t.Errorf("Expected stage 'await_query', got %q", sess.Stage)
	}
}

func TestSearchSession_MessageIDsManagement(t *testing.T) {
	chatID := int64(300)

	ss := &SearchSession{
		Query:      "test",
		Results:    sampleResults(8),
		Offset:     0,
		MessageIDs: []int{101, 102, 103, 104},
	}
	setSearchSession(chatID, ss, stageShowResults)
	defer DeleteSearchSession(chatID)

	// After "page change", messages should be cleared and new ones added.
	ss, _ = GetSearchSession(chatID)
	if len(ss.MessageIDs) != 4 {
		t.Errorf("Expected 4 message IDs, got %d", len(ss.MessageIDs))
	}

	// Simulate clearing and adding new messages (as ShowTorrentSearchResults does).
	ss.MessageIDs = nil
	ss.MessageIDs = append(ss.MessageIDs, 201, 202, 203, 204)
	setSearchSession(chatID, ss, stageShowResults)

	ss, _ = GetSearchSession(chatID)
	if len(ss.MessageIDs) != 4 {
		t.Errorf("Expected 4 new message IDs, got %d", len(ss.MessageIDs))
	}
	if ss.MessageIDs[0] != 201 {
		t.Errorf("Expected first message ID 201, got %d", ss.MessageIDs[0])
	}
}

func TestSearchSession_QueryPreservedAcrossNavigation(t *testing.T) {
	chatID := setupSearchSession(t, 12, 0)
	defer DeleteSearchSession(chatID)

	ss, _ := GetSearchSession(chatID)
	originalQuery := ss.Query

	// Navigate forward and back.
	ss.Offset += displayPageSize
	setSearchSession(chatID, ss, stageShowResults)

	ss.Offset -= displayPageSize
	setSearchSession(chatID, ss, stageShowResults)

	ss, _ = GetSearchSession(chatID)
	if ss.Query != originalQuery {
		t.Errorf("Query changed during navigation: expected %q, got %q", originalQuery, ss.Query)
	}
}

func TestSearchSession_ResultsPreservedAcrossNavigation(t *testing.T) {
	numResults := 8
	chatID := setupSearchSession(t, numResults, 0)
	defer DeleteSearchSession(chatID)

	ss, _ := GetSearchSession(chatID)
	originalCount := len(ss.Results)

	// Navigate forward and back.
	ss.Offset += displayPageSize
	setSearchSession(chatID, ss, stageShowResults)
	ss.Offset -= displayPageSize
	setSearchSession(chatID, ss, stageShowResults)

	ss, _ = GetSearchSession(chatID)
	if len(ss.Results) != originalCount {
		t.Errorf("Results count changed: expected %d, got %d", originalCount, len(ss.Results))
	}
}

package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

func TestMain(m *testing.M) {
	logutils.InitLogger("error")

	// Resolve the project root (three levels up from internal/handlers/ui).
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed")
	}
	projectRoot := filepath.Join(filepath.Dir(file), "..", "..", "..")
	localesPath := filepath.Join(projectRoot, "locales")

	cfg := &tmsconfig.Config{
		Lang:     "en",
		LangPath: localesPath,
	}
	_ = lang.InitLocalizer(cfg)

	os.Exit(m.Run())
}

func TestGetTorrentSearchKeyboard_NoMoreNoBack(t *testing.T) {
	kb := GetTorrentSearchKeyboard(false, false)

	// Should have exactly 1 row: [Cancel].
	if len(kb.Keyboard) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(kb.Keyboard))
	}
	if len(kb.Keyboard[0]) != 1 {
		t.Fatalf("Expected 1 button in row 0, got %d", len(kb.Keyboard[0]))
	}
	if kb.Keyboard[0][0].Text != lang.Translate("general.torrent_search.cancel", nil) {
		t.Errorf("Expected Cancel button, got %q", kb.Keyboard[0][0].Text)
	}
}

func TestGetTorrentSearchKeyboard_MoreOnly(t *testing.T) {
	kb := GetTorrentSearchKeyboard(true, false)

	// Should have 2 rows: [More], [Cancel].
	if len(kb.Keyboard) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(kb.Keyboard))
	}
	// Row 0: navigation with only "More".
	if len(kb.Keyboard[0]) != 1 {
		t.Fatalf("Expected 1 button in nav row, got %d", len(kb.Keyboard[0]))
	}
	if kb.Keyboard[0][0].Text != lang.Translate("general.torrent_search.more", nil) {
		t.Errorf("Expected More button, got %q", kb.Keyboard[0][0].Text)
	}
	// Row 1: Cancel.
	if len(kb.Keyboard[1]) != 1 {
		t.Fatalf("Expected 1 button in cancel row, got %d", len(kb.Keyboard[1]))
	}
}

func TestGetTorrentSearchKeyboard_BackOnly(t *testing.T) {
	kb := GetTorrentSearchKeyboard(false, true)

	// Should have 2 rows: [Back], [Cancel].
	if len(kb.Keyboard) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(kb.Keyboard))
	}
	if len(kb.Keyboard[0]) != 1 {
		t.Fatalf("Expected 1 button in nav row, got %d", len(kb.Keyboard[0]))
	}
	if kb.Keyboard[0][0].Text != lang.Translate("general.torrent_search.back", nil) {
		t.Errorf("Expected Back button, got %q", kb.Keyboard[0][0].Text)
	}
}

func TestGetTorrentSearchKeyboard_BothMoreAndBack(t *testing.T) {
	kb := GetTorrentSearchKeyboard(true, true)

	// Should have 2 rows: [Back, More], [Cancel].
	if len(kb.Keyboard) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(kb.Keyboard))
	}
	// Row 0: [Back, More].
	if len(kb.Keyboard[0]) != 2 {
		t.Fatalf("Expected 2 buttons in nav row, got %d", len(kb.Keyboard[0]))
	}
	if kb.Keyboard[0][0].Text != lang.Translate("general.torrent_search.back", nil) {
		t.Errorf("First nav button: expected Back, got %q", kb.Keyboard[0][0].Text)
	}
	if kb.Keyboard[0][1].Text != lang.Translate("general.torrent_search.more", nil) {
		t.Errorf("Second nav button: expected More, got %q", kb.Keyboard[0][1].Text)
	}
	// Row 1: [Cancel].
	if len(kb.Keyboard[1]) != 1 {
		t.Fatalf("Expected 1 button in cancel row, got %d", len(kb.Keyboard[1]))
	}
}

func TestGetTorrentSearchKeyboard_AlwaysHasCancel(t *testing.T) {
	combos := []struct {
		hasMore, hasBack bool
	}{
		{false, false},
		{true, false},
		{false, true},
		{true, true},
	}

	cancelText := lang.Translate("general.torrent_search.cancel", nil)

	for _, c := range combos {
		kb := GetTorrentSearchKeyboard(c.hasMore, c.hasBack)
		lastRow := kb.Keyboard[len(kb.Keyboard)-1]
		found := false
		for _, btn := range lastRow {
			if btn.Text == cancelText {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("hasMore=%v hasBack=%v: Cancel button not found in last row", c.hasMore, c.hasBack)
		}
	}
}

func TestGetTorrentSearchKeyboard_Properties(t *testing.T) {
	kb := GetTorrentSearchKeyboard(true, true)

	if !kb.OneTimeKeyboard {
		t.Error("Expected OneTimeKeyboard to be true")
	}
	if !kb.ResizeKeyboard {
		t.Error("Expected ResizeKeyboard to be true")
	}
}

func TestGetTorrentSearchKeyboard_BackBeforeMore(t *testing.T) {
	// Back should always be to the left of More for consistent UX.
	kb := GetTorrentSearchKeyboard(true, true)

	backText := lang.Translate("general.torrent_search.back", nil)
	moreText := lang.Translate("general.torrent_search.more", nil)

	navRow := kb.Keyboard[0]
	backIdx, moreIdx := -1, -1
	for i, btn := range navRow {
		if btn.Text == backText {
			backIdx = i
		}
		if btn.Text == moreText {
			moreIdx = i
		}
	}

	if backIdx < 0 || moreIdx < 0 {
		t.Fatal("Both Back and More should be present")
	}
	if backIdx >= moreIdx {
		t.Errorf("Back (idx=%d) should appear before More (idx=%d)", backIdx, moreIdx)
	}
}

func TestGetMainMenuKeyboard_Structure(t *testing.T) {
	kb := GetMainMenuKeyboard()

	if len(kb.Keyboard) != 1 {
		t.Fatalf("Expected 1 row in main menu, got %d", len(kb.Keyboard))
	}

	const expectedButtons = 3 // list, delete, search
	if len(kb.Keyboard[0]) != expectedButtons {
		t.Errorf("Expected %d buttons, got %d", expectedButtons, len(kb.Keyboard[0]))
	}
}

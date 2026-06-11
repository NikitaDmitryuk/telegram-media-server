package movies

import (
	"context"
	"strings"
	"testing"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

type recordingDeleteQueue struct {
	pending map[uint]struct{}
	calls   []uint
}

func (q *recordingDeleteQueue) Enqueue(movieID uint) {
	if q.pending == nil {
		q.pending = make(map[uint]struct{})
	}
	q.pending[movieID] = struct{}{}
	q.calls = append(q.calls, movieID)
}

func (q *recordingDeleteQueue) IsPendingDeletion(movieID uint) bool {
	_, ok := q.pending[movieID]
	return ok
}

func TestBotDeleteThenListHidesPendingMovie(t *testing.T) {
	ctx := context.Background()
	bot := &testutils.MockBot{}
	db := testutils.TestDatabase(t)
	cfg := testutils.TestConfig(t.TempDir())
	queue := &recordingDeleteQueue{}

	movieID, err := db.AddMovie(ctx, "Big Buck Bunny", 1024, []string{"big-buck-bunny.mp4"}, nil, 0)
	if err != nil {
		t.Fatalf("AddMovie: %v", err)
	}

	a := &app.App{Bot: bot, DB: db, Config: cfg, DeleteQueue: queue}
	DeleteMoviesHandler(a, testutils.CommandUpdate(123, 123, "user", "/rm 1"))

	if len(queue.calls) != 1 || queue.calls[0] != movieID {
		t.Fatalf("delete queue calls = %v, want [%d]", queue.calls, movieID)
	}

	bot.ClearMessages()
	ListMoviesHandler(a, testutils.CommandUpdate(123, 123, "user", "/ls"))

	msg := bot.GetLastMessage()
	if msg == nil {
		t.Fatal("expected /ls response")
	}
	if strings.Contains(msg.Text, "Big Buck Bunny") {
		t.Fatalf("pending deletion movie should be hidden from /ls, got: %s", msg.Text)
	}
	if !strings.Contains(msg.Text, "empty") {
		t.Fatalf("expected empty list after pending delete, got: %s", msg.Text)
	}
}

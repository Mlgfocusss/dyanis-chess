package play

import (
	"context"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"dyanis-chess/backend/internal/bots"
	"dyanis-chess/backend/internal/ratings"
)

// fakeRatings gives everybody 1500 and moves the winner up 10 and the loser down 10.
type fakeRatings struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeRatings) Get(_ context.Context, _, _ string) (ratings.Entry, error) {
	return ratings.Entry{Rating: 1500, Provisional: true}, nil
}

func (f *fakeRatings) Apply(_ context.Context, cat, a, b string, scoreA float64) (ratings.Change, ratings.Change, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf("%s %s %s %.1f", cat, a, b, scoreA))
	f.mu.Unlock()
	d := int((scoreA - 0.5) * 20)
	return ratings.Change{Category: cat, Before: 1500, After: 1500 + d}, ratings.Change{Category: cat, Before: 1500, After: 1500 - d}, nil
}

func (f *fakeRatings) log() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.calls...)
}

func newRatedHub(t *testing.T) (*Hub, *httptest.Server, *fakeRatings) {
	t.Helper()
	fr := &fakeRatings{}
	h := New(Config{User: testUser, Ratings: fr,
		BotThink: func(*bots.Bot, *mrand.Rand) time.Duration { return 0 }})
	srv := httptest.NewServer(h.Handler())
	t.Cleanup(func() {
		srv.Close()
		h.Close()
	})
	return h, srv, fr
}

func snapshot(t *testing.T, srv *httptest.Server, user, id string) Message {
	t.Helper()
	code, b := call(t, srv, user, "GET", "/api/games/"+id, "")
	var m Message
	if err := json.Unmarshal(b, &m); err != nil || code != 200 {
		t.Fatalf("snapshot: %d %s", code, b)
	}
	return m
}

func TestARatedGameShowsRatingsAndMovesThemWhenItEnds(t *testing.T) {
	_, srv, fr := newRatedHub(t)
	id := createGame(t, srv, "anna", `{"color":"white","timeMs":300000,"incMs":0,"rated":true}`)
	if m := snapshot(t, srv, "anna", id); !m.Game.Rated || m.Game.White.Elo != 1500 || !m.Game.White.Prov {
		t.Fatalf("creator: %+v %+v", m.Game, m.Game.White)
	}
	call(t, srv, "boris", "POST", "/api/games/"+id+"/join", "")
	m := snapshot(t, srv, "boris", id)
	if m.Game.Black.Elo != 1500 {
		t.Errorf("the joiner has no rating shown: %+v", m.Game.Black)
	}
	anna, boris := dial(t, srv, "anna", id), dial(t, srv, "boris", id)
	anna.until("state", func(m Message) bool { return m.Type == "state" })
	for i, mv := range []string{"e2e4", "e7e5", "g1f3", "b8c6"} {
		if i%2 == 0 {
			anna.send("move", mv)
		} else {
			boris.send("move", mv)
		}
		boris.untilMoves(i + 1)
	}
	boris.send("resign", "")
	end := anna.until("rating change", func(m Message) bool {
		return m.Game != nil && m.Game.Status == StatusEnded && m.Game.RatingChange != nil
	})
	rc := end.Game.RatingChange
	if rc.White.After != 1510 || rc.Black.After != 1490 || rc.White.Category != "blitz" {
		t.Errorf("changes: %+v %+v", rc.White, rc.Black)
	}
	if calls := fr.log(); len(calls) != 1 || calls[0] != "blitz anna boris 1.0" {
		t.Errorf("rating updates: %v", calls)
	}
}

func TestCasualAndBotGamesAreNotRated(t *testing.T) {
	_, srv, fr := newRatedHub(t)
	id := createGame(t, srv, "anna", `{"color":"white","timeMs":300000,"incMs":0}`)
	call(t, srv, "boris", "POST", "/api/games/"+id+"/join", "")
	anna := dial(t, srv, "anna", id)
	anna.until("state", func(m Message) bool { return m.Type == "state" })
	anna.send("resign", "")
	anna.until("end", func(m Message) bool { return m.Game != nil && m.Game.Status == StatusEnded })
	if m := snapshot(t, srv, "anna", id); m.Game.Rated || m.Game.RatingChange != nil || m.Game.White.Elo != 0 {
		t.Errorf("casual game: %+v", m.Game)
	}
	// against a bot "rated" is refused to be a rated game of two people: the bot ignores the flag
	bid := createGame(t, srv, "anna", `{"color":"white","timeMs":300000,"incMs":0,"bot":"misha","rated":true}`)
	b := dial(t, srv, "anna", bid)
	b.until("state", func(m Message) bool { return m.Type == "state" })
	b.send("resign", "")
	b.until("end", func(m Message) bool { return m.Game != nil && m.Game.Status == StatusEnded })
	time.Sleep(100 * time.Millisecond)
	if calls := fr.log(); len(calls) != 0 {
		t.Errorf("unrated games moved ratings: %v", calls)
	}
}

func TestRatedNeedsAClock(t *testing.T) {
	_, srv, _ := newRatedHub(t)
	if code, b := call(t, srv, "anna", "POST", "/api/games", `{"color":"white","rated":true}`); code != 400 {
		t.Errorf("no clock: %d %s", code, b)
	}
}

func TestMatchedGameIsRunningAtOnceWithBothRated(t *testing.T) {
	h, srv, fr := newRatedHub(t)
	id, err := h.NewMatched(context.Background(), Player{ID: "anna", Name: "Anna"}, Player{ID: "boris", Name: "Boris"}, TimeControl{BaseMs: 180000, IncMs: 2000}, true)
	if err != nil {
		t.Fatal(err)
	}
	m := snapshot(t, srv, "anna", id)
	if m.Game.Status != StatusPlaying || !m.Game.Rated || m.Game.White == nil || m.Game.Black == nil || m.Game.White.Elo != 1500 || m.Game.Black.Elo != 1500 {
		t.Fatalf("matched game: %+v", m.Game)
	}
	if m.Game.White.ID == m.Game.Black.ID {
		t.Error("one person on both seats")
	}
	// the draw of colours is not always the same
	seenWhite := map[string]bool{}
	for i := 0; i < 30 && len(seenWhite) < 2; i++ {
		id, _ := h.NewMatched(context.Background(), Player{ID: "anna"}, Player{ID: "boris"}, TimeControl{BaseMs: 180000}, false)
		seenWhite[snapshot(t, srv, "anna", id).Game.White.ID] = true
	}
	if len(seenWhite) < 2 {
		t.Error("colours are not drawn")
	}
	if _, err := h.NewMatched(context.Background(), Player{ID: "a"}, Player{ID: "b"}, TimeControl{}, true); err == nil {
		t.Error("a rated game without a clock was made")
	}
	_ = fr
}

func TestAGameGivenUpAtOnceIsNotRated(t *testing.T) {
	_, srv, fr := newRatedHub(t)
	id := createGame(t, srv, "anna", `{"color":"white","timeMs":300000,"incMs":0,"rated":true}`)
	call(t, srv, "boris", "POST", "/api/games/"+id+"/join", "")
	anna, boris := dial(t, srv, "anna", id), dial(t, srv, "boris", id)
	anna.until("state", func(m Message) bool { return m.Type == "state" })
	anna.send("move", "e2e4")
	boris.untilMoves(1)
	boris.send("resign", "")
	end := anna.until("end", func(m Message) bool {
		return m.Game != nil && m.Game.Status == StatusEnded && m.Game.RatingChange != nil
	})
	if end.Game.RatingChange.White != nil || end.Game.RatingChange.Black != nil {
		t.Errorf("a one-move game moved ratings: %+v", end.Game.RatingChange)
	}
	if calls := fr.log(); len(calls) != 0 {
		t.Errorf("rating updates: %v", calls)
	}
}

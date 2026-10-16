package puzzles

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dyanis-chess/backend/internal/ratings"
	"dyanis-chess/backend/internal/store"
)

type duelRatings struct {
	mu    sync.Mutex
	calls []string
}

func (d *duelRatings) Apply(_ context.Context, cat, a, b string, score float64) (ratings.Change, ratings.Change, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, fmt.Sprintf("%s %s %s %v", cat, a, b, score))
	return ratings.Change{Category: cat, Before: 1200, After: 1215}, ratings.Change{Category: cat, Before: 1200, After: 1185}, nil
}

type duelEnv struct {
	f   *fakeStore
	h   http.Handler
	s   *Service
	now *time.Time
	dr  *duelRatings
}

func newDuelEnv(t *testing.T) *duelEnv {
	t.Helper()
	f := &fakeStore{puzzle: &store.Puzzle{ID: "00008", FEN: fen00008, Moves: moves00008, Rating: 1900, Popularity: 95, Plays: 100, Themes: "crushing"}, rating: store.PuzzleRating{Rating: 1000}}
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	dr := &duelRatings{}
	s := New(Config{Store: f, Ratings: dr, UserID: func(r *http.Request) string { return r.Header.Get("U") }, Now: func() time.Time { return now }})
	return &duelEnv{f: f, h: s.Handler(), s: s, now: &now, dr: dr}
}

func (e *duelEnv) do(user, method, url, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("U", user)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *duelEnv) state(user, id string) duelJSON {
	var v duelJSON
	json.Unmarshal(e.do(user, "GET", "/api/duels/"+id, "").Body.Bytes(), &v)
	return v
}

func startDuel(t *testing.T, e *duelEnv) string {
	t.Helper()
	id, err := e.s.NewDuel(context.Background(), DuelPlayer{ID: "anna", Name: "Anna", Rating: 1200, Provisional: true}, DuelPlayer{ID: "boris", Name: "Boris", Rating: 1300})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestDuelClockAndVisibility(t *testing.T) {
	e := newDuelEnv(t)
	id := startDuel(t, e)
	v := e.state("anna", id)
	if v.Status != "countdown" || v.Me.Name != "Anna" || v.Opp.Name != "Boris" || v.EndsAt-v.StartsAt != int64(duelLength/time.Millisecond) {
		t.Fatalf("countdown: %+v", v)
	}
	// nobody else sees the duel, nobody anonymous either
	if rec := e.do("carl", "GET", "/api/duels/"+id, ""); rec.Code != 404 {
		t.Errorf("a stranger: %d", rec.Code)
	}
	if rec := e.do("", "GET", "/api/duels/"+id, ""); rec.Code != 401 {
		t.Errorf("nobody: %d", rec.Code)
	}
	// no puzzles and no answers before the start
	if rec := e.do("anna", "GET", "/api/duels/"+id+"/puzzle/0", ""); rec.Code != 409 {
		t.Errorf("peeking: %d", rec.Code)
	}
	if rec := e.do("anna", "POST", "/api/duels/"+id+"/progress", `{"index":0,"solved":true}`); rec.Code != 409 {
		t.Errorf("early progress: %d", rec.Code)
	}
	*e.now = e.now.Add(duelCountdown)
	if v := e.state("anna", id); v.Status != "running" {
		t.Errorf("after the countdown: %+v", v)
	}
	if _, err := e.s.NewDuel(context.Background(), DuelPlayer{ID: "x"}, DuelPlayer{ID: "x"}); err == nil {
		t.Error("a duel with oneself")
	}
}

func TestDuelScoresAreSharedAndTheWinnerIsRated(t *testing.T) {
	e := newDuelEnv(t)
	id := startDuel(t, e)
	*e.now = e.now.Add(duelCountdown + time.Second)

	// the same puzzle for both; the list is made as asked, in order
	if rec := e.do("anna", "GET", "/api/duels/"+id+"/puzzle/0", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"id":"00008"`) {
		t.Fatalf("puzzle 0: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do("boris", "GET", "/api/duels/"+id+"/puzzle/0", ""); rec.Code != 200 {
		t.Errorf("boris puzzle 0: %d", rec.Code)
	}
	if rec := e.do("anna", "GET", "/api/duels/"+id+"/puzzle/5", ""); rec.Code != 400 {
		t.Errorf("a puzzle far ahead: %d", rec.Code)
	}
	// the first puzzle starts near (average 1250 - 500 = 750), each next one is harder
	if len(e.f.picked) == 0 || e.f.picked[0][0] != 650 || e.f.picked[0][1] != 850 {
		t.Errorf("first window: %v", e.f.picked)
	}

	post := func(user, body string) (int, duelJSON) {
		rec := e.do(user, "POST", "/api/duels/"+id+"/progress", body)
		var v duelJSON
		json.Unmarshal(rec.Body.Bytes(), &v)
		return rec.Code, v
	}
	if code, v := post("anna", `{"index":0,"solved":true}`); code != 200 || v.Me.Score != 1 || v.Me.Index != 1 {
		t.Fatalf("anna solves: %d %+v", code, v)
	}
	post("anna", `{"index":1,"solved":true}`)
	if code, v := post("boris", `{"index":0,"solved":false}`); code != 200 || v.Me.Mistakes != 1 || v.Opp.Score != 2 {
		t.Errorf("boris fails and sees anna's score: %d %+v", code, v)
	}
	// out of step: told where they really are, nothing counted
	if code, v := post("anna", `{"index":0,"solved":true}`); code != 409 || v.Me.Index != 2 || v.Me.Score != 2 {
		t.Errorf("repeat: %d %+v", code, v)
	}

	*e.now = e.now.Add(duelLength + duelGrace) // past the end and the grace
	v := e.state("anna", id)
	if v.Status != "over" || v.Outcome != "win" || v.Me.Change == nil || v.Me.Change.After != 1215 {
		t.Fatalf("over for anna: %+v", v)
	}
	if b := e.state("boris", id); b.Outcome != "loss" || b.Me.Change.After != 1185 {
		t.Errorf("over for boris: %+v", b)
	}
	if len(e.f.duels) != 1 || e.f.duels[0].Result != "a" || e.f.duels[0].AScore != 2 || e.f.duels[0].BMistakes != 1 {
		t.Errorf("record: %+v", e.f.duels)
	}
	if len(e.dr.calls) != 1 || e.dr.calls[0] != "duel anna boris 1" {
		t.Errorf("ratings: %v", e.dr.calls)
	}
	if code, _ := post("boris", `{"index":1,"solved":true}`); code != 409 {
		t.Errorf("progress after the end: %d", code)
	}
}

func TestDuelTieBreaksAndDraws(t *testing.T) {
	e := newDuelEnv(t)
	id := startDuel(t, e)
	*e.now = e.now.Add(duelCountdown + time.Second)
	post := func(user, body string) { e.do(user, "POST", "/api/duels/"+id+"/progress", body) }
	post("anna", `{"index":0,"solved":true}`)
	post("boris", `{"index":0,"solved":true}`)
	post("anna", `{"index":1,"solved":false}`) // equal scores, anna has a mistake more
	*e.now = e.now.Add(duelLength + duelGrace)
	if v := e.state("boris", id); v.Outcome != "win" {
		t.Errorf("fewer mistakes wins a tie: %+v", v)
	}
	// nothing at all: a draw
	e2 := newDuelEnv(t)
	id2 := startDuel(t, e2)
	*e2.now = e2.now.Add(duelCountdown + duelLength + duelGrace)
	if v := e2.state("anna", id2); v.Outcome != "draw" || len(e2.dr.calls) != 1 || !strings.HasSuffix(e2.dr.calls[0], "0.5") {
		t.Errorf("draw: %+v %v", v, e2.dr.calls)
	}
}

package match

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dyanis-chess/backend/internal/ratings"
	"dyanis-chess/backend/internal/store"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type made struct {
	a, b string
	req  Request
}

func newSvc(t *testing.T) (*Service, *clock, *[]made) {
	t.Helper()
	c := &clock{t: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}
	var games []made
	maker := func(prefix string) Maker {
		return func(_ context.Context, a, b Seeker, req Request) (string, error) {
			games = append(games, made{a.ID, b.ID, req})
			return prefix + "1", nil
		}
	}
	s := New(Config{
		User: func(r *http.Request) *store.User {
			return &store.User{ID: r.Header.Get("U"), Name: "N" + r.Header.Get("U")}
		},
		Rating: func(context.Context, string, string) (ratings.Entry, error) { return ratings.Entry{Rating: 1500}, nil },
		Chess:  maker("g"), Duel: maker("d"), Now: c.now,
	})
	t.Cleanup(s.Close)
	return s, c, &games
}

var blitz = Request{Kind: KindChess, BaseMs: 180000, IncMs: 2000, Rated: true}

func TestCloseRatingsArePairedAtOnce(t *testing.T) {
	s, _, games := newSvc(t)
	ctx := context.Background()
	s.Join(ctx, Seeker{ID: "anna", Rating: 1500}, blitz)
	if st := s.Check("anna"); st.State != "waiting" || st.Count != 1 {
		t.Fatalf("alone: %+v", st)
	}
	s.Join(ctx, Seeker{ID: "boris", Rating: 1560}, blitz)
	if len(*games) != 1 {
		t.Fatalf("games made: %d", len(*games))
	}
	for _, u := range []string{"anna", "boris"} {
		if st := s.Check(u); st.State != "found" || st.ID != "g1" || st.Kind != KindChess {
			t.Errorf("%s: %+v", u, st)
		}
	}
}

func TestTheWindowWidensWithWaiting(t *testing.T) {
	s, c, games := newSvc(t)
	ctx := context.Background()
	s.Join(ctx, Seeker{ID: "newbie", Rating: 1200}, blitz)
	s.Join(ctx, Seeker{ID: "master", Rating: 1700}, blitz) // 500 apart: too far at first
	if len(*games) != 0 {
		t.Fatal("paired across 500 points at once")
	}
	for i := 0; i < 8; i++ { // 8 s: window 100 + 25*8 = 300, still too far
		c.add(time.Second)
		s.Check("newbie")
		s.Check("master")
		s.Pair()
	}
	if len(*games) != 0 {
		t.Fatal("paired at 300")
	}
	for i := 0; i < 10; i++ { // 18 s: window 550
		c.add(time.Second)
		s.Check("newbie")
		s.Check("master")
		s.Pair()
	}
	if len(*games) != 1 {
		t.Fatalf("not paired after waiting: %d", len(*games))
	}
}

func TestOnlyTheSameKindOfGameIsPaired(t *testing.T) {
	s, _, games := newSvc(t)
	ctx := context.Background()
	s.Join(ctx, Seeker{ID: "a", Rating: 1500}, blitz)
	other := blitz
	other.IncMs = 0
	s.Join(ctx, Seeker{ID: "b", Rating: 1500}, other) // another clock
	casual := blitz
	casual.Rated = false
	s.Join(ctx, Seeker{ID: "c", Rating: 1500}, casual) // rated and casual do not mix
	s.Join(ctx, Seeker{ID: "d", Rating: 1500}, Request{Kind: KindDuel})
	if len(*games) != 0 {
		t.Fatalf("mixed kinds: %v", *games)
	}
	s.Join(ctx, Seeker{ID: "e", Rating: 1510}, Request{Kind: KindDuel})
	if len(*games) != 1 || (*games)[0].req.Kind != KindDuel {
		t.Fatalf("duel: %v", *games)
	}
	if st := s.Check("d"); st.ID != "d1" {
		t.Errorf("duel id: %+v", st)
	}
}

func TestLeaveAndAbandonedSeeks(t *testing.T) {
	s, c, games := newSvc(t)
	ctx := context.Background()
	s.Join(ctx, Seeker{ID: "a", Rating: 1500}, blitz)
	if id := s.Leave("a"); id != "" || s.Check("a").State != "none" {
		t.Error("leave")
	}
	// somebody who stopped asking is dropped, and is not paired with
	s.Join(ctx, Seeker{ID: "ghost", Rating: 1500}, blitz)
	c.add(30 * time.Second)
	s.Pair()
	s.Join(ctx, Seeker{ID: "b", Rating: 1500}, blitz)
	if len(*games) != 0 || s.Check("ghost").State != "none" {
		t.Errorf("ghost was paired: %v", *games)
	}
	// leaving after being paired tells the game
	s.Join(ctx, Seeker{ID: "c", Rating: 1500}, blitz)
	if id := s.Leave("c"); id != "g1" {
		t.Errorf("too late to leave: %q", id)
	}
	// a person asking again replaces the old request
	s.Join(ctx, Seeker{ID: "z", Rating: 1500}, blitz)
	s.Join(ctx, Seeker{ID: "z", Rating: 1500}, Request{Kind: KindDuel})
	if st := s.Check("z"); st.Kind != KindDuel {
		t.Errorf("replaced: %+v", st)
	}
}

func TestAPI(t *testing.T) {
	s, _, _ := newSvc(t)
	h := s.Handler()
	do := func(method, user, body string) (int, Status) {
		req := httptest.NewRequest(method, "/api/match/seek", strings.NewReader(body))
		req.Header.Set("U", user)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var st Status
		json.Unmarshal(rec.Body.Bytes(), &st)
		return rec.Code, st
	}
	if code, st := do("POST", "anna", `{"kind":"chess","baseMs":180000,"incMs":0,"rated":true}`); code != 200 || st.State != "waiting" {
		t.Fatalf("post: %d %+v", code, st)
	}
	if code, st := do("POST", "boris", `{"kind":"chess","baseMs":180000,"incMs":0,"rated":true}`); code != 200 || st.State != "found" || st.ID != "g1" {
		t.Fatalf("second: %d %+v", code, st)
	}
	if _, st := do("GET", "anna", ""); st.State != "found" {
		t.Errorf("get: %+v", st)
	}
	for _, bad := range []string{`{"kind":"chess","baseMs":0}`, `{"kind":"chess","baseMs":5}`, `{"kind":"poker"}`, `nope`} {
		if code, _ := do("POST", "x", bad); code != 400 {
			t.Errorf("%s: %d", bad, code)
		}
	}
	if code, _ := do("DELETE", "anna", ""); code != 200 {
		t.Errorf("delete: %d", code)
	}
}

func TestRatingLimitsOfBothSidesApply(t *testing.T) {
	s, c, games := newSvc(t)
	ctx := context.Background()
	picky := blitz
	picky.RangeUp = 100 // will not meet anybody more than 100 points stronger
	s.Join(ctx, Seeker{ID: "picky", Rating: 1500}, picky)
	s.Join(ctx, Seeker{ID: "strong", Rating: 1580}, blitz) // within 100: paired at once
	if len(*games) != 1 {
		t.Fatalf("within the limit: %d games", len(*games))
	}
	s.Leave("picky")
	s.Leave("strong")
	*games = nil

	s.Join(ctx, Seeker{ID: "picky", Rating: 1500}, picky)
	s.Join(ctx, Seeker{ID: "stronger", Rating: 1600}, blitz) // 100 apart is fine, 101 is not
	if len(*games) != 1 {
		t.Fatalf("at the limit: %d games", len(*games))
	}
	s.Leave("picky")
	s.Leave("stronger")
	*games = nil

	s.Join(ctx, Seeker{ID: "picky", Rating: 1500}, picky)
	s.Join(ctx, Seeker{ID: "far", Rating: 1650}, blitz)
	for i := 0; i < 20; i++ { // however long they wait, the limit holds
		c.add(time.Second)
		s.Check("picky")
		s.Check("far")
		s.Pair()
	}
	if len(*games) != 0 {
		t.Fatal("paired beyond the limit")
	}
	// the other side's limit counts as well: the weaker one may not be met by somebody who allows only equals
	s.Leave("picky")
	s.Leave("far")
	proud := blitz
	proud.RangeDown = 50
	s.Join(ctx, Seeker{ID: "proud", Rating: 1500}, proud)
	s.Join(ctx, Seeker{ID: "weak", Rating: 1400}, blitz)
	for i := 0; i < 5; i++ {
		c.add(time.Second)
		s.Check("proud")
		s.Check("weak")
		s.Pair()
	}
	if len(*games) != 0 {
		t.Fatal("paired beyond the limit of the stronger")
	}
	if code := postStatus(t, s, `{"kind":"chess","baseMs":180000,"rangeUp":5000}`); code != 400 {
		t.Errorf("a limit beyond 3000: %d", code)
	}
}

func postStatus(t *testing.T, s *Service, body string) int {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/match/seek", strings.NewReader(body))
	req.Header.Set("U", "x")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec.Code
}

func TestPeopleWhoBlockedEachOtherAreNeverPaired(t *testing.T) {
	s, c, games := newSvc(t)
	blocks := map[string]map[string]bool{ // what store.BlockedWith returns: both directions
		"anna":  {"boris": true},
		"boris": {"anna": true},
	}
	s.cfg.Avoid = func(_ context.Context, id string) (map[string]bool, error) { return blocks[id], nil }
	ctx := context.Background()

	s.Join(ctx, Seeker{ID: "anna", Rating: 1500}, blitz)
	s.Join(ctx, Seeker{ID: "boris", Rating: 1500}, blitz) // the same rating, but blocked
	c.add(5 * time.Second)
	s.Pair()
	if len(*games) != 0 {
		t.Fatalf("a blocked pair was matched: %+v", *games)
	}
	// A third person is paired with the one who waited longest and is not blocked.
	s.Join(ctx, Seeker{ID: "vera", Rating: 1540}, blitz)
	if len(*games) != 1 {
		t.Fatalf("games made: %d", len(*games))
	}
	g := (*games)[0]
	if (g.a == "boris" && g.b == "anna") || (g.a == "anna" && g.b == "boris") {
		t.Errorf("paired the blocked: %+v", g)
	}
	if st := s.Check("boris"); st.State != "waiting" && st.State != "found" {
		t.Errorf("boris: %+v", st)
	}

	// Duels are matched by the same loop.
	d := Request{Kind: KindDuel}
	s.Join(ctx, Seeker{ID: "anna", Rating: 1500}, d)
	s.Join(ctx, Seeker{ID: "boris", Rating: 1500}, d)
	c.add(10 * time.Second)
	s.Pair()
	for _, m := range *games {
		if m.req.Kind == KindDuel {
			t.Errorf("a blocked pair was matched in a duel: %+v", m)
		}
	}

	// A failing lookup does not stop anybody from playing.
	s.cfg.Avoid = func(context.Context, string) (map[string]bool, error) { return nil, context.DeadlineExceeded }
	s.Join(ctx, Seeker{ID: "p", Rating: 1800}, blitz)
	s.Join(ctx, Seeker{ID: "q", Rating: 1800}, blitz)
	if st := s.Check("p"); st.State != "found" {
		t.Errorf("p: %+v", st)
	}
}

func TestAccountsOfOnePersonAreNotPairedForRating(t *testing.T) {
	c := &clock{t: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)}
	var games []made
	maker := func(_ context.Context, a, b Seeker, req Request) (string, error) {
		games = append(games, made{a.ID, b.ID, req})
		return "g1", nil
	}
	s := New(Config{
		User:   func(r *http.Request) *store.User { return &store.User{ID: r.Header.Get("U")} },
		Rating: func(context.Context, string, string) (ratings.Entry, error) { return ratings.Entry{Rating: 1500}, nil },
		Chess:  maker, Duel: maker, Now: c.now,
		Related: func(_ context.Context, id string) (map[string]bool, error) {
			switch id {
			case "anna":
				return map[string]bool{"anna2": true}, nil
			case "anna2":
				return map[string]bool{"anna": true}, nil
			}
			return nil, nil
		},
	})
	t.Cleanup(s.Close)
	ctx := context.Background()

	s.Join(ctx, Seeker{ID: "anna", Rating: 1500}, blitz)
	s.Join(ctx, Seeker{ID: "anna2", Rating: 1500}, blitz)
	if len(games) != 0 {
		t.Fatalf("a rated game between the accounts of one person was made")
	}
	c.add(15 * time.Second) // they have waited a while (but are not given up on); they still must not meet
	s.Pair()
	if len(games) != 0 {
		t.Fatalf("paired after waiting")
	}
	s.Join(ctx, Seeker{ID: "boris", Rating: 1500}, blitz) // a stranger takes the longest waiting one
	if len(games) != 1 || (games[0].a != "anna" && games[0].b != "anna") {
		t.Fatalf("games: %+v", games)
	}

	// duels count for rating too; a casual game does not
	games = nil
	s.Join(ctx, Seeker{ID: "anna", Rating: 1500}, Request{Kind: KindDuel})
	s.Join(ctx, Seeker{ID: "anna2", Rating: 1500}, Request{Kind: KindDuel})
	if len(games) != 0 {
		t.Errorf("duel between one person's accounts")
	}
	casual := Request{Kind: KindChess, BaseMs: 300000}
	s.Join(ctx, Seeker{ID: "anna", Rating: 1500}, casual)
	s.Join(ctx, Seeker{ID: "anna2", Rating: 1500}, casual)
	if len(games) != 1 {
		t.Errorf("a casual game may be played: %+v", games)
	}
}

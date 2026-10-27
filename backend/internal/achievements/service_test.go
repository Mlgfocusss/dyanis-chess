package achievements

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dyanis-chess/backend/internal/store"
)

// fake is an in-memory Store that counts how often it is asked.
type fake struct {
	mu        sync.Mutex
	users     map[string]*store.User
	facts     map[string]store.AchievementFacts
	rows      map[string][]store.AchievementRow // newest first
	accRows   []store.ReviewBrief
	brilRows  []store.BrilliantReview
	factCalls int
	accCalls  int
	brilCalls int
}

func newFake() *fake {
	return &fake{users: map[string]*store.User{}, facts: map[string]store.AchievementFacts{}, rows: map[string][]store.AchievementRow{}}
}

func (f *fake) UserByID(_ context.Context, id string) (*store.User, error) {
	if u, ok := f.users[id]; ok {
		return u, nil
	}
	return nil, store.ErrNotFound
}
func (f *fake) AchievementFacts(_ context.Context, id string, _ store.AchievementQuery) (store.AchievementFacts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.factCalls++
	return f.facts[id], nil
}
func (f *fake) AccuracyReviews(context.Context, string, float64, int, int) ([]store.ReviewBrief, error) {
	f.accCalls++
	return f.accRows, nil
}
func (f *fake) BrilliantReviews(context.Context, string, int) ([]store.BrilliantReview, error) {
	f.brilCalls++
	return f.brilRows, nil
}
func (f *fake) PuzzleDays(context.Context, string) (map[string]int, error) { return nil, nil }
func (f *fake) Achievements(_ context.Context, id string) ([]store.AchievementRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]store.AchievementRow(nil), f.rows[id]...), nil
}
func (f *fake) AddAchievements(_ context.Context, id string, keys []string, at time.Time) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var added []string
	for _, k := range keys {
		dup := false
		for _, r := range f.rows[id] {
			dup = dup || r.Key == k
		}
		if !dup {
			f.rows[id] = append([]store.AchievementRow{{Key: k, UnlockedAt: at}}, f.rows[id]...)
			added = append(added, k)
		}
	}
	return added, nil
}
func (f *fake) MarkAchievementsSeen(_ context.Context, id string, keys []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.rows[id] {
		if len(keys) == 0 {
			f.rows[id][i].Seen = true
		}
		for _, k := range keys {
			if f.rows[id][i].Key == k {
				f.rows[id][i].Seen = true
			}
		}
	}
	return nil
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newService(f *fake) (*Service, *clock) {
	c := &clock{t: time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)}
	return New(Config{Store: f, UserID: func(r *http.Request) string { return r.Header.Get("X-User") }, Now: c.now,
		Sections: [][]string{{"a", "b"}}}), c
}

func ids(m *Mine) map[string]Item {
	out := map[string]Item{}
	for _, it := range m.Items {
		out[it.ID] = it
	}
	return out
}

func TestMineRecordsNewBadgesOnce(t *testing.T) {
	f := newFake()
	f.users["u1"] = &store.User{ID: "u1", Name: "Гость 1"}
	f.facts["u1"] = store.AchievementFacts{Games: 12, BotsBeaten: 1}
	s, clk := newService(f)
	ctx := context.Background()

	m, err := s.Mine(ctx, "u1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	by := ids(m)
	if !by["first_game"].Unlocked || !by["games_10"].Unlocked || by["games_100"].Unlocked || !by["first_bot_win"].Unlocked {
		t.Errorf("unlocked wrongly: %+v", by)
	}
	if m.Unlocked != 3 || m.Total != len(Catalog) || len(m.Items) != len(Catalog) {
		t.Errorf("counts: %d of %d, %d items", m.Unlocked, m.Total, len(m.Items))
	}
	if len(m.Unseen) != 3 || by["first_game"].Seen {
		t.Errorf("a new badge is unseen: %v", m.Unseen)
	}
	if it := by["games_100"]; it.Progress != 12 || it.Target != 100 {
		t.Errorf("progress: %+v", it)
	}
	if by["first_game"].UnlockedAt == nil || !by["first_game"].UnlockedAt.Equal(clk.t) {
		t.Errorf("date: %v", by["first_game"].UnlockedAt)
	}

	// a minute later more is earned; the old date stays
	clk.t = clk.t.Add(2 * time.Minute)
	f.facts["u1"] = store.AchievementFacts{Games: 100, BotsBeaten: 1}
	m, _ = s.Mine(ctx, "u1", time.Minute)
	by = ids(m)
	if !by["games_100"].Unlocked || !by["games_100"].UnlockedAt.Equal(clk.t) {
		t.Errorf("games_100: %+v", by["games_100"])
	}
	if !by["first_game"].UnlockedAt.Equal(clk.t.Add(-2 * time.Minute)) {
		t.Errorf("the date of an earlier badge moved: %v", by["first_game"].UnlockedAt)
	}
	if len(f.rows["u1"]) != 4 {
		t.Errorf("rows: %+v", f.rows["u1"])
	}

	// once earned, a badge stays when the number falls (a friend is removed)
	f.facts["u1"] = store.AchievementFacts{}
	clk.t = clk.t.Add(2 * time.Minute)
	m, _ = s.Mine(ctx, "u1", time.Minute)
	if !ids(m)["games_100"].Unlocked || m.Unlocked != 4 {
		t.Errorf("a badge must not vanish: %d", m.Unlocked)
	}
}

func TestEvaluationIsCachedPerPerson(t *testing.T) {
	f := newFake()
	f.users["u1"] = &store.User{ID: "u1"}
	s, clk := newService(f)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if _, err := s.Mine(ctx, "u1", time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	if f.factCalls != 1 {
		t.Errorf("the facts were counted %d times in a minute", f.factCalls)
	}
	clk.t = clk.t.Add(61 * time.Second)
	s.Mine(ctx, "u1", time.Minute)
	if f.factCalls != 2 {
		t.Errorf("after the age the facts are counted again: %d", f.factCalls)
	}
	// a shorter age (the profile page) asks again sooner
	clk.t = clk.t.Add(11 * time.Second)
	s.Mine(ctx, "u1", ProfileMaxAge)
	if f.factCalls != 3 {
		t.Errorf("the profile wants fresher numbers: %d", f.factCalls)
	}
}

func TestSeenIsRemembered(t *testing.T) {
	f := newFake()
	f.users["u1"] = &store.User{ID: "u1"}
	f.facts["u1"] = store.AchievementFacts{Games: 1}
	s, _ := newService(f)
	ctx := context.Background()
	m, _ := s.Mine(ctx, "u1", time.Minute)
	if len(m.Unseen) != 1 || m.Unseen[0] != "first_game" {
		t.Fatalf("unseen: %v", m.Unseen)
	}
	if err := f.MarkAchievementsSeen(ctx, "u1", m.Unseen); err != nil {
		t.Fatal(err)
	}
	m, _ = s.Mine(ctx, "u1", time.Minute)
	if len(m.Unseen) != 0 || !ids(m)["first_game"].Seen {
		t.Errorf("after the notice: %v", m.Unseen)
	}
}

func TestReviewSearchesRunOnlyWhenNeeded(t *testing.T) {
	f := newFake()
	f.users["u1"] = &store.User{ID: "u1", Name: "Гость 1"}
	f.facts["u1"] = store.AchievementFacts{Reviews: 3}
	s, clk := newService(f)
	ctx := context.Background()

	s.Mine(ctx, "u1", 0)
	if f.accCalls != 1 || f.brilCalls != 1 {
		t.Fatalf("first look: %d %d", f.accCalls, f.brilCalls)
	}
	clk.t = clk.t.Add(time.Hour)
	s.Mine(ctx, "u1", 0)
	if f.accCalls != 1 || f.brilCalls != 1 {
		t.Errorf("no new review, no new search: %d %d", f.accCalls, f.brilCalls)
	}
	f.facts["u1"] = store.AchievementFacts{Reviews: 4}
	f.accRows = []store.ReviewBrief{{Game: []byte(`{"white":"Гость 1","black":"Миша"}`), Summary: []byte(`{"accuracy":[93,50],"plies":50}`)}}
	f.brilRows = []store.BrilliantReview{{Game: []byte(`{"white":"Гость 1","black":"Миша"}`), Moves: []int{6}}}
	m, _ := s.Mine(ctx, "u1", 0)
	if f.accCalls != 2 || f.brilCalls != 2 {
		t.Errorf("a new review is searched: %d %d", f.accCalls, f.brilCalls)
	}
	by := ids(m)
	if !by["accuracy_90"].Unlocked || !by["brilliant_move"].Unlocked {
		t.Errorf("both should be earned: %+v %+v", by["accuracy_90"], by["brilliant_move"])
	}
	// earned: never searched again
	f.facts["u1"] = store.AchievementFacts{Reviews: 5}
	clk.t = clk.t.Add(time.Hour)
	s.Mine(ctx, "u1", 0)
	if f.accCalls != 2 || f.brilCalls != 2 {
		t.Errorf("an earned badge needs no search: %d %d", f.accCalls, f.brilCalls)
	}
}

func TestPublicShowsOnlyEarnedAndNeedsAUser(t *testing.T) {
	f := newFake()
	f.users["u1"] = &store.User{ID: "u1"}
	f.facts["u1"] = store.AchievementFacts{Games: 11, Friends: 1}
	s, _ := newService(f)
	p, err := s.Public(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Unlocked != 3 || len(p.Items) != 3 || p.Total != len(Catalog) {
		t.Errorf("%+v", p)
	}
	if _, err := s.Public(context.Background(), "nobody"); err != store.ErrNotFound {
		t.Errorf("unknown person: %v", err)
	}
	if len(s.cache) != 1 {
		t.Errorf("the unknown person got into the cache: %d", len(s.cache))
	}
}

func TestHandlers(t *testing.T) {
	f := newFake()
	f.users["u1"] = &store.User{ID: "u1"}
	f.facts["u1"] = store.AchievementFacts{Games: 1}
	s, _ := newService(f)
	h := s.Handler()

	do := func(method, path, user, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if user != "" {
			req.Header.Set("X-User", user)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := do("GET", "/api/achievements", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", rec.Code)
	}
	rec := do("GET", "/api/achievements?poll=1", "u1", "")
	var m Mine
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &m) != nil || m.Unlocked != 1 || len(m.Unseen) != 1 {
		t.Fatalf("mine: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do("POST", "/api/achievements/seen", "u1", `{"ids":["nope"]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown id: %d", rec.Code)
	}
	if rec := do("POST", "/api/achievements/seen", "", `{}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("seen anonymous: %d", rec.Code)
	}
	if rec := do("POST", "/api/achievements/seen", "u1", `{"ids":["first_game"]}`); rec.Code != 200 {
		t.Errorf("seen: %d", rec.Code)
	}
	if rec := do("POST", "/api/achievements/seen", "u1", ``); rec.Code != 200 { // no body: all
		t.Errorf("seen all: %d", rec.Code)
	}
	if !f.rows["u1"][0].Seen {
		t.Error("not marked")
	}
	rec = do("GET", "/api/users/u1/achievements", "", "")
	var p Public
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &p) != nil || p.Unlocked != 1 || p.Items[0].ID != "first_game" {
		t.Fatalf("public: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "seen") || strings.Contains(rec.Body.String(), "progress") {
		t.Errorf("the public answer carries private fields: %s", rec.Body.String())
	}
	if rec := do("GET", "/api/users/zzz/achievements", "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown: %d", rec.Code)
	}
}

func TestCacheStaysBounded(t *testing.T) {
	f := newFake()
	s, _ := newService(f)
	for i := 0; i < cacheLimit+500; i++ {
		s.entryOf(string(rune('a'+i%26)) + strings.Repeat("x", i%7) + time.Duration(i).String())
	}
	if len(s.cache) > cacheLimit {
		t.Errorf("cache grew to %d", len(s.cache))
	}
}

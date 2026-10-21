package stats

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dyanis-chess/backend/internal/ratings"
	"dyanis-chess/backend/internal/store"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestBuildStreak(t *testing.T) {
	days := map[string]int{
		"2026-10-01": 1, "2026-10-02": 3, "2026-10-03": 1, "2026-10-04": 2, "2026-10-05": 1, // a run of five
		"2026-10-18": 1, "2026-10-19": 4, // the current run
	}
	// Today (the 20th) has no puzzle yet: the run of the 18th and 19th is not broken.
	s := BuildStreak(days, day("2026-10-20").Add(9*time.Hour))
	if s.Today || s.Current != 2 || s.Best != 5 {
		t.Errorf("today empty: %+v", s)
	}

	days["2026-10-20"] = 1
	s = BuildStreak(days, day("2026-10-20").Add(23*time.Hour+59*time.Minute))
	if !s.Today || s.Current != 3 || s.Best != 5 {
		t.Errorf("today done: %+v", s)
	}

	// A missed day breaks it: the 19th is empty, so on the 21st nothing is left.
	delete(days, "2026-10-19")
	delete(days, "2026-10-20")
	s = BuildStreak(days, day("2026-10-21"))
	if s.Current != 0 || s.Best != 5 {
		t.Errorf("broken: %+v", s)
	}

	// No data at all.
	s = BuildStreak(nil, day("2026-10-21"))
	if s.Current != 0 || s.Best != 0 || s.Today || len(s.Days) != ActivityDays {
		t.Errorf("empty: %+v", s)
	}

	// A streak longer than the 30 days shown still counts in full; the best is the current one.
	long := map[string]int{}
	for d := day("2026-08-01"); !d.After(day("2026-10-20")); d = d.AddDate(0, 0, 1) {
		long[d.Format("2006-01-02")] = 1
	}
	s = BuildStreak(long, day("2026-10-20"))
	if s.Current != 81 || s.Best != 81 {
		t.Errorf("long: %+v", s)
	}
}

func TestStreakDays(t *testing.T) {
	days := map[string]int{"2026-10-20": 3, "2026-10-19": 1, "2026-09-21": 7, "2026-09-20": 9}
	s := BuildStreak(days, day("2026-10-20").Add(5*time.Hour))
	if s.From != "2026-09-21" || len(s.Days) != 30 {
		t.Fatalf("window: %s %d", s.From, len(s.Days))
	}
	if s.Days[0] != 7 || s.Days[28] != 1 || s.Days[29] != 3 { // the 20th of September is outside
		t.Errorf("days: %v", s.Days)
	}
}

func TestSideOf(t *testing.T) {
	cases := []struct{ name, white, black, want string }{
		{"Дима", "Дима", "Лиза", "white"},
		{"Дима", "Лиза", " дима ", "black"},
		{"Дима", "Лиза", "Борис", "both"},
		{"Дима", "Дима", "Дима", "both"},
		{"", "", "", "both"},
	}
	for _, c := range cases {
		if got := SideOf(c.name, c.white, c.black); got != c.want {
			t.Errorf("SideOf(%q, %q, %q) = %s, want %s", c.name, c.white, c.black, got, c.want)
		}
	}
}

func TestWeakestThemes(t *testing.T) {
	tf := func(theme string, attempts, failed int) store.ThemeFailures {
		return store.ThemeFailures{Theme: theme, Attempts: attempts, Failed: failed}
	}
	got := WeakestThemes([]store.ThemeFailures{
		tf("middlegame", 50, 30), tf("short", 40, 20), // technical: never listed
		tf("fork", 10, 4), tf("pin", 5, 4), tf("skewer", 20, 4), tf("mate", 3, 1), tf("zugzwang", 2, 0),
	}, 4)
	var names []string
	for _, th := range got {
		names = append(names, th.Theme)
	}
	// 4 failures: pin (4/5), fork (4/10), skewer (4/20); then mate with 1
	want := []string{"pin", "fork", "skewer", "mate"}
	if len(names) != 4 || names[0] != want[0] || names[1] != want[1] || names[2] != want[2] || names[3] != want[3] {
		t.Errorf("order: %v, want %v", names, want)
	}
	if got := WeakestThemes(nil, 3); got == nil || len(got) != 0 {
		t.Errorf("empty must be a non-nil empty list: %#v", got)
	}
}

// fake is a Store with fixed answers.
type fake struct {
	user     *store.User
	humans   store.Tally
	bots     store.Tally
	duels    store.Tally
	reviews  []store.ReviewBrief
	count    int
	rating   store.PuzzleRating
	fails    []store.ThemeFailures
	attempts []store.AttemptRow
	days     map[string]int
	rush     map[string]store.RushBest
}

func (f *fake) UserByID(context.Context, string) (*store.User, error) { return f.user, nil }
func (f *fake) GameTally(context.Context, string) (store.Tally, store.Tally, error) {
	return f.humans, f.bots, nil
}
func (f *fake) DuelTally(context.Context, string) (store.Tally, error) { return f.duels, nil }
func (f *fake) ReviewCount(context.Context, string) (int, error)       { return f.count, nil }
func (f *fake) RecentReviews(_ context.Context, _ string, limit int) ([]store.ReviewBrief, error) {
	return f.reviews[:min(limit, len(f.reviews))], nil
}
func (f *fake) PuzzleRatingOf(context.Context, string) (store.PuzzleRating, error) {
	return f.rating, nil
}
func (f *fake) PuzzleThemeFailures(context.Context, string) ([]store.ThemeFailures, error) {
	return f.fails, nil
}
func (f *fake) RecentPuzzleAttempts(context.Context, string, int) ([]store.AttemptRow, error) {
	return f.attempts, nil
}
func (f *fake) PuzzlePointsOf(context.Context, string, string) (store.PuzzlePoints, error) {
	return store.PuzzlePoints{Total: 42, Today: 7, Streak: 2, BestStreak: 9}, nil
}
func (f *fake) PuzzleDays(context.Context, string) (map[string]int, error) { return f.days, nil }
func (f *fake) RushBests(context.Context, string) (map[string]store.RushBest, error) {
	return f.rush, nil
}

type fakeRatings struct{}

func (fakeRatings) All(context.Context, string) ([]ratings.Entry, error) {
	return []ratings.Entry{
		{Category: "bullet", Name: "Пуля", Rating: 1500, Provisional: true},
		{Category: "blitz", Name: "Блиц", Rating: 1620, Games: 4, Wins: 2, Draws: 1, Losses: 1},
		{Category: "rapid", Name: "Рапид", Rating: 1500},
		{Category: "duel", Name: "Дуэль", Rating: 1710, Games: 2, Wins: 2},
	}, nil
}

func review(id, white, black string, w, b float64, at time.Time) store.ReviewBrief {
	g, _ := json.Marshal(map[string]string{"white": white, "black": black})
	s, _ := json.Marshal(map[string]any{"accuracy": [2]float64{w, b}})
	return store.ReviewBrief{ID: id, Game: g, Summary: s, CreatedAt: at}
}

func get(t *testing.T, svc *Service, uid string) (*httptest.ResponseRecorder, Answer) {
	t.Helper()
	svc.cfg.UserID = func(*http.Request) string { return uid }
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/stats", nil))
	var a Answer
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
			t.Fatalf("body: %v\n%s", err, rec.Body.String())
		}
	}
	return rec, a
}

func TestAnswer(t *testing.T) {
	at := day("2026-10-10")
	f := &fake{
		user:   &store.User{ID: "me", Name: "Дима"},
		humans: store.Tally{Wins: 3, Draws: 1, Losses: 2},
		bots:   store.Tally{Wins: 5, Losses: 1},
		duels:  store.Tally{Wins: 2, Losses: 1},
		count:  4,
		// newest first, as the store gives them
		reviews: []store.ReviewBrief{
			review("r3", "Дима", "Лиза", 90, 50, at.Add(3*time.Hour)),
			review("r2", "Лиза", "дима", 40, 70, at.Add(2*time.Hour)),
			review("r1", "Иван", "Лиза", 60, 80, at.Add(time.Hour)), // neither is the person: the mean, 70
		},
		rating:   store.PuzzleRating{Rating: 1234, Attempts: 8, Solved: 5},
		fails:    []store.ThemeFailures{{Theme: "fork", Attempts: 3, Failed: 2}, {Theme: "short", Attempts: 8, Failed: 3}},
		attempts: []store.AttemptRow{{PuzzleID: "abcde", Solved: false, Before: 1250, After: 1234, At: at}},
		days:     map[string]int{"2026-10-19": 2, "2026-10-20": 1},
		rush:     map[string]store.RushBest{"3min": {Best: 14, Runs: 2}},
	}
	svc := New(Config{Store: f, Ratings: fakeRatings{}, Now: func() time.Time { return day("2026-10-20").Add(10 * time.Hour) }})

	if rec, _ := get(t, svc, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", rec.Code)
	}
	rec, a := get(t, svc, "me")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status %d, headers %v", rec.Code, rec.Header())
	}

	g := a.Games
	if g.Total != 12 || g.Wins != 8 || g.Draws != 1 || g.Losses != 3 || g.Humans.Total != 6 || g.Bots.Total != 6 {
		t.Errorf("games: %+v", g)
	}
	if len(g.Categories) != 3 || g.Categories[1].Rating != 1620 || g.Categories[1].Games != 4 || g.Duel == nil || g.Duel.Rating != 1710 {
		t.Errorf("categories: %+v duel %+v", g.Categories, g.Duel)
	}

	r := a.Reviews
	if r.Count != 4 || r.Sample != 3 || r.Known != 2 || r.Average == nil || *r.Average != 76.7 { // (90+70+70)/3
		t.Errorf("reviews: %+v", r)
	}
	if len(r.Series) != 3 || r.Series[0].ID != "r1" || r.Series[2].ID != "r3" || r.Series[1].Accuracy != 70 || r.Series[1].Side != "black" || r.Series[0].Side != "both" {
		t.Errorf("series: %+v", r.Series)
	}

	p := a.Puzzles
	if p.Attempts != 8 || p.Solved != 5 || p.Percent != 63 || p.Rating != 1234 || p.Rush["3min"].Best != 14 || p.Duels.Wins != 2 {
		t.Errorf("puzzles: %+v", p)
	}
	if len(p.Weak) != 1 || p.Weak[0].Theme != "fork" || len(p.History) != 1 || p.History[0].Delta != -16 || p.History[0].Puzzle != "abcde" {
		t.Errorf("weak %+v history %+v", p.Weak, p.History)
	}
	if a.Streak.Current != 2 || !a.Streak.Today || a.Streak.Best != 2 || a.Streak.Days[29] != 1 {
		t.Errorf("streak: %+v", a.Streak)
	}
}

func TestAnswerForNewcomer(t *testing.T) {
	svc := New(Config{Store: &fake{user: &store.User{ID: "me", Name: "Новичок"}, rating: store.PuzzleRating{Rating: 1000}}})
	rec, _ := get(t, svc, "me")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	// Lists are [] and not null, so the page needs no checks; the average is null.
	var raw map[string]map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ sec, key, want string }{
		{"games", "categories", "[]"}, {"reviews", "series", "[]"}, {"reviews", "average", "null"},
		{"puzzles", "weak", "[]"}, {"puzzles", "history", "[]"},
	} {
		if got := string(raw[c.sec][c.key]); got != c.want {
			t.Errorf("%s.%s = %s, want %s", c.sec, c.key, got, c.want)
		}
	}
}

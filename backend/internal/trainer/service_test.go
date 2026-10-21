package trainer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dyanis-chess/backend/internal/store"
)

// fakeStore is a person's data kept in memory.
type fakeStore struct {
	user      *store.User
	rating    store.PuzzleRating
	fails     []store.ThemeFailures
	days      map[string]int
	today     int
	onTheme   int
	repReview int
	reps      []store.RepertoireWithCounts
	moves     map[int64][]store.RepMove
	games     []*store.Game
	analyses  []*store.Analysis
	reports   []store.ReportRow
	practice  []string
	themeAsk  string
}

func (f *fakeStore) UserByID(context.Context, string) (*store.User, error) {
	if f.user == nil {
		return nil, store.ErrNotFound
	}
	return f.user, nil
}
func (f *fakeStore) PuzzleRatingOf(context.Context, string) (store.PuzzleRating, error) {
	return f.rating, nil
}
func (f *fakeStore) PuzzleThemeFailures(context.Context, string) ([]store.ThemeFailures, error) {
	return f.fails, nil
}
func (f *fakeStore) PuzzleDays(context.Context, string) (map[string]int, error) { return f.days, nil }
func (f *fakeStore) PuzzlesSince(_ context.Context, _ string, _ time.Time, theme string) (int, int, error) {
	f.themeAsk = theme
	return f.today, f.onTheme, nil
}
func (f *fakeStore) RepertoireReviewedSince(context.Context, string, time.Time) (int, error) {
	return f.repReview, nil
}
func (f *fakeStore) ListRepertoires(context.Context, string, time.Time) ([]store.RepertoireWithCounts, error) {
	return f.reps, nil
}
func (f *fakeStore) RepMoves(_ context.Context, id int64) ([]store.RepMove, error) {
	return f.moves[id], nil
}
func (f *fakeStore) ListGames(context.Context, string, int) ([]*store.Game, error) {
	return f.games, nil
}
func (f *fakeStore) ListAnalyses(context.Context, string, int) ([]*store.Analysis, error) {
	return f.analyses, nil
}
func (f *fakeStore) RecentReports(context.Context, string, int) ([]store.ReportRow, error) {
	return f.reports, nil
}
func (f *fakeStore) RecordPractice(_ context.Context, user, puzzle string, solved bool, _ time.Time) error {
	f.practice = append(f.practice, user+":"+puzzle)
	return nil
}

var now = time.Date(2026, 10, 21, 19, 30, 0, 0, time.UTC)

func newService(f *fakeStore, uid string) *Service {
	return New(Config{Store: f, UserID: func(*http.Request) string { return uid }, Now: func() time.Time { return now }})
}

func TestBuildForANewcomer(t *testing.T) {
	f := &fakeStore{user: &store.User{Name: "Гость 1234"}, rating: store.PuzzleRating{Rating: 1200}}
	ans, err := newService(f, "u").Build(context.Background(), "u", 9)
	if err != nil {
		t.Fatal(err)
	}
	if !ans.Newcomer || ids(ans.Plan) != "lesson,puzzles,play" || len(ans.Advice) != 0 || ans.Insights.Enough || ans.Insights.Reviews != 0 {
		t.Errorf("newcomer: %+v", ans)
	}
	if ans.Greeting.Hello != "Доброе утро, Гость 1234!" || !strings.Contains(ans.Greeting.Lines[0], "Я Дионис") {
		t.Errorf("greeting: %+v", ans.Greeting)
	}
	if f.themeAsk != "" {
		t.Errorf("asked about the theme %q", f.themeAsk)
	}
}

func stateOf(white, black, bot string, ucis ...string) []byte {
	var ms []string
	for _, u := range ucis {
		ms = append(ms, `{"uci":"`+u+`"}`)
	}
	return []byte(`{"white":{"name":"` + white + `"},"black":{"name":"` + black + `","bot":"` + bot + `"},"moves":[` + strings.Join(ms, ",") + `]}`)
}

func TestBuildForAPlayer(t *testing.T) {
	f := &fakeStore{
		user:   &store.User{Name: "Анна"},
		rating: store.PuzzleRating{Rating: 1350, Attempts: 12, Solved: 8},
		fails:  []store.ThemeFailures{{Theme: "fork", Attempts: 6, Failed: 4}, {Theme: "short", Attempts: 9, Failed: 8}, {Theme: "pin", Attempts: 5, Failed: 1}},
		days:   map[string]int{"2026-10-19": 3, "2026-10-20": 2},
		today:  4, onTheme: 2, repReview: 1,
		reps:  []store.RepertoireWithCounts{{Repertoire: store.Repertoire{ID: 7, Name: "Итальянская", Color: "white"}, Counts: store.RepertoireCounts{Moves: 4, Mine: 2, Due: 3}}},
		moves: map[int64][]store.RepMove{7: line("white", 1, "e2e4", "e7e5", "g1f3", "b8c6")},
	}
	// reviews: Анна is White in four of them (one of which has a different name), and the colour cannot be told in one
	mkMoves := func() []fakeMove {
		ms := make([]fakeMove, 40)
		for i := range ms {
			ms[i] = fakeMove{san: "a3", class: "good", accuracy: 90}
		}
		ms[10] = fakeMove{san: "Ng5", class: "blunder", loss: 25, accuracy: 15}
		return ms
	}
	for i, white := range []string{"Анна", "Анна", "Анна", "Кто-то", "Анна"} {
		f.reports = append(f.reports, store.ReportRow{
			ID: string(rune('a' + i)), Game: gameJSON(white, "Лиза"), Report: storedReport(t, mkMoves(), nil, [2]float64{80 - float64(i), 70}),
			CreatedAt: now.Add(-time.Duration(i+1) * time.Hour),
		})
	}
	f.reports = append(f.reports, store.ReportRow{ID: "bad", Game: gameJSON("Анна", "Лиза"), Report: []byte(`{`), CreatedAt: now.Add(-9 * time.Hour)})
	// the last game, played an hour ago against a bot and not reviewed; an older one that was
	f.games = []*store.Game{
		{ID: "g1", WhiteID: "u", Status: "ended", Result: "0-1", State: stateOf("Анна", "Лиза", "liza", "e2e4", "e7e5", "d2d4", "e5d4", "g1f3"), CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-time.Hour)},
		{ID: "g0", WhiteID: "u", Status: "ended", Result: "1-0", State: stateOf("Анна", "Борис", "", "e2e4", "e7e5", "g1f3", "b8c6"), CreatedAt: now.Add(-48 * time.Hour), UpdatedAt: now.Add(-47 * time.Hour)},
	}
	f.analyses = []*store.Analysis{{ID: "x", Status: "done", Game: gameJSON("Анна", "Борис"), CreatedAt: now.Add(-40 * time.Hour)}}

	ans, err := newService(f, "u").Build(context.Background(), "u", 19)
	if err != nil {
		t.Fatal(err)
	}
	if ans.Newcomer || ans.Name != "Анна" || ans.PuzzlesToday != 4 {
		t.Fatalf("answer: %+v", ans)
	}
	if f.themeAsk != "fork" {
		t.Errorf("the weakest theme is fork, not a technical one: %q", f.themeAsk)
	}
	if ids(ans.Plan) != "weak-theme,repertoire,review,lesson" {
		t.Fatalf("plan: %s", ids(ans.Plan))
	}
	wt, rep, rv := find(ans.Plan, "weak-theme"), find(ans.Plan, "repertoire"), find(ans.Plan, "review")
	if wt.Progress != 2 || wt.Theme != "fork" || rep.Progress != 1 || rep.Goal != 4 || rv.Href != "#online/g1" || rv.Done {
		t.Errorf("plan items: %+v %+v %+v", wt, rep, rv)
	}
	if ans.Day != (Day{Done: 0, Total: 3}) {
		t.Errorf("day: %+v", ans.Day)
	}

	in := ans.Insights
	if in.Reviews != 6 || in.Used != 4 || in.Skipped != 2 || !in.Enough || in.Average == nil {
		t.Errorf("insights: %+v", in)
	}
	if len(in.Costly) != 3 || in.Costly[0].Loss != 25 || in.Costly[0].Href != "#review/a" {
		t.Errorf("costly: %+v", in.Costly)
	}
	// one blunder in 20 own moves: 5 per 100 is many
	if in.Rate.Blunders != 5 || !strings.Contains(adviceIDs(ans.Advice), "blunders") {
		t.Errorf("rate %+v, advice %s", in.Rate, adviceIDs(ans.Advice))
	}
	// the streak is alive (yesterday), yesterday had 2 puzzles
	if ans.Streak.Current != 2 || ans.Streak.Today {
		t.Errorf("streak: %+v", ans.Streak)
	}
	if ans.Greeting.Hello != "Добрый вечер, Анна!" || !strings.Contains(ans.Greeting.Lines[0], "Серия: 2 дня") {
		t.Errorf("greeting: %+v", ans.Greeting)
	}
	// the games are put against the repository: g1 left it at White's second move, g0 stayed in
	if d, err := newService(f, "u").deviations(context.Background(), "u", f.reps, f.games, now); err != nil || d.Compared != 2 || d.Deviated != 1 {
		t.Errorf("deviations: %+v (%v)", d, err)
	}

	// the whole answer goes out as JSON the page can read
	b, err := json.Marshal(ans)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"greeting", "plan", "insights", "advice", "streak", "day", "newcomer", "puzzlesToday"} {
		if _, ok := back[k]; !ok {
			t.Errorf("no %q in the answer", k)
		}
	}
}

func TestHandlers(t *testing.T) {
	f := &fakeStore{user: &store.User{Name: "Анна"}, rating: store.PuzzleRating{Rating: 1200}}
	// not signed in: 401 for both
	anon := newService(f, "").Handler()
	for _, req := range []*http.Request{httptest.NewRequest("GET", "/api/trainer", nil), httptest.NewRequest("POST", "/api/trainer/practice", strings.NewReader(`{"puzzle":"abc12","solved":true}`))} {
		rec := httptest.NewRecorder()
		anon.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: %d", req.Method, req.URL, rec.Code)
		}
	}

	h := newService(f, "u").Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/trainer?hour=23", nil))
	var ans Answer
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &ans) != nil || ans.Greeting.Hello != "Доброй ночи, Анна!" {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	// a nonsense hour falls back to the server's (19:30 UTC)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/trainer?hour=99", nil))
	if !strings.Contains(rec.Body.String(), "Добрый вечер") {
		t.Errorf("hour fallback: %s", rec.Body.String())
	}

	// practice
	post := func(body string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/trainer/practice", strings.NewReader(body)))
		return rec.Code
	}
	if c := post(`{"puzzle":"abc12","solved":false}`); c != http.StatusNoContent {
		t.Errorf("practice: %d", c)
	}
	for _, bad := range []string{`{`, `{"puzzle":""}`, `{"puzzle":"a b"}`, `{"puzzle":"waytoolongpuzzleid"}`, `{"solved":true}`} {
		if c := post(bad); c != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, c)
		}
	}
	if len(f.practice) != 1 || f.practice[0] != "u:abc12" {
		t.Errorf("recorded: %v", f.practice)
	}
}

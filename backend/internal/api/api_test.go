package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dyanis-chess/backend/internal/analysis"
	"dyanis-chess/backend/internal/pgn"
	"dyanis-chess/backend/internal/store"
)

func post(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/analyses", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func getJob(t *testing.T, h http.Handler, id string) (int, Job) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/analyses/"+id, nil))
	var j Job
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &j); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, j
}

func waitFor(t *testing.T, h http.Handler, id, status string) Job {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, j := getJob(t, h, id); j.Status == status {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s never reached %s", id, status)
	return Job{}
}

func create(t *testing.T, h http.Handler, body string) string {
	t.Helper()
	rec := post(t, h, body)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body)
	}
	var out struct{ ID string }
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.ID == "" {
		t.Fatalf("bad response %s", rec.Body)
	}
	return out.ID
}

const foolsMate = `[White "Анна"]
[Black "Борис"]
[Date "2026.10.15"]

1. f3 e5 2. g4 Qh4# 0-1`

func jsonBody(pgnText string, extra string) string {
	b, _ := json.Marshal(pgnText)
	return `{"pgn":` + string(b) + extra + `}`
}

// The whole path with the real analysis at a tiny depth.
func TestAnalyzeEndToEnd(t *testing.T) {
	s := New(Config{})
	defer s.Close()
	h := s.Handler()

	id := create(t, h, jsonBody(foolsMate, `,"depth":3`))
	j := waitFor(t, h, id, StatusDone)
	if j.Report == nil || len(j.Report.Moves) != 4 || len(j.Report.Evals) != 5 {
		t.Fatalf("report: %+v", j.Report)
	}
	if j.Done != j.Total || j.Total != 5 {
		t.Errorf("progress %d/%d", j.Done, j.Total)
	}
	if j.Game.White != "Анна" || j.Game.Black != "Борис" || j.Game.Result != "0-1" || j.Game.Date != "2026.10.15" {
		t.Errorf("game info: %+v", j.Game)
	}
	if !strings.HasPrefix(j.Game.StartFEN, "rnbqkbnr/") {
		t.Errorf("start fen %q", j.Game.StartFEN)
	}
	if j.Report.Options.Depth != 3 {
		t.Errorf("depth = %d, want 3", j.Report.Options.Depth)
	}
}

func TestBadRequests(t *testing.T) {
	s := New(Config{MaxPlies: 4, MaxBodyBytes: 1000})
	defer s.Close()
	h := s.Handler()
	cases := []struct {
		name, body string
		want       int
	}{
		{"not json", `hello`, 400},
		{"empty pgn", `{"pgn":"  "}`, 400},
		{"no moves", jsonBody("[Event \"x\"]\n\n*", ""), 400},
		{"illegal move", jsonBody("1. e5 *", ""), 400},
		{"game out of range", jsonBody(foolsMate, `,"game":2`), 400},
		{"too long", jsonBody("1. e4 e5 2. Nf3 Nc6 3. Bb5 *", ""), 400},
		{"too big", `{"pgn":"` + strings.Repeat("a", 2000) + `"}`, 413},
	}
	for _, c := range cases {
		rec := post(t, h, c.body)
		if rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body)
			continue
		}
		var e struct{ Error string }
		if json.Unmarshal(rec.Body.Bytes(), &e) != nil || e.Error == "" {
			t.Errorf("%s: no error message in %s", c.name, rec.Body)
		}
	}
	if code, _ := getJob(t, h, "nope"); code != 404 {
		t.Errorf("unknown id: %d", code)
	}
	// Wrong method on the collection.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("PUT", "/api/analyses", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /api/analyses = %d", rec.Code)
	}
}

func TestSecondGameAndDepthClamp(t *testing.T) {
	var mu sync.Mutex
	var seen []int
	s := New(Config{MaxDepth: 6, Analyze: func(g *pgn.Game, opt analysis.Options) (*analysis.Report, error) {
		mu.Lock()
		seen = append(seen, opt.Depth)
		mu.Unlock()
		return &analysis.Report{}, nil
	}})
	defer s.Close()
	h := s.Handler()
	two := foolsMate + "\n\n[White \"C\"]\n[Black \"D\"]\n\n1. e4 e5 *\n"
	id := create(t, h, jsonBody(two, `,"game":2,"depth":99`))
	j := waitFor(t, h, id, StatusDone)
	if j.Game.White != "C" || j.Total != 3 {
		t.Errorf("picked the wrong game: %+v total %d", j.Game, j.Total)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != 6 {
		t.Errorf("depth passed to the analyzer: %v, want [6]", seen)
	}
}

func TestQueueRunsOneAtATimeAndOverflows(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 8)
	var mu sync.Mutex
	running, maxRunning := 0, 0
	s := New(Config{QueueLen: 2, Analyze: func(g *pgn.Game, opt analysis.Options) (*analysis.Report, error) {
		mu.Lock()
		running++
		if running > maxRunning {
			maxRunning = running
		}
		mu.Unlock()
		started <- struct{}{}
		<-release
		mu.Lock()
		running--
		mu.Unlock()
		return &analysis.Report{}, nil
	}})
	h := s.Handler()

	first := create(t, h, jsonBody(foolsMate, ""))
	<-started // the worker holds the first job; the queue is empty again
	if _, j := getJob(t, h, first); j.Status != StatusRunning {
		t.Errorf("first job is %s, want running", j.Status)
	}
	b := create(t, h, jsonBody(foolsMate, ""))
	c := create(t, h, jsonBody(foolsMate, ""))
	if _, j := getJob(t, h, b); j.Status != StatusQueued {
		t.Errorf("second job is %s, want queued", j.Status)
	}
	if rec := post(t, h, jsonBody(foolsMate, "")); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("overflow: %d, want 503", rec.Code)
	}
	close(release) // everything may finish now
	for _, id := range []string{first, b, c} {
		waitFor(t, h, id, StatusDone)
	}
	s.Close()
	mu.Lock()
	defer mu.Unlock()
	if maxRunning != 1 {
		t.Errorf("%d analyses ran at once", maxRunning)
	}
}

func TestAnalyzerErrorAndPanic(t *testing.T) {
	calls := 0
	s := New(Config{Analyze: func(g *pgn.Game, opt analysis.Options) (*analysis.Report, error) {
		calls++
		if calls == 1 {
			panic("boom")
		}
		return nil, http.ErrAbortHandler
	}})
	defer s.Close()
	h := s.Handler()
	a := waitFor(t, h, create(t, h, jsonBody(foolsMate, "")), StatusError)
	if !strings.Contains(a.Error, "boom") || a.Report != nil {
		t.Errorf("panic job: %+v", a)
	}
	// The worker survived the panic and still serves later jobs.
	b := waitFor(t, h, create(t, h, jsonBody(foolsMate, "")), StatusError)
	if b.Error == "" {
		t.Error("error job without message")
	}
}

func TestEviction(t *testing.T) {
	s := New(Config{KeepJobs: 2, Analyze: func(g *pgn.Game, opt analysis.Options) (*analysis.Report, error) {
		return &analysis.Report{}, nil
	}})
	defer s.Close()
	h := s.Handler()
	var ids []string
	for i := 0; i < 4; i++ {
		id := create(t, h, jsonBody(foolsMate, ""))
		waitFor(t, h, id, StatusDone)
		ids = append(ids, id)
	}
	if code, _ := getJob(t, h, ids[0]); code != 404 {
		t.Errorf("oldest job still there: %d", code)
	}
	if code, _ := getJob(t, h, ids[3]); code != 200 {
		t.Errorf("newest job gone: %d", code)
	}
}

// memStore is an in-memory Store.
type memStore struct {
	mu   sync.Mutex
	rows map[string]*store.Analysis
}

func (m *memStore) SaveAnalysis(_ context.Context, a *store.Analysis) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rows == nil {
		m.rows = map[string]*store.Analysis{}
	}
	c := *a
	m.rows[a.ID] = &c
	return nil
}

func (m *memStore) LoadAnalysis(_ context.Context, id string) (*store.Analysis, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.rows[id]; ok {
		return a, nil
	}
	return nil, store.ErrNotFound
}

func (m *memStore) ListAnalyses(_ context.Context, userID string, limit int) ([]*store.Analysis, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*store.Analysis
	for _, a := range m.rows {
		if a.UserID == userID {
			c := *a
			out = append(out, &c)
		}
	}
	return out, nil
}

func (m *memStore) DeleteAnalysis(_ context.Context, userID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.rows[id]; ok && a.UserID == userID {
		delete(m.rows, id)
		return nil
	}
	return store.ErrNotFound
}

// A finished analysis is written to the store, and a new Server (a restart)
// serves it from there.
func TestPersistenceAcrossRestart(t *testing.T) {
	db := &memStore{}
	s1 := New(Config{Store: db})
	h1 := s1.Handler()
	id := create(t, h1, jsonBody(foolsMate, `,"depth":3`))
	want := waitFor(t, h1, id, StatusDone)
	s1.Close()

	// The save happens right after the status flips; wait for it.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := db.LoadAnalysis(context.Background(), id); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("analysis never reached the store")
		}
		time.Sleep(5 * time.Millisecond)
	}

	s2 := New(Config{Store: db})
	defer s2.Close()
	code, got := getJob(t, s2.Handler(), id)
	if code != 200 || got.Status != StatusDone || got.Report == nil {
		t.Fatalf("after restart: %d %+v", code, got)
	}
	if got.Game != want.Game || got.Report.Accuracy != want.Report.Accuracy ||
		len(got.Report.Moves) != len(want.Report.Moves) || got.Total != want.Total {
		t.Errorf("stored result differs:\n got %+v\nwant %+v", got.Game, want.Game)
	}
	if code, _ := getJob(t, s2.Handler(), "unknown"); code != 404 {
		t.Errorf("unknown id with a store: %d", code)
	}
}

func TestPersistsErrorResult(t *testing.T) {
	db := &memStore{}
	s := New(Config{Store: db, Analyze: func(g *pgn.Game, opt analysis.Options) (*analysis.Report, error) {
		return nil, errors.New("no luck")
	}})
	defer s.Close()
	h := s.Handler()
	id := waitFor(t, h, create(t, h, jsonBody(foolsMate, "")), StatusError).ID
	s2 := New(Config{Store: db})
	defer s2.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if code, j := getJob(t, s2.Handler(), id); code == 200 {
			if j.Status != StatusError || j.Error != "no luck" || j.Report != nil {
				t.Errorf("stored error job: %+v", j)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("error result never stored")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// ---------------------------------------------------------------- owners

// userCfg treats the X-User header as the signed-in user.
func userCfg(db Store) Config {
	return Config{Store: db, UserID: func(r *http.Request) string { return r.Header.Get("X-User") }}
}

func as(user, method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if user != "" {
		req.Header.Set("X-User", user)
	}
	return req
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestListNeedsSignIn(t *testing.T) {
	s := New(Config{})
	defer s.Close()
	if rec := serve(s.Handler(), as("", "GET", "/api/analyses", "")); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous list: %d", rec.Code)
	}
	if rec := serve(s.Handler(), as("", "DELETE", "/api/analyses/abc", "")); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous delete: %d", rec.Code)
	}
}

func listOf(t *testing.T, h http.Handler, user string) []Listed {
	t.Helper()
	rec := serve(h, as(user, "GET", "/api/analyses", ""))
	if rec.Code != 200 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	var out struct{ Items []Listed }
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Items
}

// Each user sees only their own reviews, a link still opens for anyone, and
// the list survives a restart (it comes from the store then).
func TestOwnersListAndLinks(t *testing.T) {
	db := &memStore{}
	s := New(userCfg(db))
	h := s.Handler()

	mk := func(user string) string {
		rec := serve(h, as(user, "POST", "/api/analyses", jsonBody(foolsMate, `,"depth":3`)))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("POST as %q: %d %s", user, rec.Code, rec.Body)
		}
		var out struct{ ID string }
		json.Unmarshal(rec.Body.Bytes(), &out)
		return out.ID
	}
	a1, a2, anon := mk("alice"), mk("alice"), mk("")
	b1 := mk("bob")
	for _, id := range []string{a1, a2, anon, b1} {
		waitFor(t, h, id, StatusDone)
	}
	// The save follows the status flip; wait until all four are stored.
	deadline := time.Now().Add(5 * time.Second)
	for {
		db.mu.Lock()
		n := len(db.rows)
		db.mu.Unlock()
		if n == 4 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("analyses never reached the store")
		}
		time.Sleep(5 * time.Millisecond)
	}

	check := func(h http.Handler) {
		t.Helper()
		got := listOf(t, h, "alice")
		if len(got) != 2 || got[0].ID == got[1].ID {
			t.Fatalf("alice's list: %+v", got)
		}
		for _, l := range got {
			if l.ID != a1 && l.ID != a2 {
				t.Errorf("alice sees %s, which is not hers", l.ID)
			}
			if l.Status != StatusDone || l.Game.White != "Анна" || l.Summary == nil || l.Summary.Plies != 4 || l.Summary.Accuracy[0] == 0 {
				t.Errorf("row: %+v summary %+v", l, l.Summary)
			}
		}
		if got := listOf(t, h, "bob"); len(got) != 1 || got[0].ID != b1 {
			t.Errorf("bob's list: %+v", got)
		}
		if got := listOf(t, h, "carol"); len(got) != 0 {
			t.Errorf("carol's list: %+v", got)
		}
		// The link works for everybody, signed in or not.
		if code, j := getJob(t, h, a1); code != 200 || j.Report == nil {
			t.Errorf("link of alice's review: %d", code)
		}
	}
	check(h)
	s.Close()
	s2 := New(userCfg(db))
	defer s2.Close()
	check(s2.Handler())
}

func TestDeleteOwnReviewOnly(t *testing.T) {
	db := &memStore{}
	s := New(userCfg(db))
	defer s.Close()
	h := s.Handler()
	rec := serve(h, as("alice", "POST", "/api/analyses", jsonBody(foolsMate, `,"depth":3`)))
	var out struct{ ID string }
	json.Unmarshal(rec.Body.Bytes(), &out)
	waitFor(t, h, out.ID, StatusDone)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := db.LoadAnalysis(context.Background(), out.ID); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never stored")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if rec := serve(h, as("bob", "DELETE", "/api/analyses/"+out.ID, "")); rec.Code != http.StatusNotFound {
		t.Errorf("bob deleting alice's review: %d", rec.Code)
	}
	if code, _ := getJob(t, h, out.ID); code != 200 {
		t.Fatalf("review vanished after a foreign delete: %d", code)
	}
	if rec := serve(h, as("alice", "DELETE", "/api/analyses/"+out.ID, "")); rec.Code != http.StatusNoContent {
		t.Errorf("alice deleting her review: %d", rec.Code)
	}
	if code, _ := getJob(t, h, out.ID); code != http.StatusNotFound {
		t.Errorf("review still there after delete: %d", code)
	}
	if got := listOf(t, h, "alice"); len(got) != 0 {
		t.Errorf("list after delete: %+v", got)
	}
}

func TestDeleteRefusedWhileRunning(t *testing.T) {
	release := make(chan struct{})
	s := New(Config{UserID: func(r *http.Request) string { return r.Header.Get("X-User") },
		Analyze: func(g *pgn.Game, opt analysis.Options) (*analysis.Report, error) {
			<-release
			return nil, errors.New("stopped")
		}})
	defer s.Close()
	defer close(release)
	h := s.Handler()
	rec := serve(h, as("alice", "POST", "/api/analyses", jsonBody(foolsMate, "")))
	var out struct{ ID string }
	json.Unmarshal(rec.Body.Bytes(), &out)
	if rec := serve(h, as("alice", "DELETE", "/api/analyses/"+out.ID, "")); rec.Code != http.StatusConflict {
		t.Errorf("delete while running: %d", rec.Code)
	}
	// An unfinished review is listed, so a reload does not lose it.
	if got := listOf(t, h, "alice"); len(got) != 1 || got[0].ID != out.ID {
		t.Errorf("list during analysis: %+v", got)
	}
}

func TestOpeningEndpoint(t *testing.T) {
	s := New(Config{})
	defer s.Close()
	get := func(q string) (int, struct {
		Opening *struct{ ECO, Name string }
		Book    bool
	}) {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/opening?san="+q, nil))
		var out struct {
			Opening *struct{ ECO, Name string }
			Book    bool
		}
		json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	if code, o := get(""); code != 200 || o.Opening != nil || o.Book {
		t.Errorf("no moves: %d %+v", code, o)
	}
	if code, o := get("e4,e5,Nf3,Nc6,Bb5"); code != 200 || o.Opening == nil || !strings.Contains(o.Opening.Name, "Ruy Lopez") || !o.Book {
		t.Errorf("Ruy Lopez: %d %+v", code, o)
	}
	// out of theory: the name stays, the book flag goes
	if code, o := get("e4,e5,Nf3,Nc6,Bb5,a6,Ba4,Nf6,a3,h6,h3"); code != 200 || o.Opening == nil || o.Book {
		t.Errorf("past theory: %d %+v", code, o)
	}
	if code, _ := get("e4,e5,Qxx9"); code != 400 {
		t.Errorf("bad move: %d", code)
	}
	if code, _ := get(strings.Repeat("Nf3,Nf6,Ng1,Ng8,", 31)); code != http.StatusBadRequest {
		t.Errorf("a game of 124 plies: %d", code)
	}
}

package repertoire

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dyanis-chess/backend/internal/pgn"
	"dyanis-chess/backend/internal/store"
)

var t0 = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

func TestLadder(t *testing.T) {
	// right answers climb the ladder with growing pauses
	box, lapses := 0, 0
	var days []float64
	for i := 0; i < 8; i++ {
		c := Review(box, lapses, true, t0)
		box, lapses = c.Box, c.Lapses
		days = append(days, c.DueAt.Sub(t0).Hours()/24)
	}
	want := []float64{1, 3, 7, 14, 30, 90, 90, 90}
	for i := range want {
		if days[i] != want[i] {
			t.Fatalf("pauses %v, want %v", days, want)
		}
	}
	if box != MaxBox {
		t.Errorf("box %d, top of the ladder is %d", box, MaxBox)
	}
	// a wrong answer falls back two steps and comes back in minutes
	c := Review(5, 0, false, t0)
	if c.Box != 3 || c.Lapses != 1 || c.DueAt.Sub(t0) != 10*time.Minute {
		t.Errorf("after a failure at step 5: %+v", c)
	}
	if c := Review(1, 2, false, t0); c.Box != 0 || c.Lapses != 3 {
		t.Errorf("after a failure at step 1: %+v", c)
	}
	if Learned(store.LearnedBox-1) || !Learned(store.LearnedBox) {
		t.Error("learned threshold")
	}
}

// A tiny repertoire for White: 1.e4 e5 2.Nf3 (and 1...c5 2.Nf3 d6) with the
// opponent's replies and my moves marked.
func mv(id, parent int64, uci, san string, ply int, mine bool) store.RepMove {
	return store.RepMove{ID: id, RepertoireID: 1, ParentID: parent, UCI: uci, SAN: san, Ply: ply, Mine: mine}
}

func sample() []store.RepMove {
	return []store.RepMove{
		mv(1, 0, "e2e4", "e4", 1, true),
		mv(2, 1, "e7e5", "e5", 2, false),
		mv(3, 2, "g1f3", "Nf3", 3, true),
		mv(4, 1, "c7c5", "c5", 2, false),
		mv(5, 4, "g1f3", "Nf3", 3, true),
		mv(6, 5, "d7d6", "d6", 4, false),
	}
}

func TestSelect(t *testing.T) {
	ms := sample()
	// nothing was ever asked: new moves come in the order they were added
	// (line after line), only the person's own, up to the limit
	sel := Select(ms, t0, 10, 10)
	if fmt.Sprint(sel.New) != "[1 3 5]" || sel.NewTotal != 3 {
		t.Fatalf("new: %+v", sel)
	}
	if one := Select(ms, t0, 1, 10); fmt.Sprint(one.New) != "[1]" {
		t.Errorf("one new move: %v", one.New)
	}
	// once 1 was asked, the next ones follow; 1 itself is not due before its time
	ms[0].DueAt = sql.NullTime{Time: t0.Add(24 * time.Hour), Valid: true}
	sel = Select(ms, t0, 10, 10)
	if fmt.Sprint(sel.New) != "[3 5]" || len(sel.Due) != 0 || sel.NewTotal != 2 {
		t.Errorf("after 1 was asked: %+v", sel)
	}
	sel = Select(ms, t0, 1, 10)
	if fmt.Sprint(sel.New) != "[3]" || sel.NewTotal != 2 {
		t.Errorf("one new move per session: %+v", sel)
	}
	// due moves: the longest overdue first, then up to the limit
	ms[0].DueAt = sql.NullTime{Time: t0.Add(-time.Hour), Valid: true}
	ms[2].DueAt = sql.NullTime{Time: t0.Add(-48 * time.Hour), Valid: true}
	ms[4].DueAt = sql.NullTime{Time: t0.Add(-24 * time.Hour), Valid: true}
	sel = Select(ms, t0, 10, 2)
	if fmt.Sprint(sel.Due) != "[3 5]" || sel.DueTotal != 3 || len(sel.New) != 0 {
		t.Errorf("due: %+v", sel)
	}
	// the opponent's moves are never cards
	for _, id := range append(sel.Due, sel.New...) {
		if !ms[id-1].Mine {
			t.Errorf("move %d is the opponent's", id)
		}
	}
}

func TestBuildDrill(t *testing.T) {
	ms := sample()
	// real FENs are needed for the legal moves of the position of the question
	ms[0].FEN = "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq - 0 1"
	ms[1].FEN = "rnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq - 0 2"
	ms[2].FEN = "rnbqkbnr/pppp1ppp/8/4p3/4P3/5N2/PPPP1PPP/RNBQKB1R b KQkq - 1 2"
	d, ok := BuildDrill(ms, 3, true)
	if !ok {
		t.Fatal("no drill")
	}
	if len(d.Path) != 2 || d.Path[0].SAN != "e4" || d.Path[1].SAN != "e5" || d.FEN != ms[1].FEN {
		t.Errorf("path %+v fen %s", d.Path, d.FEN)
	}
	if d.Answer.UCI != "g1f3" || d.Answer.FEN != ms[2].FEN || !d.New || d.Move != 3 {
		t.Errorf("answer %+v", d)
	}
	if len(d.Legal) != 29 || len(d.Alt) != 0 {
		t.Errorf("legal %d alt %v", len(d.Legal), d.Alt)
	}
	// the first move: the position of the question is the start position
	d, _ = BuildDrill(ms, 1, false)
	if len(d.Path) != 0 || len(d.Legal) != 20 {
		t.Errorf("first move: path %d legal %d", len(d.Path), len(d.Legal))
	}
	// other repertoire moves in the same position are alternatives
	ms = append(ms, mv(7, 0, "d2d4", "d4", 1, true))
	if d, _ = BuildDrill(ms, 1, false); fmt.Sprint(d.Alt) != "[d2d4]" {
		t.Errorf("alt %v", d.Alt)
	}
	if _, ok := BuildDrill(ms, 99, false); ok {
		t.Error("a drill for a missing move")
	}
}

func TestNextDue(t *testing.T) {
	ms := sample()
	if NextDue(ms, t0) != nil {
		t.Error("nothing was asked yet")
	}
	ms[0].DueAt = sql.NullTime{Time: t0.Add(5 * time.Hour), Valid: true}
	ms[2].DueAt = sql.NullTime{Time: t0.Add(2 * time.Hour), Valid: true}
	ms[4].DueAt = sql.NullTime{Time: t0.Add(-time.Hour), Valid: true} // due already
	if got := NextDue(ms, t0); got == nil || *got != t0.Add(2*time.Hour).Format(time.RFC3339) {
		t.Errorf("next due %v", got)
	}
}

func TestGaps(t *testing.T) {
	ms := sample()
	// 6 (1...c5 2.Nf3 d6) ends at the opponent's move: I have no answer yet
	if g := Gaps(ms); fmt.Sprint(g) != "[6]" {
		t.Errorf("gaps %v", g)
	}
	ms = append(ms, mv(7, 6, "b1c3", "Nc3", 5, true))
	if g := Gaps(ms); len(g) != 0 {
		t.Errorf("gaps after answering: %v", g)
	}
}

// ------------------------------------------------------------ the handlers

type memStore struct {
	mu    sync.Mutex
	reps  []store.Repertoire
	moves []store.RepMove
	next  int64
	games []*store.Game
	scans map[int64]store.RepScan
}

func (m *memStore) ListGames(_ context.Context, userID string, limit int) ([]*store.Game, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*store.Game
	for _, g := range m.games {
		if g.WhiteID == userID || g.BlackID == userID {
			out = append(out, g)
		}
	}
	return out, nil
}

func (m *memStore) CreateRepertoire(_ context.Context, userID, name, color string) (*store.Repertoire, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.reps {
		if r.UserID == userID {
			n++
		}
	}
	if n >= store.MaxRepertoires {
		return nil, store.ErrLimit
	}
	r := store.Repertoire{ID: int64(len(m.reps) + 1), UserID: userID, Name: name, Color: color}
	m.reps = append(m.reps, r)
	return &r, nil
}

func (m *memStore) Repertoire(_ context.Context, userID string, id int64) (*store.Repertoire, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.reps {
		if r.ID == id && r.UserID == userID {
			return &r, nil
		}
	}
	return nil, store.ErrNotFound
}

func (m *memStore) ListRepertoires(_ context.Context, userID string, now time.Time) ([]store.RepertoireWithCounts, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []store.RepertoireWithCounts
	for _, r := range m.reps {
		if r.UserID == userID {
			c := store.RepertoireCounts{}
			for _, x := range m.moves {
				if x.RepertoireID == r.ID {
					c.Moves++
					if x.Mine {
						c.Mine++
					}
				}
			}
			out = append(out, store.RepertoireWithCounts{Repertoire: r, Counts: c})
		}
	}
	return out, nil
}

func (m *memStore) RenameRepertoire(_ context.Context, userID string, id int64, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.reps {
		if m.reps[i].ID == id && m.reps[i].UserID == userID {
			m.reps[i].Name = name
			return nil
		}
	}
	return store.ErrNotFound
}

func (m *memStore) DeleteRepertoire(_ context.Context, userID string, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.reps {
		if m.reps[i].ID == id && m.reps[i].UserID == userID {
			m.reps = append(m.reps[:i], m.reps[i+1:]...)
			return nil
		}
	}
	return store.ErrNotFound
}

func (m *memStore) RepMoves(_ context.Context, repID int64) ([]store.RepMove, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []store.RepMove
	for _, x := range m.moves {
		if x.RepertoireID == repID {
			out = append(out, x)
		}
	}
	return out, nil
}

func (m *memStore) RepMove(_ context.Context, repID, id int64) (store.RepMove, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, x := range m.moves {
		if x.RepertoireID == repID && x.ID == id {
			return x, nil
		}
	}
	return store.RepMove{}, store.ErrNotFound
}

func (m *memStore) AddRepMove(_ context.Context, x store.RepMove) (store.RepMove, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, y := range m.moves {
		if y.RepertoireID == x.RepertoireID && y.ParentID == x.ParentID && y.UCI == x.UCI {
			return y, false, nil
		}
	}
	m.next++
	x.ID = m.next
	m.moves = append(m.moves, x)
	return x, true, nil
}

func (m *memStore) AddRepMovesBulk(_ context.Context, repID int64, moves []store.NewRepMove) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, x := range m.moves {
		if x.RepertoireID == repID {
			n++
		}
	}
	if n+len(moves) > store.MaxRepMoves {
		return store.ErrLimit
	}
	ids := make([]int64, len(moves))
	for i, nm := range moves {
		parent := nm.ParentID
		if nm.ParentIndex >= 0 {
			parent = ids[nm.ParentIndex]
		}
		m.next++
		ids[i] = m.next
		m.moves = append(m.moves, store.RepMove{ID: m.next, RepertoireID: repID, ParentID: parent, UCI: nm.UCI, SAN: nm.SAN, FEN: nm.FEN, Ply: nm.Ply, Mine: nm.Mine})
	}
	return nil
}

func (m *memStore) DeleteRepMove(_ context.Context, repID, id int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doomed := map[int64]bool{id: true}
	for changed := true; changed; {
		changed = false
		for _, x := range m.moves {
			if x.RepertoireID == repID && doomed[x.ParentID] && !doomed[x.ID] {
				doomed[x.ID], changed = true, true
			}
		}
	}
	var keep []store.RepMove
	for _, x := range m.moves {
		if !(x.RepertoireID == repID && doomed[x.ID]) {
			keep = append(keep, x)
		}
	}
	n := len(m.moves) - len(keep)
	if n == 0 {
		return 0, store.ErrNotFound
	}
	m.moves = keep
	return n, nil
}

func (m *memStore) ReviewRepMove(_ context.Context, repID, id int64, next func(box, lapses int) store.Card) (store.Card, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.moves {
		x := &m.moves[i]
		if x.RepertoireID == repID && x.ID == id && x.Mine {
			c := next(x.Box, x.Lapses)
			x.Box, x.Lapses, x.DueAt = c.Box, c.Lapses, sql.NullTime{Time: c.DueAt, Valid: true}
			x.Reviews++
			return c, nil
		}
	}
	return store.Card{}, store.ErrNotFound
}

func (m *memStore) SaveRepScan(_ context.Context, userID string, sc store.RepScan) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.reps {
		if r.ID == sc.RepertoireID && r.UserID == userID {
			if m.scans == nil {
				m.scans = map[int64]store.RepScan{}
			}
			m.scans[sc.RepertoireID] = sc
			return nil
		}
	}
	return store.ErrNotFound
}

func (m *memStore) RepScan(_ context.Context, userID string, id int64) (store.RepScan, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.reps {
		if r.ID == id && r.UserID == userID {
			if sc, ok := m.scans[id]; ok {
				return sc, nil
			}
		}
	}
	return store.RepScan{}, store.ErrNotFound
}

func (m *memStore) RepOverviews(_ context.Context, userID string) (map[int64]store.RepOverview, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[int64]store.RepOverview{}
	for _, r := range m.reps {
		if r.UserID != userID {
			continue
		}
		var mv []store.RepMove
		for _, x := range m.moves {
			if x.RepertoireID == r.ID {
				mv = append(mv, x)
			}
		}
		o := store.RepOverview{Open: len(Gaps(mv))}
		for _, x := range mv {
			o.MaxMove = max(o.MaxMove, x.ID)
		}
		if sc, ok := m.scans[r.ID]; ok {
			sc := sc
			o.Scan = &sc
		}
		out[r.ID] = o
	}
	return out, nil
}

type api struct {
	t    *testing.T
	h    http.Handler
	user string
	now  time.Time
	mem  *memStore
}

func newAPI(t *testing.T) *api {
	a := &api{t: t, user: "anna", now: t0, mem: &memStore{}}
	s := New(Config{Store: a.mem, UserID: func(*http.Request) string { return a.user }, Now: func() time.Time { return a.now }})
	a.h = s.Handler()
	return a
}

func (a *api) call(method, url, body string) (int, map[string]any) {
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (a *api) mustCall(method, url, body string, want int) map[string]any {
	a.t.Helper()
	code, out := a.call(method, url, body)
	if code != want {
		a.t.Fatalf("%s %s = %d %v, want %d", method, url, code, out, want)
	}
	return out
}

func id(v any) int64 { return int64(v.(float64)) }

func TestRepertoireFlow(t *testing.T) {
	a := newAPI(t)
	rep := a.mustCall("POST", "/api/repertoires", `{"name":"  Итальянка  ","color":"white"}`, 201)
	if rep["name"] != "Итальянка" {
		t.Errorf("name %v", rep["name"])
	}
	base := fmt.Sprintf("/api/repertoires/%d", id(rep["id"]))

	// 1.e4 is mine (White), 1...e5 the opponent's, 2.Nf3 mine
	m1 := a.mustCall("POST", base+"/moves", `{"parent":0,"uci":"e2e4"}`, 201)["move"].(map[string]any)
	m2 := a.mustCall("POST", base+"/moves", fmt.Sprintf(`{"parent":%d,"uci":"e7e5"}`, id(m1["id"])), 201)["move"].(map[string]any)
	m3 := a.mustCall("POST", base+"/moves", fmt.Sprintf(`{"parent":%d,"uci":"g1f3"}`, id(m2["id"])), 201)["move"].(map[string]any)
	if m1["mine"] != true || m2["mine"] != false || m3["mine"] != true || m3["san"] != "Nf3" {
		t.Errorf("moves: %v %v %v", m1, m2, m3)
	}
	// the same move again is not a new one; an illegal one is refused
	if out := a.mustCall("POST", base+"/moves", `{"parent":0,"uci":"e2e4"}`, 200); out["created"] != false {
		t.Errorf("duplicate: %v", out)
	}
	a.mustCall("POST", base+"/moves", `{"parent":0,"uci":"e2e5"}`, 400)
	a.mustCall("POST", base+"/moves", `{"parent":999,"uci":"e2e4"}`, 404)

	got := a.mustCall("GET", base, "", 200)
	counts := got["repertoire"].(map[string]any)["counts"].(map[string]any)
	if counts["moves"] != 3.0 || counts["mine"] != 2.0 || counts["new"] != 2.0 || counts["due"] != 0.0 {
		t.Errorf("counts %v", counts)
	}
	if len(got["moves"].([]any)) != 3 || len(got["gaps"].([]any)) != 0 {
		t.Errorf("tree %v", got)
	}

	// training: both of my moves are new, asked in the order of the line
	tr := a.mustCall("GET", base+"/train", "", 200)
	drills := tr["drills"].([]any)
	if len(drills) != 2 || drills[0].(map[string]any)["new"] != true || id(drills[0].(map[string]any)["move"]) != id(m1["id"]) ||
		id(drills[1].(map[string]any)["move"]) != id(m3["id"]) {
		t.Fatalf("session %v", tr)
	}
	// one new move per session on request
	if one := a.mustCall("GET", base+"/train?new=1", "", 200)["drills"].([]any); len(one) != 1 {
		t.Errorf("new=1: %v", one)
	}
	// answered right: it climbs to step 1 and is due tomorrow
	r := a.mustCall("POST", base+"/review", fmt.Sprintf(`{"move":%d,"correct":true}`, id(m1["id"])), 200)
	if r["box"] != 1.0 || r["learned"] != false {
		t.Errorf("review %v", r)
	}
	tr = a.mustCall("GET", base+"/train", "", 200)
	drills = tr["drills"].([]any)
	if len(drills) != 1 || id(drills[0].(map[string]any)["move"]) != id(m3["id"]) {
		t.Fatalf("after the first answer: %v", tr)
	}
	// the next day the first move is due again, and comes before the new one
	a.now = t0.Add(25 * time.Hour)
	drills = a.mustCall("GET", base+"/train", "", 200)["drills"].([]any)
	if len(drills) != 2 || id(drills[0].(map[string]any)["move"]) != id(m1["id"]) || drills[0].(map[string]any)["new"] != false {
		t.Fatalf("a day later: %v", drills)
	}
	// a wrong answer: back down the ladder
	r = a.mustCall("POST", base+"/review", fmt.Sprintf(`{"move":%d,"correct":false}`, id(m1["id"])), 200)
	if r["box"] != 0.0 || r["lapses"] != 1.0 {
		t.Errorf("failure %v", r)
	}
	// the opponent's move is not a card
	a.mustCall("POST", base+"/review", fmt.Sprintf(`{"move":%d,"correct":true}`, id(m2["id"])), 404)

	// deleting a move deletes the lines after it
	del := a.mustCall("DELETE", fmt.Sprintf("%s/moves/%d", base, id(m2["id"])), "", 200)
	if del["deleted"] != 2.0 {
		t.Errorf("deleted %v", del)
	}
	a.mustCall("DELETE", fmt.Sprintf("%s/moves/%d", base, id(m2["id"])), "", 404)

	// rename, list, delete
	a.mustCall("PATCH", base, `{"name":"Новое имя"}`, 200)
	lst := a.mustCall("GET", "/api/repertoires", "", 200)["repertoires"].([]any)
	if len(lst) != 1 || lst[0].(map[string]any)["name"] != "Новое имя" {
		t.Errorf("list %v", lst)
	}
	if code, _ := a.call("DELETE", base, ""); code != 204 {
		t.Errorf("delete %d", code)
	}
	a.mustCall("GET", base, "", 404)
}

func TestRepertoireAccess(t *testing.T) {
	a := newAPI(t)
	rep := a.mustCall("POST", "/api/repertoires", `{"name":"Моё","color":"black"}`, 201)
	base := fmt.Sprintf("/api/repertoires/%d", id(rep["id"]))
	// the first move of a Black repertoire is White's: not mine
	m := a.mustCall("POST", base+"/moves", `{"parent":0,"uci":"e2e4"}`, 201)["move"].(map[string]any)
	if m["mine"] != false {
		t.Errorf("White's move in a Black repertoire: %v", m)
	}
	// the gap: a line that ends with the opponent's move
	if g := a.mustCall("GET", base, "", 200)["gaps"].([]any); len(g) != 1 {
		t.Errorf("gaps %v", g)
	}
	// somebody else sees nothing
	a.user = "boris"
	a.mustCall("GET", base, "", 404)
	a.mustCall("POST", base+"/moves", `{"parent":0,"uci":"d2d4"}`, 404)
	if l := a.mustCall("GET", "/api/repertoires", "", 200)["repertoires"].([]any); len(l) != 0 {
		t.Errorf("boris sees %v", l)
	}
	// and nobody signed in is turned away
	a.user = ""
	a.mustCall("GET", "/api/repertoires", "", 401)
	a.mustCall("POST", "/api/repertoires", `{"name":"x","color":"white"}`, 401)
}

func TestRepertoireValidation(t *testing.T) {
	a := newAPI(t)
	a.mustCall("POST", "/api/repertoires", `{"name":"","color":"white"}`, 400)
	a.mustCall("POST", "/api/repertoires", `{"name":"x","color":"green"}`, 400)
	a.mustCall("POST", "/api/repertoires", `not json`, 400)
	a.mustCall("POST", "/api/repertoires", `{"name":"`+strings.Repeat("я", 81)+`","color":"white"}`, 400)
	a.mustCall("GET", "/api/repertoires/abc", "", 404)
}

// ------------------------------------------------------- games vs repository

// A White repository: 1.e4 e5 2.Nf3 Nc6 3.Bc4 and 1...c5 2.Nf3 d6, with real moves.
func italian() []store.RepMove {
	return []store.RepMove{
		mv(1, 0, "e2e4", "e4", 1, true),
		mv(2, 1, "e7e5", "e5", 2, false),
		mv(3, 2, "g1f3", "Nf3", 3, true),
		mv(4, 3, "b8c6", "Nc6", 4, false),
		mv(5, 4, "f1c4", "Bc4", 5, true),
		mv(6, 1, "c7c5", "c5", 2, false),
		mv(7, 6, "g1f3", "Nf3", 3, true),
		mv(8, 7, "d7d6", "d6", 4, false),
	}
}

func TestCompareFollowsTheTree(t *testing.T) {
	ms := italian()
	// the whole game inside the tree
	d := Compare(ms, "white", []string{"e2e4", "e7e5", "g1f3", "b8c6", "f1c4"})
	if d.Status != StatusInBook || d.Ply != 0 || d.Moves != 5 || len(d.Line) != 5 || d.Line[4] != "Bc4" {
		t.Errorf("in book: %+v", d)
	}
	// I forgot my own move: 3.Bb5 instead of 3.Bc4 would be a deviation at ply 5; here 2.Nc3 instead of 2.Nf3
	d = Compare(ms, "white", []string{"e2e4", "e7e5", "b1c3"})
	if d.Status != StatusYouDeviated || d.Ply != 3 || d.Node != 2 || d.Played.SAN != "Nc3" {
		t.Fatalf("you deviated: %+v", d)
	}
	if len(d.Expected) != 1 || d.Expected[0].SAN != "Nf3" || d.Expected[0].ID != 3 || !d.Expected[0].Mine {
		t.Errorf("expected: %+v", d.Expected)
	}
	if len(d.Line) != 2 || d.Line[0] != "e4" || d.Line[1] != "e5" {
		t.Errorf("line before the deviation: %v", d.Line)
	}
	if d.Played.FEN == "" {
		t.Error("no position after the played move")
	}
	// the opponent left my preparation: 1...e6 where the repository has e5 and c5
	d = Compare(ms, "white", []string{"e2e4", "e7e6", "d2d4"})
	if d.Status != StatusOpponentDeviated || d.Ply != 2 || d.Node != 1 || d.Played.SAN != "e6" || len(d.Expected) != 2 {
		t.Errorf("opponent deviated: %+v", d)
	}
	// the repository stops: 1.e4 c5 2.Nf3 d6 3.d4 has nothing after d6
	d = Compare(ms, "white", []string{"e2e4", "c7c5", "g1f3", "d7d6", "d2d4"})
	if d.Status != StatusBookEnded || d.Ply != 5 || d.Node != 8 || d.Played.SAN != "d4" || len(d.Expected) != 0 {
		t.Errorf("book ended: %+v", d)
	}
}

func TestCompareForBlackAndEdgeCases(t *testing.T) {
	// a Black repository: 1.e4 is White's (not mine), 1...c5 mine
	ms := []store.RepMove{
		mv(1, 0, "e2e4", "e4", 1, false),
		mv(2, 1, "c7c5", "c5", 2, true),
		mv(3, 2, "g1f3", "Nf3", 3, false),
	}
	d := Compare(ms, "black", []string{"e2e4", "e7e5"})
	if d.Status != StatusYouDeviated || d.Ply != 2 || d.Expected[0].SAN != "c5" || !d.Expected[0].Mine {
		t.Errorf("black forgot: %+v", d)
	}
	d = Compare(ms, "black", []string{"d2d4"})
	if d.Status != StatusOpponentDeviated || d.Ply != 1 || d.Node != 0 {
		t.Errorf("white left the line at move 1: %+v", d)
	}
	// an empty repository, an empty game, an illegal move
	if d := Compare(nil, "white", []string{"e2e4"}); d.Status != StatusEmpty {
		t.Errorf("empty: %+v", d)
	}
	if d := Compare(ms, "black", nil); d.Status != StatusInBook || d.Moves != 0 {
		t.Errorf("no moves: %+v", d)
	}
	if d := Compare(ms, "black", []string{"e2e5"}); d.Status != StatusBroken {
		t.Errorf("illegal move: %+v", d)
	}
	// castling is the engine's e1g1, as the game stores it
	cs := []store.RepMove{mv(1, 0, "e2e4", "e4", 1, true), mv(2, 1, "e7e5", "e5", 2, false), mv(3, 2, "g1f3", "Nf3", 3, true),
		mv(4, 3, "b8c6", "Nc6", 4, false), mv(5, 4, "f1c4", "Bc4", 5, true), mv(6, 5, "g8f6", "Nf6", 6, false), mv(7, 6, "e1g1", "O-O", 7, true)}
	if d := Compare(cs, "white", []string{"e2e4", "e7e5", "g1f3", "b8c6", "f1c4", "g8f6", "e1g1"}); d.Status != StatusInBook || d.Line[6] != "O-O" {
		t.Errorf("castling: %+v", d)
	}
}

func gameState(white, black string, ucis ...string) []byte {
	type mvr struct {
		UCI string `json:"uci"`
	}
	var ms []mvr
	for _, u := range ucis {
		ms = append(ms, mvr{u})
	}
	b, _ := json.Marshal(map[string]any{
		"white":   map[string]any{"id": "anna", "name": white},
		"black":   map[string]any{"id": "bot:misha", "name": black, "bot": "misha"},
		"moves":   ms,
		"endedMs": 1700000000000,
	})
	return b
}

func TestGamesAgainstTheRepository(t *testing.T) {
	a := newAPI(t)
	repo := a.mustCall("POST", "/api/repertoires", `{"name":"Итальянка","color":"white"}`, 201)
	base := fmt.Sprintf("/api/repertoires/%d", id(repo["id"]))

	// an empty repository has nothing to compare
	a.mem.games = []*store.Game{{ID: "g0", WhiteID: "anna", BlackID: "", Status: "ended", Result: "1-0", State: gameState("Анна", "Миша", "e2e4", "e7e5")}}
	out := a.mustCall("GET", base+"/games", "", 200)
	if g := out["games"].([]any); len(g) != 1 || g[0].(map[string]any)["deviation"].(map[string]any)["status"] != "empty" {
		t.Errorf("empty repository: %v", out)
	}

	// 1.e4 e5 2.Nf3 (mine: e4, Nf3)
	m1 := a.mustCall("POST", base+"/moves", `{"parent":0,"uci":"e2e4"}`, 201)["move"].(map[string]any)
	m2 := a.mustCall("POST", base+"/moves", fmt.Sprintf(`{"parent":%d,"uci":"e7e5"}`, id(m1["id"])), 201)["move"].(map[string]any)
	m3 := a.mustCall("POST", base+"/moves", fmt.Sprintf(`{"parent":%d,"uci":"g1f3"}`, id(m2["id"])), 201)["move"].(map[string]any)

	a.mem.games = []*store.Game{
		{ID: "stayed", WhiteID: "anna", BlackID: "bot:misha", Status: "ended", Result: "1-0", State: gameState("Анна", "Миша", "e2e4", "e7e5", "g1f3", "b8c6")},
		{ID: "forgot", WhiteID: "anna", BlackID: "bot:misha", Status: "ended", Result: "0-1", State: gameState("Анна", "Миша", "e2e4", "e7e5", "b1c3", "g8f6")},
		{ID: "opponent", WhiteID: "anna", BlackID: "bot:misha", Status: "ended", Result: "1-0", State: gameState("Анна", "Миша", "e2e4", "d7d6", "d2d4")},
		{ID: "black", WhiteID: "boris", BlackID: "anna", Status: "ended", Result: "1-0", State: gameState("Борис", "Анна", "e2e4", "e7e5")}, // I was Black: not for a White repository
		{ID: "short", WhiteID: "anna", BlackID: "bot:misha", Status: "playing", State: gameState("Анна", "Миша", "e2e4")},                   // one move: nothing to say
	}
	out = a.mustCall("GET", base+"/games", "", 200)
	games := out["games"].([]any)
	if len(games) != 3 {
		t.Fatalf("%d games, want 3: %v", len(games), games)
	}
	by := map[string]map[string]any{}
	for _, g := range games {
		gm := g.(map[string]any)
		by[gm["id"].(string)] = gm
	}
	dev := func(gid string) map[string]any { return by[gid]["deviation"].(map[string]any) }
	if dev("stayed")["status"] != "book-ended" || dev("stayed")["ply"] != 4.0 {
		// the repository ends after 2.Nf3: Nc6 is a move it has nothing for
		t.Errorf("stayed: %v", dev("stayed"))
	}
	if dev("forgot")["status"] != "you-deviated" || dev("forgot")["ply"] != 3.0 {
		t.Fatalf("forgot: %v", dev("forgot"))
	}
	exp := dev("forgot")["expected"].([]any)[0].(map[string]any)
	if exp["san"] != "Nf3" || id(exp["id"]) != id(m3["id"]) || exp["mine"] != true {
		t.Errorf("expected move: %v", exp)
	}
	if dev("opponent")["status"] != "opponent-deviated" || dev("opponent")["node"] != m1["id"] {
		t.Errorf("opponent: %v", dev("opponent"))
	}
	if by["forgot"]["opponent"] != "Миша" || by["forgot"]["bot"] != true || by["forgot"]["result"] != "0-1" {
		t.Errorf("game header: %v", by["forgot"])
	}
	counts := out["counts"].(map[string]any)
	if counts["you-deviated"] != 1.0 || counts["opponent-deviated"] != 1.0 || counts["book-ended"] != 1.0 {
		t.Errorf("counts: %v", counts)
	}
	// somebody else's repository gives nothing away
	a.user = "boris"
	a.mustCall("GET", base+"/games", "", 404)
}

// ------------------------------------------------------- pasted games

const pastedFile = `[Event "a"]
[White "Dyanis"]
[Black "Rival"]
[Date "2024.03.17"]
[Result "1-0"]

1. e4 e5 2. Nf3 Nc6 3. Bc4 1-0

[Event "b"]
[White "Other"]
[Black "Dyanis"]
[Date "2024.??.??"]
[Result "0-1"]

1. e4 e5 2. Nc3 Nf6 0-1

[Event "c"]
[White "Dyanis"]
[Black "Rival"]
[Result "*"]

1. e4 e6 *

[Event "d"]
[White "Dyanis"]
[Black "Rival"]
[SetUp "1"]
[FEN "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq - 0 1"]
[Result "*"]

1... e5 2. Nf3 *
`

func TestComparePasted(t *testing.T) {
	games, err := pgn.ParseString(pastedFile)
	if err != nil {
		t.Fatal(err)
	}
	// without a name every game is taken as the person's, in the repository's colour
	items, skipped := comparePasted(italian(), "white", "", games)
	if len(items) != 3 || skipped != 1 {
		t.Fatalf("no name: %d games, %d skipped", len(items), skipped)
	}
	if items[0].Dev.Status != StatusInBook || items[0].Opponent != "Rival" || items[0].Date != "17.03.2024" {
		t.Errorf("first: %+v", items[0])
	}
	if items[1].Dev.Status != StatusYouDeviated || items[1].Date != "2024" {
		t.Errorf("second: %+v", items[1])
	}
	if items[2].Dev.Status != StatusOpponentDeviated || items[2].Date != "" {
		t.Errorf("third: %+v", items[2])
	}
	// with a name, the games of the other colour are left out
	items, skipped = comparePasted(italian(), "white", " dyanis ", games)
	if len(items) != 2 || skipped != 2 {
		t.Errorf("white as Dyanis: %d games, %d skipped", len(items), skipped)
	}
	items, _ = comparePasted(italian(), "black", "DYANIS", games)
	if len(items) != 1 || items[0].Opponent != "Other" {
		t.Errorf("black as Dyanis: %+v", items)
	}
}

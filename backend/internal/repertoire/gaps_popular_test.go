package repertoire

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dyanis-chess/backend/internal/explorer"
	"dyanis-chess/backend/internal/store"
	"dyanis-chess/engine"
)

// ------------------------------------------------------------------ helpers

// fenAfter is the position after the moves (UCI) from the start.
func fenAfter(t testing.TB, ucis ...string) string {
	t.Helper()
	pos, err := engine.ParseFEN(engine.StartFEN)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range ucis {
		ok := false
		for _, m := range pos.LegalMoves() {
			if m.String() == u {
				pos.Play(m)
				ok = true
				break
			}
		}
		if !ok {
			t.Fatalf("illegal move %s after %v", u, ucis)
		}
	}
	return pos.FEN()
}

// buildTree makes the moves of a repertoire of `color` from lines of UCI moves; lines share
// their common beginning. Ids count from 1 in the order the moves first appear.
func buildTree(t testing.TB, color string, lines ...[]string) []store.RepMove {
	t.Helper()
	var out []store.RepMove
	ids := map[string]int64{} // path -> id
	for _, line := range lines {
		path, parent := "", int64(0)
		pos, _ := engine.ParseFEN(engine.StartFEN)
		for i, u := range line {
			path += u + " "
			var san string
			ok := false
			for _, m := range pos.LegalMoves() {
				if m.String() == u {
					san, ok = pos.SAN(m), true
					pos.Play(m)
					break
				}
			}
			if !ok {
				t.Fatalf("illegal move %s in %v", u, line)
			}
			if id, seen := ids[path]; seen {
				parent = id
				continue
			}
			id := int64(len(out) + 1)
			ids[path] = id
			ply := i + 1
			out = append(out, store.RepMove{ID: id, RepertoireID: 1, ParentID: parent, UCI: u, SAN: san, FEN: pos.FEN(), Ply: ply,
				Mine: (ply%2 == 1) == (color == "white")})
			parent = id
		}
	}
	return out
}

type mm struct {
	uci, san string
	games    int
}

func answer(total int, moves ...mm) explorer.Response {
	r := explorer.Response{Available: true, Total: total, Moves: []explorer.Move{}}
	for _, m := range moves {
		r.Moves = append(r.Moves, explorer.Move{UCI: m.uci, SAN: m.san, Games: m.games})
	}
	return r
}

// fakeStats is an explorer that knows some positions, remembers what it was asked and cheats
// like the real one: a second look at a position is a cache hit.
type fakeStats struct {
	mu        sync.Mutex
	off       bool
	data      map[string]explorer.Response
	cache     map[string]bool
	calls     int // uncached lookups
	waitAfter int // from this many uncached lookups on, Lichess asks to wait (0: never)
}

func newFake(data map[string]explorer.Response) *fakeStats {
	return &fakeStats{data: data, cache: map[string]bool{}}
}

func (f *fakeStats) Enabled() bool { return !f.off }

func (f *fakeStats) Cached(fen, _, _ string) (explorer.Response, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cache[fen] {
		return f.data[fen], true
	}
	return explorer.Response{}, false
}

func (f *fakeStats) Lookup(_ context.Context, fen, _, _ string) (explorer.Response, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cache[fen] {
		return f.data[fen], true, nil
	}
	if f.waitAfter > 0 && f.calls >= f.waitAfter {
		return explorer.Response{}, false, &explorer.WaitError{Wait: 30 * time.Second}
	}
	f.calls++
	f.cache[fen] = true
	r, ok := f.data[fen]
	if !ok {
		r = answer(0)
	}
	return r, false, nil
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func scan(t *testing.T, moves []store.RepMove, color string, src StatsSource, o GapOptions) GapScan {
	t.Helper()
	return ScanGaps(context.Background(), moves, color, src, nil, o, t0)
}

func gapKey(g Gap) string { return fmt.Sprintf("%s:%s:%d", g.Kind, g.SAN, g.At) }

// the replies to 1.e4 used by the tests below (thousands of games, shares as in the comments)
func afterE4() explorer.Response {
	return answer(10000,
		mm{"e7e5", "e5", 4000}, mm{"c7c5", "c5", 3000}, mm{"e7e6", "e6", 1000}, mm{"c7c6", "c6", 900},
		mm{"d7d5", "d5", 400}, mm{"a7a6", "a6", 50})
}

// ------------------------------------------------------------------- tests

func TestMissingRepliesWeightAndOrder(t *testing.T) {
	moves := buildTree(t, "white", []string{"e2e4"})
	f := newFake(map[string]explorer.Response{fenAfter(t, "e2e4"): afterE4()})
	sc := scan(t, moves, "white", f, GapOptions{})

	var got []string
	for _, g := range sc.Gaps {
		got = append(got, fmt.Sprintf("%s %.2f", g.SAN, g.Weight))
		if g.Kind != GapMissing || g.At != 1 || g.Source != "lichess" || len(g.Path) != 1 || g.Path[0] != "e4" {
			t.Errorf("gap %+v", g)
		}
	}
	// d5 is 4% (below the 5% threshold) and a6 is 0.5%: not holes
	if want := "[e5 0.40 c5 0.30 e6 0.10 c6 0.09]"; fmt.Sprint(got) != want {
		t.Fatalf("holes %v, want %v", got, want)
	}
	if sc.Gaps[0].Pct != 40 || sc.Gaps[0].Games != 4000 || sc.Gaps[0].UCI != "e7e5" || sc.Gaps[0].FEN != fenAfter(t, "e2e4") {
		t.Errorf("first hole %+v", sc.Gaps[0])
	}
	if !near(sc.Coverage, 0.11) {
		t.Errorf("coverage %v, want 0.11", sc.Coverage)
	}
	if sc.Counts.Missing != 4 || sc.Counts.Answer != 0 || sc.Counts.Gaps != 4 || sc.Partial || sc.Source != "lichess" || !sc.Available {
		t.Errorf("counts %+v partial=%v source=%s", sc.Counts, sc.Partial, sc.Source)
	}
	if f.calls != 1 {
		t.Errorf("asked Lichess %d times, want 1", f.calls)
	}
}

func TestRepliesAlreadyInTheTreeAreNotMissing(t *testing.T) {
	// 1.e4 e5 2.Nf3 and 1.e4 c5 (no answer yet)
	moves := buildTree(t, "white", []string{"e2e4", "e7e5", "g1f3"}, []string{"e2e4", "c7c5"})
	f := newFake(map[string]explorer.Response{
		fenAfter(t, "e2e4"):                 afterE4(),
		fenAfter(t, "e2e4", "e7e5", "g1f3"): answer(4000, mm{"b8c6", "Nc6", 2400}, mm{"g8f6", "Nf6", 800}, mm{"d7d6", "d6", 400}, mm{"f8c5", "Bc5", 80}),
	})
	sc := scan(t, moves, "white", f, GapOptions{})

	var got []string
	for _, g := range sc.Gaps {
		got = append(got, fmt.Sprintf("%s %s %.3f", g.Kind, g.SAN, g.Weight))
	}
	// e5 and c5 are in the tree. c5 has no answer (a hole of the first kind, 30%); after 2.Nf3 the
	// replies Nc6 (.4*.6), Nf6 (.4*.2), d6 (.4*.1) are missing; Bc5 is 2% of them: too rare
	want := "[answer c5 0.300 missing Nc6 0.240 missing e6 0.100 missing c6 0.090 missing Nf6 0.080 missing d6 0.040]"
	if fmt.Sprint(got) != want {
		t.Fatalf("holes\n%v\nwant\n%v", got, want)
	}
	if sc.Counts.Missing != 5 || sc.Counts.Answer != 1 {
		t.Errorf("counts %+v", sc.Counts)
	}
	// holes: c5 .3 + e6 .1 + c6 .09 + (.24 + .08 + .04) = .85; the rest, 15%, is covered
	if !near(sc.Coverage, 0.15) {
		t.Errorf("coverage %v, want 0.15", sc.Coverage)
	}
	c5 := sc.Gaps[0]
	if c5.Pct != 30 || c5.Games != 3000 || c5.At != 4 || c5.SAN != "c5" {
		t.Errorf("the unanswered c5 knows how popular it is: %+v", c5)
	}
}

func TestRepertoireOfBlackStartsWithTheOpponent(t *testing.T) {
	// 1.e4 c5 2.Nf3 d6: the opponent's moves are e4 and Nf3, mine c5 and d6
	moves := buildTree(t, "black", []string{"e2e4", "c7c5", "g1f3", "d7d6"})
	f := newFake(map[string]explorer.Response{
		engine.StartFEN: answer(1000, mm{"e2e4", "e4", 500}, mm{"d2d4", "d4", 300}, mm{"g1f3", "Nf3", 110}, mm{"c2c4", "c4", 60}, mm{"b2b3", "b3", 10}),
	})
	f.data[fenAfter(t, "e2e4", "c7c5")] = answer(500, mm{"g1f3", "Nf3", 300}, mm{"b1c3", "Nc3", 100}, mm{"c2c3", "c3", 55})
	sc := scan(t, moves, "black", f, GapOptions{})
	var got []string
	for _, g := range sc.Gaps {
		got = append(got, fmt.Sprintf("%s %s %.3f@%d", g.Kind, g.SAN, g.Weight, g.At))
	}
	// at the start: d4 (.3), Nf3 (.11), c4 (.06) are missing (the tree's Nf3 comes after 1.e4 c5).
	// After 1.e4 c5 (weight .5): Nf3 is in the tree, Nc3 (.5*.2 = .1) and c3 (.5*.11) are missing.
	// Nothing is known about the position after 2.Nf3 d6 (no statistics), so the line ends there.
	want := "[missing d4 0.300@0 missing Nf3 0.110@0 missing Nc3 0.100@2 missing c4 0.060@0 missing c3 0.055@2]"
	if fmt.Sprint(got) != want {
		t.Errorf("holes %v, want %v", got, want)
	}
}

func TestMovesOutsideTheStatisticsAreSearchedWithoutWeight(t *testing.T) {
	// 1.e4 a5: the explorer does not list a5 at all; the line ends with the opponent's move
	moves := buildTree(t, "white", []string{"e2e4", "a7a5"})
	f := newFake(map[string]explorer.Response{fenAfter(t, "e2e4"): afterE4()})
	sc := scan(t, moves, "white", f, GapOptions{})
	var ans *Gap
	for i, g := range sc.Gaps {
		if g.Kind == GapAnswer {
			ans = &sc.Gaps[i]
		}
	}
	if ans == nil || ans.SAN != "a5" || ans.Weight != 0 || ans.Pct != 0 {
		t.Fatalf("the unanswered a5: %+v", ans)
	}
	// it stays at the end of the list, behind the weighty holes
	if sc.Gaps[len(sc.Gaps)-1].Kind != GapAnswer || sc.Counts.Answer != 1 {
		t.Errorf("order %v", sc.Gaps)
	}
}

func TestThresholds(t *testing.T) {
	moves := buildTree(t, "white", []string{"e2e4"})
	fen := fenAfter(t, "e2e4")
	// too few games in the position: nothing to be missing
	f := newFake(map[string]explorer.Response{fen: answer(40, mm{"e7e5", "e5", 30}, mm{"c7c5", "c5", 10})})
	if sc := scan(t, moves, "white", f, GapOptions{}); len(sc.Gaps) != 0 || !near(sc.Coverage, 1) {
		t.Errorf("a thin position: %+v", sc)
	}
	// minPct moves the line
	f = newFake(map[string]explorer.Response{fen: afterE4()})
	sc := scan(t, moves, "white", f, GapOptions{MinPct: 9.5})
	if len(sc.Gaps) != 3 { // e5, c5, e6 (10%)
		t.Errorf("minPct 9.5: %d holes", len(sc.Gaps))
	}
	sc = scan(t, moves, "white", f, GapOptions{MinPct: 3})
	if len(sc.Gaps) != 5 { // plus c6 9% and d5 4%
		t.Errorf("minPct 3: %d holes", len(sc.Gaps))
	}
	// a hole lighter than minWeight is not listed
	sc = scan(t, moves, "white", f, GapOptions{MinPct: 3, MinWeight: 0.05})
	if len(sc.Gaps) != 4 { // d5 (.04) drops out
		t.Errorf("minWeight .05: %d holes", len(sc.Gaps))
	}
	// the list is cut at Limit, the counts are not
	sc = scan(t, moves, "white", f, GapOptions{Limit: 2})
	if len(sc.Gaps) != 2 || sc.Counts.Gaps != 4 {
		t.Errorf("limit: %d listed, %d counted", len(sc.Gaps), sc.Counts.Gaps)
	}
}

func TestEmptyRepertoire(t *testing.T) {
	// White with nothing yet: the hole is the first move itself
	sc := scan(t, nil, "white", newFake(nil), GapOptions{})
	if len(sc.Gaps) != 1 || sc.Gaps[0].Kind != GapAnswer || sc.Gaps[0].At != 0 || sc.Gaps[0].Weight != 1 || sc.Coverage != 0 {
		t.Errorf("empty repertoire of White: %+v", sc)
	}
	// Black with nothing yet: every popular first move of White is missing
	f := newFake(map[string]explorer.Response{engine.StartFEN: answer(1000, mm{"e2e4", "e4", 500}, mm{"d2d4", "d4", 400})})
	sc = scan(t, nil, "black", f, GapOptions{})
	if len(sc.Gaps) != 2 || sc.Gaps[0].SAN != "e4" || !near(sc.Coverage, 0.1) {
		t.Errorf("empty repertoire of Black: %+v", sc)
	}
}

func TestAlternativeAnswersCountAsOneAnswer(t *testing.T) {
	// two moves of mine against 1.e4 e5: Nf3 (a hole below) and Bc4 (also a hole below):
	// the chance of ending in a hole is that of the better branch, not the sum
	moves := buildTree(t, "white", []string{"e2e4", "e7e5", "g1f3"}, []string{"e2e4", "e7e5", "f1c4"})
	f := newFake(map[string]explorer.Response{
		fenAfter(t, "e2e4"):                 answer(1000, mm{"e7e5", "e5", 1000}),
		fenAfter(t, "e2e4", "e7e5", "g1f3"): answer(1000, mm{"b8c6", "Nc6", 600}),
		fenAfter(t, "e2e4", "e7e5", "f1c4"): answer(1000, mm{"g8f6", "Nf6", 500}, mm{"b8c6", "Nc6", 200}),
	})
	sc := scan(t, moves, "white", f, GapOptions{})
	if len(sc.Gaps) != 3 {
		t.Fatalf("holes of both branches are listed: %d", len(sc.Gaps))
	}
	if !near(sc.Coverage, 0.4) { // Nf3 branch leaves .6 uncovered, Bc4 branch .7: the better one counts
		t.Errorf("coverage %v, want 0.4", sc.Coverage)
	}
}

func TestBudgetPartialAndResuming(t *testing.T) {
	// 1.e4 e5 2.Nf3 Nc6 3.Bb5 a6 4.Ba4 Nf6 5.O-O: every opponent position is in the explorer
	line := []string{"e2e4", "e7e5", "g1f3", "b8c6", "f1b5", "a7a6", "b5a4", "g8f6", "e1g1"}
	moves := buildTree(t, "white", line)
	data := map[string]explorer.Response{}
	for i := 1; i < len(line); i += 2 { // positions where Black moves
		next := line[i]
		other := map[string]string{"e7e5": "c7c5", "b8c6": "g8f6", "a7a6": "g8f6", "g8f6": "f8e7"}[next]
		data[fenAfter(t, line[:i]...)] = answer(1000, mm{next, "x" + next, 600}, mm{other, "x" + other, 300})
	}
	f := newFake(data)
	sc := scan(t, moves, "white", f, GapOptions{Budget: 2})
	if f.calls > 2 || sc.Counts.Fetched != 2 {
		t.Fatalf("budget 2: %d requests, fetched %d", f.calls, sc.Counts.Fetched)
	}
	if !sc.Partial || sc.Counts.Pending == 0 || len(sc.Pending) == 0 {
		t.Fatalf("a search that ran out of requests says it is partial: %+v", sc)
	}
	if sc.Pending[0].Weight <= 0 || sc.Pending[0].At == 0 {
		t.Errorf("pending %+v", sc.Pending)
	}
	partialCoverage := sc.Coverage
	// again: what was asked is in the cache (free), and two more are asked
	sc2 := scan(t, moves, "white", f, GapOptions{Budget: 2})
	if sc2.Counts.FromCache < 2 || sc2.Counts.Fetched != 2 || f.calls != 4 {
		t.Errorf("second call: %+v, %d requests in all", sc2.Counts, f.calls)
	}
	if sc2.Counts.Checked <= sc.Counts.Checked {
		t.Errorf("checked %d then %d", sc.Counts.Checked, sc2.Counts.Checked)
	}
	// with enough budget it finishes, and the coverage is no longer a bound
	sc3 := scan(t, moves, "white", f, GapOptions{})
	if sc3.Partial || sc3.Counts.Pending != 0 || len(sc3.Pending) != 0 {
		t.Errorf("finished search: %+v", sc3)
	}
	if sc3.Coverage < partialCoverage {
		t.Errorf("coverage fell from %v to %v", partialCoverage, sc3.Coverage)
	}
	// the cache answers a repeat in full without any request
	calls := f.calls
	if sc4 := scan(t, moves, "white", f, GapOptions{}); f.calls != calls || sc4.Counts.Fetched != 0 {
		t.Errorf("a repeated search asked again: %d -> %d", calls, f.calls)
	}
}

func TestDefaultBudgetIsForty(t *testing.T) {
	// a long line of 60 positions where the opponent moves, every one in the explorer with the
	// move of the line as its popular reply (weight 0.9 per step): too many for one call
	// (all the positions differ: one reached twice would be searched once)
	moves := buildTree(t, "white", distinctLine(t, 120))
	data := map[string]explorer.Response{}
	for _, m := range moves {
		if !m.Mine {
			continue
		}
		for _, c := range moves {
			if c.ParentID == m.ID {
				data[m.FEN] = answer(1000, mm{c.UCI, c.SAN, 900})
			}
		}
	}
	f := newFake(data)
	sc := scan(t, moves, "white", f, GapOptions{})
	if f.calls != DefaultBudget || sc.Counts.Fetched != DefaultBudget || !sc.Partial {
		t.Errorf("%d requests (fetched %d, partial %v), budget %d", f.calls, sc.Counts.Fetched, sc.Partial, DefaultBudget)
	}
}

func TestNoTokenFallsBackToTheory(t *testing.T) {
	moves := buildTree(t, "white", []string{"e2e4"})
	for _, src := range []StatsSource{nil, func() StatsSource { f := newFake(nil); f.off = true; return f }()} {
		sc := scan(t, moves, "white", src, GapOptions{})
		if sc.Source != "theory" || sc.Available || sc.Partial {
			t.Fatalf("source %q available %v partial %v", sc.Source, sc.Available, sc.Partial)
		}
		have := map[string]bool{}
		for _, g := range sc.Gaps {
			have[g.SAN] = true
			if g.Source != "theory" || g.Games != 0 || g.Lines == 0 || g.Weight <= 0 || g.Kind != GapMissing {
				t.Errorf("theory hole %+v", g)
			}
		}
		if !have["e5"] || !have["c5"] {
			t.Errorf("theory knows e5 and c5 against 1.e4: %v", have)
		}
		for i := 1; i < len(sc.Gaps); i++ {
			if sc.Gaps[i].Weight > sc.Gaps[i-1].Weight {
				t.Errorf("not sorted by weight at %d", i)
			}
		}
	}
}

func TestLichessAsksToWaitFallsBackMidway(t *testing.T) {
	line := []string{"e2e4", "e7e5", "g1f3", "b8c6", "f1b5"}
	moves := buildTree(t, "white", line)
	f := newFake(map[string]explorer.Response{
		fenAfter(t, "e2e4"):                 answer(1000, mm{"e7e5", "e5", 700}, mm{"c7c5", "c5", 200}),
		fenAfter(t, "e2e4", "e7e5", "g1f3"): answer(700, mm{"b8c6", "Nc6", 500}, mm{"g8f6", "Nf6", 150}),
	})
	f.waitAfter = 1 // the first position is answered, then Lichess asks to wait
	sc := scan(t, moves, "white", f, GapOptions{})
	if !sc.Limited || sc.RetryAfter < 30 || sc.Source != "mixed" || sc.Partial {
		t.Fatalf("limited=%v retry=%d source=%s partial=%v", sc.Limited, sc.RetryAfter, sc.Source, sc.Partial)
	}
	if f.calls != 1 {
		t.Errorf("asked %d times after being told to wait", f.calls)
	}
	var fromLichess, fromTheory int
	for _, g := range sc.Gaps {
		if g.Source == "lichess" {
			fromLichess++
		} else {
			fromTheory++
		}
	}
	if fromLichess == 0 || fromTheory == 0 {
		t.Errorf("a mixed search has holes of both sources: %d and %d", fromLichess, fromTheory)
	}
}

// ---------------------------------------------------------------- the handler

func gapsAPI(t *testing.T, stats StatsSource) *api {
	a := newAPI(t)
	s := New(Config{Store: a.mem, UserID: func(*http.Request) string { return a.user }, Now: func() time.Time { return a.now }, Stats: stats})
	a.h = s.Handler()
	return a
}

func (a *api) raw(method, url string) (int, []byte) {
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, httptest.NewRequest(method, url, strings.NewReader("")))
	return rec.Code, rec.Body.Bytes()
}

func TestGapsEndpoint(t *testing.T) {
	f := newFake(map[string]explorer.Response{fenAfter(t, "e2e4"): afterE4()})
	a := gapsAPI(t, f)
	rep := a.mustCall("POST", "/api/repertoires", `{"name":"E4","color":"white"}`, 201)
	base := fmt.Sprintf("/api/repertoires/%d", id(rep["id"]))
	a.mustCall("POST", base+"/moves", `{"parent":0,"uci":"e2e4"}`, 201)

	// nothing saved yet
	if code, _ := a.raw("GET", base+"/gaps?saved=1"); code != 204 {
		t.Errorf("saved before any search: %d", code)
	}
	// the list already shows the lines that end with the opponent's move... none: the line ends with mine
	list := a.mustCall("GET", "/api/repertoires", "", 200)["repertoires"].([]any)[0].(map[string]any)["gaps"].(map[string]any)
	if list["scanned"] != false || list["open"].(float64) != 0 {
		t.Errorf("summary before a search: %v", list)
	}

	out := a.mustCall("GET", base+"/gaps?minPct=5&limit=3", "", 200)
	gaps := out["gaps"].([]any)
	if len(gaps) != 3 || gaps[0].(map[string]any)["san"] != "e5" || out["source"] != "lichess" || out["partial"] != false {
		t.Fatalf("gaps %v", out)
	}
	counts := out["counts"].(map[string]any)
	if counts["missing"].(float64) != 4 || counts["gaps"].(float64) != 4 {
		t.Errorf("counts %v", counts)
	}
	if f.calls != 1 {
		t.Errorf("requests %d", f.calls)
	}

	// saved: the same list, and nobody was asked
	code, body := a.raw("GET", base+"/gaps?saved=1")
	var saved GapScan
	if code != 200 || json.Unmarshal(body, &saved) != nil || len(saved.Gaps) != 3 || saved.Stale {
		t.Fatalf("saved: %d %s", code, body)
	}
	if f.calls != 1 {
		t.Errorf("saved list asked Lichess")
	}
	// the list says what the search found, without asking
	list = a.mustCall("GET", "/api/repertoires", "", 200)["repertoires"].([]any)[0].(map[string]any)["gaps"].(map[string]any)
	if list["scanned"] != true || list["missing"].(float64) != 4 || list["stale"] != false || list["source"] != "lichess" {
		t.Errorf("summary after a search: %v", list)
	}
	// a changed tree makes the saved search stale
	a.mustCall("POST", base+"/moves", fmt.Sprintf(`{"parent":%d,"uci":"e7e5"}`, 1), 201)
	_, body = a.raw("GET", base+"/gaps?saved=1")
	json.Unmarshal(body, &saved)
	if !saved.Stale {
		t.Error("the tree changed but the saved search is not stale")
	}
	list = a.mustCall("GET", "/api/repertoires", "", 200)["repertoires"].([]any)[0].(map[string]any)["gaps"].(map[string]any)
	if list["stale"] != true || list["open"].(float64) != 1 { // 1...e5 has no answer
		t.Errorf("summary after a change: %v", list)
	}

	// somebody else's repertoire, bad parameters
	a.user = "boris"
	a.mustCall("GET", base+"/gaps", "", 404)
	a.user = "anna"
	a.mustCall("GET", base+"/gaps?ratings=9999", "", 400)
	// without Lichess the reference answers
	b := gapsAPI(t, nil)
	rep = b.mustCall("POST", "/api/repertoires", `{"name":"E4","color":"white"}`, 201)
	base = fmt.Sprintf("/api/repertoires/%d", id(rep["id"]))
	b.mustCall("POST", base+"/moves", `{"parent":0,"uci":"e2e4"}`, 201)
	out = b.mustCall("GET", base+"/gaps", "", 200)
	if out["source"] != "theory" || out["available"] != false || len(out["gaps"].([]any)) == 0 {
		t.Errorf("theory: %v", out)
	}
}

func TestHourlyQuotaStopsRequestsToLichessButNotTheCache(t *testing.T) {
	f := newFake(map[string]explorer.Response{fenAfter(t, "e2e4"): afterE4()})
	a := gapsAPI(t, f)
	svc := New(Config{Store: a.mem, UserID: func(*http.Request) string { return a.user }, Now: func() time.Time { return a.now }, Stats: f})
	a.h = svc.Handler()
	rep := a.mustCall("POST", "/api/repertoires", `{"name":"E4","color":"white"}`, 201)
	base := fmt.Sprintf("/api/repertoires/%d", id(rep["id"]))
	a.mustCall("POST", base+"/moves", `{"parent":0,"uci":"e2e4"}`, 201)

	svc.spendQuota("anna", hourlyQuota, a.now) // the hour's requests are used up
	out := a.mustCall("GET", base+"/gaps", "", 200)
	if f.calls != 0 || out["partial"] != true || out["counts"].(map[string]any)["fetched"].(float64) != 0 {
		t.Fatalf("over the quota: %d requests, %v", f.calls, out)
	}
	a.now = a.now.Add(61 * time.Minute) // a new hour
	out = a.mustCall("GET", base+"/gaps", "", 200)
	if f.calls != 1 || out["partial"] != false {
		t.Fatalf("after the hour: %d requests, %v", f.calls, out)
	}
	// what the search made is counted
	if got := svc.takeQuota("anna", 1000, a.now); got != hourlyQuota-1 {
		t.Errorf("quota left %d, want %d", got, hourlyQuota-1)
	}
	if got := svc.takeQuota("boris", 40, a.now); got != 40 {
		t.Errorf("another person's quota: %d", got)
	}
}

func TestNegativeBudgetMeansNoRequests(t *testing.T) {
	moves := buildTree(t, "white", []string{"e2e4"})
	f := newFake(map[string]explorer.Response{fenAfter(t, "e2e4"): afterE4()})
	sc := scan(t, moves, "white", f, GapOptions{Budget: -1})
	if f.calls != 0 || !sc.Partial || len(sc.Pending) != 1 || sc.Source == "theory" {
		t.Errorf("no budget: %d requests, partial=%v pending=%v source=%s", f.calls, sc.Partial, sc.Pending, sc.Source)
	}
}

// distinctLine is a legal line of n plies whose positions are all different (a fixed pseudo-random game).
func distinctLine(t testing.TB, n int) []string {
	t.Helper()
	pos, _ := engine.ParseFEN(engine.StartFEN)
	seen := map[string]bool{PosKey(pos.FEN()): true}
	var line []string
	state := uint32(12345)
	for len(line) < n {
		legal := pos.LegalMoves()
		if len(legal) == 0 {
			t.Fatalf("the game ended after %d plies", len(line))
		}
		state = state*1664525 + 1013904223
		start := int(state>>8) % len(legal)
		picked := false
		for i := range legal {
			m := legal[(start+i)%len(legal)]
			c := pos.Copy()
			c.Play(m)
			if k := PosKey(c.FEN()); !seen[k] {
				seen[k] = true
				pos.Play(m)
				line = append(line, m.String())
				picked = true
				break
			}
		}
		if !picked {
			t.Fatal("no new position")
		}
	}
	return line
}

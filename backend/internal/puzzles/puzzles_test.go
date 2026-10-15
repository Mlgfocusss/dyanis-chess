package puzzles

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dyanis-chess/backend/internal/store"
)

// The first puzzle of the Lichess database (id 00008).
const (
	fen00008   = "r6k/pp2r2p/4Rp1Q/3p4/8/1N1P2R1/PqP2bPP/7K b - - 0 24"
	moves00008 = "f2g3 e6e7 b2b1 b3c1 b1c1 h6c1"
)

func TestBuildLine(t *testing.T) {
	l, err := BuildLine(fen00008, moves00008)
	if err != nil {
		t.Fatal(err)
	}
	if l.Orientation != "white" { // Black plays the first move, so the person has White
		t.Errorf("orientation %q", l.Orientation)
	}
	if len(l.Steps) != 6 {
		t.Fatalf("%d steps", len(l.Steps))
	}
	if s := l.Steps[0]; s.UCI != "f2g3" || s.SAN != "Bxg3" || s.Legal != nil {
		t.Errorf("the opponent's step has no legal list: %+v", s)
	}
	s1 := l.Steps[1]
	if s1.UCI != "e6e7" || len(s1.Legal) < 10 || !contains(s1.Legal, "e6e7") {
		t.Errorf("first person's step: %+v", s1)
	}
	if l.Steps[0].FEN == fen00008 || !strings.Contains(l.Steps[0].FEN, " w ") {
		t.Errorf("fen after the opponent's move: %s", l.Steps[0].FEN)
	}
	// The last move, Qxc1 ... is not mate here; no alternatives were invented.
	if last := l.Steps[5]; len(last.Alt) != 0 {
		t.Errorf("alternatives at the last step: %+v", last.Alt)
	}
}

func TestBuildLineMateAlternatives(t *testing.T) {
	// Back-rank mate: Rd8 is the line, but Ra8 mates too (Rook d1 -> a... use two rooks).
	// White rooks on d1 and e1 against a king on g8 with pawns on f7 g7 h7: Rd8# and Re8# both mate.
	l, err := BuildLine("6k1/5ppp/8/8/8/8/5PPP/3RR1K1 b - - 0 1", "g8h8 d1d8")
	if err != nil {
		t.Fatal(err)
	}
	st := l.Steps[1]
	if len(st.Alt) != 1 || st.Alt[0].UCI != "e1e8" || st.Alt[0].SAN != "Re8#" {
		t.Errorf("alternatives: %+v", st.Alt)
	}
}

func TestBuildLineRejectsBadRows(t *testing.T) {
	for name, c := range map[string][2]string{
		"illegal move": {fen00008, "f2g3 e6a1"},
		"too short":    {fen00008, "f2g3"},
		"bad fen":      {"nonsense", "a1a2 a2a3"},
	} {
		if _, err := BuildLine(c[0], c[1]); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func TestNewRating(t *testing.T) {
	// Equal ratings: a win gains half of K, a loss loses half of K.
	if got := NewRating(1500, 100, 1500, true); got != 1510 {
		t.Errorf("win: %d", got)
	}
	if got := NewRating(1500, 100, 1500, false); got != 1490 {
		t.Errorf("loss: %d", got)
	}
	// A newcomer moves faster than a veteran.
	if a, b := NewRating(1000, 0, 1000, true), NewRating(1000, 100, 1000, true); a <= b {
		t.Errorf("newcomer +%d, veteran +%d", a-1000, b-1000)
	}
	// Beating a much harder puzzle gains nearly all of K; failing an easy one loses nearly all of it.
	if got := NewRating(1000, 100, 2000, true); got < 1019 {
		t.Errorf("hard win: %d", got)
	}
	if got := NewRating(1000, 100, 100, false); got > 981 {
		t.Errorf("easy loss: %d", got)
	}
	if got := NewRating(MinRating, 100, 3000, false); got != MinRating {
		t.Errorf("floor: %d", got)
	}
}

// ---------------------------------------------------------------- handlers

type fakeStore struct {
	puzzle   *store.Puzzle
	picked   [][3]int // user-less log of (lo, hi, skip) requests
	noneLow  int      // windows narrower than this find nothing
	rating   store.PuzzleRating
	recorded []bool
	tried    bool
}

func (f *fakeStore) PuzzleByID(_ context.Context, id string) (*store.Puzzle, error) {
	if id != f.puzzle.ID {
		return nil, store.ErrNotFound
	}
	return f.puzzle, nil
}

func (f *fakeStore) PickPuzzle(_ context.Context, _, theme string, lo, hi int, skip bool) (*store.Puzzle, error) {
	s := 0
	if skip {
		s = 1
	}
	f.picked = append(f.picked, [3]int{lo, hi, s})
	if theme == "nosuch" || hi-lo < 2*f.noneLow {
		return nil, store.ErrNotFound
	}
	return f.puzzle, nil
}

func (f *fakeStore) PuzzleThemes(context.Context) ([]store.ThemeCount, error) {
	return []store.ThemeCount{{Theme: "mate", Count: 5}, {Theme: "fork", Count: 3}}, nil
}

func (f *fakeStore) PuzzleRatingOf(context.Context, string) (store.PuzzleRating, error) {
	return f.rating, nil
}

func (f *fakeStore) RecordPuzzleAttempt(_ context.Context, _, _ string, solved bool, update func(int, int) int) (store.PuzzleAttempt, error) {
	if f.tried {
		return store.PuzzleAttempt{Before: f.rating.Rating, After: f.rating.Rating, Record: f.rating}, nil
	}
	f.recorded = append(f.recorded, solved)
	after := update(f.rating.Rating, f.rating.Attempts)
	rec := store.PuzzleRating{Rating: after, Attempts: f.rating.Attempts + 1, Solved: f.rating.Solved}
	return store.PuzzleAttempt{Counted: true, Before: f.rating.Rating, After: after, Record: rec}, nil
}

func newTest(user string) (*fakeStore, http.Handler) {
	f := &fakeStore{
		puzzle: &store.Puzzle{ID: "00008", FEN: fen00008, Moves: moves00008, Rating: 1900, Popularity: 95, Plays: 100, Themes: "crushing hangingPiece long"},
		rating: store.PuzzleRating{Rating: 1200, Attempts: 30, Solved: 20},
	}
	s := New(Config{Store: f, UserID: func(*http.Request) string { return user }})
	return f, s.Handler()
}

func do(h http.Handler, method, url, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestNext(t *testing.T) {
	f, h := newTest("u1")
	rec := do(h, "GET", "/api/puzzles/next", "")
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var got struct {
		Puzzle struct {
			ID     string
			Rating int
			Themes []string
			Fen    string
			Steps  []map[string]any
		}
		You *Record
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Puzzle.ID != "00008" || got.Puzzle.Fen != fen00008 || len(got.Puzzle.Steps) != 6 || len(got.Puzzle.Themes) != 3 {
		t.Errorf("puzzle: %+v", got.Puzzle)
	}
	if got.You == nil || got.You.Rating != 1200 || got.You.Attempts != 30 {
		t.Errorf("you: %+v", got.You)
	}
	// The first window is centred on the person's rating, untried puzzles only.
	if f.picked[0] != [3]int{1100, 1300, 1} {
		t.Errorf("first request: %v", f.picked[0])
	}
}

func TestNextWidensTheWindow(t *testing.T) {
	f, h := newTest("u1")
	f.noneLow = 300 // nothing within 300 of the rating
	if rec := do(h, "GET", "/api/puzzles/next", ""); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	last := f.picked[len(f.picked)-1]
	if last[1]-last[0] != 800 { // the 400 window is the first that is wide enough
		t.Errorf("windows tried: %v", f.picked)
	}
	if rec := do(h, "GET", "/api/puzzles/next?theme=nosuch", ""); rec.Code != 404 {
		t.Errorf("unknown theme: %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/puzzles/next?theme=a%27%3B--", ""); rec.Code != 400 {
		t.Errorf("a theme with punctuation: %d", rec.Code)
	}
}

func TestNextForNobody(t *testing.T) {
	f, h := newTest("")
	rec := do(h, "GET", "/api/puzzles/next", "")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), `"you"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if f.picked[0][0] != store.StartPuzzleRating-100 {
		t.Errorf("an anonymous visitor gets puzzles around the start rating: %v", f.picked[0])
	}
	if rec := do(h, "POST", "/api/puzzles/00008/result", `{"solved":true}`); rec.Code != 401 {
		t.Errorf("result without signing in: %d", rec.Code)
	}
}

func TestResult(t *testing.T) {
	f, h := newTest("u1")
	rec := do(h, "POST", "/api/puzzles/00008/result", `{"solved":true}`)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var got resultResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	want := NewRating(1200, 30, 1900, true)
	if !got.Counted || got.Before != 1200 || got.After != want || got.You.Rating != want || got.You.Attempts != 31 {
		t.Errorf("%+v, want after=%d", got, want)
	}
	if len(f.recorded) != 1 || !f.recorded[0] {
		t.Errorf("recorded: %v", f.recorded)
	}
	f.tried = true
	rec = do(h, "POST", "/api/puzzles/00008/result", `{"solved":false}`)
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Counted || got.After != 1200 {
		t.Errorf("a repeat try: %+v", got)
	}
	if rec := do(h, "POST", "/api/puzzles/zzzzz/result", `{"solved":true}`); rec.Code != 404 {
		t.Errorf("unknown puzzle: %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/puzzles/00008/result", `not json`); rec.Code != 400 {
		t.Errorf("bad body: %d", rec.Code)
	}
}

func TestByIDAndThemes(t *testing.T) {
	_, h := newTest("u1")
	if rec := do(h, "GET", "/api/puzzles/00008", ""); rec.Code != 200 {
		t.Errorf("by id: %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/puzzles/none", ""); rec.Code != 404 {
		t.Errorf("unknown id: %d", rec.Code)
	}
	rec := do(h, "GET", "/api/puzzles/themes", "")
	var got struct{ Themes []themeJSON }
	json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != 200 || len(got.Themes) != 2 || got.Themes[0].ID != "mate" || got.Themes[0].Count != 5 {
		t.Errorf("themes: %d %s", rec.Code, rec.Body)
	}
}

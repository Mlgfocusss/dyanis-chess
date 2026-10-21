package trainer

import (
	"encoding/json"
	"testing"
	"time"

	"dyanis-chess/backend/internal/store"
)

const (
	startFEN    = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
	rookEnding  = "8/5pk1/6p1/8/8/6P1/5PK1/3R4 w - - 0 1"
	queenRooks  = "3qr1k1/8/8/8/8/8/8/3QR1K1 w - - 0 1" // 28: still a middlegame
	threeRooksB = "r3r1k1/8/8/8/8/8/8/R3R1KB w - - 0 1" // 23: an endgame
)

func TestMaterial(t *testing.T) {
	for fen, want := range map[string]int{
		startFEN: 62, rookEnding: 5, queenRooks: 28, threeRooksB: 23,
		"8/8/8/8/8/8/8/K6k w - - 0 1": 0, "": 0,
	} {
		if got := Material(fen); got != want {
			t.Errorf("Material(%q) = %d, want %d", fen, got, want)
		}
	}
}

func repeat(fen string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fen
	}
	return out
}

func countPhases(ph []Phase) map[Phase]int {
	m := map[Phase]int{}
	for _, p := range ph {
		m[p]++
	}
	return m
}

func TestPhasesOf(t *testing.T) {
	// a quiet game on a full board: the opening takes 20 half-moves, the rest is a middlegame
	got := PhasesOf(repeat(startFEN, 30), make([]bool, 30))
	if c := countPhases(got); c[PhaseOpening] != 20 || c[PhaseMiddlegame] != 10 || c[PhaseEndgame] != 0 {
		t.Errorf("full board: %v", c)
	}
	if got[19] != PhaseOpening || got[20] != PhaseMiddlegame {
		t.Errorf("the 20th half-move is the last of the opening: %v %v", got[19], got[20])
	}

	// the material falls to the limit before half-move 25: from there on an endgame, and it stays one
	fens := append(repeat(startFEN, 24), repeat(threeRooksB, 3)...)
	fens = append(fens, repeat(queenRooks, 3)...) // a pawn queens: the endgame does not turn back
	got = PhasesOf(fens, make([]bool, 30))
	if c := countPhases(got); c[PhaseOpening] != 20 || c[PhaseMiddlegame] != 4 || c[PhaseEndgame] != 6 {
		t.Errorf("endgame: %v", c)
	}
	if got[23] != PhaseMiddlegame || got[24] != PhaseEndgame || got[29] != PhaseEndgame {
		t.Errorf("endgame starts at half-move 25: %v", got)
	}
	if Material(threeRooksB) > EndgameMaterial || Material(queenRooks) <= EndgameMaterial {
		t.Fatalf("the fixtures are on the wrong sides of the limit")
	}

	// theory stretches the opening
	book := make([]bool, 30)
	for i := 0; i < 26; i++ {
		book[i] = true
	}
	got = PhasesOf(repeat(startFEN, 30), book)
	if c := countPhases(got); c[PhaseOpening] != 26 || c[PhaseMiddlegame] != 4 {
		t.Errorf("long theory: %v", c)
	}
	// a book move later in the game does not make an opening of it
	late := make([]bool, 30)
	late[22] = true
	if c := countPhases(PhasesOf(repeat(startFEN, 30), late)); c[PhaseOpening] != 20 {
		t.Errorf("late book move: %v", c)
	}

	// an endgame position in the opening stays the opening until half-move 20
	got = PhasesOf(repeat(rookEnding, 30), make([]bool, 30))
	if c := countPhases(got); c[PhaseOpening] != 20 || c[PhaseEndgame] != 10 {
		t.Errorf("early endgame: %v", c)
	}
	// no positions known: a middlegame
	if c := countPhases(PhasesOf(nil, make([]bool, 25))); c[PhaseMiddlegame] != 5 {
		t.Errorf("no fens: %v", c)
	}
}

func TestSeverity(t *testing.T) {
	for _, c := range []struct {
		class string
		loss  float64
		want  string
	}{
		{"inaccuracy", 6, "inaccuracy"}, {"mistake", 12, "mistake"}, {"blunder", 40, "blunder"},
		{"miss", 7, "inaccuracy"}, {"miss", 10, "mistake"}, {"miss", 25, "blunder"},
		{"best", 0, ""}, {"good", 3, ""}, {"book", 0, ""}, {"great", 0, ""},
	} {
		if got := Severity(c.class, c.loss); got != c.want {
			t.Errorf("Severity(%s, %v) = %q, want %q", c.class, c.loss, got, c.want)
		}
	}
}

// ---------------------------------------------------------------- reading a stored review

type fakeMove struct {
	san, class     string
	loss, accuracy float64
	book           bool
}

// storedReport builds the JSON of a report; move i is White's when i is even. fens[i] is the position
// before move i+1 (the start position where it is not given).
func storedReport(t *testing.T, moves []fakeMove, fens []string, acc [2]float64) []byte {
	t.Helper()
	doc := map[string]any{"accuracy": acc}
	var ms, evals []map[string]any
	for i, m := range moves {
		color := "white"
		if i%2 == 1 {
			color = "black"
		}
		ms = append(ms, map[string]any{"ply": i + 1, "color": color, "san": m.san, "class": m.class,
			"loss": m.loss, "accuracy": m.accuracy, "bestSan": "Qd1", "book": m.book})
	}
	for i := 0; i <= len(moves); i++ {
		f := startFEN
		if i < len(fens) {
			f = fens[i]
		}
		evals = append(evals, map[string]any{"ply": i, "fen": f, "winPct": 50.0})
	}
	doc["moves"], doc["evals"] = ms, evals
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func gameJSON(white, black string) []byte {
	b, _ := json.Marshal(map[string]string{"white": white, "black": black})
	return b
}

func TestReadReview(t *testing.T) {
	moves := make([]fakeMove, 30)
	for i := range moves {
		moves[i] = fakeMove{san: "a3", class: "good", accuracy: 90}
		if i < 6 {
			moves[i] = fakeMove{san: "e4", class: "book", accuracy: 100, book: true}
		}
	}
	moves[8] = fakeMove{san: "Ng5", class: "blunder", loss: 30, accuracy: 20} // ply 9, White's
	moves[11] = fakeMove{san: "h6", class: "mistake", loss: 12, accuracy: 50} // ply 12, Black's
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	row := store.ReportRow{ID: "rev1", Game: gameJSON("Анна", "Борис"), Report: storedReport(t, moves, nil, [2]float64{81.5, 64}), CreatedAt: at}

	white, why := ReadReview(row, "анна ")
	if why != "" || white.Side != "white" || white.Accuracy != 81.5 || white.Opponent != "Борис" || len(white.Moves) != 15 {
		t.Fatalf("white: %+v (%q)", white, why)
	}
	black, why := ReadReview(row, "Борис")
	if why != "" || black.Side != "black" || black.Accuracy != 64 || black.Opponent != "Анна" || len(black.Moves) != 15 {
		t.Fatalf("black: %+v (%q)", black, why)
	}
	// White's moves: plies 1, 3, 5 are theory, ply 9 is the blunder, the rest are in the opening or the middlegame
	if !white.Moves[0].Book || white.Moves[0].Ply != 1 || white.Moves[4].Ply != 9 || white.Moves[4].Class != "blunder" || white.Moves[4].Best != "Qd1" {
		t.Errorf("white moves: %+v", white.Moves[:6])
	}
	if white.Moves[4].Phase != PhaseOpening || white.Moves[10].Phase != PhaseMiddlegame {
		t.Errorf("phases: %v %v", white.Moves[4].Phase, white.Moves[10].Phase)
	}

	// the colour cannot be told: neither name, or both
	if r, why := ReadReview(row, "Кто-то"); r != nil || why != SkipColour {
		t.Errorf("a stranger: %v %q", r, why)
	}
	both := store.ReportRow{ID: "x", Game: gameJSON("Анна", "анна"), Report: row.Report}
	if r, why := ReadReview(both, "Анна"); r != nil || why != SkipColour {
		t.Errorf("same name on both sides: %v %q", r, why)
	}
	if r, why := ReadReview(row, ""); r != nil || why != SkipColour {
		t.Errorf("no name: %v %q", r, why)
	}
	// a broken report
	if r, why := ReadReview(store.ReportRow{ID: "y", Game: row.Game, Report: []byte(`{`)}, "Анна"); r != nil || why != SkipUnreadable {
		t.Errorf("broken: %v %q", r, why)
	}
	if r, why := ReadReview(store.ReportRow{ID: "z", Game: row.Game, Report: []byte(`{}`)}, "Анна"); r != nil || why != SkipUnreadable {
		t.Errorf("empty: %v %q", r, why)
	}
}

// ---------------------------------------------------------------- the picture

// review builds a review of the person with n moves: the phases and the classes come from f.
func review(id string, at time.Time, accuracy float64, moves []OwnMove) *Review {
	return &Review{ID: id, At: at, Side: "white", Accuracy: accuracy, Opponent: "Соперник", Moves: moves}
}

func movesOf(n int, phase Phase, acc float64) []OwnMove {
	out := make([]OwnMove, n)
	for i := range out {
		out[i] = OwnMove{Ply: 2*i + 1, SAN: "a3", Class: "good", Accuracy: acc, Phase: phase}
	}
	return out
}

func TestBuildInsights(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	// no reviews: the shell, not enough
	empty := BuildInsights(nil, 0, 0)
	if empty.Enough || empty.Average != nil || len(empty.Phases) != 3 || empty.Trend != nil || len(empty.Series) != 0 || empty.Need != MinReviews {
		t.Errorf("empty: %+v", empty)
	}

	// two games are still not enough, but they are counted
	two := BuildInsights([]*Review{review("a", t0, 80, movesOf(10, PhaseOpening, 80)), review("b", t0, 60, movesOf(10, PhaseOpening, 80))}, 5, 3)
	if two.Enough || two.Used != 2 || two.Skipped != 3 || two.Reviews != 5 || *two.Average != 70 {
		t.Errorf("two: %+v", two)
	}

	// three games: ten moves of every phase each, one slip of each kind and a book move
	mk := func(id string, day int) *Review {
		var ms []OwnMove
		ms = append(ms, movesOf(10, PhaseOpening, 96)...)
		ms = append(ms, movesOf(10, PhaseMiddlegame, 80)...)
		ms = append(ms, movesOf(10, PhaseEndgame, 60)...)
		ms[1].Class, ms[1].Loss = "inaccuracy", 6
		ms[12].Class, ms[12].Loss = "mistake", 12
		ms[25].Class, ms[25].Loss = "blunder", 30+float64(day)
		ms[25].Accuracy = 10
		ms[0].Book, ms[0].Class, ms[0].Accuracy = true, "book", 100
		return review(id, t0.AddDate(0, 0, day), 70+float64(day), ms)
	}
	revs := []*Review{mk("new", 2), mk("mid", 1), mk("old", 0)} // newest first
	in := BuildInsights(revs, 3, 0)
	if !in.Enough || in.Used != 3 || *in.Average != 71 {
		t.Fatalf("three: %+v", in)
	}
	if len(in.Series) != 3 || in.Series[0].ID != "old" || in.Series[2].ID != "new" {
		t.Errorf("series is oldest first: %+v", in.Series)
	}
	// every phase has 30 moves minus the book ones; the blunder is in the endgame
	op, mid, end := in.Phases[0], in.Phases[1], in.Phases[2]
	if op.ID != "opening" || op.Moves != 27 || op.Accuracy == nil || *op.Accuracy != 96 {
		t.Errorf("opening: %+v", op)
	}
	if *mid.Accuracy != 80 || mid.Moves != 30 {
		t.Errorf("middlegame: %+v", mid)
	}
	if end.Accuracy == nil || *end.Accuracy != 55 || end.Moves != 30 {
		t.Errorf("endgame: %+v", end)
	}
	// slips per 100 moves: 3 of each kind in 90 moves
	if in.Rate.Moves != 90 || in.Rate.Inaccuracies != 3.3 || in.Rate.Mistakes != 3.3 || in.Rate.Blunders != 3.3 {
		t.Errorf("rate: %+v", in.Rate)
	}
	// the dearest moves: the three blunders, the dearest from the newest game
	if len(in.Costly) != CostlyTop || in.Costly[0].Review != "new" || in.Costly[0].Loss != 32 || in.Costly[1].Review != "mid" {
		t.Errorf("costly: %+v", in.Costly)
	}
	c := in.Costly[0]
	if c.Href != "#review/new" || c.Class != "blunder" || c.Opponent != "Соперник" || MoveLabel(c) != "6.a3" {
		t.Errorf("costly[0]: %+v %s", c, MoveLabel(c))
	}
	if in.Trend != nil {
		t.Errorf("three reviews have nothing to compare with: %+v", in.Trend)
	}

	// a few thin phases are not drawn
	thin := BuildInsights([]*Review{review("a", t0, 80, movesOf(4, PhaseEndgame, 50)), review("b", t0, 80, movesOf(3, PhaseEndgame, 50)), review("c", t0, 80, movesOf(0, PhaseEndgame, 50))}, 3, 0)
	if thin.Phases[2].Accuracy != nil || thin.Phases[2].Moves != 7 {
		t.Errorf("thin endgame: %+v", thin.Phases[2])
	}
}

func TestCostlyKeepsOnlyRealMistakes(t *testing.T) {
	ms := movesOf(10, PhaseMiddlegame, 90)
	ms[0].Class, ms[0].Loss = "inaccuracy", 6 // too small to list
	ms[1].Class, ms[1].Loss = "miss", 15      // a miss that costs as much as a mistake
	revs := []*Review{review("a", time.Now(), 80, ms), review("b", time.Now(), 80, nil), review("c", time.Now(), 80, nil)}
	in := BuildInsights(revs, 3, 0)
	if len(in.Costly) != 1 || in.Costly[0].Ply != 3 || in.Costly[0].Class != "mistake" {
		t.Errorf("costly: %+v", in.Costly)
	}
}

func TestTrendOf(t *testing.T) {
	at := time.Now()
	mk := func(accs ...float64) []*Review {
		var out []*Review
		for i, a := range accs {
			out = append(out, review(string(rune('a'+i)), at, a, nil))
		}
		return out
	}
	rep := func(v float64, n int) []float64 {
		out := make([]float64, n)
		for i := range out {
			out[i] = v
		}
		return out
	}
	// 10 recent at 80, 10 before at 76.8: +3,2
	tr := TrendOf(mk(append(rep(80, 10), rep(76.8, 10)...)...))
	if tr == nil || tr.Delta != 3.2 || tr.Text != "+3,2 пункта" || tr.RecentCount != 10 || tr.PreviousCount != 10 {
		t.Errorf("trend: %+v", tr)
	}
	// the groups are the newest 10 and the next 10, never more
	tr = TrendOf(mk(append(append(rep(80, 10), rep(70, 10)...), rep(0, 5)...)...))
	if tr == nil || tr.Delta != 10 || tr.PreviousCount != 10 || tr.Text != "+10 пунктов" {
		t.Errorf("window: %+v", tr)
	}
	// fewer than twenty: the newer half against the older half
	tr = TrendOf(mk(append(rep(70, 4), rep(75, 4)...)...))
	if tr == nil || tr.Text != "−5 пунктов" || tr.RecentCount != 4 || tr.PreviousCount != 4 {
		t.Errorf("halves: %+v", tr)
	}
	if tr := TrendOf(mk(rep(70, 6)...)); tr == nil || tr.RecentCount != 3 || tr.PreviousCount != 3 || tr.Text != "без изменений" {
		t.Errorf("six: %+v", tr)
	}
	// five reviews: the older half has only 2
	if tr := TrendOf(mk(rep(70, 5)...)); tr != nil {
		t.Errorf("older group too small: %+v", tr)
	}
	if TrendOf(nil) != nil || TrendOf(mk(70, 70)) != nil {
		t.Errorf("nothing to compare")
	}
}

func TestPointsText(t *testing.T) {
	for _, c := range []struct {
		d    float64
		want string
	}{
		{3.2, "+3,2 пункта"}, {-1.5, "−1,5 пункта"}, {1, "+1 пункт"}, {2, "+2 пункта"}, {5, "+5 пунктов"}, {11, "+11 пунктов"},
		{21, "+21 пункт"}, {-12, "−12 пунктов"}, {0, "без изменений"}, {0.04, "без изменений"}, {0.6, "+0,6 пункта"},
	} {
		if got := PointsText(c.d); got != c.want {
			t.Errorf("PointsText(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestPlural(t *testing.T) {
	for n, want := range map[int]string{0: "дней", 1: "день", 2: "дня", 4: "дня", 5: "дней", 11: "дней", 12: "дней", 14: "дней", 21: "день", 22: "дня", 111: "дней", 101: "день"} {
		if got := Plural(n, "день", "дня", "дней"); got != want {
			t.Errorf("Plural(%d) = %q, want %q", n, got, want)
		}
	}
	if Num(3.0) != "3" || Num(3.25) != "3,3" && Num(3.25) != "3,2" || Num(61) != "61" || Num(88.4) != "88,4" {
		t.Errorf("Num: %s %s %s %s", Num(3.0), Num(3.25), Num(61), Num(88.4))
	}
}

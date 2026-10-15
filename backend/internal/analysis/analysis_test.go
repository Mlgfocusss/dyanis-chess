package analysis

import (
	"math"
	"strings"
	"testing"

	"dyanis-chess/backend/internal/eco"
	"dyanis-chess/backend/internal/pgn"
	"dyanis-chess/engine"
)

func TestWinPercent(t *testing.T) {
	if got := WinPercent(0, DefaultWinK); math.Abs(got-50) > 1e-9 {
		t.Errorf("WinPercent(0) = %v", got)
	}
	if WinPercent(200, 400) <= WinPercent(100, 400) {
		t.Error("win percent must grow with the score")
	}
	if a, b := WinPercent(150, 400), WinPercent(-150, 400); math.Abs(a+b-100) > 1e-9 {
		t.Errorf("curve not symmetric: %v + %v", a, b)
	}
	if WinPercent(950_000, 400) != 100 || WinPercent(-950_000, 400) != 0 {
		t.Error("mate scores must map to 0/100")
	}
}

func TestMoveAccuracy(t *testing.T) {
	if got := MoveAccuracy(0); got < 99.9 || got > 100 {
		t.Errorf("MoveAccuracy(0) = %v", got)
	}
	prev := 101.0
	for _, loss := range []float64{0, 2, 5, 10, 20, 40, 100} {
		a := MoveAccuracy(loss)
		if a >= prev || a < 0 {
			t.Errorf("MoveAccuracy(%v) = %v, not decreasing/non-negative", loss, a)
		}
		prev = a
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		loss float64
		want Class
	}{
		{0.5, Excellent}, {1.99, Excellent}, {2, Good}, {4.9, Good},
		{5, Inaccuracy}, {9.9, Inaccuracy}, {10, Mistake}, {19.9, Mistake}, {20, Blunder}, {80, Blunder},
	}
	for _, c := range cases {
		if got := DefaultThresholds.classify(c.loss); got != c.want {
			t.Errorf("classify(%v) = %s, want %s", c.loss, got, c.want)
		}
	}
}

// Fool's mate: 2. g4?? allows Qh4#. Depth-limited so the test is
// deterministic and quick.
func TestFoolsMate(t *testing.T) {
	g, err := pgn.ParseOne("1. f3 e5 2. g4 Qh4# 0-1")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Analyze(g, Options{Depth: 4, Workers: 2, NoBook: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Evals) != 5 || len(rep.Moves) != 4 {
		t.Fatalf("got %d evals, %d moves", len(rep.Evals), len(rep.Moves))
	}
	g4 := rep.Moves[2]
	if g4.SAN != "g4" || g4.Class != Blunder {
		t.Errorf("g4 = %+v, want a blunder", g4)
	}
	if rep.Moves[3].SAN != "Qh4#" || rep.Moves[3].Class != Best {
		t.Errorf("Qh4# = %+v, want best", rep.Moves[3])
	}
	if last := rep.Evals[4]; last.Terminal != "checkmate" || last.WinPct != 0 {
		t.Errorf("final position: %+v, want checkmate with White at 0%%", last)
	}
	// After 2...Qh4# is available, the position before it is mate in 1 for Black.
	if e := rep.Evals[3]; e.Mate != -1 {
		t.Errorf("position before Qh4#: Mate = %d, want -1", e.Mate)
	}
	if rep.Moves[3].Color != "black" || rep.Moves[0].Color != "white" {
		t.Error("colors wrong")
	}
	if !(rep.Accuracy[0] < rep.Accuracy[1]) {
		t.Errorf("accuracy white %.1f should be below black %.1f", rep.Accuracy[0], rep.Accuracy[1])
	}
	if len(rep.Key) == 0 || rep.Key[0] != 3 {
		t.Errorf("key moments = %v, want ply 3 first", rep.Key)
	}
	if rep.Counts[0][Blunder] != 1 {
		t.Errorf("white blunders = %d", rep.Counts[0][Blunder])
	}
}

func TestEmptyAndForced(t *testing.T) {
	g, _ := pgn.ParseOne("*")
	rep, err := Analyze(g, Options{Depth: 3})
	if err != nil || len(rep.Evals) != 1 || len(rep.Moves) != 0 {
		t.Fatalf("empty game: %v %+v", err, rep)
	}

	// White is in check from the rook on a1 and has exactly one legal reply.
	g, err = pgn.ParseOne(`[SetUp "1"]
[FEN "7k/8/8/8/8/8/6PP/r5K1 w - - 0 1"]

1. Kf2 *`)
	if err != nil {
		t.Fatal(err)
	}
	rep, err = Analyze(g, Options{Depth: 3})
	if err != nil {
		t.Fatal(err)
	}
	if m := rep.Moves[0]; !m.Forced || m.Class != Best || m.Loss != 0 {
		t.Errorf("forced move: %+v", m)
	}
}

func TestBlackStart(t *testing.T) {
	g, err := pgn.ParseOne(`[SetUp "1"]
[FEN "4k3/8/8/8/8/8/4P3/4K3 b - - 0 1"]

1... Kd7 2. e4 *`)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Analyze(g, Options{Depth: 3})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Moves[0].Color != "black" || rep.Moves[1].Color != "white" {
		t.Errorf("colors: %s %s", rep.Moves[0].Color, rep.Moves[1].Color)
	}
}

func TestAnalyzePositionRepetitionHistory(t *testing.T) {
	b := engine.NewPosition()
	ev := AnalyzePosition(b, []uint64{b.Hash()}, Options{Depth: 3})
	if ev.BestUCI == "" || ev.Terminal != "" {
		t.Errorf("start position: %+v", ev)
	}
}

func TestPlayerAccuracyPunishesDecisiveErrors(t *testing.T) {
	game := func(losses ...float64) float64 {
		var a []float64
		for _, l := range losses {
			a = append(a, MoveAccuracy(l))
		}
		return playerAccuracy(a)
	}
	clean := make([]float64, 50)
	queen := append([]float64{40}, make([]float64, 49)...)
	slips := append([]float64{5, 5, 5, 5, 5}, make([]float64, 45)...)

	if got := game(clean...); got < 99.9 {
		t.Errorf("perfect game = %.1f", got)
	}
	// One queen blunder followed by 49 perfect moves is still a bad game.
	if got := game(queen...); got > 50 {
		t.Errorf("queen blunder + perfect play = %.1f, want a bad score (< 50)", got)
	}
	// Five small slips are a good game.
	if got := game(slips...); got < 90 {
		t.Errorf("five small slips = %.1f, want >= 90", got)
	}
	if game(queen...) >= game(slips...) {
		t.Error("a decisive blunder must score below several small slips")
	}
}

// hashesOf returns the position hashes of a SAN line from the start.
func hashesOf(t *testing.T, moves string) []uint64 {
	t.Helper()
	g, err := pgn.ParseOne(moves + " *")
	if err != nil {
		t.Fatal(err)
	}
	b := engine.NewPosition()
	hs := []uint64{b.Hash()}
	for _, m := range g.Moves {
		b.Play(m.Move)
		hs = append(hs, b.Hash())
	}
	return hs
}

func TestMarkBook(t *testing.T) {
	// Named: 1.e4 (ply 1), 1.e4 e5 2.Nf3 (ply 3), 1.e4 e5 2.Nf3 Nc6 3.Bb5 (ply 5).
	// Plies 2 and 4 are on the lines but unnamed; ply 6 is off every line.
	book, err := eco.Parse("eco\tname\tpgn\n" +
		"B00\tKing's Pawn\t1. e4\n" +
		"C40\tKing's Knight\t1. e4 e5 2. Nf3\n" +
		"C60\tRuy Lopez\t1. e4 e5 2. Nf3 Nc6 3. Bb5\n")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, moves string
		want        int
		last        string
	}{
		{"full line, unnamed plies inside", "1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 4. Ba4", 5, "Ruy Lopez"},
		{"leaves after a gap", "1. e4 e5 2. Nf3 d6 3. d4", 3, "King's Knight"},
		{"leaves right after the first move", "1. e4 c5 2. Nf3 Nc6", 1, "King's Pawn"},
		{"out at once", "1. a3 e5", 0, ""},
		{"no moves", "", 0, ""},
	}
	for _, c := range cases {
		hs := hashesOf(t, c.moves)
		named, n := markBook(book, hs)
		if n != c.want {
			t.Errorf("%s: bookPlies = %d, want %d", c.name, n, c.want)
		}
		last := ""
		if n > 0 {
			var best int
			for ply, o := range named {
				if ply > best {
					best, last = ply, o.Name
				}
			}
		}
		if last != c.last {
			t.Errorf("%s: last name %q, want %q", c.name, last, c.last)
		}
		for ply := range named {
			if ply > n {
				t.Errorf("%s: named ply %d beyond book end %d", c.name, ply, n)
			}
		}
	}
	if _, n := markBook(nil, hashesOf(t, "1. e4")); n != 0 {
		t.Errorf("nil book: bookPlies = %d", n)
	}
}

func TestAnalyzeWithBook(t *testing.T) {
	// Ruy Lopez, then 6...Bxf2?? (White's knight is not even attacked: a
	// plain blunder, out of theory since the reference ends before it).
	g, err := pgn.ParseOne("1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 4. Ba4 Nf6 5. O-O Be7 6. Re1 b5 7. Bb3 d6 8. c3 O-O 9. h3 Qd7?? 10. Bxf7+ *")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Analyze(g, Options{Depth: 4, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Opening == nil || !strings.Contains(rep.Opening.Name, "Ruy Lopez") {
		t.Fatalf("opening = %+v", rep.Opening)
	}
	if rep.BookPlies < 10 || rep.BookPlies >= len(rep.Moves)-1 {
		t.Errorf("bookPlies = %d of %d", rep.BookPlies, len(rep.Moves))
	}
	for i, m := range rep.Moves {
		inBook := i < rep.BookPlies
		if m.Book != inBook || (m.Class == Book) != inBook {
			t.Errorf("ply %d %s: book=%v class=%s, want book=%v", m.Ply, m.SAN, m.Book, m.Class, inBook)
		}
	}
	if rep.Counts[0][Book]+rep.Counts[1][Book] != rep.BookPlies {
		t.Errorf("book counts %v vs %d plies", rep.Counts, rep.BookPlies)
	}
	// Theory is not scored: with NoBook the opening moves count too, so
	// the two reports must differ in nothing but the labelling and accuracy.
	plain, err := Analyze(g, Options{Depth: 4, Workers: 2, NoBook: true})
	if err != nil {
		t.Fatal(err)
	}
	if plain.Opening != nil || plain.BookPlies != 0 || plain.Moves[0].Class == Book {
		t.Errorf("NoBook still marked theory: %+v", plain.Opening)
	}
	for i := range rep.Evals {
		if rep.Evals[i].FEN != plain.Evals[i].FEN || rep.Evals[i].Score != plain.Evals[i].Score {
			t.Errorf("eval %d differs between book and no-book runs", i)
		}
	}
}

func TestAllMovesInBookScoreFull(t *testing.T) {
	g, _ := pgn.ParseOne("1. e4 e5 *")
	rep, err := Analyze(g, Options{Depth: 3})
	if err != nil {
		t.Fatal(err)
	}
	if rep.BookPlies != 2 || rep.Accuracy != [2]float64{100, 100} {
		t.Errorf("bookPlies %d accuracy %v", rep.BookPlies, rep.Accuracy)
	}
}

func TestCustomStartHasNoBook(t *testing.T) {
	g, _ := pgn.ParseOne(`[SetUp "1"]
[FEN "4k3/8/8/8/8/8/4P3/4K3 w - - 0 1"]

1. e4 *`)
	rep, err := Analyze(g, Options{Depth: 3})
	if err != nil || rep.BookPlies != 0 || rep.Opening != nil {
		t.Errorf("err %v bookPlies %d opening %v", err, rep.BookPlies, rep.Opening)
	}
}

package analysis

import (
	"math"
	"testing"

	"dyanis-chess/internal/board"
	"dyanis-chess/internal/pgn"
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
	rep, err := Analyze(g, Options{Depth: 4, Workers: 2})
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
	b := board.NewInitialBoard()
	ev := AnalyzePosition(b, []uint64{b.Hash()}, Options{Depth: 3})
	if ev.BestUCI == "" || ev.Terminal != "" {
		t.Errorf("start position: %+v", ev)
	}
}

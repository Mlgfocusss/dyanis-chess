package search

import (
	"math/rand"
	"testing"

	"dyanis-chess/engine/internal/board"
	"dyanis-chess/engine/internal/movegen"
)

// TestSEEFastPathMatchesFull: the pre-check in SEE must never change the
// result — compared against seeFull on every capture of many random games.
func TestSEEFastPathMatchesFull(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	starts := []string{
		board.StartFEN,
		"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1",
		"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1",
	}
	checked, fast := 0, 0
	for g := 0; g < 300; g++ {
		b, err := board.FromFEN(starts[g%len(starts)])
		if err != nil {
			t.Fatal(err)
		}
		for ply := 0; ply < 120; ply++ {
			moves := movegen.GenerateLegalMoves(b)
			if len(moves) == 0 {
				break
			}
			for _, m := range moves {
				if !m.IsCapture() {
					continue
				}
				checked++
				if got, want := SEE(b, m), seeFull(b, m); got != want {
					t.Fatalf("SEE %d != seeFull %d for %v in %s", got, want, m, b.ToFEN())
				}
				if seeFull(b, m) == seeValue(b.PieceAt(m.To).Type()) {
					fast++
				}
			}
			b.MakeMove(moves[rng.Intn(len(moves))])
		}
	}
	t.Logf("captures checked: %d (result == victim value in %d)", checked, fast)
	if checked < 1000 {
		t.Fatalf("too few captures exercised: %d", checked)
	}
}

// TestSEEQuiet: a quiet move onto an attacked square is judged by the
// exchange the opponent can start there; a safe square scores 0.
func TestSEEQuiet(t *testing.T) {
	cases := []struct {
		fen, move string
		want      int
	}{
		// The e5 pawn attacks d4: the undefended queen is lost.
		{"4k3/8/8/4p3/8/8/8/3QK3 w - - 0 1", "d1d4", -seeValue(board.Queen)},
		{"4k3/8/8/4p3/8/8/8/3QK3 w - - 0 1", "d1d2", 0},
		// Defended by the c3 pawn: knight for pawn.
		{"4k3/8/8/4p3/8/2P5/2N5/4K3 w - - 0 1", "c2d4", seeValue(board.Pawn) - seeValue(board.Knight)},
	}
	for _, c := range cases {
		b, err := board.FromFEN(c.fen)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, m := range movegen.GenerateLegalMoves(b) {
			if m.String() == c.move {
				found = true
				if got := seeQuiet(b, m); got != c.want {
					t.Errorf("%s in %s: seeQuiet %d, want %d", c.move, c.fen, got, c.want)
				}
			}
		}
		if !found {
			t.Errorf("move %s not legal in %s", c.move, c.fen)
		}
	}
}

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

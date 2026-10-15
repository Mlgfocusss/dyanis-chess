package bots

import (
	"context"
	"math/rand"
	"testing"

	"dyanis-chess/engine"
)

// priorOf is the opening bonus of a move written in UCI, in the position after the given SAN moves.
func priorOf(t *testing.T, fen, uci string) float64 {
	t.Helper()
	pos, err := engine.ParseFEN(fen)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range pos.LegalMoves() {
		if m.String() == uci {
			return openingPrior(pos, m)
		}
	}
	t.Fatalf("%s is not legal in %s", uci, fen)
	return 0
}

func TestOpeningPriorPrefersTheCentre(t *testing.T) {
	const start = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
	good := map[string]float64{"e2e4": 50, "d2d4": 50, "g1f3": 35, "b1c3": 35, "c2c4": 25}
	for uci, want := range good {
		if got := priorOf(t, start, uci); got != want {
			t.Errorf("%s: %v, want %v", uci, got, want)
		}
	}
	for _, uci := range []string{"a2a3", "a2a4", "h2h3", "h2h4", "b1a3", "g1h3", "f2f3", "g2g4"} {
		if got := priorOf(t, start, uci); got >= 0 {
			t.Errorf("%s: %v, want a penalty", uci, got)
		}
	}
	// the same habits for Black, counted from Black's side
	afterE4 := "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1"
	if got := priorOf(t, afterE4, "e7e5"); got != 50 {
		t.Errorf("e5: %v", got)
	}
	if got := priorOf(t, afterE4, "a7a6"); got >= 0 {
		t.Errorf("a6: %v", got)
	}
	// castling is welcome, the king walk is not
	castle := "r1bqk2r/pppp1ppp/2n2n2/2b1p3/2B1P3/5N2/PPPP1PPP/RNBQK2R w KQkq - 4 4"
	if got := priorOf(t, castle, "e1g1"); got != 50 {
		t.Errorf("O-O: %v", got)
	}
	if got := priorOf(t, castle, "e1e2"); got >= 0 {
		t.Errorf("Ke2: %v", got)
	}
}

func TestOpeningPriorStopsAfterTheOpening(t *testing.T) {
	// the same a-pawn move, at move 15, is no longer judged by habit
	late := "r4rk1/pp3ppp/2n1bn2/3p4/3P4/2N1BN2/PP3PPP/R4RK1 w - - 0 15"
	if got := priorOf(t, late, "a2a4"); got != 0 {
		t.Errorf("a4 at move 15: %v", got)
	}
}

// Dionis (1800) used to play a3 or h4 as often as e4. Now he takes the centre.
func TestAnEighteenHundredBotTakesTheCentre(t *testing.T) {
	if testing.Short() {
		t.Skip("plays many searches")
	}
	b, _ := Get("dionis")
	rng := rand.New(rand.NewSource(7))
	pos := engine.NewPosition()
	rim, centre := 0, 0
	const games = 60
	for i := 0; i < games; i++ {
		mv, err := b.Choose(context.Background(), pos, []uint64{pos.Hash()}, 0, 0, rng)
		if err != nil {
			t.Fatal(err)
		}
		switch mv.String() {
		case "a2a3", "a2a4", "h2h3", "h2h4", "b1a3", "g1h3", "g2g4", "b2b4", "f2f3", "f2f4":
			rim++
		case "e2e4", "d2d4", "g1f3", "b1c3", "c2c4", "e2e3", "d2d3", "c2c3", "g2g3":
			centre++
		}
	}
	if rim > games/8 || centre < games*85/100 {
		t.Errorf("of %d first moves: %d sound, %d on the edge", games, centre, rim)
	}
}

package movegen

import (
	"math/rand"
	"testing"

	"dyanis-chess/internal/board"
)

// HasLegalMove must agree with "GenerateLegalMoves is non-empty" and
// GameStatusFromAny with GameStatusFromMoves, on every position of many
// random games and on known mate/stalemate positions.
func TestHasLegalMoveMatchesFullGeneration(t *testing.T) {
	check := func(b *board.Board, where string) {
		legal := GenerateLegalMoves(b)
		if got := HasLegalMove(b); got != (len(legal) > 0) {
			t.Fatalf("%s: HasLegalMove=%v but %d legal moves (fen %s)", where, got, len(legal), b.ToFEN())
		}
		if a, c := GameStatusFromAny(b, len(legal) > 0), GameStatusFromMoves(b, legal); a != c {
			t.Fatalf("%s: status %v vs %v", where, a, c)
		}
	}

	for _, fen := range []string{
		"7k/5Q2/6K1/8/8/8/8/8 b - - 0 1",                                       // stalemate
		"R6k/8/6K1/8/8/8/8/8 b - - 0 1",                                        // back-rank mate
		"rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 1 3",        // fool's mate
		"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1", // kiwipete
		"8/8/8/8/8/8/8/K1k5 w - - 0 1",                                         // bare kings
	} {
		b, err := board.FromFEN(fen)
		if err != nil {
			t.Fatal(err)
		}
		check(b, "known "+fen)
	}

	rng := rand.New(rand.NewSource(7))
	for game := 0; game < 300; game++ {
		b := board.NewInitialBoard()
		for ply := 0; ply < 160; ply++ {
			legal := GenerateLegalMoves(b)
			check(b, "random game")
			if len(legal) == 0 {
				break
			}
			b.MakeMove(legal[rng.Intn(len(legal))])
		}
	}
}

package search

import (
	"math/rand"
	"testing"
	"time"

	"dyanis-chess/engine/internal/board"
	"dyanis-chess/engine/internal/nnue"
)

func randomTestNetwork(t *testing.T, hidden int, seed int64) *nnue.Network {
	t.Helper()
	n, err := nnue.NewBucketed(hidden, 8, nnue.SCReLU, 255, 64, 400)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(seed))
	for _, s := range [][]int16{n.W1, n.B1, n.W2} {
		for i := range s {
			s[i] = int16(rng.Intn(129) - 64)
		}
	}
	for i := range n.B2 {
		n.B2[i] = int32(rng.Intn(20001) - 10000)
	}
	return n
}

// withNetwork runs f with n as the search's network and nnueVerify on:
// every network evaluation is checked against an accumulator computed from
// scratch, so a move the search makes without telling the accumulator
// stack panics.
func withNetwork(t *testing.T, n *nnue.Network, f func()) {
	t.Helper()
	prevNet, prevVerify := network, nnueVerify
	SetNetwork(n)
	nnueVerify = true
	defer func() { SetNetwork(prevNet); nnueVerify = prevVerify }()
	f()
}

var nnueSearchFENs = []string{
	board.StartFEN,
	"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1", // castling, ep, promotions
	"n1n5/PPPk4/8/8/8/8/4Kppp/5N1N b - - 0 1",                              // promotions with captures
	"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1",                            // endgame, null-move guards
	"r1bq1rk1/pp2bppp/2n1pn2/3p4/2PP4/2N1PN2/PP1QBPPP/R3KB1R w KQ - 0 9",   // quiet middlegame (null moves)
	"6k1/5ppp/8/8/8/8/5PPP/3R2K1 w - - 0 1",                                // mate in 1
}

func TestSearchWithNetworkKeepsAccumulatorInSync(t *testing.T) {
	withNetwork(t, randomTestNetwork(t, 32, 1), func() {
		for _, fen := range nnueSearchFENs {
			b, err := board.FromFEN(fen)
			if err != nil {
				t.Fatal(err)
			}
			before := b.ToFEN()
			var info SearchInfo
			if _, err := BestMoveTimedInfoTT(b, 6, time.Hour, NewTranspositionTable(), func(i SearchInfo) { info = i }); err != nil {
				t.Fatal(err)
			}
			if _, err := BestMoveTimedMTShared(b, 5, time.Hour, 3, NewTranspositionTable(), nil); err != nil {
				t.Fatal(err)
			}
			if _, err := BestMove(b, 4); err != nil {
				t.Fatal(err)
			}
			if b.ToFEN() != before {
				t.Fatalf("search changed the board: %s -> %s", before, b.ToFEN())
			}
			t.Logf("%s: depth %d, %d nodes", fen, info.Depth, info.Nodes)
		}
	})
}

// The network really is what the search evaluates with: a different
// network gives a different root score, and switching back to nil
// restores the hand-written eval's exact search.
func TestSetNetworkSwitchesEvaluation(t *testing.T) {
	b, _ := board.FromFEN(nnueSearchFENs[4])
	search := func() SearchInfo {
		var info SearchInfo
		if _, err := BestMoveTimedInfoTT(b, 5, time.Hour, NewTranspositionTable(), func(i SearchInfo) { info = i }); err != nil {
			t.Fatal(err)
		}
		return info
	}
	hand := search()
	var a, c SearchInfo
	withNetwork(t, randomTestNetwork(t, 32, 2), func() { a = search() })
	withNetwork(t, randomTestNetwork(t, 32, 3), func() { c = search() })
	if a.Score == hand.Score && c.Score == hand.Score {
		t.Fatalf("networks give the hand eval's score %d", hand.Score)
	}
	if a.Score == c.Score && a.Nodes == c.Nodes {
		t.Fatal("two different networks searched identically")
	}
	if again := search(); again.Score != hand.Score || again.Nodes != hand.Nodes {
		t.Fatalf("after SetNetwork(nil): score %d nodes %d, want %d %d", again.Score, again.Nodes, hand.Score, hand.Nodes)
	}

	// A table reused across a network change must not serve static evals
	// cached by the other evaluator. (Its hash table of search results is
	// another matter: the UCI session replaces it on an EvalFile change.)
	tt := NewTranspositionTable()
	tt.nnueReset(b)
	hand = SearchInfo{Score: tt.evaluate(b, 0)}
	n := randomTestNetwork(t, 32, 2)
	acc := n.NewAccumulator()
	n.Refresh(acc, b)
	want := n.Evaluate(acc, b.SideToMove, b.Occupied().Count())
	if want == hand.Score {
		t.Fatal("test needs a network that disagrees with the hand eval here")
	}
	withNetwork(t, n, func() {
		tt.nnueReset(b)
		if got := tt.evaluate(b, 0); got != want {
			t.Fatalf("after switching to a network: evaluate %d, network says %d (hand eval %d)", got, want, hand.Score)
		}
	})
	tt.nnueReset(b)
	if got := tt.evaluate(b, 0); got != hand.Score {
		t.Fatalf("after switching back: evaluate %d, hand eval %d", got, hand.Score)
	}
}

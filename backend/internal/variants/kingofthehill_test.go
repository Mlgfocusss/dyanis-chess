package variants

import (
	"strings"
	"testing"
)

// hillAt makes a King of the Hill game standing in the position of the FEN.
func hillAt(t *testing.T, fen string) *Game {
	t.Helper()
	g, err := NewGame(kothRules{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	g.Edit(func(p *Position) { *p = *mustFEN(fen) })
	return g
}

func TestHillPerft(t *testing.T) {
	// no king reaches the centre in four plies: the numbers of chess
	want := []uint64{20, 400, 8902, 197281}
	depth := len(want)
	if testing.Short() {
		depth = 3
	}
	p := NewPosition()
	for d := 1; d <= depth; d++ {
		if got := Perft(kothRules{}, p, d); got != want[d-1] {
			t.Errorf("perft(%d) = %d, want %d", d, got, want[d-1])
		}
	}
}

// The white king walks to each of the four hill squares while Black shuffles.
func TestHillWhiteKingWalksIn(t *testing.T) {
	for _, target := range []string{"d4", "e4", "d5", "e5"} {
		g := hillAt(t, "k7/7p/8/8/8/8/7P/4K3 w - - 0 1")
		// e1 -> e2 -> e3 -> target (e4 / d4 straight), d5 and e5 take one more step
		path := map[string][]string{
			"e4": {"e1e2", "e2e3", "e3e4"},
			"d4": {"e1e2", "e2e3", "e3d4"},
			"d5": {"e1e2", "e2e3", "e3d3", "d3c4", "c4d5"},
			"e5": {"e1e2", "e2e3", "e3f4", "f4f5", "f5e5"},
		}[target]
		black := []string{"a8a7", "a7a8", "a8a7", "a7a8", "a8a7"}
		for i, a := range path {
			if _, _, over := g.Over(); over {
				t.Fatalf("%s: over before %s", target, a)
			}
			if _, err := g.Apply(a); err != nil {
				t.Fatalf("%s: %s: %v", target, a, err)
			}
			if i < len(path)-1 {
				if _, err := g.Apply(black[i]); err != nil {
					t.Fatalf("%s: %s: %v", target, black[i], err)
				}
			}
		}
		res, reason, over := g.Over()
		if !over || res != ResultWhite || reason != ReasonKingOfTheHill {
			t.Errorf("%s: %q %q %v", target, res, reason, over)
		}
		if g.Legal() != nil {
			t.Errorf("%s: moves after the end", target)
		}
		if _, err := g.Apply("a8a7"); err != ErrOver {
			t.Errorf("%s: an action after the end: %v", target, err)
		}
	}
}

func TestHillBlackKingWinsToo(t *testing.T) {
	g := hillAt(t, "4k3/7p/8/8/8/8/7P/K7 b - - 0 1")
	playActions(t, g, "e8e7 a1a2 e7e6 a2a1 e6e5")
	res, reason, over := g.Over()
	if !over || res != ResultBlack || reason != ReasonKingOfTheHill {
		t.Errorf("black king on e5: %q %q %v", res, reason, over)
	}
}

func TestHillStillHasCheckmate(t *testing.T) {
	st, err := Replay("kingofthehill", Options{}, strings.Fields("f2f3 e7e5 g2g4 d8h4"))
	if err != nil {
		t.Fatal(err)
	}
	if res, reason, over := st.Over(); !over || res != ResultBlack || reason != ReasonCheckmate {
		t.Errorf("fool's mate: %q %q %v", res, reason, over)
	}
}

func TestHillKingCannotStepIntoAttack(t *testing.T) {
	// a black pawn on c5 attacks d4: the king may not go there (the move is into check) and nothing is won
	g := hillAt(t, "k7/8/8/2p5/8/4K3/8/8 w - - 0 1")
	legal := g.Legal()
	if contains(legal, "e3d4") {
		t.Errorf("the king steps into the pawn's fire: %v", legal)
	}
	if !contains(legal, "e3e4") {
		t.Errorf("e4 is free: %v", legal)
	}
}

func TestHillRegisteredAndReplayed(t *testing.T) {
	v, ok := Get("kingofthehill")
	if !ok || v.Info().Name != "Царь горы" || v.Info().Hidden {
		t.Fatalf("not registered: %+v", v)
	}
	st, err := Replay("kingofthehill", Options{}, strings.Fields("e2e4 a7a6 e1e2 a6a5 e2e3 a5a4 e3d4"))
	if err != nil {
		t.Fatal(err)
	}
	if res, reason, over := st.Over(); !over || res != ResultWhite || reason != ReasonKingOfTheHill {
		t.Errorf("king on d4: %q %q %v", res, reason, over)
	}
	if _, ok := st.(Finisher); !ok {
		t.Error("a Finisher is needed for the server")
	}
}

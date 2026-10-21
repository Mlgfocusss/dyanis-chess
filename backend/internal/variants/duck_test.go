package variants

import (
	"math/rand"
	"strings"
	"testing"
)

func duckOf(t *testing.T) *duckState {
	t.Helper()
	st, err := Get0("duck", Options{})
	if err != nil {
		t.Fatal(err)
	}
	return st.(*duckState)
}

func play(t *testing.T, st State, actions ...string) {
	t.Helper()
	for _, a := range actions {
		if _, err := st.Apply(a); err != nil {
			t.Fatalf("%s: %v (legal %v)", a, err, st.Legal())
		}
	}
}

func TestDuckTurnIsMoveThenDuck(t *testing.T) {
	d := duckOf(t)
	if len(d.Legal()) != 20 || d.View(White).Extra.(duckExtra).Duck != "" {
		t.Fatalf("start: %d moves, extra %+v", len(d.Legal()), d.View(White).Extra)
	}
	if note, err := d.Apply("e2e4"); err != nil || note != "e4" {
		t.Fatalf("e2e4: %q %v", note, err)
	}
	// the duck step belongs to White still: the clock must not change hands
	if d.Turn() != White {
		t.Fatalf("turn after the piece: %v", d.Turn())
	}
	v := d.View(White)
	if v.Turn != White || len(v.Legal) != 32 || v.Extra.(duckExtra).Step != "duck" {
		t.Fatalf("duck step view: turn %v, %d actions, %+v", v.Turn, len(v.Legal), v.Extra)
	}
	if bv := d.View(Black); len(bv.Legal) != 0 || bv.Turn != White {
		t.Errorf("Black is offered %d actions in White's duck step", len(bv.Legal))
	}
	// a piece move is not a duck, the duck does not go onto a piece or off the board
	for _, bad := range []string{"e7e5", "@e4", "@e7", "@e9", "x", ""} {
		if _, err := d.Apply(bad); err == nil {
			t.Errorf("%q accepted in the duck step", bad)
		}
	}
	if d.Turn() != White || !d.mid {
		t.Error("a refused action changed the step")
	}
}

func TestDuckBlocksAndMoves(t *testing.T) {
	d := duckOf(t)
	play(t, d, "e2e4", "@e5")
	if d.Turn() != Black {
		t.Fatalf("turn after the duck: %v", d.Turn())
	}
	if d.View(NoSide).Board["e5"] != "xD" {
		t.Fatalf("the duck is not on e5: %v", d.View(NoSide).Board)
	}
	// a pawn cannot jump onto the duck
	if contains(d.Legal(), "e7e5") || !contains(d.Legal(), "e7e6") {
		t.Errorf("e7 pawn with the duck on e5: %v", d.Legal())
	}
	play(t, d, "d7d6")
	// the duck must go to another square than e5: 31 free squares of 32, none of them e5
	if contains(d.Legal(), "@e5") || len(d.Legal()) != 31 {
		t.Errorf("duck squares: %d, e5 %v", len(d.Legal()), contains(d.Legal(), "@e5"))
	}
	play(t, d, "@e3")
	if b := d.View(NoSide).Board; b["e3"] != "xD" || b["e5"] != "" {
		t.Errorf("the duck did not move: %v", b)
	}
}

func TestDuckStopsLines(t *testing.T) {
	d := duckOf(t)
	d.Game.Edit(func(p *Position) { *p = *mustFEN("r3k3/8/8/8/8/8/8/4K3 w - - 0 1") })
	play(t, d, "e1e2", "@a5") // the duck on a5 stands in the file of Black's rook
	moves := d.Legal()
	if contains(moves, "a8a5") || contains(moves, "a8a4") || !contains(moves, "a8a6") || !contains(moves, "a8a7") {
		t.Errorf("the rook must stop above the duck: %v", moves)
	}
	if contains(moves, "a8a5") {
		t.Error("the duck was taken")
	}
	play(t, d, "e8e7", "@h5") // the duck leaves the file
	play(t, d, "e2e3", "@h3")
	if !contains(d.Legal(), "a8a1") {
		t.Errorf("a-file must be open again: %v", d.Legal())
	}
}

func TestDuckKingCaptureEndsTheGame(t *testing.T) {
	d := duckOf(t)
	d.Game.Edit(func(p *Position) { *p = *mustFEN("4k3/p7/8/8/8/8/8/4K2R w - - 0 1") })
	play(t, d, "h1h8", "@a1")
	// Black does not answer the "check" that does not exist
	if res, _, over := d.Over(); over {
		t.Fatalf("over after a rook on h8: %s", res)
	}
	play(t, d, "a7a6", "@h2")
	play(t, d, "h8e8")
	res, reason, over := d.Over()
	if !over || res != ResultWhite || reason != ReasonKingCaptured {
		t.Fatalf("king taken: %q %q %v", res, reason, over)
	}
	if d.Legal() != nil || d.View(White).Legal != nil {
		t.Error("actions after the end")
	}
	if _, err := d.Apply("@a3"); err != ErrOver {
		t.Errorf("duck after the end: %v", err)
	}
}

func TestDuckKingMayWalkIntoAttack(t *testing.T) {
	d := duckOf(t)
	d.Game.Edit(func(p *Position) { *p = *mustFEN("4k3/8/8/8/8/8/8/r3K3 w - - 0 1") })
	// the white king stands on e1 attacked by the rook a1: no check, so it may stay or step along the rank
	if !contains(d.Legal(), "e1d1") || !contains(d.Legal(), "e1f1") || !contains(d.Legal(), "e1e2") {
		t.Errorf("a king under attack may go anywhere: %v", d.Legal())
	}
	if len(d.View(White).Check) != 0 {
		t.Errorf("check shown: %v", d.View(White).Check)
	}
}

func TestDuckCastlingIgnoresAttacksButNotTheDuck(t *testing.T) {
	d := duckOf(t)
	// the rook a8 attacks a1 and the king's squares? Black rook on f8 attacks f1: the king passes f1
	d.Game.Edit(func(p *Position) { *p = *mustFEN("4kr2/8/8/8/8/8/8/R3K2R w KQ - 0 1") })
	legal := d.Legal()
	if !contains(legal, "e1g1") || !contains(legal, "e1c1") {
		t.Fatalf("castling across an attacked square must be allowed: %v", legal)
	}
	// the duck on d1 forbids the queen-side castling
	d.Game.Edit(func(p *Position) { p.B[MakeSq(3, 0)] = Piece{Kind: Duck} })
	legal = d.Legal()
	if contains(legal, "e1c1") || !contains(legal, "e1g1") {
		t.Errorf("the duck on d1: %v", legal)
	}
	// the duck on f1 forbids the king side
	d.Game.Edit(func(p *Position) { p.B[MakeSq(3, 0)] = Piece{}; p.B[MakeSq(5, 0)] = Piece{Kind: Duck} })
	if contains(d.Legal(), "e1g1") {
		t.Errorf("castling through the duck: %v", d.Legal())
	}
}

func TestDuckNoMovesWins(t *testing.T) {
	d := duckOf(t)
	// Black is locked in its corner but for g6; the duck on g6 leaves it no move, and the side that cannot move wins
	d.Game.Edit(func(p *Position) { *p = *mustFEN("5brk/4p1pp/4P2p/7P/8/8/8/K7 w - - 0 1") })
	play(t, d, "a1a2")
	if d.Turn() != White || d.View(White).Extra.(duckExtra).Step != "duck" {
		t.Fatal("duck step expected")
	}
	if _, err := d.Apply("@g6"); err != nil {
		t.Fatal(err)
	}
	res, reason, over := d.Over()
	if !over || res != ResultBlack || reason != ReasonNoMoves {
		t.Errorf("Black cannot move and wins: %q %q %v", res, reason, over)
	}
}

func TestDuckFlagLoses(t *testing.T) {
	d := duckOf(t)
	d.Game.Edit(func(p *Position) { *p = *mustFEN("4k3/8/8/8/8/8/8/4K3 w - - 0 1") })
	if got := d.Timeout(White); got != ResultBlack {
		t.Errorf("white flag against a bare king: %q", got)
	}
}

func TestDuckNotation(t *testing.T) {
	d := duckOf(t)
	note1, _ := d.Apply("g1f3")
	note2, _ := d.Apply("@e5")
	if note1 != "Nf3" || note2 != "@e5" {
		t.Errorf("notes %q %q", note1, note2)
	}
}

// Random games with random ducks: the turn alternates in pairs, there is one duck, the position
// is restored exactly by replaying the actions.
func TestDuckRandomGamesAndReplay(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	ended := map[string]int{}
	for g := 0; g < 120; g++ {
		o := Options{Seed: int64(g)}
		st, _ := Get0("duck", o)
		var actions []string
		for i := 0; i < 400; i++ {
			if _, reason, over := st.Over(); over {
				ended[reason]++
				break
			}
			legal := st.Legal()
			if len(legal) == 0 {
				t.Fatalf("game %d: not over, yet no actions", g)
			}
			who := st.Turn()
			a := legal[rng.Intn(len(legal))]
			// take a king when one is there
			for _, l := range legal {
				d := st.(*duckState)
				if !d.mid {
					if to, ok := ParseSq(l[2:4]); ok && d.pos.B[to].Kind == King {
						a = l
					}
				}
			}
			if _, err := st.Apply(a); err != nil {
				t.Fatalf("game %d: %s: %v", g, a, err)
			}
			actions = append(actions, a)
			d := st.(*duckState)
			ducks := 0
			for _, pc := range d.pos.B {
				if pc.Kind == Duck {
					ducks++
				}
			}
			if ducks > 1 || (len(actions) > 1 && ducks == 0) {
				t.Fatalf("game %d: %d ducks after %v", g, ducks, actions)
			}
			if strings.HasPrefix(a, "@") {
				if d.Turn() == who && !isOver(st) {
					t.Fatalf("game %d: the turn did not pass after the duck", g)
				}
			} else if !isOver(st) && d.Turn() != who {
				t.Fatalf("game %d: the turn passed before the duck", g)
			}
		}
		again, err := Replay("duck", o, actions)
		if err != nil {
			t.Fatal(err)
		}
		a, b := st.(*duckState), again.(*duckState)
		if a.pos.FEN() != b.pos.FEN() || a.mid != b.mid {
			t.Fatalf("game %d: replay differs", g)
		}
		ra, rr, oa := st.Over()
		rb, rs, ob := again.Over()
		if ra != rb || rr != rs || oa != ob {
			t.Fatalf("game %d: the result differs after replay", g)
		}
	}
	t.Logf("random duck games ended: %v", ended)
	if ended[ReasonKingCaptured] == 0 {
		t.Error("no random game ended with a king taken")
	}
}

func isOver(st State) bool { _, _, o := st.Over(); return o }

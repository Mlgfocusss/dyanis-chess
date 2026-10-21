package variants

import (
	"math/rand"
	"strings"
	"testing"
)

func newBug(t *testing.T) *Bug {
	t.Helper()
	b, err := NewBug(Options{})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func bugPlay(t *testing.T, b *Bug, board int, actions string) {
	t.Helper()
	for _, a := range strings.Fields(actions) {
		if _, err := b.Apply(board, a); err != nil {
			t.Fatalf("board %d %s: %v", board, a, err)
		}
	}
}

func TestBughouseSeats(t *testing.T) {
	want := map[int]struct {
		board   int
		side    Side
		partner int
		team    int
	}{
		0: {0, White, 3, 0},
		1: {0, Black, 2, 1},
		2: {1, White, 1, 1},
		3: {1, Black, 0, 0},
	}
	for seat, w := range want {
		board, side := BugPlace(seat)
		if board != w.board || side != w.side || BugPartner(seat) != w.partner || BugTeam(seat) != w.team {
			t.Errorf("seat %d: board %d side %v partner %d team %d", seat, board, side, BugPartner(seat), BugTeam(seat))
		}
		if BugSeat(board, side) != seat {
			t.Errorf("BugSeat does not invert BugPlace for %d", seat)
		}
		if BugTeam(BugPartner(seat)) != BugTeam(seat) {
			t.Errorf("partners of seat %d are not in one team", seat)
		}
	}
	if BugResultFor(0) != ResultWhite || BugResultFor(1) != ResultBlack {
		t.Error("results of the teams")
	}
}

func TestBughouseCaptureGoesToThePartner(t *testing.T) {
	b := newBug(t)
	bugPlay(t, b, 0, "e2e4 d7d5 e4d5") // White of board 0 (seat 0) takes a pawn
	// the partner of seat 0 is Black of board 1 (seat 3)
	if got := b.Position(1).Pocket[Black][Pawn]; got != 1 {
		t.Fatalf("Black of board 1 should hold a pawn, holds %v", b.Position(1).Pocket[Black])
	}
	if b.Position(0).Pocket[White][Pawn] != 0 || b.Position(0).Pocket[Black][Pawn] != 0 {
		t.Error("the pocket of the board where the piece was taken is untouched")
	}
	// Black of board 1 cannot drop yet (White of board 1 is to move), then can
	if len(dropActions(b.Legal(1))) != 0 {
		t.Error("White of board 1 holds nothing")
	}
	bugPlay(t, b, 1, "e2e4")
	view := b.View(1, Black)
	if len(dropActions(view.Legal)) == 0 || !contains(view.Legal, "P@e5") {
		t.Fatalf("Black of board 1 is to move and holds a pawn: %v", view.Legal)
	}
	extra := view.Extra.(PocketExtra)
	if extra.Black["P"] != 1 || len(extra.White) != 0 {
		t.Errorf("pockets of board 1 in the view: %+v", extra)
	}
	note, err := b.Apply(1, "P@e5")
	if err != nil || note != "P@e5" {
		t.Fatalf("drop: %q %v", note, err)
	}
	if b.Position(1).Pocket[Black][Pawn] != 0 || b.Position(1).B[MakeSq(4, 4)] != (Piece{Kind: Pawn, Side: Black}) {
		t.Error("the pawn did not leave the pocket for e5")
	}

	// and the other way: Black of board 1 takes, White of board 0 (seat 0) gets it
	b2 := newBug(t)
	bugPlay(t, b2, 1, "e2e4 d7d5 e4d5 d8d5") // Black takes a pawn on board 1: seat 3, partner seat 0
	if b2.Position(0).Pocket[White][Pawn] != 1 {
		t.Errorf("White of board 0 should hold a pawn: %v", b2.Position(0).Pocket)
	}
	if b2.Position(1).Pocket[White][Pawn] != 0 && b2.Position(1).Pocket[Black][Pawn] != 0 {
		t.Error("pockets of the board where the pieces were taken are untouched")
	}
}

func TestBughouseOnlyTheBoardOfTheActionMoves(t *testing.T) {
	b := newBug(t)
	bugPlay(t, b, 0, "e2e4")
	if b.Turn(0) != Black || b.Turn(1) != White {
		t.Errorf("turns: %v %v", b.Turn(0), b.Turn(1))
	}
	if _, err := b.Apply(0, "e2e4"); err == nil {
		t.Error("e2e4 is not legal twice")
	}
	if _, err := b.Apply(2, "e2e4"); err != ErrBugBoard {
		t.Errorf("a third board: %v", err)
	}
	if b.Legal(2) != nil || b.Turn(2) != NoSide {
		t.Error("no board 2")
	}
}

func TestBughouseMateEndsTheGameForTheTeam(t *testing.T) {
	b := newBug(t)
	bugPlay(t, b, 0, "f2f3 e7e5 g2g4 d8h4") // Black of board 0 (seat 1, team 1) mates
	res, reason, over := b.Over()
	if !over || res != ResultBlack || reason != ReasonCheckmate {
		t.Fatalf("result %s %s %v", res, reason, over)
	}
	if len(b.View(0, White).Legal) != 0 || len(b.Legal(1)) != 0 {
		t.Error("no legal actions after the end")
	}
	if _, err := b.Apply(1, "e2e4"); err != ErrOver {
		t.Errorf("a move after the end: %v", err)
	}

	// the same mate on board 1: Black of board 1 is seat 3, team 0
	c := newBug(t)
	bugPlay(t, c, 1, "f2f3 e7e5 g2g4 d8h4")
	if res, _, over := c.Over(); !over || res != ResultWhite {
		t.Errorf("mate by seat 3 should give team 0 the win: %s %v", res, over)
	}
}

func TestBughouseMateByDropAndBlockByDrop(t *testing.T) {
	b := newBug(t)
	b.boards[0].Edit(func(p *Position) {
		*p = *mustFEN("7k/6pp/5N2/8/8/8/8/6K1 w - - 0 1")
		p.Pocket[White][Queen] = 1
	})
	note, err := b.Apply(0, "Q@g8")
	if err != nil || note != "Q@g8#" {
		t.Fatalf("%q %v", note, err)
	}
	if res, _, over := b.Over(); !over || res != ResultWhite {
		t.Errorf("mate by a drop: %s %v", res, over)
	}

	c := newBug(t)
	c.boards[0].Edit(func(p *Position) {
		*p = *mustFEN("6k1/5ppp/8/8/8/8/8/R5K1 w - - 0 1")
		p.Pocket[Black][Rook] = 1
	})
	if _, err := c.Apply(0, "a1a8"); err != nil {
		t.Fatal(err)
	}
	if _, _, over := c.Over(); over {
		t.Error("Black holds a rook to block with: no mate")
	}
}

func TestBughouseStalemateWaitsForThePartner(t *testing.T) {
	b := newBug(t)
	b.boards[1].Edit(func(p *Position) { *p = *mustFEN("7k/5Q2/6K1/8/8/8/8/8 b - - 0 1") })
	if _, _, over := b.Over(); over {
		t.Fatal("a stalemate does not end a bughouse game")
	}
	if len(b.Legal(1)) != 0 || b.Turn(1) != Black {
		t.Fatalf("Black of board 1 has no move and keeps the turn: %v %v", b.Legal(1), b.Turn(1))
	}
	// White of board 0 (seat 0) takes a piece: its partner, Black of board 1, may drop it
	bugPlay(t, b, 0, "e2e4 d7d5 e4d5")
	if len(b.Legal(1)) == 0 {
		t.Fatal("a pawn in hand frees the stalemated player")
	}
	if _, _, over := b.Over(); over {
		t.Error("not over")
	}
}

// Random games: the pieces of both boards and all pockets always make 64 (a promoted piece is not new material),
// pockets never go negative, ReplayBug gives the same game.
func TestBughouseRandomGames(t *testing.T) {
	games, plies := 40, 240
	if testing.Short() {
		games = 10
	}
	rng := rand.New(rand.NewSource(11))
	for gi := 0; gi < games; gi++ {
		b := newBug(t)
		var played []BugAction
		for ply := 0; ply < plies; ply++ {
			if _, _, over := b.Over(); over {
				break
			}
			board := rng.Intn(2)
			legal := b.Legal(board)
			if len(legal) == 0 {
				board = 1 - board
				legal = b.Legal(board)
				if len(legal) == 0 {
					break // both boards are waiting for a piece: a standstill
				}
			}
			a := legal[rng.Intn(len(legal))]
			if d := dropActions(legal); len(d) > 0 && rng.Intn(3) == 0 {
				a = d[rng.Intn(len(d))]
			}
			if _, err := b.Apply(board, a); err != nil {
				t.Fatalf("game %d: board %d %s: %v", gi, board, a, err)
			}
			played = append(played, BugAction{Board: board, Action: a})
			total := 0
			for i := 0; i < 2; i++ {
				p := b.Position(i)
				for _, pc := range p.B {
					if !pc.Empty() {
						total++
					}
				}
				for s := White; s <= Black; s++ {
					for _, n := range p.Pocket[s] {
						if n < 0 {
							t.Fatalf("game %d: negative pocket", gi)
						}
						total += int(n)
					}
				}
			}
			if total != 64 {
				t.Fatalf("game %d after %s: %d pieces in all", gi, a, total)
			}
		}
		r, err := ReplayBug(Options{}, played)
		if err != nil {
			t.Fatalf("game %d: replay: %v", gi, err)
		}
		for i := 0; i < 2; i++ {
			if r.Position(i).FEN() != b.Position(i).FEN() || r.Position(i).Pocket != b.Position(i).Pocket {
				t.Fatalf("game %d board %d: replay differs", gi, i)
			}
		}
		_, _, replayOver := r.Over()
		if _, _, over := b.Over(); over != replayOver {
			t.Fatalf("game %d: replay disagrees on the end", gi)
		}
	}
}

func TestBughouseIsRegisteredButNotAState(t *testing.T) {
	v := pocketVariant(t, "bughouse")
	if _, ok := v.(BugVariant); !ok {
		t.Fatal("bughouse must be a BugVariant")
	}
	if _, err := v.New(Options{}); err == nil {
		t.Error("a State of bughouse makes no sense")
	}
	if _, err := v.(BugVariant).NewBug(Options{}); err != nil {
		t.Error(err)
	}
}

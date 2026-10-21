package variants

import (
	"math/rand"
	"strings"
	"testing"
)

func TestChess960Ranks(t *testing.T) {
	seen := map[string]int{}
	for n := 0; n < Chess960Count; n++ {
		rank, err := Chess960Rank(n)
		if err != nil {
			t.Fatal(err)
		}
		if len(rank) != 8 {
			t.Fatalf("%d: %q", n, rank)
		}
		if prev, dup := seen[rank]; dup {
			t.Fatalf("%d and %d are both %s", prev, n, rank)
		}
		seen[rank] = n
		// the bishops on squares of both colours, the king between the rooks, and the right pieces
		var bishops []int
		count := map[byte]int{}
		for i := 0; i < 8; i++ {
			count[rank[i]]++
			if rank[i] == 'B' {
				bishops = append(bishops, i)
			}
		}
		if count['B'] != 2 || count['N'] != 2 || count['R'] != 2 || count['Q'] != 1 || count['K'] != 1 {
			t.Fatalf("%d: %s has the wrong pieces", n, rank)
		}
		if (bishops[0]+bishops[1])%2 != 1 {
			t.Errorf("%d: %s, the bishops stand on one colour", n, rank)
		}
		k, r1, r2 := strings.IndexByte(rank, 'K'), strings.IndexByte(rank, 'R'), strings.LastIndexByte(rank, 'R')
		if !(r1 < k && k < r2) {
			t.Errorf("%d: %s, the king is not between the rooks", n, rank)
		}
	}
	// numbers that are known by heart (Wikipedia, Lichess)
	for n, want := range map[int]string{0: "BBQNNRKR", 1: "BQNBNRKR", 518: "RNBQKBNR", 959: "RKRNNQBB"} {
		if got, _ := Chess960Rank(n); got != want {
			t.Errorf("setup %d is %s, want %s", n, got, want)
		}
	}
	for _, bad := range []int{-1, 960, 5000} {
		if _, err := Chess960Rank(bad); err == nil {
			t.Errorf("number %d accepted", bad)
		}
	}
}

// Every setup is a position the base can play: the pawns, the mirrored black
// rank, the two castling rights, and at least the moves of the pawns and knights.
func TestChess960AllSetupsStart(t *testing.T) {
	for n := 0; n < Chess960Count; n++ {
		st, err := Get0("chess960", Options{Seed: int64(n), Params: map[string]any{"position": float64(n)}})
		if err != nil {
			t.Fatal(err)
		}
		p := st.(*Game).Position()
		rank, _ := Chess960Rank(n)
		for f := 0; f < 8; f++ {
			if p.B[MakeSq(f, 0)] != (Piece{Kind: KindOf(rank[f]), Side: White}) || p.B[MakeSq(f, 7)] != (Piece{Kind: KindOf(rank[f]), Side: Black}) {
				t.Fatalf("%d: file %d is wrong: %s", n, f, p.Placement())
			}
		}
		for _, s := range []Side{White, Black} {
			if p.Castle[s][0] == NoSq || p.Castle[s][1] == NoSq {
				t.Fatalf("%d: %v lost a castling right at the start: %s", n, s, p.FEN())
			}
		}
		if got := len(st.Legal()); got < 18 {
			t.Fatalf("%d: only %d moves in %s", n, got, p.FEN())
		}
	}
}

func TestChess960NumberFromSeedAndLobby(t *testing.T) {
	// the seed draws when the lobby names nothing
	for _, seed := range []int64{0, 1, 518, 959, 960, 1 << 40, 1<<62 + 12345} {
		st, _ := Get0("chess960", Options{Seed: seed})
		x := st.View(NoSide).Extra.(map[string]any)
		if want := int(seed % 960); x["number"] != want {
			t.Errorf("seed %d: setup %v, want %d", seed, x["number"], want)
		}
	}
	st, _ := Get0("chess960", Options{Seed: 7, Params: map[string]any{"position": 518}})
	x := st.View(White).Extra.(map[string]any)
	if x["number"] != 518 || x["rank"] != "RNBQKBNR" {
		t.Errorf("a named setup: %v", x)
	}
	if got := st.(*Game).Position().Placement(); got != "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR" {
		t.Errorf("518 must be the classical setup: %s", got)
	}
}

func TestChess960CheckOptions(t *testing.T) {
	for _, bad := range []any{960, -1, 3.5, "518", true, []any{1}} {
		if _, _, err := Prepare("chess960", map[string]any{"position": bad}, 5); err == nil {
			t.Errorf("position %v accepted", bad)
		}
	}
	if _, _, err := Prepare("chess960", map[string]any{"colour": 1}, 5); err == nil {
		t.Error("an unknown choice was accepted")
	}
	// JSON numbers are floats
	_, o, err := Prepare("chess960", map[string]any{"position": float64(959)}, 5)
	if err != nil || o.Params["position"] != 959 || o.Params["number"] != 959 {
		t.Fatalf("%v %v", o.Params, err)
	}
	// nothing named: the seed's number is written to the record, not fixed
	_, o, err = Prepare("chess960", nil, 123)
	if err != nil || o.Params["number"] != 123 {
		t.Fatalf("%v %v", o.Params, err)
	}
	if _, fixed := o.Params["position"]; fixed {
		t.Error("an unnamed setup must stay unnamed, so that a rematch draws again")
	}
	// the options of the finished game go back through Prepare for the rematch
	_, again, err := Prepare("chess960", o.Params, 700)
	if err != nil || again.Params["number"] != 700 {
		t.Errorf("rematch of a drawn setup: %v %v", again.Params, err)
	}
	_, fixedAgain, _ := Prepare("chess960", map[string]any{"position": 42, "number": 42}, 700)
	if fixedAgain.Params["number"] != 42 {
		t.Errorf("rematch of a named setup: %v", fixedAgain.Params)
	}
	// null in the lobby means "random"
	_, o, err = Prepare("chess960", map[string]any{"position": nil}, 9)
	if err != nil || o.Params["number"] != 9 {
		t.Errorf("null position: %v %v", o.Params, err)
	}
}

func playAll(t *testing.T, st State, actions ...string) {
	t.Helper()
	for _, a := range actions {
		if _, err := st.Apply(a); err != nil {
			t.Fatalf("%s: %v (legal: %v)", a, err, st.Legal())
		}
	}
}

// In 518 the castling is the ordinary one, written as the king taking its rook.
func TestChess960ClassicalCastling(t *testing.T) {
	st, _ := Get0("chess960", Options{Params: map[string]any{"position": 518}})
	playAll(t, st, "e2e4", "e7e5", "g1f3", "b8c6", "f1e2", "g8f6")
	legal := st.Legal()
	if !contains(legal, "e1h1") || contains(legal, "e1g1") {
		t.Fatalf("O-O is the king taking its rook: %v", legal)
	}
	note, err := st.Apply("e1h1")
	if err != nil || note != "O-O" {
		t.Fatalf("%q %v", note, err)
	}
	p := st.(*Game).Position()
	if p.B[MakeSq(6, 0)] != (Piece{Kind: King, Side: White}) || p.B[MakeSq(5, 0)] != (Piece{Kind: Rook, Side: White}) {
		t.Errorf("after O-O: %s", p.Placement())
	}
}

// Castlings that look nothing like the classical one.
func TestChess960UnusualCastlings(t *testing.T) {
	cases := []struct {
		name, fen string
		legal     []string // castlings that must be offered
		illegal   []string // castlings that must not be
		play      string
		placement string // of the first rank after play, white pieces
	}{
		{"king on b, rooks on a and h", "4k3/8/8/8/8/8/8/RK5R w HA - 0 1", []string{"b1a1", "b1h1"}, nil, "b1a1", "2KR3R"},
		{"the king stays where it is", "4k3/8/8/8/8/8/8/5RKR w H - 0 1", nil, []string{"g1h1"}, "", ""}, // the rook of f1 is in the way: see next case
		{"the king stays, the rook crosses", "4k3/8/8/8/8/8/8/6KR w H - 0 1", []string{"g1h1"}, nil, "g1h1", "5RK1"},
		{"the rook goes through the king's square", "4k3/8/8/8/8/8/8/R3K2R w HA - 0 1", []string{"e1a1", "e1h1"}, nil, "e1a1", "2KR3R"},
		{"king on g, rook on a: far queen side", "4k3/8/8/8/8/8/8/R5K1 w A - 0 1", []string{"g1a1"}, nil, "g1a1", "2KR4"},
		{"an attacked square on the king's way", "5r1k/8/8/8/8/8/8/RK5R w HA - 0 1", []string{"b1a1"}, []string{"b1h1"}, "", ""},
		{"a piece in the way", "4k3/8/8/8/8/8/8/R2K1N1R w HA - 0 1", []string{"d1a1"}, []string{"d1h1"}, "", ""},
		{"the king is in check", "4k3/8/8/8/8/8/8/R2rK2R w HA - 0 1", nil, []string{"e1a1", "e1h1"}, "", ""},
	}
	for _, c := range cases {
		g, _ := NewGame(chess960Rules{}, Options{Params: map[string]any{"position": 518}})
		g.Edit(func(p *Position) { *p = *mustFEN(c.fen) })
		legal := g.Legal()
		for _, a := range c.legal {
			if !contains(legal, a) {
				t.Errorf("%s: %s must be legal; legal: %v", c.name, a, legal)
			}
		}
		for _, a := range c.illegal {
			if contains(legal, a) {
				t.Errorf("%s: %s must not be legal", c.name, a)
			}
		}
		if c.play == "" {
			continue
		}
		note, err := g.Apply(c.play)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if note != "O-O" && note != "O-O-O" {
			t.Errorf("%s: the note is %q", c.name, note)
		}
		got := g.Position().Placement()
		if want := "4k3/8/8/8/8/8/8/" + c.placement; got != want {
			t.Errorf("%s: after %s: %s, want %s", c.name, c.play, got, want)
		}
		if p := g.Position(); p.Castle[White] != [2]Sq{NoSq, NoSq} {
			t.Errorf("%s: the castling rights stay after castling", c.name)
		}
	}
}

// The Chess960 positions of the perft tests, through the variant itself (the
// castling written as king-takes-rook does not change the count of moves).
func TestChess960Perft(t *testing.T) {
	for _, c := range []struct {
		fen  string
		want []uint64
	}{
		{"bqnb1rkr/pp3ppp/3ppn2/2p5/5P2/P2P4/NPP1P1PP/BQ1BNRKR w HFhf - 2 9", []uint64{21, 528, 12189}},
		{"2nnrbkr/p1qppppp/8/1ppb4/6PP/3PP3/PPP2P2/BQNNRBKR w HEhe - 1 9", []uint64{21, 807, 18002}},
		{"b1q1rrkb/pppppppp/3nn3/8/P7/1PPP4/4PPPP/BQNNRKRB w GE - 1 9", []uint64{20, 479, 10471}},
	} {
		p := mustFEN(c.fen)
		for d, w := range c.want {
			if got := Perft(chess960Rules{}, p, d+1); got != w {
				t.Errorf("perft(%d) of %s = %d, want %d", d+1, c.fen, got, w)
			}
		}
	}
}

// A game of random moves in many setups is restored exactly from its options and actions.
func TestChess960Replay(t *testing.T) {
	castled := 0
	for seed := int64(1); seed <= 60; seed++ {
		o := Options{Seed: seed * 7919}
		rng := rand.New(rand.NewSource(seed))
		st, _ := Get0("chess960", o)
		var actions []string
		for ply := 0; ply < 70; ply++ {
			legal := st.Legal()
			if len(legal) == 0 {
				break
			}
			a := legal[rng.Intn(len(legal))]
			// take a castling whenever one is on offer
			for _, l := range legal {
				p := st.(*Game).Position()
				from, _ := ParseSq(l[:2])
				to, _ := ParseSq(l[2:4])
				if len(l) == 4 && p.B[from].Kind == King && p.B[to].Kind == Rook && p.B[to].Side == p.Turn {
					a = l
					castled++
				}
			}
			if _, err := st.Apply(a); err != nil {
				t.Fatalf("seed %d: %s: %v", seed, a, err)
			}
			actions = append(actions, a)
		}
		again, err := Replay("chess960", o, actions)
		if err != nil {
			t.Fatal(err)
		}
		if st.(*Game).Position().FEN() != again.(*Game).Position().FEN() {
			t.Fatalf("seed %d: the replay differs", seed)
		}
	}
	if castled == 0 {
		t.Error("no castling was played in 60 games")
	}
}

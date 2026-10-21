package variants

import (
	"strings"
	"testing"
)

// atomicAt makes an atomic game standing in the position of the FEN.
func atomicAt(t *testing.T, fen string) *atomicGame {
	t.Helper()
	g, err := NewGame(atomicRules{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	g.Edit(func(p *Position) { *p = *mustFEN(fen) })
	return &atomicGame{Game: g}
}

func playActions(t *testing.T, st State, actions string) {
	t.Helper()
	for _, a := range strings.Fields(actions) {
		if _, err := st.Apply(a); err != nil {
			t.Fatalf("%s: %v (legal: %v)", a, err, st.Legal())
		}
	}
}

func boardOf(st State) map[string]string { return st.View(NoSide).Board }

func TestAtomicPerft(t *testing.T) {
	// the numbers of Fairy-Stockfish and python-chess for atomic from the start
	want := []uint64{20, 400, 8902, 197326}
	depth := len(want)
	if testing.Short() {
		depth = 3
	}
	p := NewPosition()
	for d := 1; d <= depth; d++ {
		if got := Perft(atomicRules{}, p, d); got != want[d-1] {
			t.Errorf("perft(%d) = %d, want %d", d, got, want[d-1])
		}
	}
}

func TestAtomicExplosion(t *testing.T) {
	// c3xd4: the capturing pawn, the pawn taken, the knight c5 and the bishop e3 go;
	// the pawns c4 and e5 beside the blast stay
	g := atomicAt(t, "4k3/8/8/2n1p3/2Pp4/2P1B3/8/7K w - - 0 1")
	if _, err := g.Apply("c3d4"); err != nil {
		t.Fatal(err)
	}
	b := boardOf(g)
	for _, sq := range []string{"c3", "d4", "c5", "e3"} {
		if _, ok := b[sq]; ok {
			t.Errorf("%s should be empty: %v", sq, b)
		}
	}
	if b["c4"] != "wP" || b["e5"] != "bP" || b["e8"] != "bK" || b["h1"] != "wK" || len(b) != 4 {
		t.Errorf("survivors: %v", b)
	}
	ex, ok := g.View(White).Extra.(map[string]any)
	if !ok {
		t.Fatalf("no extra after an explosion: %v", g.View(White).Extra)
	}
	bl := ex["blast"].(*blast)
	if bl.Center != "d4" || len(bl.Squares) != 3 || bl.Pieces["c5"] != "bN" || bl.Pieces["e3"] != "wB" || bl.Pieces["d4"] != "bP" {
		t.Errorf("blast: %+v", bl)
	}
	// a quiet move clears it
	if _, err := g.Apply("e8e7"); err != nil {
		t.Fatal(err)
	}
	if g.View(White).Extra != nil {
		t.Errorf("a quiet move left a blast: %v", g.View(White).Extra)
	}
}

func TestAtomicEnPassantExplosion(t *testing.T) {
	g := atomicAt(t, "4k3/2nppp2/4b3/4P3/8/8/8/4K3 b - - 0 1")
	playActions(t, g, "d7d5")
	if !contains(g.Legal(), "e5d6") {
		t.Fatalf("en passant missing: %v", g.Legal())
	}
	playActions(t, g, "e5d6")
	b := boardOf(g)
	// centre d6: the pawn on d5 (taken), the capturing pawn, the knight c7 and bishop e6 go
	for _, sq := range []string{"d5", "d6", "e5", "c7", "e6", "d7"} {
		if _, ok := b[sq]; ok {
			t.Errorf("%s should be empty: %v", sq, b)
		}
	}
	if b["e7"] != "bP" || b["f7"] != "bP" || len(b) != 4 {
		t.Errorf("survivors: %v", b)
	}
}

func TestAtomicKingsTouch(t *testing.T) {
	// the rook e8 stares at the white king, but the kings touch: no check, any move is allowed
	g := atomicAt(t, "4r3/8/8/8/8/8/3k4/R3K3 w - - 0 1")
	if v := g.View(White); len(v.Check) != 0 {
		t.Errorf("check shown while the kings touch: %v", v.Check)
	}
	if !contains(g.Legal(), "a1a2") {
		t.Errorf("a rook move that ignores the 'check': %v", g.Legal())
	}
	// the same position with the black king far away: White must answer the check
	g = atomicAt(t, "4r3/8/8/8/8/8/7k/R3K3 w - - 0 1")
	if v := g.View(White); len(v.Check) != 1 || v.Check[0] != "e1" {
		t.Errorf("check missing: %v", v.Check)
	}
	if contains(g.Legal(), "a1a2") {
		t.Errorf("a1a2 ignores a real check: %v", g.Legal())
	}
	// a king may step next to the other king (it is an illegal move in chess)
	g = atomicAt(t, "8/8/8/4k3/8/4K3/8/Q7 w - - 0 1")
	if !contains(g.Legal(), "e3e4") || !contains(g.Legal(), "e3d4") {
		t.Errorf("kings may touch: %v", g.Legal())
	}
}

func TestAtomicKingNeverTakes(t *testing.T) {
	g := atomicAt(t, "4k3/8/8/4p3/4K3/8/8/Q7 w - - 0 1")
	if contains(g.Legal(), "e4e5") {
		t.Errorf("the king took a pawn: %v", g.Legal())
	}
	if !contains(g.Legal(), "e4d3") {
		t.Errorf("an ordinary king move is missing: %v", g.Legal())
	}
}

func TestAtomicCannotBlowUpYourOwnKing(t *testing.T) {
	// Qxd2 would blow up the pawn d2 and the king e1 next to it
	g := atomicAt(t, "7k/8/8/8/3Q4/8/3p4/4K3 w - - 0 1")
	if contains(g.Legal(), "d4d2") {
		t.Errorf("a capture next to your own king: %v", g.Legal())
	}
	// ... but a capture two squares from the king is fine
	g = atomicAt(t, "7k/8/8/8/3Q4/8/2p5/4K3 w - - 0 1")
	if !contains(g.Legal(), "d4c4") {
		t.Fatalf("sanity: %v", g.Legal())
	}
	// both kings in one blast: the mover's own king is gone, so it is refused
	g = atomicAt(t, "4k3/3p4/4K3/8/8/3Q4/8/8 w - - 0 1")
	if contains(g.Legal(), "d3d7") {
		t.Errorf("Qxd7 blows up both kings: %v", g.Legal())
	}
}

func TestAtomicBlowingUpTheKingWinsEvenInCheck(t *testing.T) {
	// the rook a8 checks the white king; the queen takes d7 and the black king e8 goes up with it
	g := atomicAt(t, "r3k3/3p4/8/8/8/3Q4/8/K7 w - - 0 1")
	if v := g.View(White); len(v.Check) != 1 {
		t.Fatalf("the white king should be in check: %v", v.Check)
	}
	if !contains(g.Legal(), "d3d7") {
		t.Fatalf("the winning capture is not legal: %v", g.Legal())
	}
	note, err := g.Apply("d3d7")
	if err != nil {
		t.Fatal(err)
	}
	if note != "Qxd7" {
		t.Errorf("notation %q (no + or # after a blown king)", note)
	}
	res, reason, over := g.Over()
	if !over || res != ResultWhite || reason != ReasonKingExploded {
		t.Errorf("result %q %q %v", res, reason, over)
	}
	if g.Legal() != nil {
		t.Errorf("moves after the end: %v", g.Legal())
	}
	if _, err := g.Apply("a8a1"); err != ErrOver {
		t.Errorf("a move after the end: %v", err)
	}
}

func TestAtomicMateAndStalemate(t *testing.T) {
	// the queen cannot be taken by the king, nor by a piece next to the king: mate
	g := atomicAt(t, "7k/8/6Q1/8/8/8/8/K7 w - - 0 1")
	note, err := g.Apply("g6g7")
	if err != nil || note != "Qg7#" {
		t.Fatalf("%q %v", note, err)
	}
	if res, reason, over := g.Over(); !over || res != ResultWhite || reason != ReasonCheckmate {
		t.Errorf("mate: %q %q %v", res, reason, over)
	}
	// a black rook that could take the queen but would blow up its own king does not save it
	g = atomicAt(t, "6rk/8/6Q1/8/8/8/8/K7 w - - 0 1")
	if _, err := g.Apply("g6g7"); err != nil {
		t.Fatal(err)
	}
	if res, reason, over := g.Over(); !over || res != ResultWhite || reason != ReasonCheckmate {
		t.Errorf("mate with an unusable defender: %q %q %v", res, reason, over)
	}
	// stalemate is a draw
	g = atomicAt(t, "7k/8/6Q1/8/8/8/8/K7 b - - 0 1")
	if res, reason, over := g.Over(); !over || res != ResultDraw || reason != ReasonStalemate {
		t.Errorf("stalemate: %q %q %v", res, reason, over)
	}
}

func TestAtomicCapturingPromotionIsAQueen(t *testing.T) {
	g := atomicAt(t, "r3k3/1P6/8/8/8/8/8/4K3 w - - 0 1")
	legal := g.Legal()
	if !contains(legal, "b7a8q") || contains(legal, "b7a8n") || contains(legal, "b7a8r") || contains(legal, "b7a8b") {
		t.Errorf("capturing promotion: %v", legal)
	}
	for _, a := range []string{"b7b8q", "b7b8r", "b7b8b", "b7b8n"} {
		if !contains(legal, a) {
			t.Errorf("%s missing: %v", a, legal)
		}
	}
	if _, err := g.Apply("b7a8q"); err != nil {
		t.Fatal(err)
	}
	if b := boardOf(g); len(b) != 2 {
		t.Errorf("the promoted queen and the rook should both be gone: %v", b)
	}
}

func TestAtomicCastlingRightsFollowTheBlast(t *testing.T) {
	g := atomicAt(t, "4k3/p7/2b5/8/8/8/6P1/R3K2R b KQ - 0 1")
	if _, err := g.Apply("c6g2"); err != nil {
		t.Fatal(err)
	}
	p := g.Position()
	if p.B[MakeSq(7, 0)].Kind != NoKind {
		t.Fatal("the rook h1 should have exploded")
	}
	if p.Castle[White][0] != NoSq || p.Castle[White][1] != MakeSq(0, 0) {
		t.Errorf("rights after the blast: %v", p.Castle[White])
	}
	if contains(g.Legal(), "e1g1") || !contains(g.Legal(), "e1c1") {
		t.Errorf("castling: %v", g.Legal())
	}
}

func TestAtomicInsufficientMaterial(t *testing.T) {
	cases := []struct {
		fen  string
		draw bool
	}{
		{"4k3/8/8/8/8/8/8/4K3 w - - 0 1", true},
		{"4k3/8/8/8/8/8/8/3NK3 w - - 0 1", true},
		{"4k3/8/8/8/8/8/8/3RK3 w - - 0 1", true},
		{"4k3/8/8/8/8/8/8/3QK3 w - - 0 1", false},
		{"4k3/8/8/8/8/8/4P3/4K3 w - - 0 1", false},
		{"4k3/8/8/8/8/8/8/N1N1K3 w - - 0 1", false},
		{"4k3/8/8/8/8/B7/8/2B1K3 w - - 0 1", true},  // two bishops on one colour
		{"4k3/8/8/8/8/8/8/2B1KB2 w - - 0 1", false}, // two bishops on both colours
		{"2b1k3/8/8/8/8/8/8/2B1K3 w - - 0 1", true}, // one bishop each, opposite colours: no one can take
		{"4kb2/8/8/8/8/8/8/2B1K3 w - - 0 1", false}, // one bishop each, same colour: they can take each other
		{"4kn2/8/8/8/8/8/8/3NK3 w - - 0 1", false},
	}
	for _, c := range cases {
		g := atomicAt(t, c.fen)
		_, reason, over := g.Over()
		if over != c.draw || over && reason != ReasonInsufficient {
			t.Errorf("%s: over=%v reason=%q, want draw=%v", c.fen, over, reason, c.draw)
		}
	}
}

func TestAtomicTimeout(t *testing.T) {
	g := atomicAt(t, "4k3/8/8/8/8/8/8/3NK3 w - - 0 1")
	g.Edit(func(p *Position) { p.B[MakeSq(3, 7)] = Piece{Kind: Pawn, Side: Black} })
	if got := g.Timeout(Black); got != ResultWhite {
		t.Errorf("white can win, black flags: %q", got)
	}
	g = atomicAt(t, "4k3/8/8/8/8/8/8/3RK3 w - - 0 1")
	// the game is already a draw by material, but the flag rule is the same
	if got := g.Timeout(Black); got != ResultDraw {
		t.Errorf("a lone rook cannot mate: %q", got)
	}
}

func TestAtomicRegisteredAndReplayed(t *testing.T) {
	v, ok := Get("atomic")
	if !ok || v.Info().Name == "" || v.Info().Short == "" || v.Info().Hidden {
		t.Fatalf("atomic is not registered properly: %+v", v)
	}
	// exd5: both pawns go; Qxd5 is an ordinary capture of nothing: the queen just lands on d5
	st, err := Replay("atomic", Options{}, []string{"e2e4", "d7d5", "e4d5", "d8d5"})
	if err != nil {
		t.Fatal(err)
	}
	b := boardOf(st)
	if b["d5"] != "bQ" || b["e4"] != "" || b["e2"] != "" || b["d7"] != "" {
		t.Errorf("board after the replay: %v", b)
	}
	if _, err := Replay("atomic", Options{}, []string{"e2e5"}); err == nil {
		t.Error("an impossible action replayed")
	}
}

func TestAtomicFoolsMateStillMates(t *testing.T) {
	st, _ := Replay("atomic", Options{}, strings.Fields("f2f3 e7e5 g2g4 d8h4"))
	if res, reason, over := st.Over(); !over || res != ResultBlack || reason != ReasonCheckmate {
		t.Errorf("fool's mate: %q %q %v", res, reason, over)
	}
}

// A whole game: the king is blown up by a capture next to it. It also runs
// the State through the interfaces the server uses.
func TestAtomicWholeGame(t *testing.T) {
	var st State
	st, err := Replay("atomic", Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 1.e4 e5 2.Nf3 Nc6 3.Nxe5 Nxe5 (blast e5: knight f3->e5 gone, pawn e5 gone, knight c6 gone? c6 is not adjacent to e5)
	playActions(t, st, "e2e4 e7e5 g1f3 b8c6 f3e5")
	b := boardOf(st)
	// Nxe5: centre e5: pieces next to e5 (d4 d5 d6 e4 e6 f4 f5 f6): pawns e4 stays; d6 f6 e6 nothing; black queen d8? no
	if _, ok := b["e5"]; ok {
		t.Errorf("e5 should be empty: %v", b)
	}
	if b["e4"] != "wP" {
		t.Errorf("the pawn on e4 survives the blast: %v", b)
	}
	if _, ok := st.(Finisher); !ok {
		t.Error("the state must be a Finisher")
	}
	if _, ok := st.(Timeouter); !ok {
		t.Error("the state must be a Timeouter")
	}
	// the king explodes: Black's e8 king beside a capture on d7 or f7 or e7
	st, _ = Replay("atomic", Options{}, strings.Fields("e2e4 e7e6 d1h5 a7a6 h5f7"))
	if res, reason, over := st.Over(); !over || res != ResultWhite || reason != ReasonKingExploded {
		t.Errorf("Qxf7 blows up the king: %q %q %v", res, reason, over)
	}
}

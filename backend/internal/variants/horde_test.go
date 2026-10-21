package variants

import (
	"strings"
	"testing"
)

// hordeAt makes a Horde game standing in the position of the FEN.
func hordeAt(t *testing.T, fen string) *Game {
	t.Helper()
	g, err := NewGame(hordeRules{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	g.Edit(func(p *Position) { *p = *mustFEN(fen) })
	return g
}

func TestHordeStart(t *testing.T) {
	g, err := NewGame(hordeRules{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	p := g.Position()
	pawns, kings := 0, 0
	for _, pc := range p.B {
		if pc.Side == White {
			if pc.Kind == Pawn {
				pawns++
			} else {
				kings++
			}
		}
	}
	if pawns != 36 || kings != 0 || p.KingSq(White) != NoSq {
		t.Errorf("the horde is %d pawns and %d other pieces", pawns, kings)
	}
	if p.Turn != White {
		t.Error("White moves first")
	}
	for _, sq := range []string{"b5", "c5", "f5", "g5"} {
		s, _ := ParseSq(sq)
		if p.B[s] != (Piece{Kind: Pawn, Side: White}) {
			t.Errorf("no white pawn on %s", sq)
		}
	}
	for _, sq := range []string{"a5", "d5", "e5", "h5"} {
		s, _ := ParseSq(sq)
		if !p.B[s].Empty() {
			t.Errorf("a piece on %s", sq)
		}
	}
	if p.Castle[Black][0] == NoSq || p.Castle[Black][1] == NoSq || p.Castle[White][0] != NoSq {
		t.Errorf("castling rights: %v", p.Castle)
	}
	if v := g.View(White); len(v.Check) != 0 {
		t.Errorf("White is never in check: %v", v.Check)
	}
	if ex := g.View(NoSide).Extra.(map[string]any); ex["horde"] != 36 {
		t.Errorf("extra: %v", ex)
	}
}

func TestHordePerft(t *testing.T) {
	// the numbers of Fairy-Stockfish and python-chess for horde from the start
	want := []uint64{8, 128, 1274, 23310}
	p := mustFEN(HordeFEN)
	for d := 1; d <= len(want); d++ {
		if got := Perft(hordeRules{}, p, d); got != want[d-1] {
			t.Errorf("perft(%d) = %d, want %d", d, got, want[d-1])
		}
	}
}

func TestHordeFirstRankPawnsStepTwo(t *testing.T) {
	g := hordeAt(t, "4k3/8/8/8/8/8/8/P7 w - - 0 1")
	legal := g.Legal()
	if !contains(legal, "a1a3") || !contains(legal, "a1a2") {
		t.Fatalf("first rank pawn: %v", legal)
	}
	playActions(t, g, "a1a3")
	if g.Position().EP != NoSq {
		t.Errorf("en passant square %v after a double step from the first rank", g.Position().EP)
	}
	// both squares must be empty
	g = hordeAt(t, "4k3/8/8/8/8/8/P7/P7 w - - 0 1")
	if contains(g.Legal(), "a1a3") {
		t.Errorf("a1a3 over a pawn: %v", g.Legal())
	}
	g = hordeAt(t, "4k3/8/8/8/8/p7/8/P7 w - - 0 1")
	if contains(g.Legal(), "a1a3") || !contains(g.Legal(), "a1a2") {
		t.Errorf("a1a3 onto a pawn: %v", g.Legal())
	}
}

func TestHordeNoEnPassantAfterAFirstRankStep(t *testing.T) {
	// the black pawn b3 stands beside a3: it could take en passant after a2a4, but not after a1a3
	g := hordeAt(t, "4k3/8/8/8/8/1p6/8/P7 w - - 0 1")
	playActions(t, g, "a1a3")
	if contains(g.Legal(), "b3a3") {
		t.Fatal("sanity")
	}
	for _, a := range g.Legal() {
		if strings.HasPrefix(a, "b3") && a != "b3b2" {
			t.Errorf("an en passant capture after a first-rank double step: %v", g.Legal())
		}
	}
}

func TestHordeSecondRankDoubleStepAllowsEnPassant(t *testing.T) {
	g := hordeAt(t, "4k3/8/8/8/1p6/8/P7/8 w - - 0 1")
	playActions(t, g, "a2a4")
	if !contains(g.Legal(), "b4a3") {
		t.Errorf("en passant after a2a4: %v", g.Legal())
	}
}

func TestHordeBlackWinsByTakingEverything(t *testing.T) {
	g := hordeAt(t, "4k3/8/8/8/b7/1P6/8/8 b - - 0 1")
	if _, _, over := g.Over(); over {
		t.Fatal("over at once")
	}
	playActions(t, g, "a4b3")
	res, reason, over := g.Over()
	if !over || res != ResultBlack || reason != ReasonHordeDestroyed {
		t.Errorf("%q %q %v", res, reason, over)
	}
	if g.Legal() != nil {
		t.Error("moves after the end")
	}
}

func TestHordeWhiteMates(t *testing.T) {
	g := hordeAt(t, "7k/6pp/8/8/8/8/8/R7 w - - 0 1")
	note, err := g.Apply("a1a8")
	if err != nil || note != "Ra8#" {
		t.Fatalf("%q %v", note, err)
	}
	if res, reason, over := g.Over(); !over || res != ResultWhite || reason != ReasonCheckmate {
		t.Errorf("%q %q %v", res, reason, over)
	}
}

func TestHordeStalemate(t *testing.T) {
	// the lone white pawn is blocked: White has no move, and no king to be in check
	g := hordeAt(t, "4k3/8/8/8/8/p7/P7/8 w - - 0 1")
	res, reason, over := g.Over()
	if !over || res != ResultDraw || reason != ReasonStalemate {
		t.Errorf("white stalemate: %q %q %v", res, reason, over)
	}
	// Black stalemated by the horde
	g = hordeAt(t, "7k/5Q2/6P1/8/8/8/8/8 b - - 0 1")
	if res, reason, over := g.Over(); !over || res != ResultDraw || reason != ReasonStalemate {
		t.Errorf("black stalemate: %q %q %v", res, reason, over)
	}
}

func TestHordeHasNoInsufficientMaterialDraw(t *testing.T) {
	// a lone knight against a bare king: chess calls it dead, in Horde Black may still take the knight and win
	g := hordeAt(t, "4k3/8/8/8/8/8/8/4N3 w - - 0 1")
	if _, _, over := g.Over(); over {
		t.Error("the game must go on")
	}
}

func TestHordeTimeout(t *testing.T) {
	g := hordeAt(t, "4k3/8/8/8/8/8/8/4N3 w - - 0 1")
	if got := g.Timeout(Black); got != ResultDraw {
		t.Errorf("a lone knight cannot mate a bare king: %q", got)
	}
	if got := g.Timeout(White); got != ResultBlack {
		t.Errorf("white flags: %q", got)
	}
	g = hordeAt(t, "4k3/8/8/8/8/8/8/3PN3 w - - 0 1")
	if got := g.Timeout(Black); got != ResultWhite {
		t.Errorf("a pawn promotes: %q", got)
	}
	g = hordeAt(t, "r3k3/8/8/8/8/8/8/4N3 w - - 0 1")
	if got := g.Timeout(Black); got != ResultWhite {
		t.Errorf("a knight against an army can still mate: %q", got)
	}
}

func TestHordePromotionBothWays(t *testing.T) {
	// a white pawn promotes on the eighth rank, a black pawn on the first
	g := hordeAt(t, "8/P7/4k3/8/8/8/p7/8 w - - 0 1")
	if !contains(g.Legal(), "a7a8q") || !contains(g.Legal(), "a7a8n") {
		t.Errorf("white promotion: %v", g.Legal())
	}
	playActions(t, g, "a7a8q")
	if !contains(g.Legal(), "a2a1q") {
		t.Errorf("black promotion: %v", g.Legal())
	}
}

func TestHordeRegisteredAndReplayed(t *testing.T) {
	v, ok := Get("horde")
	if !ok || v.Info().Name != "Орда" || v.Info().Hidden {
		t.Fatalf("not registered: %+v", v)
	}
	st, err := Replay("horde", Options{}, strings.Fields("b5b6 a7a6 g5g6 h7g6 d4d5"))
	if err != nil {
		t.Fatal(err)
	}
	if b := boardOf(st); b["d5"] != "wP" || b["g6"] != "bP" {
		t.Errorf("board: %v", b)
	}
	if _, ok := st.(Finisher); !ok {
		t.Error("a Finisher is needed for the server")
	}
}

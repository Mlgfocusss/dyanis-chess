package analysis

import (
	"testing"

	"dyanis-chess/backend/internal/pgn"
	"dyanis-chess/engine"
)

func TestMaterial(t *testing.T) {
	cases := []struct {
		fen    string
		bal    int
		pieces int
	}{
		{engine.StartFEN, 0, 32},
		{"4k3/8/8/8/8/8/4P3/4K3 w - - 0 1", 1, 3},
		{"4k3/8/8/8/8/8/8/Q3K3 w - - 0 1", 9, 3},
		{"r3k3/8/8/8/8/8/8/R2QK3 b - - 0 1", 9, 5},
		{"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPP1/RNBQKBNR w KQkq - 0 1", -1, 31},
	}
	for _, c := range cases {
		if bal, n := material(c.fen); bal != c.bal || n != c.pieces {
			t.Errorf("material(%s) = %d, %d; want %d, %d", c.fen, bal, n, c.bal, c.pieces)
		}
	}
}

func TestPVTrace(t *testing.T) {
	// White knight takes a pawn, Black king takes the knight, then a quiet move.
	pos, _ := engine.ParseFEN("4k3/8/5p2/8/4N3/8/8/4K3 w - - 0 1")
	var pv []engine.Move
	cur := pos.Copy()
	for _, san := range []string{"Nxf6+", "Kf7", "Kd2"} {
		m, err := cur.ParseSAN(san)
		if err != nil {
			t.Fatal(err)
		}
		pv = append(pv, m)
		cur.Play(m)
	}
	bal, capt := pvTrace(pos, pv)
	if len(bal) != 3 || bal[0] != 3 || bal[1] != 3 || bal[2] != 3 {
		// Nxf6 wins the pawn: knight 3 - 0 pawn... (White 3, Black 0 after the capture)
		t.Logf("balances %v", bal)
	}
	want := []bool{true, false, false}
	for i, w := range want {
		if capt[i] != w {
			t.Errorf("ply %d capture = %v, want %v", i+1, capt[i], w)
		}
	}
	// A line that stops being legal is cut there.
	bad := append([]engine.Move{}, pv[0], pv[0])
	if b, _ := pvTrace(pos, bad); len(b) != 1 {
		t.Errorf("an illegal continuation was followed: %v", b)
	}
}

func TestSacrificed(t *testing.T) {
	// Before: even. After the move: even; the engine's reply captures a knight
	// (Black +3 for White's point of view = -3), then a quiet ply.
	before := PositionEval{bal: 0}
	after := PositionEval{bal: 0, pvBal: []int{-3, -3}, pvCap: []bool{true, false}}
	if got := sacrificed(before, after, +1); got != 3 {
		t.Errorf("white gave up %d, want 3", got)
	}
	if got := sacrificed(before, after, -1); got != -3 {
		t.Errorf("black, for whom the same line wins a piece, gave up %d, want -3", got)
	}
	// An exchange: the capture is answered by a recapture; nothing is given up.
	exch := PositionEval{bal: 3, pvBal: []int{0, 3, 3}, pvCap: []bool{true, true, false}}
	if got := sacrificed(PositionEval{bal: 0}, exch, +1); got != -3 {
		// white took a knight first (bal 3), lost one, recaptured: ahead by 3
		t.Errorf("exchange: gave up %d, want -3", got)
	}
	// The engine's best reply is a quiet move: the offered piece is not taken, so no sacrifice.
	quiet := PositionEval{bal: 0, pvBal: []int{0}, pvCap: []bool{false}}
	if got := sacrificed(before, quiet, +1); got != 0 {
		t.Errorf("quiet reply: %d", got)
	}
	// No line at all: only the move itself counts.
	if got := sacrificed(PositionEval{bal: 0}, PositionEval{bal: -2}, +1); got != 2 {
		t.Errorf("no line: %d", got)
	}
}

func TestBrilliantNeedsASacrificeThatWorks(t *testing.T) {
	before := PositionEval{bal: 0}
	sac := PositionEval{bal: 0, pvBal: []int{-3, -3}, pvCap: []bool{true, false}}
	if !isBrilliant(before, sac, +1, 55, 52, false) {
		t.Error("a sound piece sacrifice in an equal position is brilliant")
	}
	if isBrilliant(before, sac, +1, 90, 88, false) {
		t.Error("a sacrifice in a position already won is not")
	}
	if isBrilliant(before, sac, +1, 55, 30, false) {
		t.Error("a sacrifice that leaves the mover worse is not")
	}
	if isBrilliant(before, sac, +1, 55, 52, true) {
		t.Error("a promotion is not a sacrifice")
	}
	if isBrilliant(before, PositionEval{bal: 0, pvBal: []int{0, 0}, pvCap: []bool{true, false}}, +1, 55, 52, false) {
		t.Error("an even trade is not a sacrifice")
	}
	if isBrilliant(before, PositionEval{bal: 0, pvBal: []int{-1, -1}, pvCap: []bool{true, false}}, +1, 55, 52, false) {
		t.Error("a pawn is not enough")
	}
	// A sacrifice for a forced mate counts even in a won position.
	mate := sac
	mate.Mate = 2
	if !isBrilliant(before, mate, +1, 99, 100, false) {
		t.Error("a sacrifice that forces mate is brilliant")
	}
	mate.Mate = -2 // the mate is Black's: not the mover's
	if isBrilliant(before, mate, +1, 99, 100, false) {
		t.Error("the opponent's mate does not make a sacrifice brilliant")
	}
}

func TestGreatNeedsAnOnlyMoveInAnOpenPosition(t *testing.T) {
	if !isGreat(30, 60, 50, 0) {
		t.Error("only move in an even position")
	}
	if isGreat(10, 60, 50, 0) {
		t.Error("alternatives only a little worse")
	}
	if isGreat(30, 10, 50, 0) {
		t.Error("the position is lost anyway")
	}
	if isGreat(30, 99, 98, 0) {
		t.Error("the position was already decided")
	}
	if isGreat(30, 60, 4, 0) {
		t.Error("lost before the move")
	}
	if isGreat(30, 60, 50, 25) {
		t.Error("answering the opponent's blunder is not a find")
	}
}

func TestOnlyMoveGap(t *testing.T) {
	opt := &Options{}
	opt.fill()
	gapOf := func(fen string) (float64, float64) {
		pos, err := engine.ParseFEN(fen)
		if err != nil {
			t.Fatal(err)
		}
		res, err := engine.Search(pos, nil, engine.Limits{Depth: 10, HashMB: 8})
		if err != nil {
			t.Fatal(err)
		}
		return onlyMoveGap(pos, []uint64{pos.Hash()}, res.Best, opt)
	}
	// Taking the undefended rook is far better than anything else.
	if pct, gap := gapOf("6k1/5ppp/8/8/8/8/5PPP/r3R1K1 w - - 0 1"); gap < GreatGap || pct < 60 {
		t.Errorf("rook capture: pct %.1f gap %.1f", pct, gap)
	}
	// A quiet position with several good moves has no only move: the best one leads by a rounding
	// error at most, far below the gap that makes a move "great".
	if _, gap := gapOf(engine.StartFEN); gap >= GreatGap {
		t.Errorf("start position: gap %.1f", gap)
	}
	// With fewer than three legal moves there is no choice to test.
	if _, gap := gapOf("k7/8/1K6/8/8/8/8/R7 b - - 0 1"); gap != 0 {
		t.Errorf("a single legal move: gap %.1f", gap)
	}
}

const operaGame = `[White "Morphy"]
[Black "Duke of Brunswick"]

1. e4 e5 2. Nf3 d6 3. d4 Bg4 4. dxe5 Bxf3 5. Qxf3 dxe5 6. Bc4 Nf6 7. Qb3 Qe7 8. Nc3 c6 9. Bg5 b5 10. Nxb5 cxb5 11. Bxb5+ Nbd7 12. O-O-O Rd8 13. Rxd7 Rxd7 14. Rd1 Qe6 15. Bxd7+ Nxd7 16. Qb8+ Nxb8 17. Rd8# 1-0`

func TestOperaQueenSacrificeIsBrilliant(t *testing.T) {
	games, err := pgn.ParseString(operaGame)
	if err != nil || len(games) != 1 {
		t.Fatal(err)
	}
	rep, err := Analyze(games[0], Options{Depth: 10, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	var brilliant []int
	for _, m := range rep.Moves {
		if m.Class == Brilliant {
			brilliant = append(brilliant, m.Ply)
		}
	}
	// 16.Qb8+!! is ply 31; nothing else in this game is a queen sacrifice.
	found := false
	for _, p := range brilliant {
		if p == 31 {
			found = true
		}
	}
	if !found {
		t.Errorf("16.Qb8+ not brilliant; brilliant plies %v", brilliant)
	}
	if m := rep.Moves[30]; m.Loss != 0 || m.Accuracy < 99 {
		t.Errorf("a brilliant move must have lost nothing: %+v", m)
	}
	if rep.Counts[0][Brilliant] < 1 {
		t.Errorf("counts do not include it: %v", rep.Counts[0])
	}
	// Opening theory is never brilliant or great.
	for _, m := range rep.Moves[:rep.BookPlies] {
		if m.Class != Book {
			t.Errorf("ply %d in theory was classed %s", m.Ply, m.Class)
		}
	}

	// Without the special classes the same move is just best.
	plain, err := Analyze(games[0], Options{Depth: 10, Workers: 2, NoSpecial: true})
	if err != nil {
		t.Fatal(err)
	}
	if c := plain.Moves[30].Class; c != Best {
		t.Errorf("with NoSpecial 16.Qb8+ is %s, want best", c)
	}
	for _, m := range plain.Moves {
		if m.Class == Brilliant || m.Class == Great {
			t.Errorf("NoSpecial still produced %s at ply %d", m.Class, m.Ply)
		}
	}
}

func TestMiss(t *testing.T) {
	if !isMiss(Inaccuracy, 15, 75) || !isMiss(Mistake, 12, 62) {
		t.Error("letting a gift go is a miss")
	}
	if isMiss(Blunder, 30, 90) {
		t.Error("a blunder stays a blunder")
	}
	if isMiss(Inaccuracy, 3, 75) {
		t.Error("the opponent did not err")
	}
	if isMiss(Mistake, 15, 45) {
		t.Error("the mover was not better")
	}
	if isMiss(Good, 20, 80) || isMiss(Best, 20, 80) {
		t.Error("a good move is not a miss")
	}
}

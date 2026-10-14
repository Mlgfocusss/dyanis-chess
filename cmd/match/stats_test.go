package main

import (
	"math"
	"testing"
)

// Pairs where both games agree (opening bias dominates) must give a wider
// pair-level interval than the per-game trinomial one would claim, and pairs
// where the colors cancel must give a narrower one — and the mean must match.
func TestPentaMatchesTallyMean(t *testing.T) {
	p := penta{3, 5, 20, 12, 10} // 50 pairs
	// Same data as games: pair score k/2 -> expand into per-game W/D/L.
	var tl tally
	for k, c := range p {
		for i := 0; i < c; i++ {
			switch k {
			case 0:
				tl.l += 2
			case 1:
				tl.l++
				tl.d++
			case 2:
				tl.d += 2 // (also stands in for W+L; same mean)
			case 3:
				tl.w++
				tl.d++
			case 4:
				tl.w += 2
			}
		}
	}
	if math.Abs(p.mean()-tl.mean()) > 1e-12 {
		t.Fatalf("mean differs: penta %v tally %v", p.mean(), tl.mean())
	}
}

func TestPentaVarianceCancelsOpeningBias(t *testing.T) {
	// 20 pairs, every pair 1-1 (one win, one loss): per game it looks like
	// wild +/-; per pair there is no variance at all.
	p := penta{0, 0, 20, 0, 0}
	if v := p.variance(); v != 0 {
		t.Fatalf("variance = %v, want 0", v)
	}
	if got := p.mean(); got != 0.5 {
		t.Fatalf("mean = %v, want 0.5", got)
	}
}

func TestPentaEloAndLLRSign(t *testing.T) {
	good := penta{2, 4, 20, 25, 19} // clearly positive
	elo, ci := good.eloReport()
	if elo <= 0 || ci <= 0 || math.IsInf(ci, 0) {
		t.Fatalf("elo %v ci %v", elo, ci)
	}
	if good.llr(0, 10) <= 0 {
		t.Fatalf("llr should favour H1 for a winning score, got %v", good.llr(0, 10))
	}
	bad := penta{19, 25, 20, 4, 2}
	if bad.llr(0, 10) >= 0 {
		t.Fatalf("llr should favour H0 for a losing score, got %v", bad.llr(0, 10))
	}
	var empty penta
	if empty.llr(0, 10) != 0 {
		t.Fatal("empty llr must be 0")
	}
}

func TestAdjudication(t *testing.T) {
	adj := adjudication{winCP: 1000, winMoves: 2, drawCP: 10, drawMoves: 2, drawPly: 6}
	known := func(n int) []bool {
		k := make([]bool, n)
		for i := range k {
			k[i] = true
		}
		return k
	}

	// Four plies (2 moves each) all >= +1000 for white: white wins.
	s := []int{0, 0, 1200, 1500, 1100, 2000}
	if r, _, ok := adj.check(s, known(len(s))); !ok || r != whiteWins {
		t.Fatalf("want white win, got %v %v", r, ok)
	}
	// Same for black.
	s = []int{-1200, -1500, -1100, -1000}
	if r, _, ok := adj.check(s, known(len(s))); !ok || r != blackWins {
		t.Fatalf("want black win, got %v %v", r, ok)
	}
	// One engine disagreeing breaks the streak.
	s = []int{1200, 1500, 900, 2000}
	if _, _, ok := adj.check(s, known(len(s))); ok {
		t.Fatal("must not adjudicate when one ply is below the threshold")
	}
	// A ply with no score breaks it too.
	s = []int{1200, 1500, 1100, 2000}
	k := known(4)
	k[2] = false
	if _, _, ok := adj.check(s, k); ok {
		t.Fatal("must not adjudicate across an unscored ply")
	}
	// Draw only from drawPly on.
	s = []int{0, 3, -5, 0}
	if _, _, ok := adj.check(s, known(len(s))); ok {
		t.Fatal("draw adjudication before drawPly")
	}
	s = []int{50, 80, 90, 70, 0, 3, -5, 0}
	if r, _, ok := adj.check(s, known(len(s))); !ok || r != draw {
		t.Fatalf("want draw, got %v %v", r, ok)
	}
	// Disabled rules never fire.
	if _, _, ok := (adjudication{}).check([]int{5000, 5000, 5000, 5000, 5000, 5000, 5000, 5000}, known(8)); ok {
		t.Fatal("zero adjudication must be off")
	}
}

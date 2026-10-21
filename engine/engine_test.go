package engine

import (
	"strings"
	"testing"
	"time"
)

// These tests are the contract of the facade: whatever engine stands
// behind this package, they must pass. See engine/README.md.

func play(t *testing.T, p *Position, sans ...string) {
	t.Helper()
	for _, s := range sans {
		m, err := p.ParseSAN(s)
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		p.Play(m)
	}
}

func TestPositionBasics(t *testing.T) {
	p := NewPosition()
	if p.FEN() != StartFEN || p.SideToMove() != White {
		t.Fatalf("start: %q %v", p.FEN(), p.SideToMove())
	}
	if n := len(p.LegalMoves()); n != 20 {
		t.Errorf("legal moves at start = %d, want 20", n)
	}
	h0 := p.Hash()
	c := p.Copy()
	play(t, p, "e4")
	if p.SideToMove() != Black || p.Hash() == h0 {
		t.Error("Play did not change the position")
	}
	if c.Hash() != h0 || c.FEN() != StartFEN {
		t.Error("Copy is not independent")
	}
	q, err := ParseFEN(p.FEN())
	if err != nil || q.Hash() != p.Hash() {
		t.Errorf("FEN round trip: %v", err)
	}
	if _, err := ParseFEN("not a fen"); err == nil {
		t.Error("bad FEN accepted")
	}
}

func TestSANAndMoves(t *testing.T) {
	p := NewPosition()
	m, err := p.ParseSAN("Nf3")
	if err != nil || m.String() != "g1f3" || p.SAN(m) != "Nf3" {
		t.Fatalf("Nf3: %v %v %q", m, err, p.SAN(m))
	}
	if _, err := p.ParseSAN("Ke4"); err == nil {
		t.Error("illegal move accepted")
	}
	found := false
	for _, lm := range p.LegalMoves() {
		if lm == m {
			found = true
		}
	}
	if !found {
		t.Error("moves from ParseSAN and LegalMoves are not comparable with ==")
	}
	// Transposition: same position, same hash.
	a, b := NewPosition(), NewPosition()
	play(t, a, "Nf3", "d5", "d4")
	play(t, b, "d4", "d5", "Nf3")
	if a.Hash() != b.Hash() {
		t.Error("transposed positions have different hashes")
	}
}

func TestOutcome(t *testing.T) {
	p := NewPosition()
	if out := p.Outcome(nil); out.Over || out.Reason != "ongoing" {
		t.Errorf("start: %+v", out)
	}
	play(t, p, "f3", "e5", "g4", "Qh4#")
	out := p.Outcome(nil)
	if !out.Over || !out.Checkmate || out.Reason != "checkmate" {
		t.Errorf("fool's mate: %+v", out)
	}
	stale, _ := ParseFEN("7k/5Q2/6K1/8/8/8/8/8 b - - 0 1")
	if out := stale.Outcome(nil); !out.Over || out.Checkmate || !strings.Contains(out.Reason, "stalemate") {
		t.Errorf("stalemate: %+v", out)
	}
	// Threefold repetition through the history argument.
	r := NewPosition()
	hist := []uint64{r.Hash()}
	for i := 0; i < 2; i++ {
		for _, s := range []string{"Nf3", "Nf6", "Ng1", "Ng8"} {
			play(t, r, s)
			hist = append(hist, r.Hash())
		}
	}
	if out := r.Outcome(hist); !out.Over || !strings.Contains(out.Reason, "repetition") {
		t.Errorf("repetition: %+v", out)
	}
}

func TestSearch(t *testing.T) {
	// Mate in one for White: 1.Qh7#.
	p, err := ParseFEN("7k/8/5KQ1/8/8/8/8/8 w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Search(p, nil, Limits{Depth: 4, HashMB: 4})
	if err != nil {
		t.Fatal(err)
	}
	if p.SAN(res.Best) != "Qg7#" && p.SAN(res.Best) != "Qh7#" && p.SAN(res.Best) != "Qh6#" {
		t.Errorf("best = %s, want a mating queen move", p.SAN(res.Best))
	}
	if res.Score < MateScore-1000 {
		t.Errorf("score %d is not a mate score", res.Score)
	}
	if res.Depth < 1 || res.Nodes < 1 || len(res.PV) == 0 || res.PV[0] != res.Best {
		t.Errorf("result: %+v", res)
	}
	if p.FEN() != "7k/8/5KQ1/8/8/8/8/8 w - - 0 1" {
		t.Error("Search modified the position it was given")
	}

	// Same depth, same answer.
	again, _ := Search(p, nil, Limits{Depth: 4, HashMB: 4})
	if again.Best != res.Best || again.Score != res.Score || again.Nodes != res.Nodes {
		t.Error("fixed-depth search is not reproducible")
	}

	// Timed search returns something playable.
	start := NewPosition()
	tr, err := Search(start, nil, Limits{MoveTime: 50 * time.Millisecond, MaxDepth: 8, HashMB: 4})
	if err != nil || len(start.LegalMoves()) == 0 {
		t.Fatal(err)
	}
	ok := false
	for _, m := range start.LegalMoves() {
		ok = ok || m == tr.Best
	}
	if !ok {
		t.Errorf("timed search returned an illegal move %v", tr.Best)
	}

	// No legal move: an error, not a panic.
	stale, _ := ParseFEN("7k/5Q2/6K1/8/8/8/8/8 b - - 0 1")
	if _, err := Search(stale, nil, Limits{Depth: 2, HashMB: 1}); err == nil {
		t.Error("search of a stalemate position succeeded")
	}
}

func TestLoadNetworkErrors(t *testing.T) {
	if err := LoadNetwork(""); err == nil {
		t.Error("empty path accepted")
	}
	if err := LoadNetwork("does-not-exist.bin"); err == nil {
		t.Error("missing file accepted")
	}
}

func TestSearchLines(t *testing.T) {
	// A position with a clear best move (the queen is hanging) and many others.
	p, err := ParseFEN("rnb1kbnr/pppp1ppp/8/4p3/4P2q/5N2/PPPP1PPP/RNBQKB1R w KQkq - 2 3")
	if err != nil {
		t.Fatal(err)
	}
	lim := Limits{Depth: 6, HashMB: 8}
	single, err := Search(p, nil, lim)
	if err != nil {
		t.Fatal(err)
	}
	lines, err := SearchLines(p, nil, lim, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	// The first line is the ordinary search; the others start with other moves and are no better.
	if lines[0].Best != single.Best || lines[0].Score != single.Score {
		t.Errorf("first line %v (%d) differs from Search %v (%d)", lines[0].Best, lines[0].Score, single.Best, single.Score)
	}
	seen := map[Move]bool{}
	for i, l := range lines {
		if seen[l.Best] {
			t.Errorf("line %d repeats the move %v", i+1, l.Best)
		}
		seen[l.Best] = true
		if len(l.PV) == 0 || l.PV[0] != l.Best {
			t.Errorf("line %d: PV %v does not start with %v", i+1, l.PV, l.Best)
		}
		if i > 0 && l.Score > lines[0].Score {
			t.Errorf("line %d (%d) is better than the first (%d)", i+1, l.Score, lines[0].Score)
		}
	}
	if p.FEN() != "rnb1kbnr/pppp1ppp/8/4p3/4P2q/5N2/PPPP1PPP/RNBQKB1R w KQkq - 2 3" {
		t.Error("SearchLines modified the position")
	}

	// Fewer legal moves than lines: as many lines as moves, each a different move.
	two, _ := ParseFEN("7k/8/5K2/8/8/8/8/6R1 b - - 0 1") // a black king in a corner with few moves
	legal := len(two.LegalMoves())
	got, err := SearchLines(two, nil, Limits{Depth: 3, HashMB: 1}, 10)
	if err != nil || len(got) != legal {
		t.Fatalf("lines = %d (err %v), legal moves = %d", len(got), err, legal)
	}
}

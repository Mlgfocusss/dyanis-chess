package eco

import (
	"strings"
	"testing"

	"dyanis-chess/backend/internal/pgn"
	"dyanis-chess/engine"
)

// hashAfter plays the SAN moves from the initial position and returns the hash.
func hashAfter(t *testing.T, moves string) uint64 {
	t.Helper()
	g, err := pgn.ParseOne(moves + " *")
	if err != nil {
		t.Fatal(err)
	}
	b := engine.NewPosition()
	for _, m := range g.Moves {
		b.Play(m.Move)
	}
	return b.Hash()
}

func TestDefaultLoads(t *testing.T) {
	b := Default()
	// 3869 lines in the reference; transpositions collapse a few.
	if b.Len() < 3000 {
		t.Fatalf("book has only %d positions", b.Len())
	}
	if Default() != b {
		t.Error("Default is not cached")
	}
}

func TestLookup(t *testing.T) {
	b := Default()
	cases := []struct {
		moves, eco, namePart string
	}{
		{"1. e4 e5 2. Nf3 Nc6 3. Bb5 a6", "C", "Ruy Lopez"},
		{"1. e4 c5 2. Nf3 d6 3. d4 cxd4 4. Nxd4 Nf6 5. Nc3 a6", "B", "Sicilian Defense: Najdorf"},
		{"1. d4 d5 2. c4 e6", "D", "Queen's Gambit Declined"},
	}
	for _, c := range cases {
		o, ok := b.Lookup(hashAfter(t, c.moves))
		if !ok {
			t.Errorf("%q: not found", c.moves)
			continue
		}
		if !strings.HasPrefix(o.ECO, c.eco) || !strings.Contains(o.Name, c.namePart) {
			t.Errorf("%q: got %s %s", c.moves, o.ECO, o.Name)
		}
	}
	if _, ok := b.Lookup(hashAfter(t, "1. a4 h5 2. a5 h4 3. a6 h3 4. axb7")); ok {
		t.Error("nonsense line found in book")
	}
	if _, ok := (*Book)(nil).Lookup(1); ok {
		t.Error("nil book found something")
	}
}

// A transposition must give the same name as the direct route.
func TestTransposition(t *testing.T) {
	b := Default()
	a, ok1 := b.Lookup(hashAfter(t, "1. d4 Nf6 2. c4 e6 3. Nc3 Bb4"))
	c, ok2 := b.Lookup(hashAfter(t, "1. c4 e6 2. Nc3 Nf6 3. d4 Bb4"))
	if !ok1 || !ok2 || a != c {
		t.Errorf("Nimzo-Indian by two routes: %v %v, %v %v", a, ok1, c, ok2)
	}
}

func TestParse(t *testing.T) {
	tsv := "eco\tname\tpgn\n" +
		"A00\tShort\t1. e4\n" +
		"A00\tLong same position\t1. e4 e5 2. Nf3\n" +
		"A00\tTransposed\t1. Nf3 e5 2. e4\n"
	b, err := Parse(tsv)
	if err != nil {
		t.Fatal(err)
	}
	if b.Len() != 2 {
		t.Errorf("len = %d, want 2", b.Len())
	}
	// Two lines end in the same position and are equally long, so
	// the one listed first is kept.
	o, _ := b.Lookup(hashAfter(t, "1. e4 e5 2. Nf3"))
	if o.Name != "Long same position" || o.Plies != 3 {
		t.Errorf("got %+v", o)
	}
	if _, err := Parse("eco\tname\tpgn\nA00\tBroken\t1. e5\n"); err == nil {
		t.Error("illegal first move accepted")
	}
	if _, err := Parse("A00\tonly two\n"); err == nil {
		t.Error("short line accepted")
	}
}

// A line named at ply 9 passes through unnamed positions: they are theory too.
func TestInBook(t *testing.T) {
	b := Default()
	// 4.Ba4 Nf6 is not named on its own but 5.O-O (Morphy Defense) is.
	for _, moves := range []string{
		"1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 4. Ba4",
		"1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 4. Ba4 Nf6",
		"1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 4. Ba4 Nf6 5. O-O",
	} {
		if !b.InBook(hashAfter(t, moves)) {
			t.Errorf("%q should be theory", moves)
		}
	}
	if b.InBook(hashAfter(t, "1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 4. Ba4 Nf6 5. O-O h6")) {
		t.Error("5...h6 is not theory")
	}
	if (*Book)(nil).InBook(1) {
		t.Error("nil book has theory")
	}
}

package openings

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"

	"dyanis-chess/internal/board"
)

// testdata/golden.txt was dumped from cmd/match's bookOpenings/
// randomOpenings before they moved here: the same seed must keep giving
// the same FENs, or match results across the move stop being comparable.
// Each block is a header "book|random seed n plies maxEval count" followed
// by count FENs.
func TestOpeningsMatchGolden(t *testing.T) {
	f, err := os.Open("testdata/golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	blocks := 0
	for sc.Scan() {
		h := strings.Fields(sc.Text())
		if len(h) != 6 {
			t.Fatalf("bad header %q", sc.Text())
		}
		var v [5]int
		for i := range v {
			if v[i], err = strconv.Atoi(h[i+1]); err != nil {
				t.Fatal(err)
			}
		}
		want := make([]string, v[4])
		for i := range want {
			sc.Scan()
			want[i] = sc.Text()
		}
		var got []string
		switch h[0] {
		case "book":
			if got, err = FromBook("../../assets/gm2001.bin", v[1], v[2], v[3], int64(v[0])); err != nil {
				t.Fatal(err)
			}
		case "random":
			got = Random(v[1], v[2], v[3], int64(v[0]))
		default:
			t.Fatalf("bad kind %q", h[0])
		}
		if len(got) != len(want) {
			t.Fatalf("%v: %d openings, want %d", h, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%v: opening %d = %q, want %q", h, i, got[i], want[i])
			}
		}
		blocks++
	}
	if blocks != 5 {
		t.Fatalf("%d blocks checked, want 5", blocks)
	}
}

// Openings drawn from the shipped books must be real, legal, distinct,
// within the eval window and reproducible from the seed.
func TestBookOpenings(t *testing.T) {
	for _, name := range []string{"gm2001.bin", "performance.bin", "komodo.bin"} {
		path := "../../assets/" + name
		if _, err := os.Stat(path); err != nil {
			t.Skipf("%s not available", name)
		}
		got, err := FromBook(path, 300, 8, 80, 1)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d distinct openings for 300 requested", name, len(got))
		if len(got) == 0 {
			t.Fatalf("%s: no openings generated", name)
		}
		seen := map[string]bool{}
		for _, fen := range got {
			if seen[fen] {
				t.Fatalf("%s: duplicate opening %s", name, fen)
			}
			seen[fen] = true
			pos, err := board.FromFEN(fen)
			if err != nil {
				t.Fatalf("%s: bad FEN %q: %v", name, fen, err)
			}
			if !Accept(pos, 80) {
				t.Fatalf("%s: opening %q violates the filter", name, fen)
			}
		}
		again, _ := FromBook(path, 300, 8, 80, 1)
		if len(again) != len(got) || (len(got) > 0 && again[0] != got[0]) {
			t.Fatalf("%s: same seed gave different openings", name)
		}
	}
}

func TestFromBookRandomZeroIsFromBook(t *testing.T) {
	a, err := FromBook("../../assets/gm2001.bin", 50, 8, 80, 3)
	if err != nil {
		t.Fatal(err)
	}
	b, err := FromBookRandom("../../assets/gm2001.bin", 50, 8, 0, 80, 3)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(a, "\n") != strings.Join(b, "\n") {
		t.Fatal("FromBookRandom with randPlies=0 differs from FromBook")
	}
	c, err := FromBookRandom("../../assets/gm2001.bin", 50, 8, 4, 150, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 50 {
		t.Fatalf("FromBookRandom gave %d openings, want 50", len(c))
	}
}

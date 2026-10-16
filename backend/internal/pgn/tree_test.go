package pgn

import (
	"strings"
	"testing"
)

// lines lists every line of the tree as SAN strings, main line first.
func lines(n *Node, prefix []string, out *[]string) {
	if len(n.Children) == 0 {
		if len(prefix) > 0 {
			*out = append(*out, strings.Join(prefix, " "))
		}
		return
	}
	for _, c := range n.Children {
		lines(c, append(append([]string{}, prefix...), c.SAN), out)
	}
}

func linesOf(t *Tree) []string {
	var out []string
	lines(&t.Root, nil, &out)
	return out
}

func TestParseTreesKeepsVariations(t *testing.T) {
	trees, err := ParseTreesString(`[Event "Italian"]

1. e4 e5 2. Nf3 (2. Bc4 Nf6 (2... Bc5 3. c3) 3. d4) 2... Nc6 3. Bc4 {the main line} $1 *
`)
	if err != nil || len(trees) != 1 {
		t.Fatalf("%d trees (%v)", len(trees), err)
	}
	got := linesOf(trees[0])
	want := []string{
		"e4 e5 Nf3 Nc6 Bc4",
		"e4 e5 Bc4 Nf6 d4",
		"e4 e5 Bc4 Bc5 c3",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("lines:\n got %q\nwant %q", got, want)
	}
	if n := trees[0].Count(); n != 10 {
		t.Errorf("count %d", n)
	}
	// the FEN of a node is the position after its move
	e4 := trees[0].Root.Children[0]
	if !strings.HasPrefix(e4.FEN, "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b ") {
		t.Errorf("fen after e4: %s", e4.FEN)
	}
}

func TestParseTreesJoinsRepeatedMoves(t *testing.T) {
	trees, err := ParseTreesString("1. e4 e5 (1... e5 2. Nf3) (1... c5) 2. Nc3 *\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := linesOf(trees[0]); strings.Join(got, "|") != "e4 e5 Nf3|e4 e5 Nc3|e4 c5" {
		t.Errorf("lines: %q", got)
	}
}

func TestParseTreesSeveralGamesAndErrors(t *testing.T) {
	trees, err := ParseTreesString("[Event \"A\"]\n\n1. d4 d5 *\n\n[Event \"B\"]\n\n1. c4 e5 (1... c5) *\n")
	if err != nil || len(trees) != 2 || trees[1].Tags["Event"] != "B" {
		t.Fatalf("%v %v", trees, err)
	}
	for name, src := range map[string]string{
		"illegal move in a variation": "1. e4 e5 (1... e4) *",
		"variation before any move":   "(1. e4) 1. d4 *",
		"unclosed variation":          "1. e4 (1. d4 d5 *",
		"stray close":                 "1. e4 ) e5 *",
	} {
		if _, err := ParseTreesString(src); err == nil {
			t.Errorf("%s: no error", name)
		} else if pe, ok := err.(*ParseError); !ok || pe.Game != 1 {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestTreeStringRoundTrip(t *testing.T) {
	src := `[Event "Test"]
[Orientation "white"]

1. e4 e5 (1... c5 2. Nf3 (2. c3 d5) 2... d6) 2. Nf3 (2. f4 exf4) 2... Nc6 3. Bb5 *
`
	trees, err := ParseTreesString(src)
	if err != nil {
		t.Fatal(err)
	}
	out := trees[0].String()
	if !strings.Contains(out, `[Orientation "white"]`) {
		t.Errorf("a tag was lost:\n%s", out)
	}
	if !strings.Contains(strings.Join(strings.Fields(out), " "), "1. e4 e5 (1... c5 2. Nf3 (2. c3 d5) 2... d6) 2. Nf3 (2. f4 exf4) 2... Nc6 3. Bb5 *") {
		t.Errorf("movetext:\n%s", out)
	}
	again, err := ParseTreesString(out)
	if err != nil {
		t.Fatal(err)
	}
	if a, b := strings.Join(linesOf(trees[0]), "|"), strings.Join(linesOf(again[0]), "|"); a != b {
		t.Errorf("round trip changed the lines:\n%s\n%s", a, b)
	}
}

func TestTreeStringBlackToMoveStart(t *testing.T) {
	trees, err := ParseTreesString("[SetUp \"1\"]\n[FEN \"rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq - 0 1\"]\n\n1... e5 (1... c5) 2. Nf3 *\n")
	if err != nil {
		t.Fatal(err)
	}
	if out := trees[0].String(); !strings.Contains(out, "1... e5 (1... c5) 2. Nf3 *") {
		t.Errorf("movetext:\n%s", out)
	}
}

func TestParseStillSkipsVariations(t *testing.T) {
	g, err := ParseOne("1. e4 e5 (1... c5 2. Nf3) 2. Nf3 *")
	if err != nil || len(g.Moves) != 3 {
		t.Fatalf("%v %v", g, err)
	}
}

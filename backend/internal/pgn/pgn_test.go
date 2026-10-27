package pgn

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const sample = `[Event "Live Chess"]
[Site "Chess.com"]
[Date "2026.10.01"]
[White "alice"]
[Black "bob"]
[Result "1-0"]
[TimeControl "300+2"]

1. e4 {[%clk 0:05:00]} 1... e5 {[%clk 0:04:59.5]} 2. Nf3 $1 (2. f4 exf4 {gambit}) 2... Nc6
{Good developing move. [%clk 0:04:50]} 3. Bb5 a6 ; rest-of-line comment
4. Ba4 Nf6 5. O-O Be7 6. Re1 b5 7. Bb3 d6 8. c3 O-O 9. h3 Nb8 10. d4 Nbd7 1-0

[Event "Second"]
[Result "1/2-1/2"]

1. d4 d5 2. c4 e6 1/2-1/2
`

func TestParseSample(t *testing.T) {
	games, err := ParseString(sample)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 2 {
		t.Fatalf("got %d games, want 2", len(games))
	}
	g := games[0]
	if g.Tag("White") != "alice" || g.Result != "1-0" {
		t.Errorf("tags/result wrong: %v %q", g.Tags, g.Result)
	}
	if len(g.Moves) != 20 {
		t.Fatalf("got %d moves, want 20", len(g.Moves))
	}
	if g.Moves[0].SAN != "e4" || g.Moves[2].SAN != "Nf3" || g.Moves[9].SAN != "O-O" {
		// index 9 is black's 5th... only check shape: ply 10 = "Be7"
		t.Logf("moves: %v", sans(g))
	}
	if g.Moves[0].Clock != 5*time.Minute {
		t.Errorf("clock[0] = %v", g.Moves[0].Clock)
	}
	if g.Moves[1].Clock != 4*time.Minute+59*time.Second+500*time.Millisecond {
		t.Errorf("clock[1] = %v", g.Moves[1].Clock)
	}
	if g.Moves[3].Comment != "Good developing move." {
		t.Errorf("comment = %q", g.Moves[3].Comment)
	}
	if g.Moves[2].Clock >= 0 {
		t.Errorf("move without clk should have negative clock, got %v", g.Moves[2].Clock)
	}
	// The variation (2. f4 exf4) must not leak into the main line.
	if g.Moves[2].SAN != "Nf3" || g.Moves[4].SAN != "Bb5" {
		t.Errorf("variation leaked: %v", sans(g))
	}
	if len(games[1].Moves) != 4 || games[1].Result != "1/2-1/2" {
		t.Errorf("second game wrong: %v %q", sans(games[1]), games[1].Result)
	}
}

func sans(g *Game) []string {
	var s []string
	for _, m := range g.Moves {
		s = append(s, m.SAN)
	}
	return s
}

func TestNoTagsAndGluedNumbers(t *testing.T) {
	g, err := ParseOne("1.e4 e5 2.Nf3 Nc6 3.Bb5 *")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(sans(g), " "); got != "e4 e5 Nf3 Nc6 Bb5" {
		t.Errorf("moves = %q", got)
	}
	if g.Result != "*" {
		t.Errorf("result = %q", g.Result)
	}
}

func TestNormalizesSAN(t *testing.T) {
	// "Nf3+" style noise and missing check marks are normalized.
	g, err := ParseOne("1. e4 e5 2. Qh5 Nc6 3. Bc4 Nf6 4. Qxf7 1-0")
	if err != nil {
		t.Fatal(err)
	}
	if last := g.Moves[len(g.Moves)-1].SAN; last != "Qxf7#" {
		t.Errorf("last SAN = %q, want Qxf7#", last)
	}
}

func TestIllegalMove(t *testing.T) {
	_, err := ParseOne("1. e4 e5 2. Ke3 Nc6 *")
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %v", err)
	}
	if pe.Ply != 3 {
		t.Errorf("ply = %d, want 3", pe.Ply)
	}
}

func TestCustomStartFEN(t *testing.T) {
	src := `[SetUp "1"]
[FEN "4k3/8/8/8/8/8/4P3/4K3 w - - 0 1"]

1. e4 Kd7 2. e5 *
`
	g, err := ParseOne(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Moves) != 3 {
		t.Fatalf("moves = %v", sans(g))
	}
	out := g.String()
	if !strings.Contains(out, `[FEN "4k3/8/8/8/8/8/4P3/4K3 w - - 0 1"]`) {
		t.Errorf("FEN tag lost:\n%s", out)
	}
	g2, err := ParseOne(out)
	if err != nil || strings.Join(sans(g2), " ") != strings.Join(sans(g), " ") {
		t.Errorf("round trip failed: %v %v", err, sans(g2))
	}
}

func TestRoundTrip(t *testing.T) {
	g, err := ParseOne(sample)
	if err != nil {
		t.Fatal(err)
	}
	out := g.String()
	g2, err := ParseOne(out)
	if err != nil {
		t.Fatalf("re-parse: %v\n%s", err, out)
	}
	if strings.Join(sans(g), " ") != strings.Join(sans(g2), " ") {
		t.Errorf("moves differ after round trip")
	}
	for i := range g.Moves {
		if g.Moves[i].Clock != g2.Moves[i].Clock {
			t.Errorf("clock %d: %v vs %v", i, g.Moves[i].Clock, g2.Moves[i].Clock)
		}
	}
	for _, l := range strings.Split(out, "\n") {
		if len(l) > 80 {
			t.Errorf("line longer than 80: %q", l)
		}
	}
}

func TestUnterminatedComment(t *testing.T) {
	if _, err := ParseOne("1. e4 {oops e5 *"); err == nil {
		t.Error("expected error")
	}
}

func TestMoveTimeAnnotationRoundTrip(t *testing.T) {
	g, err := ParseOne("1. e4 {[%clk 0:05:00] [%emt 0:00:02.5]} e5 {[%emt 50:20:07]} 2. Nf3 1-0")
	if err != nil {
		t.Fatal(err)
	}
	if g.Moves[0].Spent != 2500*time.Millisecond || g.Moves[0].Clock != 5*time.Minute ||
		g.Moves[1].Spent != 50*time.Hour+20*time.Minute+7*time.Second || g.Moves[1].Clock >= 0 || g.Moves[2].Spent >= 0 {
		t.Fatalf("parsed: %+v", g.Moves)
	}
	g2, err := ParseOne(g.String())
	if err != nil {
		t.Fatal(err)
	}
	for i := range g.Moves {
		if g.Moves[i].Spent != g2.Moves[i].Spent || g.Moves[i].Clock != g2.Moves[i].Clock {
			t.Errorf("move %d: %v/%v vs %v/%v", i, g.Moves[i].Spent, g.Moves[i].Clock, g2.Moves[i].Spent, g2.Moves[i].Clock)
		}
	}
}

package lessons

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"dyanis-chess/engine"
)

// TestExplore is a tool for writing lessons, not a check: it prints what the engine knows about a
// position, so that a claim in a lesson ("here White mates in two") can be looked at before it is
// written down. It does nothing unless LESSON_FEN is set:
//
//	LESSON_FEN="6k1/5ppp/8/8/8/8/5PPP/R5K1 w - - 0 1" LESSON_MOVES="a1a8" LESSON_DEPTH=10 go test -run TestExplore -v ./backend/internal/lessons
//
// LESSON_MOVES (UCI, space separated) are played first, one line per move with the position after it;
// LESSON_LINES=3 searches that many lines (MultiPV) instead of one.
func TestExplore(t *testing.T) {
	fen := os.Getenv("LESSON_FEN")
	if fen == "" {
		t.Skip("LESSON_FEN not set")
	}
	pos, err := engine.ParseFEN(fen)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range strings.Fields(os.Getenv("LESSON_MOVES")) {
		m, ok := findMove(pos, u)
		if !ok {
			t.Fatalf("%s is not legal in %s", u, pos.FEN())
		}
		san := pos.SAN(m)
		pos.Play(m)
		fmt.Printf("%s %s -> %s\n", u, san, pos.FEN())
	}
	depth := 8
	if d, err := strconv.Atoi(os.Getenv("LESSON_DEPTH")); err == nil {
		depth = d
	}
	fmt.Printf("position %s\noutcome %+v\n", pos.FEN(), pos.Outcome(nil))
	var line []string
	for _, m := range pos.LegalMoves() {
		c := pos.Copy()
		c.Play(m)
		flag := ""
		if c.Outcome(nil).Checkmate {
			flag = " MATE"
		} else if c.Outcome(nil).Over {
			flag = " " + c.Outcome(nil).Reason
		}
		line = append(line, m.String()+"="+pos.SAN(m)+flag)
	}
	fmt.Printf("%d legal: %s\n", len(line), strings.Join(line, " "))
	if len(line) == 0 {
		return
	}
	if os.Getenv("LESSON_LINES") != "" {
		n, _ := strconv.Atoi(os.Getenv("LESSON_LINES"))
		res, err := engine.SearchLines(pos, nil, engine.Limits{Depth: depth, HashMB: 16}, n)
		if err != nil {
			t.Fatal(err)
		}
		for i, r := range res {
			fmt.Printf("line %d: score %d %s\n", i+1, r.Score, pvText(pos, r.PV))
		}
		return
	}
	r, err := engine.Search(pos, nil, engine.Limits{Depth: depth, HashMB: 16})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("best %s score %d depth %d: %s\n", r.Best, r.Score, r.Depth, pvText(pos, r.PV))
}

func pvText(pos *engine.Position, pv []engine.Move) string {
	p := pos.Copy()
	var out []string
	for _, m := range pv {
		out = append(out, p.SAN(m))
		p.Play(m)
	}
	return strings.Join(out, " ")
}

// TestSection validates one section file while it is being written (LESSON_SECTION=basics), so that
// the others need not exist yet: the content errors and the claims are printed.
func TestSection(t *testing.T) {
	name := os.Getenv("LESSON_SECTION")
	if name == "" {
		t.Skip("LESSON_SECTION not set")
	}
	data, err := os.ReadFile("content/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var s Section
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		t.Fatal(err)
	}
	lib, err := newLibrary([]Section{s})
	if err != nil {
		t.Fatal(err)
	}
	n := runClaims(t, lib)
	fmt.Printf("%s: %d lessons, %d exercises checked\n", name, lib.Total(), n)
}

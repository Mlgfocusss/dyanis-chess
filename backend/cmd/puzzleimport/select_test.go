package main

import (
	"strconv"
	"strings"
	"testing"
)

const header = "PuzzleId,FEN,Moves,Rating,RatingDeviation,Popularity,NbPlays,Themes,GameUrl,OpeningTags\n"

// A real puzzle (Lichess 00008); rows below reuse its position.
const (
	fen   = "r6k/pp2r2p/4Rp1Q/3p4/8/1N1P2R1/PqP2bPP/7K b - - 0 24"
	moves = "f2g3 e6e7 b2b1 b3c1 b1c1 h6c1"
)

func row(id string, rating, dev, pop, plays int, mv string) string {
	f := []string{id, fen, mv, strconv.Itoa(rating), strconv.Itoa(dev), strconv.Itoa(pop), strconv.Itoa(plays), "crushing long", "https://lichess.org/x", ""}
	return strings.Join(f, ",") + "\n"
}

func TestPick(t *testing.T) {
	c := criteria{maxDeviation: 90, minPopularity: 85, minPlays: 200, bucket: 100, perBucket: 2}
	in := header +
		row("a0001", 1010, 80, 95, 1000, moves) + // bucket 10
		row("a0002", 1020, 80, 99, 1000, moves) + // bucket 10: best
		row("a0003", 1030, 80, 90, 1000, moves) + // bucket 10: third, over the cap
		row("a0004", 1040, 80, 99, 5000, moves) + // bucket 10: best, more plays
		row("b0001", 1510, 80, 95, 1000, moves) + // bucket 15
		row("c0001", 1520, 120, 99, 1000, moves) + // rating not settled
		row("c0002", 1530, 80, 50, 1000, moves) + // not popular
		row("c0003", 1540, 80, 99, 10, moves) + // hardly played
		row("c0004", 1550, 80, 99, 1000, "f2g3 e6a1") + // illegal line
		"broken,row\n"

	got, st, err := pick(strings.NewReader(in), c)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range got {
		ids = append(ids, p.ID)
	}
	if want := "a0002 a0004 b0001"; strings.Join(ids, " ") != want {
		t.Errorf("kept %v, want %s", ids, want)
	}
	if st.rows != 10 || st.filtered != 3 || st.badLine != 1 || st.malformed != 1 {
		t.Errorf("stats %+v", st)
	}
	if got[0].Themes != "crushing long" || got[0].Moves != moves || got[0].Plays != 1000 {
		t.Errorf("fields: %+v", got[0])
	}
}

func TestPickRejectsOtherFiles(t *testing.T) {
	if _, _, err := pick(strings.NewReader("a,b,c\n1,2,3\n"), criteria{bucket: 100, perBucket: 1}); err == nil {
		t.Error("a CSV without the puzzle columns was accepted")
	}
	if _, _, err := pick(strings.NewReader(""), criteria{bucket: 100, perBucket: 1}); err == nil {
		t.Error("an empty file was accepted")
	}
}

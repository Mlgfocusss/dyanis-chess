package search

import (
	"testing"
	"time"

	"dyanis-chess/internal/board"
	"dyanis-chess/internal/movegen"
)

func TestIsRepetitionSeesGameHistory(t *testing.T) {
	tt := NewTranspositionTable()
	tt.SetGameHistory([]uint64{11, 22, 33, 99}) // 99 is the root itself
	tt.recordPathHash(0, 99)
	cases := []struct {
		ply, clock int
		hash       uint64
		want       bool
	}{
		{1, 10, 33, true},  // ply 1 is the root's child: 33 is ply -1, inside the window
		{0, 10, 33, false}, // the root is never a repetition
		{1, 1, 33, false},  // clock 1: the window stops at the root
		{1, 2, 33, true},   // clock 2: reaches ply -1
		{1, 2, 22, false},  // ... but not ply -2
		{1, 3, 22, true},
		{1, 10, 11, true},
		{1, 10, 44, false},
		{1, 10, 99, true}, // the root, seen from its child
	}
	for _, c := range cases {
		if got := tt.isRepetition(c.ply, c.hash, c.clock); got != c.want {
			t.Errorf("isRepetition(ply %d, %d, clock %d) = %v, want %v", c.ply, c.hash, c.clock, got, c.want)
		}
	}
	if tt.NewSearch().isRepetition(1, 33, 10) != true {
		t.Error("NewSearch must carry the game history over")
	}
	tt.SetGameHistory(nil)
	if tt.isRepetition(1, 33, 10) {
		t.Error("SetGameHistory(nil) must clear it")
	}
}

// White is winning (queen up). Every white move but one leads to a
// position the game has already been through; with the game history the
// search must play the remaining move and keep a winning score instead of
// walking into the repetition.
func TestSearchAvoidsGameRepetitionWhenWinning(t *testing.T) {
	const fen = "7k/8/5K2/8/8/8/8/6Q1 w - - 50 80"
	root, err := board.FromFEN(fen)
	if err != nil {
		t.Fatal(err)
	}
	var keep board.Move
	var seen []uint64
	for _, m := range movegen.GenerateLegalMoves(root) {
		if m.String() == "f6e5" {
			keep = m
			continue
		}
		u := root.MakeMove(m)
		seen = append(seen, root.Hash())
		root.UnmakeMove(m, u)
	}
	if keep == (board.Move{}) {
		t.Fatal("f6e5 is not legal in the test position")
	}
	hist := append(seen, root.Hash())

	check := func(name string, m board.Move, score int) {
		t.Helper()
		if m != keep {
			t.Errorf("%s: played %v, want %v (every other move repeats the game)", name, m, keep)
		}
		if score < 500 {
			t.Errorf("%s: score %d, want a winning score", name, score)
		}
	}

	tt := NewTranspositionTable()
	tt.SetGameHistory(hist)
	var info SearchInfo
	m, err := BestMoveTimedInfoTT(root, 6, time.Hour, tt.NewSearch(), func(i SearchInfo) { info = i })
	if err != nil {
		t.Fatal(err)
	}
	check("single thread", m, info.Score)

	tt = NewTranspositionTable()
	tt.SetGameHistory(hist)
	m, err = BestMoveTimedMTShared(root, 6, time.Hour, 2, tt, func(i SearchInfo) { info = i })
	if err != nil {
		t.Fatal(err)
	}
	check("lazy smp", m, info.Score)

	// Without the history the same search is free to pick anything; it
	// must still find a legal winning move (sanity check of the setup).
	m, err = BestMoveTimedInfoTT(root, 6, time.Hour, NewTranspositionTable(), func(i SearchInfo) { info = i })
	if err != nil || info.Score < 500 {
		t.Fatalf("no history: move %v score %d err %v", m, info.Score, err)
	}
}

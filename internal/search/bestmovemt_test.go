package search

import (
	"testing"
	"time"

	"dyanis-chess/internal/board"
)

// TestBestMoveTimedMTFallsBackSingleThreaded checks that threads<=1
// really does take the exact BestMoveTimedInfo code path (see
// BestMoveTimedMT's own comment) rather than merely behaving
// similarly — same mate-in-1 position several other tests already use.
func TestBestMoveTimedMTFallsBackSingleThreaded(t *testing.T) {
	const fen = "6k1/5ppp/8/8/8/8/8/R5K1 w - - 0 1"
	b, err := board.FromFEN(fen)
	if err != nil {
		t.Fatalf("FEN parse failed: %v", err)
	}

	m, err := BestMoveTimedMT(b, 3, time.Second, 1, nil)
	if err != nil {
		t.Fatalf("BestMoveTimedMT: %v", err)
	}
	if want := "a1a8"; m.String() != want {
		t.Errorf("expected mate-in-1 move %s, got %s", want, m)
	}
}

// TestBestMoveTimedMTStillFindsMate runs the same mate-in-1 position
// through several concurrent threads — the point isn't that
// multi-threading finds something a single thread couldn't (it's a
// mate in 1; any thread alone would find it instantly), it's that
// running several threads concurrently, sharing one hash table,
// doesn't corrupt the search into missing an answer every single
// thread would trivially find on its own.
func TestBestMoveTimedMTStillFindsMate(t *testing.T) {
	const fen = "6k1/5ppp/8/8/8/8/8/R5K1 w - - 0 1"
	b, err := board.FromFEN(fen)
	if err != nil {
		t.Fatalf("FEN parse failed: %v", err)
	}

	m, err := BestMoveTimedMT(b, 3, time.Second, 4, nil)
	if err != nil {
		t.Fatalf("BestMoveTimedMT: %v", err)
	}
	if want := "a1a8"; m.String() != want {
		t.Errorf("expected mate-in-1 move %s, got %s", want, m)
	}
}

// TestBestMoveTimedMTKeepsWinningTacticAtDepth is the same "capture
// the hanging rook" regression check the single-threaded search
// already has (TestPrefersWinningMaterial /
// TestNullMovePruningKeepsCorrectAnswerAtDepth), run through several
// concurrent threads instead of one.
func TestBestMoveTimedMTKeepsWinningTacticAtDepth(t *testing.T) {
	const fen = "3r2k1/8/8/8/8/8/8/3Q2K1 w - - 0 1"
	b, err := board.FromFEN(fen)
	if err != nil {
		t.Fatalf("FEN parse failed: %v", err)
	}

	m, err := BestMoveTimedMT(b, 5, time.Second, 4, nil)
	if err != nil {
		t.Fatalf("BestMoveTimedMT: %v", err)
	}
	if want := "d1d8"; m.String() != want {
		t.Errorf("expected the engine to capture the hanging rook with %s, got %s", want, m)
	}
}

// TestPersistentTTAcrossMovesStillFindsCorrectAnswers simulates what
// a UCI session now does: ONE TranspositionTable kept alive across
// several separate move searches, each on a DIFFERENT position (as a
// real game would produce) — the shared hash table must never cause
// a WRONG answer on a later position just because it holds entries
// left over from an earlier, unrelated one.
func TestPersistentTTAcrossMovesStillFindsCorrectAnswers(t *testing.T) {
	tt := NewTranspositionTable()

	const mateFen = "6k1/5ppp/8/8/8/8/8/R5K1 w - - 0 1"
	mateBoard, err := board.FromFEN(mateFen)
	if err != nil {
		t.Fatalf("FEN parse failed: %v", err)
	}
	m, err := BestMoveInfoTT(mateBoard, 1, tt.NewSearch(), nil)
	if err != nil {
		t.Fatalf("BestMoveInfoTT: %v", err)
	}
	if want := "a1a8"; m.String() != want {
		t.Errorf("position 1: expected mate-in-1 move %s, got %s", want, m)
	}

	const rookFen = "3r2k1/8/8/8/8/8/8/3Q2K1 w - - 0 1"
	rookBoard, err := board.FromFEN(rookFen)
	if err != nil {
		t.Fatalf("FEN parse failed: %v", err)
	}
	m, err = BestMoveInfoTT(rookBoard, 4, tt.NewSearch(), nil)
	if err != nil {
		t.Fatalf("BestMoveInfoTT: %v", err)
	}
	if want := "d1d8"; m.String() != want {
		t.Errorf("position 2 (same tt reused, unrelated to position 1): expected %s, got %s", want, m)
	}
}

// TestBestMoveTimedMTSharedKeepsTTWarmAcrossCalls is
// TestPersistentTTAcrossMovesStillFindsCorrectAnswers's multi-threaded
// counterpart: the same *TranspositionTable reused across repeated
// BestMoveTimedMTShared calls (as UCI's movetime path now does every
// "go") must keep producing the correct answer, not just on the
// first call.
func TestBestMoveTimedMTSharedKeepsTTWarmAcrossCalls(t *testing.T) {
	tt := NewTranspositionTable()

	const fen = "3r2k1/8/8/8/8/8/8/3Q2K1 w - - 0 1"
	b, err := board.FromFEN(fen)
	if err != nil {
		t.Fatalf("FEN parse failed: %v", err)
	}

	for i := 0; i < 2; i++ {
		m, err := BestMoveTimedMTShared(b, 5, time.Second, 4, tt, nil)
		if err != nil {
			t.Fatalf("call %d: BestMoveTimedMTShared: %v", i, err)
		}
		if want := "d1d8"; m.String() != want {
			t.Errorf("call %d: expected %s, got %s", i, want, m)
		}
	}
}

// BestMoveTimedMT calls, each itself multi-threaded, all at once, on
// DIFFERENT board positions — this is the real target for -race,
// exercising exactly the bug class BestMoveTimedMT's own comment
// calls out: each thread must operate on its own b.Copy(), never the
// caller's shared *board.Board, or concurrent MakeMove/UnmakeMove
// calls would corrupt shared state, not just race on it. Two
// different starting positions (rather than the same one twice) also
// means two entirely independent goroutine groups genuinely
// overlapping in time, not just two calls that happen to run back to
// back.
//
//	go test -race -run TestBestMoveTimedMTConcurrentSearchesDontCorruptBoards ./internal/search
func TestBestMoveTimedMTConcurrentSearchesDontCorruptBoards(t *testing.T) {
	positions := []struct {
		fen  string
		want string
	}{
		{"6k1/5ppp/8/8/8/8/8/R5K1 w - - 0 1", "a1a8"},
		{"3r2k1/8/8/8/8/8/8/3Q2K1 w - - 0 1", "d1d8"},
	}

	results := make([]string, len(positions))
	errs := make([]error, len(positions))
	done := make(chan int, len(positions))

	for i, p := range positions {
		i, p := i, p
		go func() {
			b, err := board.FromFEN(p.fen)
			if err != nil {
				errs[i] = err
				done <- i
				return
			}
			m, err := BestMoveTimedMT(b, 4, time.Second, 4, nil)
			results[i] = m.String()
			errs[i] = err
			done <- i
		}()
	}
	for range positions {
		<-done
	}

	for i, p := range positions {
		if errs[i] != nil {
			t.Fatalf("position %d: BestMoveTimedMT: %v", i, errs[i])
		}
		if results[i] != p.want {
			t.Errorf("position %d: expected %s, got %s", i, p.want, results[i])
		}
	}
}

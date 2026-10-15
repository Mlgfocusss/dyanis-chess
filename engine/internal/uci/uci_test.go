package uci

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
	"time"

	"dyanis-chess/engine/internal/board"
	"dyanis-chess/engine/internal/book"
)

// encode mirrors Polyglot's 16-bit move packing (same scheme used in
// book_test.go and search's opening_book_test.go), for building
// synthetic book entries without hand-computing bit patterns.
func encode(fromFile, fromRank, toFile, toRank int, promo uint16) uint16 {
	return uint16(toFile) | uint16(toRank)<<3 | uint16(fromFile)<<6 | uint16(fromRank)<<9 | promo<<12
}

func run(t *testing.T, input string, depth, threads int, bk book.Source) string {
	t.Helper()
	var out bytes.Buffer
	Loop(bufio.NewReader(strings.NewReader(input)), bufio.NewWriter(&out), depth, threads, bk)
	return out.String()
}

func TestUCIHandshake(t *testing.T) {
	got := run(t, "uci\nquit\n", 3, 1, nil)
	if !strings.Contains(got, "id name") || !strings.Contains(got, "id author") || !strings.Contains(got, "uciok") {
		t.Errorf("expected a full uci handshake, got %q", got)
	}
}

func TestIsReady(t *testing.T) {
	got := run(t, "isready\nquit\n", 3, 1, nil)
	if !strings.Contains(got, "readyok") {
		t.Errorf("expected readyok, got %q", got)
	}
}

func TestPositionStartposMovesThenGo(t *testing.T) {
	got := run(t, "position startpos moves e2e4 e7e5\ngo depth 1\nquit\n", 3, 1, nil)
	if !strings.Contains(got, "bestmove ") {
		t.Errorf("expected a bestmove line, got %q", got)
	}
}

func TestPositionFEN(t *testing.T) {
	got := run(t, "position fen 4k3/8/8/8/8/8/8/4K2R w K - 0 1 moves\ngo depth 1\nquit\n", 2, 1, nil)
	if !strings.Contains(got, "bestmove ") {
		t.Errorf("expected a bestmove line, got %q", got)
	}
}

func TestGoDepthOverridesSessionDefault(t *testing.T) {
	got := run(t, "position startpos\ngo depth 2\nquit\n", 5, 1, nil)
	if !strings.Contains(got, "bestmove ") {
		t.Errorf("expected a bestmove line, got %q", got)
	}
}

func TestGoWithoutDepthUsesSessionDefault(t *testing.T) {
	got := run(t, "position startpos\ngo\nquit\n", 1, 1, nil)
	if !strings.Contains(got, "bestmove ") {
		t.Errorf("expected a bestmove line using the default depth, got %q", got)
	}
}

func TestUnknownCommandIsIgnoredNotFatal(t *testing.T) {
	got := run(t, "setoption name Foo value Bar\nisready\nquit\n", 3, 1, nil)
	if !strings.Contains(got, "readyok") {
		t.Errorf("unknown command should be ignored, expected readyok afterward, got %q", got)
	}
}

func TestGoPrefersBookMove(t *testing.T) {
	start := board.NewInitialBoard()
	bk := book.New([]book.Entry{
		{Key: start.Hash(), Move: encode(4, 1, 4, 3, 0), Weight: 10}, // e2e4
	})

	got := run(t, "position startpos\ngo depth 3\nquit\n", 3, 1, bk)
	if !strings.Contains(got, "bestmove e2e4") {
		t.Errorf("expected the book move e2e4, got %q", got)
	}
}

func TestQuitStopsTheLoop(t *testing.T) {
	// If "quit" didn't stop the loop, this would hang reading past
	// EOF instead of returning, and the test would time out.
	got := run(t, "quit\nisready\n", 3, 1, nil)
	if strings.Contains(got, "readyok") {
		t.Error("commands after quit should never be processed")
	}
}

func TestGoMovetimeUsesIterativeDeepening(t *testing.T) {
	got := run(t, "position startpos\ngo movetime 200\nquit\n", 5, 1, nil)
	if !strings.Contains(got, "bestmove ") {
		t.Errorf("expected a bestmove line from a movetime-based search, got %q", got)
	}
}

// TestUCIHandshakeAdvertisesThreadsOption checks the "uci" response
// lists the Threads spin option — a GUI needs to see this to offer
// setting it at all.
func TestUCIHandshakeAdvertisesThreadsOption(t *testing.T) {
	got := run(t, "uci\nquit\n", 3, 1, nil)
	if !strings.Contains(got, "option name Threads") {
		t.Errorf("expected a Threads option to be advertised, got %q", got)
	}
}

// TestSetOptionThreadsIsAccepted checks "setoption name Threads value
// N" doesn't break the session (same "not fatal" contract
// TestUnknownCommandIsIgnoredNotFatal already covers for genuinely
// unknown commands) and that a movetime-based go still completes
// normally afterward. Full multi-threaded search correctness is
// covered by the search package's own TestBestMoveTimedMT* tests, not
// re-verified here — this only checks the UCI plumbing wires the
// value through without erroring.
func TestSetOptionThreadsIsAccepted(t *testing.T) {
	got := run(t, "setoption name Threads value 4\nposition startpos\ngo movetime 200\nquit\n", 3, 1, nil)
	if !strings.Contains(got, "bestmove ") {
		t.Errorf("expected a bestmove line after setting Threads, got %q", got)
	}
}

// TestSetOptionThreadsIgnoresInvalidValue checks a garbage value
// (non-numeric, or below 1) is ignored rather than crashing the
// session or leaving threads at some invalid state — mirrors
// handleSetOption's own "err != nil || n < 1: return" guard.
func TestSetOptionThreadsIgnoresInvalidValue(t *testing.T) {
	got := run(t, "setoption name Threads value banana\nisready\nquit\n", 3, 1, nil)
	if !strings.Contains(got, "readyok") {
		t.Errorf("expected an invalid Threads value to be ignored, not break the session, got %q", got)
	}
}

// TestSessionKeepsTranspositionTableWarmAcrossMoves is a plumbing
// smoke test for session.tt (see its own doc comment): several "go"
// commands in ONE session, each after a "position" that extends the
// previous one by one more move — the same shape a real game
// produces — must all keep completing normally. The actual claim
// that the table stays warm and correct across calls (not just that
// nothing crashes) is verified at the search-package level by
// TestPersistentTTAcrossMovesStillFindsCorrectAnswers and
// TestBestMoveTimedMTSharedKeepsTTWarmAcrossCalls — this test only
// confirms the session wiring itself doesn't break across repeated
// use, which a UCI-text-output test is the right level to check.
func TestSessionKeepsTranspositionTableWarmAcrossMoves(t *testing.T) {
	got := run(t, strings.Join([]string{
		"position startpos moves e2e4",
		"go depth 3",
		"position startpos moves e2e4 e7e5",
		"go depth 3",
		"position startpos moves e2e4 e7e5 g1f3",
		"go movetime 100",
		"quit",
	}, "\n")+"\n", 3, 1, nil)

	if count := strings.Count(got, "bestmove "); count != 3 {
		t.Errorf("expected 3 bestmove lines (one per go), got %d in %q", count, got)
	}
}

func TestClockBudgetMs(t *testing.T) {
	cases := []struct {
		name                   string
		timeMs, incMs, movesTo int
		want                   int
	}{
		{"even share, default 30 moves", 60000, 0, 0, 60000/30 - overheadMs},
		{"increment adds 3/4", 60000, 2000, 0, 60000/30 + 1500 - overheadMs},
		{"movestogo respected", 60000, 0, 10, 6000 - overheadMs},
		{"capped to a third of the clock", 1000, 5000, 0, 1000/3 - overheadMs},
		{"never below 1 ms", 20, 0, 0, 1},
	}
	for _, c := range cases {
		if got := clockBudgetMs(c.timeMs, c.incMs, c.movesTo); got != c.want {
			t.Errorf("%s: clockBudgetMs(%d,%d,%d) = %d, want %d", c.name, c.timeMs, c.incMs, c.movesTo, got, c.want)
		}
	}
}

// With only a game clock (no movetime, no depth), "go" must search by
// time and stay well inside it — not run the fixed session depth.
func TestGoWithClockUsesBudget(t *testing.T) {
	start := time.Now()
	got := run(t, "position fen r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1\ngo wtime 3000 btime 3000 winc 0 binc 0\nquit\n", 3, 1, nil)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("clock search took %v with 3s on the clock", elapsed)
	}
	if !strings.Contains(got, "bestmove ") {
		t.Errorf("expected a bestmove, got %q", got)
	}
	// The session depth is 3; a ~70 ms budget searches well past it.
	if !strings.Contains(got, "info depth 4 ") {
		t.Errorf("expected the clock search to pass the session depth of 3, got %q", got)
	}
}

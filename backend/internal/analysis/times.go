package analysis

import (
	"regexp"
	"strconv"

	"dyanis-chess/backend/internal/pgn"
)

// The time each move took, for the report. Two sources, in this order:
//
//  1. "[%emt h:mm:ss]" on the move: the time itself (our own online games write it).
//  2. "[%clk h:mm:ss]": the clock left after the move. The move's time is then the clock the
//     same side had after its previous move, plus the increment, minus the clock now. The first
//     move of each side starts from the base time of the TimeControl tag. When that tag has no
//     increment ("300", "-", "1/86400", "40/7200:3600": we only read "base" and "base+inc")
//     the increment is guessed from the pairs: an increment shows as the clock going UP over a
//     move, and a move made at once is the one that shows it best.
//
// A move whose time cannot be worked out has no time (the report then leaves the field out).

var timeControlRe = regexp.MustCompile(`^\s*(\d+)(?:\+(\d+))?\s*$`)

// maxGuessedIncMs bounds the guessed increment: a "rise" of the clock beyond it is not an increment.
const maxGuessedIncMs = 180_000

// moveTimes returns the time of every move of g in milliseconds, -1 where unknown. startBlack says
// that Black moves first (the game began from a position with Black to move).
func moveTimes(g *pgn.Game, startBlack bool) []int64 {
	n := len(g.Moves)
	out := make([]int64, n)
	for i := range out {
		out[i] = -1
	}
	if n == 0 {
		return out
	}

	baseMs, incMs := int64(-1), int64(-1)
	if m := timeControlRe.FindStringSubmatch(g.Tag("TimeControl")); m != nil {
		if b, err := strconv.ParseInt(m[1], 10, 64); err == nil && b > 0 {
			baseMs = b * 1000
		}
		incMs = 0
		if m[2] != "" {
			if v, err := strconv.ParseInt(m[2], 10, 64); err == nil {
				incMs = v * 1000
			}
		}
	}

	side := func(i int) int {
		if startBlack {
			return (i + 1) % 2
		}
		return i % 2
	}

	// the clock-based times need the increment, known from the tag or guessed from the clocks
	if incMs < 0 {
		incMs = guessIncrement(g, side)
	}

	var prev [2]int64 // clock after the side's previous move, -1 unknown
	prev[0], prev[1] = baseMs, baseMs
	for i, m := range g.Moves {
		c := side(i)
		switch {
		case m.Spent >= 0:
			out[i] = m.Spent.Milliseconds()
		case m.Clock >= 0 && prev[c] >= 0:
			out[i] = max(0, prev[c]+incMs-m.Clock.Milliseconds())
		}
		if m.Clock >= 0 {
			prev[c] = m.Clock.Milliseconds()
		} else {
			prev[c] = -1
		}
	}
	return out
}

// guessIncrement estimates the per-move increment from the clocks alone: the largest rise of a
// side's clock between two of its consecutive moves, 0 if the clock only ever went down.
func guessIncrement(g *pgn.Game, side func(int) int) int64 {
	var prev [2]int64
	prev[0], prev[1] = -1, -1
	var inc int64
	for i, m := range g.Moves {
		c := side(i)
		if m.Clock < 0 {
			prev[c] = -1
			continue
		}
		cur := m.Clock.Milliseconds()
		if prev[c] >= 0 && cur-prev[c] > inc {
			inc = cur - prev[c]
		}
		prev[c] = cur
	}
	if inc > maxGuessedIncMs {
		return 0
	}
	return inc
}

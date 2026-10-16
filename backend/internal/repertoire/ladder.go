// Package repertoire is the opening trainer: a person builds a tree of their
// own lines and then learns them by answering "what is your move here?" until
// they know them for good.
//
// The learning is spaced repetition on a ladder: a move answered right climbs
// one step and is asked again after a longer pause; a move answered wrong falls
// back two steps and is asked again in minutes.
package repertoire

import (
	"time"

	"dyanis-chess/backend/internal/store"
)

// delays[b] is how long a move that has reached step b rests before it is asked
// again. A move that was just failed comes back after delays[0] whatever step it
// falls to, so that it is asked again soon.
var delays = []time.Duration{
	10 * time.Minute,
	24 * time.Hour,
	3 * 24 * time.Hour,
	7 * 24 * time.Hour,
	14 * 24 * time.Hour,
	30 * 24 * time.Hour,
	90 * 24 * time.Hour,
}

// MaxBox is the top step of the ladder; a move there is asked every 90 days.
var MaxBox = len(delays) - 1

// Review gives the state of a move after one answer.
func Review(box, lapses int, correct bool, now time.Time) store.Card {
	if correct {
		box = min(box+1, MaxBox)
		return store.Card{Box: box, DueAt: now.Add(delays[box]), Lapses: lapses}
	}
	return store.Card{Box: max(box-2, 0), DueAt: now.Add(delays[0]), Lapses: lapses + 1}
}

// Learned says whether a move has been known well enough to stop worrying
// about it for a month.
func Learned(box int) bool { return box >= store.LearnedBox }

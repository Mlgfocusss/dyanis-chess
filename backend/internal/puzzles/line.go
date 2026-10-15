// Package puzzles serves the puzzle trainer: it picks a puzzle near a
// person's rating, prepares its line for the browser, and keeps the rating.
//
// The puzzles are the Lichess database (CC0). Each is a position, a
// solution line and a rating. The first move of the line is the opponent's
// (the one that creates the puzzle); the person plays the next, and the
// line alternates from there.
package puzzles

import (
	"fmt"
	"strings"

	"dyanis-chess/engine"
)

// Alt is another move that also solves the puzzle at one step: any move that
// gives checkmate is as good as the line's own move (the rule Lichess uses).
type Alt struct {
	UCI string `json:"uci"`
	SAN string `json:"san"`
	FEN string `json:"fen"`
}

// Step is one move of the line, ready for the browser, which has no rules
// of its own: every position it needs is here.
type Step struct {
	UCI string `json:"uci"`
	SAN string `json:"san"`
	FEN string `json:"fen"` // the position after the move

	// The person's steps only: the legal moves before the step, and the
	// other moves that give mate.
	Legal []string `json:"legal,omitempty"`
	Alt   []Alt    `json:"alt,omitempty"`
}

// Line is a puzzle prepared for the browser.
type Line struct {
	StartFEN    string `json:"fen"`         // before the opponent's first move
	Orientation string `json:"orientation"` // the person's colour
	Steps       []Step `json:"steps"`       // Steps[0] is the opponent's move; the person plays the odd ones
}

// BuildLine plays the solution on the board and records the position after
// every move. It fails when the line is not legal, which means a bad row.
func BuildLine(fen, moves string) (*Line, error) {
	pos, err := engine.ParseFEN(fen)
	if err != nil {
		return nil, fmt.Errorf("puzzle fen: %w", err)
	}
	ucis := strings.Fields(moves)
	if len(ucis) < 2 {
		return nil, fmt.Errorf("puzzle line has %d moves, need at least 2", len(ucis))
	}
	line := &Line{StartFEN: pos.FEN()}
	for i, u := range ucis {
		mine := i%2 == 1 // the person's turn
		legal := pos.LegalMoves()
		var mv engine.Move
		found := false
		for _, m := range legal {
			if m.String() == u {
				mv, found = m, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("puzzle move %d (%s) is not legal in %s", i+1, u, pos.FEN())
		}
		if i == 0 {
			// the person plays the colour that is to move after the opponent's move
			if pos.SideToMove() == engine.White {
				line.Orientation = "black"
			} else {
				line.Orientation = "white"
			}
		}
		st := Step{UCI: u, SAN: pos.SAN(mv)}
		if mine {
			for _, m := range legal {
				st.Legal = append(st.Legal, m.String())
			}
		}
		var alts []Alt
		if mine {
			for _, m := range legal {
				if m == mv {
					continue
				}
				c := pos.Copy()
				c.Play(m)
				if c.Outcome(nil).Checkmate {
					alts = append(alts, Alt{UCI: m.String(), SAN: pos.SAN(m), FEN: c.FEN()})
				}
			}
		}
		pos.Play(mv)
		st.FEN = pos.FEN()
		st.Alt = alts
		line.Steps = append(line.Steps, st)
	}
	return line, nil
}

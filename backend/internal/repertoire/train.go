package repertoire

import (
	"sort"
	"time"

	"dyanis-chess/backend/internal/store"
	"dyanis-chess/engine"
)

// Step is one move of a line with the position after it.
type Step struct {
	UCI string `json:"uci"`
	SAN string `json:"san"`
	FEN string `json:"fen"`
}

// Drill is one question of a training session: the line is played up to a
// position, and the person has to find their move there.
type Drill struct {
	Move   int64    `json:"move"`   // the card (the repertoire's move that is the answer)
	New    bool     `json:"new"`    // never asked before: it is shown first, then asked
	Path   []Step   `json:"path"`   // moves from the start position up to the question
	FEN    string   `json:"fen"`    // the position of the question
	Answer Step     `json:"answer"` // the repertoire's move
	Alt    []string `json:"alt"`    // the repertoire's other moves in the same position (UCI)
	Legal  []string `json:"legal"`  // legal moves in the position of the question (UCI)
}

// Selection is what a session consists of: ids of cards, in the order to ask them.
type Selection struct {
	Due, New []int64
	// DueTotal and NewTotal are how many there were before the limits were applied.
	DueTotal, NewTotal int
}

// Select picks the cards of a session from a repertoire's moves: the due ones
// (the longest overdue first), then up to maxNew new ones. New moves are taken
// in the order they were added to the repertoire: line after line, each from
// its start, since a move is always added after the moves before it.
func Select(moves []store.RepMove, now time.Time, maxNew, maxDue int) Selection {
	var due, fresh []store.RepMove
	for _, m := range moves {
		if !m.Mine {
			continue
		}
		switch {
		case !m.DueAt.Valid:
			fresh = append(fresh, m)
		case !m.DueAt.Time.After(now):
			due = append(due, m)
		}
	}
	sort.SliceStable(due, func(i, j int) bool { return due[i].DueAt.Time.Before(due[j].DueAt.Time) })
	sort.SliceStable(fresh, func(i, j int) bool { return fresh[i].ID < fresh[j].ID })

	sel := Selection{DueTotal: len(due), NewTotal: len(fresh)}
	for i, m := range due {
		if i >= maxDue {
			break
		}
		sel.Due = append(sel.Due, m.ID)
	}
	for _, m := range fresh {
		if len(sel.New) >= maxNew {
			break
		}
		sel.New = append(sel.New, m.ID)
	}
	return sel
}

// NextDue is when the first of the moves that rest now comes due (RFC 3339), or
// nil when none is waiting.
func NextDue(moves []store.RepMove, now time.Time) *string {
	var first time.Time
	for _, m := range moves {
		if m.Mine && m.DueAt.Valid && m.DueAt.Time.After(now) && (first.IsZero() || m.DueAt.Time.Before(first)) {
			first = m.DueAt.Time
		}
	}
	if first.IsZero() {
		return nil
	}
	s := first.UTC().Format(time.RFC3339)
	return &s
}

// BuildDrill makes the question for a card: the line from the start up to the
// card's position, the answer, the other repertoire moves there, and the legal
// moves (the browser has no rules of its own).
func BuildDrill(moves []store.RepMove, id int64, isNew bool) (Drill, bool) {
	byID := make(map[int64]store.RepMove, len(moves))
	for _, m := range moves {
		byID[m.ID] = m
	}
	card, ok := byID[id]
	if !ok {
		return Drill{}, false
	}
	var rev []Step
	for p := card.ParentID; p != 0; p = byID[p].ParentID {
		a, ok := byID[p]
		if !ok {
			return Drill{}, false
		}
		rev = append(rev, Step{UCI: a.UCI, SAN: a.SAN, FEN: a.FEN})
	}
	path := make([]Step, 0, len(rev))
	for i := len(rev) - 1; i >= 0; i-- {
		path = append(path, rev[i])
	}
	fen := engine.StartFEN
	if len(path) > 0 {
		fen = path[len(path)-1].FEN
	}
	pos, err := engine.ParseFEN(fen)
	if err != nil {
		return Drill{}, false
	}
	d := Drill{
		Move: card.ID, New: isNew, Path: path, FEN: fen,
		Answer: Step{UCI: card.UCI, SAN: card.SAN, FEN: card.FEN},
		Alt:    []string{}, Legal: []string{},
	}
	for _, m := range moves {
		if m.ParentID == card.ParentID && m.ID != card.ID {
			d.Alt = append(d.Alt, m.UCI)
		}
	}
	for _, m := range pos.LegalMoves() {
		d.Legal = append(d.Legal, m.String())
	}
	return d, true
}

// Gaps lists the lines that stop at the opponent's move: the person has no
// answer there yet. Each is the id of that last move.
func Gaps(moves []store.RepMove) []int64 {
	hasChild := map[int64]bool{}
	for _, m := range moves {
		hasChild[m.ParentID] = true
	}
	var out []int64
	for _, m := range moves {
		if !m.Mine && !hasChild[m.ID] {
			out = append(out, m.ID)
		}
	}
	return out
}

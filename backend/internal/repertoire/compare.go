package repertoire

import (
	"dyanis-chess/backend/internal/store"
	"dyanis-chess/engine"
)

// Statuses of a game compared with a repository of moves.
const (
	// StatusInBook: the game stayed in the repository to its last move.
	StatusInBook = "in-book"
	// StatusYouDeviated: at your own move the repository had an answer, and you played another.
	StatusYouDeviated = "you-deviated"
	// StatusOpponentDeviated: the opponent played a move other than those the repository expects.
	StatusOpponentDeviated = "opponent-deviated"
	// StatusBookEnded: the repository has no moves at this position at all: the line stops here.
	StatusBookEnded = "book-ended"
	// StatusEmpty: the repository has no moves yet.
	StatusEmpty = "empty"
	// StatusBroken: the game's moves cannot be replayed.
	StatusBroken = "broken"
)

// Expected is a move of the repository that was available at the position of the deviation.
type Expected struct {
	ID   int64  `json:"id"`   // the move in the tree
	UCI  string `json:"uci"`  //
	SAN  string `json:"san"`  //
	Mine bool   `json:"mine"` // the person's own move (a card), as opposed to an expected reply of the opponent
}

// Deviation says where a game left the repository and how.
type Deviation struct {
	Status string `json:"status"`
	// Ply is the half-move (1-based) at which the game left the repository; 0 when it did not.
	Ply int `json:"ply"`
	// Node is the move of the tree that the position before Ply stands after (0: the start).
	Node int64 `json:"node"`
	// Line is the moves played before Ply, in SAN.
	Line []string `json:"line"`
	// Played is the move that left the repository.
	Played *Step `json:"played,omitempty"`
	// Expected are the moves the repository had at that position.
	Expected []Expected `json:"expected"`
	// Moves is how many half-moves the game had in all.
	Moves int `json:"moves"`
}

// Compare follows a game's moves (UCI) through a repository's tree for a colour and
// reports where the game first stepped off it. A move is "mine" when its ply has the
// parity of the colour, as in the tree itself.
func Compare(moves []store.RepMove, color string, game []string) Deviation {
	dev := Deviation{Status: StatusInBook, Line: []string{}, Expected: []Expected{}, Moves: len(game)}
	if len(moves) == 0 {
		dev.Status = StatusEmpty
		return dev
	}
	children := map[int64][]store.RepMove{}
	for _, m := range moves {
		children[m.ParentID] = append(children[m.ParentID], m)
	}
	pos := engine.NewPosition()
	var node int64
	for i, u := range game {
		ply := i + 1
		var mv engine.Move
		found := false
		for _, m := range pos.LegalMoves() {
			if m.String() == u {
				mv, found = m, true
				break
			}
		}
		if !found {
			dev.Status = StatusBroken
			return dev
		}
		san := pos.SAN(mv)
		var next *store.RepMove
		for _, c := range children[node] {
			if c.UCI == u {
				c := c
				next = &c
				break
			}
		}
		if next != nil {
			dev.Line = append(dev.Line, san)
			node = next.ID
			pos.Play(mv)
			continue
		}
		// the game leaves the repository here
		dev.Ply, dev.Node = ply, node
		after := pos.Copy()
		after.Play(mv)
		dev.Played = &Step{UCI: u, SAN: san, FEN: after.FEN()}
		kids := children[node]
		for _, c := range kids {
			dev.Expected = append(dev.Expected, Expected{ID: c.ID, UCI: c.UCI, SAN: c.SAN, Mine: c.Mine})
		}
		mineTurn := (ply%2 == 1) == (color == "white")
		switch {
		case len(kids) == 0:
			dev.Status = StatusBookEnded
		case mineTurn:
			dev.Status = StatusYouDeviated
		default:
			dev.Status = StatusOpponentDeviated
		}
		return dev
	}
	return dev
}

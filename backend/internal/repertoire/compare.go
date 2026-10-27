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
	// Transposed says that the game came to a position of the repository by another order of
	// moves than the tree has (that is not leaving it); TransposedPly is the half-move at which
	// it first stood in a position of the repository that the tree reaches differently (0: never).
	Transposed    bool `json:"transposed"`
	TransposedPly int  `json:"transposedPly"`
}

// Compare follows a game's moves (UCI) through a repository for a colour and reports where the
// game stepped off it. A move is "mine" when its ply has the parity of the colour, as in the tree
// itself.
//
// The repository is looked at by positions, not by paths: a game that comes to a position of the
// repository by another order of moves (1.c4 e6 2.d4 Nf6 for 1.d4 Nf6 2.c4 e6) has not left it.
// The game left the repository after the last of its positions that the repository has; when it
// went away from the repository and came back through such a position, that is not a deviation.
func Compare(moves []store.RepMove, color string, game []string) Deviation {
	dev := Deviation{Status: StatusInBook, Line: []string{}, Expected: []Expected{}, Moves: len(game)}
	if len(moves) == 0 {
		dev.Status = StatusEmpty
		return dev
	}
	g := NewGraph(moves)

	// replay the game: the legal moves up to the first one that is not
	pos := engine.NewPosition()
	fens := []string{pos.FEN()} // fens[i]: the position after i plies
	keys := []string{startKey}
	var sans []string
	broken := false
	for _, u := range game {
		var mv engine.Move
		found := false
		for _, m := range pos.LegalMoves() {
			if m.String() == u {
				mv, found = m, true
				break
			}
		}
		if !found {
			broken = true
			break
		}
		sans = append(sans, pos.SAN(mv))
		pos.Play(mv)
		fens = append(fens, pos.FEN())
		keys = append(keys, PosKey(pos.FEN()))
	}
	n := len(sans)

	// in[i]: the position after i plies is the repository's; node[i]: a node of the tree standing in it
	in := make([]bool, n+1)
	node := make([]int64, n+1)
	in[0] = true
	for i := 1; i <= n; i++ {
		var next int64
		ok, jump := false, false
		if in[i-1] {
			if c, found := g.Reply(node[i-1], game[i-1]); found {
				next, ok, jump = c.ID, true, c.ParentID != node[i-1]
			}
		}
		if !ok && g.Has(keys[i]) {
			next, ok, jump = g.Rep(keys[i]), true, true
		}
		if !ok {
			continue
		}
		in[i], node[i] = true, next
		if !in[i-1] {
			jump = true // back from outside the repository
		}
		if jump && dev.TransposedPly == 0 {
			dev.TransposedPly = i
		}
	}
	dev.Transposed = dev.TransposedPly > 0

	last := 0
	for i := n; i > 0; i-- {
		if in[i] {
			last = i
			break
		}
	}
	dev.Line = append(dev.Line, sans[:last]...)
	if last == n {
		if broken {
			dev.Status = StatusBroken
		}
		return dev
	}

	// the game leaves the repository after `last` plies, with the move number last+1
	ply := last + 1
	at := node[last]
	dev.Ply, dev.Node = ply, at
	dev.Played = &Step{UCI: game[last], SAN: sans[last], FEN: fens[ply]}
	kids := g.Replies(at)
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

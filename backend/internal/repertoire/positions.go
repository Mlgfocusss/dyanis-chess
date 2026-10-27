package repertoire

import (
	"fmt"
	"sort"
	"strings"

	"dyanis-chess/backend/internal/store"
	"dyanis-chess/engine"
)

// Transpositions. The tree of a repository stores paths (a node is a move with its parent), but
// one position can be reached by different orders of moves: 1.d4 Nf6 2.c4 e6 and 1.c4 e6 2.d4 Nf6.
// What moves of the repository mean is decided by the position, not by the path: two nodes whose
// positions are the same (PosKey) are one "class", and the replies known in a class are the replies
// of every node in it. The tree itself is not changed (no migration): the key is worked out from
// the FEN every node already stores. web/src/openings/tree.js does the same on the page and
// positions_test.go / tree.test.js hold the same examples.

// startKey is the key of the initial position, the position "before" the first moves (node 0).
var startKey = PosKey(engine.StartFEN)

// PosKey is the part of a FEN that makes up a position: the pieces, the side to move, the
// castling rights, and the en passant square only when a pawn can really take on it (as the
// hash of the ECO reference counts it). The move counters are left out.
func PosKey(fen string) string {
	f := strings.Fields(fen)
	if len(f) < 2 {
		return strings.TrimSpace(fen)
	}
	castling, ep := "-", "-"
	if len(f) > 2 {
		castling = f[2]
	}
	if len(f) > 3 {
		ep = f[3]
	}
	if ep != "-" && !epCapturable(f[0], f[1], ep) {
		ep = "-"
	}
	return f[0] + " " + f[1] + " " + castling + " " + ep
}

// epCapturable says whether a pawn of the side to move stands beside the pawn that has just
// moved two squares (the pins are not looked at, as in the hash of the engine).
func epCapturable(board, turn, ep string) bool {
	if len(ep) != 2 {
		return false
	}
	file, rank := int(ep[0]-'a'), int(ep[1]-'0')
	if file < 0 || file > 7 {
		return false
	}
	// the capturing pawn stands on the rank of the pushed pawn: the 4th for Black to move, the 5th for White
	// (row 4 and row 3 counting from the 8th rank)
	var row int
	var pawn byte
	switch {
	case turn == "b" && rank == 3:
		row, pawn = 4, 'p'
	case turn == "w" && rank == 6:
		row, pawn = 3, 'P'
	default:
		return false
	}
	ranks := strings.Split(board, "/")
	if len(ranks) != 8 {
		return false
	}
	squares := make([]byte, 0, 8)
	for i := 0; i < len(ranks[row]); i++ {
		c := ranks[row][i]
		if c >= '1' && c <= '8' {
			for n := 0; n < int(c-'0'); n++ {
				squares = append(squares, '.')
			}
			continue
		}
		squares = append(squares, c)
	}
	if len(squares) != 8 {
		return false
	}
	return (file > 0 && squares[file-1] == pawn) || (file < 7 && squares[file+1] == pawn)
}

// Graph is a repository's moves with their positions: the tree seen as positions.
type Graph struct {
	Moves    []store.RepMove
	byID     map[int64]store.RepMove
	children map[int64][]store.RepMove
	key      map[int64]string
	class    map[string][]int64 // position key -> nodes in that position, the start (0) first, then by id
}

// NewGraph indexes the moves of a repository (in the order they were added). A move without a
// FEN (only in tests) is a position of its own.
func NewGraph(moves []store.RepMove) *Graph {
	g := &Graph{
		Moves:    moves,
		byID:     make(map[int64]store.RepMove, len(moves)),
		children: map[int64][]store.RepMove{},
		key:      make(map[int64]string, len(moves)+1),
		class:    map[string][]int64{},
	}
	g.key[0] = startKey
	g.class[startKey] = []int64{0}
	for _, m := range moves {
		g.byID[m.ID] = m
		g.children[m.ParentID] = append(g.children[m.ParentID], m)
		k := PosKey(m.FEN)
		if m.FEN == "" {
			k = fmt.Sprintf("#%d", m.ID)
		}
		g.key[m.ID] = k
		g.class[k] = append(g.class[k], m.ID)
	}
	for _, ids := range g.class {
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	}
	return g
}

// Key is the position after the move id (the initial position for 0).
func (g *Graph) Key(id int64) string {
	if k, ok := g.key[id]; ok {
		return k
	}
	return fmt.Sprintf("#%d", id)
}

// Has says whether some node of the repository stands in the position.
func (g *Graph) Has(key string) bool { return len(g.class[key]) > 0 }

// Same says whether two nodes stand in the same position.
func (g *Graph) Same(a, b int64) bool { return a == b || g.Key(a) == g.Key(b) }

// Class lists the nodes in the position of the node id (id included), by id.
func (g *Graph) Class(id int64) []int64 { return g.class[g.Key(id)] }

// ply of a node; the start has 0.
func (g *Graph) ply(id int64) int { return g.byID[id].Ply }

// Rep is the node that stands for a position in lists: the one with the fewest plies played,
// then the smallest id (the initial position itself when the position is the initial one).
func (g *Graph) Rep(key string) int64 {
	ids := g.class[key]
	if len(ids) == 0 {
		return 0
	}
	best := ids[0]
	for _, id := range ids[1:] {
		if g.ply(id) < g.ply(best) {
			best = id
		}
	}
	return best
}

// Replies are the moves the repository has in the position of the node id: its own children
// first, then those of the other nodes in the same position, one for each move.
func (g *Graph) Replies(id int64) []store.RepMove {
	out := append([]store.RepMove(nil), g.children[id]...)
	ids := g.Class(id)
	if len(ids) <= 1 {
		return out
	}
	seen := make(map[string]bool, len(out))
	for _, m := range out {
		seen[m.UCI] = true
	}
	for _, other := range ids {
		if other == id {
			continue
		}
		for _, m := range g.children[other] {
			if !seen[m.UCI] {
				seen[m.UCI] = true
				out = append(out, m)
			}
		}
	}
	return out
}

// Reply is the move of the repository in the position of the node id with the move uci.
func (g *Graph) Reply(id int64, uci string) (store.RepMove, bool) {
	for _, m := range g.Replies(id) {
		if m.UCI == uci {
			return m, true
		}
	}
	return store.RepMove{}, false
}

// CardKey names the question a move of the person answers: the position it is played in and the
// move. Moves of different nodes with the same CardKey are one card.
func (g *Graph) CardKey(m store.RepMove) string { return g.Key(m.ParentID) + "|" + m.UCI }

// CardGroup is one card of the person: the move asked, and every node of the tree that holds the
// same question (reached by another order of moves).
type CardGroup struct {
	Rep     store.RepMove // the node whose state counts: the most asked one, then the first added
	Members []int64       // ids of all the nodes of the card, Rep's included
}

// Cards groups the person's own moves into cards, in the order of their first node.
func (g *Graph) Cards() []CardGroup {
	index := map[string]int{}
	var out []CardGroup
	for _, m := range g.Moves {
		if !m.Mine {
			continue
		}
		k := g.CardKey(m)
		i, ok := index[k]
		if !ok {
			index[k] = len(out)
			out = append(out, CardGroup{Rep: m, Members: []int64{m.ID}})
			continue
		}
		c := &out[i]
		c.Members = append(c.Members, m.ID)
		if m.Reviews > c.Rep.Reviews { // the strictly more asked one; ties keep the first added
			c.Rep = m
		}
	}
	return out
}

// CardOf finds the card the node id belongs to.
func (g *Graph) CardOf(id int64) (CardGroup, bool) {
	m, ok := g.byID[id]
	if !ok || !m.Mine {
		return CardGroup{}, false
	}
	k := g.CardKey(m)
	var out CardGroup
	found := false
	for _, c := range g.Cards() {
		if g.CardKey(c.Rep) == k {
			out, found = c, true
			break
		}
	}
	return out, found
}

// Edges counts the different moves of the repository: a move played in the same position by
// another order of moves is counted once.
func (g *Graph) Edges() int {
	seen := make(map[string]bool, len(g.Moves))
	for _, m := range g.Moves {
		seen[g.CardKey(m)] = true
	}
	return len(seen)
}

// UnansweredLines lists the lines that end with the opponent's move in a position where the
// repository has no move of the person at all (nor in the same position reached otherwise). One
// node stands for each such position (Rep).
func (g *Graph) UnansweredLines() []int64 {
	seen := map[string]bool{}
	var out []int64
	for _, m := range g.Moves {
		if m.Mine {
			continue
		}
		k := g.Key(m.ID)
		if seen[k] || len(g.Replies(m.ID)) > 0 {
			continue
		}
		seen[k] = true
		out = append(out, g.Rep(k))
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

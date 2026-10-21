package lessons

import (
	"errors"
	"fmt"

	"dyanis-chess/engine"
)

// LessonRef points at another lesson.
type LessonRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// SectionRef names the section a lesson belongs to.
type SectionRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// LessonJSON is a lesson as the browser gets it.
type LessonJSON struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Icon        string      `json:"icon"`
	Minutes     int         `json:"minutes"`
	Section     SectionRef  `json:"section"`
	Next        *LessonRef  `json:"next,omitempty"` // the lesson after this one, nil after the last
	Exercises   int         `json:"exercises"`
	Steps       []StepJSON  `json:"steps"`
	Progress    *ProgressJS `json:"progress,omitempty"` // set by the service for a signed-in person
}

// StepJSON is a step ready for the browser, which has no rules of chess: a text step carries the
// positions of its demonstration, an exercise the tree of its solutions.
type StepJSON struct {
	Type        string     `json:"type"`
	Title       string     `json:"title,omitempty"`
	FEN         string     `json:"fen"`
	Orientation string     `json:"orientation"`
	Text        string     `json:"text"`
	Arrows      [][]string `json:"arrows,omitempty"` // [from, to, colour], the colour a token name
	Highlights  []string   `json:"highlights,omitempty"`
	Demo        []DemoMove `json:"demo,omitempty"`

	Hint     string    `json:"hint,omitempty"`
	Success  string    `json:"success,omitempty"`
	Miss     string    `json:"miss,omitempty"`
	Mistakes []Mistake `json:"mistakes,omitempty"`
	Tree     *Node     `json:"tree,omitempty"`
}

// DemoMove is one move of a demonstration with the position after it.
type DemoMove struct {
	UCI string `json:"uci"`
	SAN string `json:"san"`
	FEN string `json:"fen"`
}

// Node is a position where the person moves. Moves are the moves that solve the exercise (the
// first is the main line); anything else in Legal is a wrong move.
type Node struct {
	FEN   string   `json:"fen"`
	Legal []string `json:"legal"`
	Moves []Accept `json:"moves"`
}

// Accept is a right move. When Reply is set the opponent answers with it; then Next is where the
// person moves again, or nil when the exercise ends with the reply. Without a Reply the exercise
// ends with the move itself.
type Accept struct {
	UCI   string `json:"uci"`
	SAN   string `json:"san"`
	FEN   string `json:"fen"` // the position after the move
	Mate  bool   `json:"mate,omitempty"`
	Reply *Reply `json:"reply,omitempty"`
	Next  *Node  `json:"next,omitempty"`

	endsHere bool // a solution ends with this move (a text of its own for the checks below)
	replyEnd bool // a solution ends with the reply
}

// Reply is the opponent's scripted answer.
type Reply struct {
	UCI string `json:"uci"`
	SAN string `json:"san"`
	FEN string `json:"fen"`
}

// buildTree merges the solution chains into a tree. Two chains that begin alike must agree on the
// opponent's reply, and a chain may not stop where another goes on: the author is told.
func buildTree(start *engine.Position, chains [][]string) (*Node, error) {
	if len(chains) == 0 {
		return nil, errors.New("an exercise needs at least one solution")
	}
	root := &Node{FEN: start.FEN()}
	// the positions of the nodes, kept so that the legal moves can be filled in at the end
	positions := map[*Node]*engine.Position{root: start.Copy()}
	for ci, chain := range chains {
		if len(chain) == 0 {
			return nil, fmt.Errorf("solution %d is empty", ci+1)
		}
		cur := root
		pos := start.Copy()
		var acc *Accept
		for i, u := range chain {
			mv, ok := findMove(pos, u)
			if !ok {
				return nil, fmt.Errorf("solution %d: move %d (%s) is not legal in %s", ci+1, i+1, u, pos.FEN())
			}
			san := pos.SAN(mv)
			if i%2 == 0 { // the person's move
				acc = nil
				for k := range cur.Moves {
					if cur.Moves[k].UCI == u {
						acc = &cur.Moves[k]
					}
				}
				after := pos.Copy()
				after.Play(mv)
				if acc == nil {
					cur.Moves = append(cur.Moves, Accept{UCI: u, SAN: san, FEN: after.FEN(), Mate: after.Outcome(nil).Checkmate})
					acc = &cur.Moves[len(cur.Moves)-1]
				}
				if acc.endsHere || acc.replyEnd {
					return nil, fmt.Errorf("solution %d goes on after %s, where another solution stops", ci+1, u)
				}
				if i == len(chain)-1 {
					if acc.Reply != nil {
						return nil, fmt.Errorf("solution %d stops at %s, where another solution goes on", ci+1, u)
					}
					acc.endsHere = true
				}
				pos = after
				continue
			}
			// the opponent's reply
			after := pos.Copy()
			after.Play(mv)
			if acc.Reply == nil {
				acc.Reply = &Reply{UCI: u, SAN: san, FEN: after.FEN()}
			} else if acc.Reply.UCI != u {
				return nil, fmt.Errorf("solution %d: two different replies (%s, %s) to %s", ci+1, acc.Reply.UCI, u, acc.UCI)
			}
			pos = after
			if i == len(chain)-1 {
				if acc.Next != nil {
					return nil, fmt.Errorf("solution %d stops at the reply %s, where another solution goes on", ci+1, u)
				}
				acc.replyEnd = true
				continue
			}
			if acc.Next == nil {
				acc.Next = &Node{FEN: pos.FEN()}
				positions[acc.Next] = pos.Copy()
			}
			cur = acc.Next
		}
	}
	// the legal moves of every node, and every move that mates is a right one as well
	var walk func(n *Node) error
	walk = func(n *Node) error {
		pos := positions[n]
		legal := pos.LegalMoves()
		if len(legal) == 0 {
			return fmt.Errorf("no legal moves in %s", n.FEN)
		}
		for _, m := range legal {
			u := m.String()
			n.Legal = append(n.Legal, u)
			if hasAccept(n, u) {
				continue
			}
			after := pos.Copy()
			after.Play(m)
			if after.Outcome(nil).Checkmate {
				n.Moves = append(n.Moves, Accept{UCI: u, SAN: pos.SAN(m), FEN: after.FEN(), Mate: true})
			}
		}
		for _, a := range n.Moves {
			if a.Next != nil {
				if err := walk(a.Next); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}
	return root, nil
}

func hasAccept(n *Node, uci string) bool {
	for _, a := range n.Moves {
		if a.UCI == uci {
			return true
		}
	}
	return false
}

// nodesByDepth lists the nodes of a tree by how many moves of the person lead to them.
func nodesByDepth(root *Node) [][]*Node {
	var out [][]*Node
	level := []*Node{root}
	for len(level) > 0 {
		out = append(out, level)
		var next []*Node
		for _, n := range level {
			for _, a := range n.Moves {
				if a.Next != nil {
					next = append(next, a.Next)
				}
			}
		}
		level = next
	}
	return out
}

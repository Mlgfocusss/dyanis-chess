package pgn

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"dyanis-chess/engine"
)

// Node is a move in the tree of a game: the move itself and what may follow it. The first child
// continues the game's main line, the others are variations (alternatives to that first move).
type Node struct {
	SAN      string      // normalized SAN, as in Move
	Move     engine.Move // the resolved move
	FEN      string      // the position after the move
	Children []*Node
}

// Tree is a game with its variations: the moves after the start position are
// Root.Children (Root itself holds no move). Comments, NAGs and clocks are dropped.
type Tree struct {
	Tags     map[string]string
	TagOrder []string
	StartFEN string
	Root     Node
	Result   string
}

// ParseTrees reads every game from r, with all its variations, like Parse
// reads their main lines. A variation that repeats a move of another line joins it.
func ParseTrees(r io.Reader) ([]*Tree, error) { return scanGames(r, buildTree) }

// ParseTreesString is ParseTrees of a string.
func ParseTreesString(s string) ([]*Tree, error) { return ParseTrees(strings.NewReader(s)) }

// Count is the number of moves in the tree (a move shared by lines counts once).
func (t *Tree) Count() int {
	n := 0
	var walk func(*Node)
	walk = func(x *Node) {
		for _, c := range x.Children {
			n++
			walk(c)
		}
	}
	walk(&t.Root)
	return n
}

// frame is the state outside a variation that is being read.
type frame struct {
	cur, prev      *Node
	pos, posBefore *engine.Position
	ply            int
}

func buildTree(idx int, tagLines []string, movetext string) (*Tree, error) {
	// the tags, the start position and the result are read as Parse reads them
	head, err := buildGame(idx, tagLines, "")
	if err != nil {
		return nil, err
	}
	t := &Tree{Tags: head.Tags, TagOrder: head.TagOrder, StartFEN: head.StartFEN, Result: head.Result}
	pos, err := head.Position()
	if err != nil {
		return nil, &ParseError{Game: idx, Msg: fmt.Sprintf("bad FEN tag: %v", err)}
	}
	toks, err := tokenizeOpt(movetext, true)
	if err != nil {
		return nil, &ParseError{Game: idx, Msg: err.Error()}
	}

	cur := &t.Root                 // the node whose position pos is
	var prev *Node                 // the parent of cur
	var posBefore *engine.Position // the position before cur's move
	ply := 0                       // half-moves played on the line being read
	var stack []frame
	for _, tk := range toks {
		switch tk.kind {
		case tokResult:
			if len(stack) == 0 {
				t.Result = tk.text
			}
		case tokOpen:
			if prev == nil || posBefore == nil {
				return nil, &ParseError{Game: idx, Ply: ply + 1, Msg: "a variation must follow a move"}
			}
			stack = append(stack, frame{cur, prev, pos, posBefore, ply})
			cur, pos, ply = prev, posBefore.Copy(), ply-1
			prev, posBefore = nil, nil // the variation's own first move sets them
		case tokClose:
			if len(stack) == 0 {
				return nil, &ParseError{Game: idx, Msg: "unbalanced )"}
			}
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			cur, prev, pos, posBefore, ply = f.cur, f.prev, f.pos, f.posBefore, f.ply
		case tokMove:
			ply++
			m, err := pos.ParseSAN(tk.text)
			if err != nil {
				return nil, &ParseError{Game: idx, Ply: ply, Msg: err.Error()}
			}
			san := pos.SAN(m)
			before := pos.Copy()
			pos.Play(m)
			child := childFor(cur, m)
			if child == nil {
				child = &Node{SAN: san, Move: m, FEN: pos.FEN()}
				cur.Children = append(cur.Children, child)
			}
			prev, cur, posBefore = cur, child, before
		}
	}
	if len(stack) != 0 {
		return nil, &ParseError{Game: idx, Msg: "unbalanced ("}
	}
	return t, nil
}

func childFor(n *Node, m engine.Move) *Node {
	for _, c := range n.Children {
		if c.Move.String() == m.String() {
			return c
		}
	}
	return nil
}

// String renders the game as PGN with its variations in parentheses: the tags
// as Game.String writes them, the movetext wrapped at 80 columns.
func (t *Tree) String() string {
	g := &Game{Tags: t.Tags, TagOrder: t.TagOrder, StartFEN: t.StartFEN, Result: t.Result}
	head := g.String() // the tags, a blank line, the result
	cut := strings.LastIndex(head, "\n\n")
	if cut >= 0 {
		head = head[:cut+2]
	}

	startPly, num := 0, 1
	if t.StartFEN != "" && t.StartFEN != engine.StartFEN {
		f := strings.Fields(t.StartFEN)
		if len(f) > 1 && f[1] == "b" {
			startPly = 1
		}
		if len(f) > 5 {
			if n, err := strconv.Atoi(f[5]); err == nil && n > 0 {
				num = n
			}
		}
	}
	w := &treeWriter{startPly: startPly, num: num}
	w.line(&t.Root, 0, startPly == 1)
	result := t.Result
	if result == "" {
		result = "*"
	}
	w.words = append(w.words, result)
	return head + wrap(w.words)
}

type treeWriter struct {
	words         []string
	startPly, num int
	open          int // "(" waiting to stick to the next word
}

func (w *treeWriter) add(s string) {
	w.words = append(w.words, strings.Repeat("(", w.open)+s)
	w.open = 0
}

// line writes the moves that follow n: the main continuation, then the variations,
// then the rest of the main line. ply counts from the start position; number says
// whether a black move needs its number ("12...": at the start of a variation or after one).
func (w *treeWriter) line(n *Node, ply int, number bool) {
	for len(n.Children) > 0 {
		first := n.Children[0]
		w.move(first, ply, number)
		for _, v := range n.Children[1:] {
			w.open++
			w.move(v, ply, true)
			w.line(v, ply+1, false)
			w.words[len(w.words)-1] += ")"
		}
		number = len(n.Children) > 1
		n, ply = first, ply+1
	}
}

func (w *treeWriter) move(n *Node, ply int, number bool) {
	black := (ply+w.startPly)%2 == 1
	switch {
	case !black:
		w.add(fmt.Sprintf("%d.", w.num+(ply+w.startPly)/2))
	case number:
		w.add(fmt.Sprintf("%d...", w.num+(ply+w.startPly)/2))
	}
	w.add(n.SAN)
}

// wrap joins words with spaces, breaking lines before they pass 80 columns.
func wrap(words []string) string {
	var sb strings.Builder
	line := 0
	for _, w := range words {
		if line > 0 && line+1+len(w) > 80 {
			sb.WriteByte('\n')
			line = 0
		}
		if line > 0 {
			sb.WriteByte(' ')
			line++
		}
		sb.WriteString(w)
		line += len(w)
	}
	sb.WriteByte('\n')
	return sb.String()
}

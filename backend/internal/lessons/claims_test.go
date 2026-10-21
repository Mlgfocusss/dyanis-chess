package lessons

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"dyanis-chess/engine"
)

// A step may carry a "claim": what the author says about the position, which the engine checks
// here so that the lessons never teach a wrong thing. The claims:
//
//	mate1          the solutions mate in one (and every mating move is accepted)
//	mate2          mate in two: no mate in one now, every solution forces mate in two
//	               whatever the reply, and every move that forces it is written down
//	best           the engine's best move (depth 8) is one of the solutions
//	wins-material  as best, and after the scripted line the person is at least two pawns up
//	check1         every solution gives check and every checking move is a solution
//	stalemate      every solution stalemates the opponent
//	no-stalemate   no solution stalemates, although some legal move does
//	defend-mate1   the opponent threatens mate in one: a move is a solution exactly when it leaves the opponent no mate in one
//	wins           the solutions are exactly the moves after which the opponent, searched to depth 11, stands lost (800 points or worse)
//	holds          the solutions are exactly the moves after which the opponent does not stand better than 450 points
//	promotes       every solution promotes a pawn
//	capture        every solution takes a piece
//	none           nothing to verify (a drill of the rules; the solutions are legal by construction)
var knownClaims = map[string]bool{"mate1": true, "mate2": true, "best": true, "wins-material": true, "check1": true,
	"stalemate": true, "no-stalemate": true, "defend-mate1": true, "wins": true, "holds": true, "promotes": true, "capture": true, "none": true}

const claimDepth = 8

var pieceValue = map[rune]int{'p': 1, 'n': 3, 'b': 3, 'r': 5, 'q': 9}

// balance is White's material minus Black's, in pawns, from a FEN.
func balance(fen string) int {
	place, _, _ := strings.Cut(fen, " ")
	n := 0
	for _, r := range place {
		if v, ok := pieceValue[r|0x20]; ok {
			if r >= 'a' {
				n -= v
			} else {
				n += v
			}
		}
	}
	return n
}

func mates(pos *engine.Position) []engine.Move {
	var out []engine.Move
	for _, m := range pos.LegalMoves() {
		c := pos.Copy()
		c.Play(m)
		if c.Outcome(nil).Checkmate {
			out = append(out, m)
		}
	}
	return out
}

// forcesMate2 says whether the move m leads to a mate in one whatever the opponent answers.
func forcesMate2(pos *engine.Position, m engine.Move) bool {
	after := pos.Copy()
	after.Play(m)
	replies := after.LegalMoves()
	if len(replies) == 0 {
		return false
	}
	for _, r := range replies {
		c := after.Copy()
		c.Play(r)
		if len(mates(c)) == 0 {
			return false
		}
	}
	return true
}

func givesCheck(pos *engine.Position, m engine.Move) bool {
	c := pos.Copy()
	c.Play(m)
	return inCheck(c.FEN())
}

func isCapture(pos *engine.Position, m engine.Move) bool {
	if strings.Contains(pos.SAN(m), "x") {
		return true
	}
	return false
}

// bestIsASolution reports whether the engine's best move is one of the solutions.
func bestIsASolution(pos *engine.Position, root *Node, depth int) (string, bool, error) {
	r, err := engine.Search(pos, nil, engine.Limits{Depth: depth, HashMB: 16})
	if err != nil {
		return "", false, err
	}
	return r.Best.String(), hasAccept(root, r.Best.String()), nil
}

// followMain plays the main line of the tree from the root: the first move at each node and the reply.
func followMain(start *engine.Position, root *Node) (*engine.Position, error) {
	pos := start.Copy()
	n := root
	for n != nil {
		a := n.Moves[0]
		for _, c := range []string{a.UCI} {
			m, ok := findMove(pos, c)
			if !ok {
				return nil, fmt.Errorf("move %s", c)
			}
			pos.Play(m)
		}
		if a.Reply == nil {
			break
		}
		m, ok := findMove(pos, a.Reply.UCI)
		if !ok {
			return nil, fmt.Errorf("reply %s", a.Reply.UCI)
		}
		pos.Play(m)
		n = a.Next
	}
	return pos, nil
}

func TestClaims(t *testing.T) {
	if claimed := runClaims(t, mustLoad(t)); claimed < 60 {
		t.Errorf("only %d exercises checked", claimed)
	}
}

// runClaims checks the claim of every exercise and returns how many exercises there are.
func runClaims(t *testing.T, lib *Library) int {
	claimed := 0
	for _, sec := range lib.Sections {
		for _, l := range sec.Lessons {
			for i, st := range l.Steps {
				if st.Type != TypeExercise {
					continue
				}
				where := fmt.Sprintf("%s step %d", l.ID, i+1)
				if st.Claim == "" {
					t.Errorf("%s: an exercise must state its claim (use \"none\" for a drill of the rules)", where)
					continue
				}
				if !knownClaims[claimBase(st.Claim)] {
					t.Errorf("%s: unknown claim %q", where, st.Claim)
					continue
				}
				claimed++
				checkClaim(t, where, lib.byID[l.ID].json.Steps[i], st)
			}
		}
	}
	return claimed
}

// claimBase splits "best-12" into "best": a number after the last dash is the search depth (8 when it
// is left out).
func claimBase(c string) string {
	if i := strings.LastIndex(c, "-"); i >= 0 {
		if _, err := strconv.Atoi(c[i+1:]); err == nil {
			return c[:i]
		}
	}
	return c
}

func claimDepthOf(c string) int {
	if i := strings.LastIndex(c, "-"); i >= 0 {
		if d, err := strconv.Atoi(c[i+1:]); err == nil {
			return d
		}
	}
	return claimDepth
}

func checkClaim(t *testing.T, where string, sj StepJSON, st Step) {
	pos, err := engine.ParseFEN(st.FEN)
	if err != nil {
		t.Errorf("%s: %v", where, err)
		return
	}
	root := sj.Tree
	var firsts []engine.Move
	for _, a := range root.Moves {
		m, _ := findMove(pos, a.UCI)
		firsts = append(firsts, m)
	}
	fail := func(format string, a ...any) { t.Errorf("%s (%s): %s", where, st.Claim, fmt.Sprintf(format, a...)) }
	switch claimBase(st.Claim) {
	case "wins", "holds":
		depth := 12
		if strings.Contains(st.Claim, "-") {
			depth = claimDepthOf(st.Claim)
		}
		for _, m := range pos.LegalMoves() {
			c := pos.Copy()
			c.Play(m)
			good := false
			if o := c.Outcome(nil); o.Over {
				good = o.Checkmate == (claimBase(st.Claim) == "wins")
				if claimBase(st.Claim) == "holds" {
					good = !o.Checkmate
				}
			} else {
				r, err := engine.Search(c, nil, engine.Limits{Depth: depth - 1, HashMB: 16})
				if err != nil {
					fail("%v", err)
					return
				}
				if claimBase(st.Claim) == "wins" {
					good = r.Score <= -800
				} else {
					good = r.Score < 450
				}
			}
			if good != slices.Contains(firsts, m) {
				fail("%s: engine says good=%v, solution=%v", pos.SAN(m), good, slices.Contains(firsts, m))
			}
		}
	case "mate1":
		for _, a := range root.Moves {
			if !a.Mate {
				fail("%s does not mate", a.SAN)
			}
		}
	case "mate2":
		if len(mates(pos)) > 0 {
			fail("there is a mate in one")
		}
		for _, m := range firsts {
			if !forcesMate2(pos, m) {
				fail("%s does not force a mate in two", pos.SAN(m))
			}
		}
		for _, m := range pos.LegalMoves() {
			if !slices.Contains(firsts, m) && forcesMate2(pos, m) {
				fail("%s also mates in two and is not written down", pos.SAN(m))
			}
		}
		end, err := followMain(pos, root)
		if err != nil || !end.Outcome(nil).Checkmate {
			fail("the main line does not end in mate (%v)", err)
		}
	case "best", "wins-material":
		best, ok, err := bestIsASolution(pos, root, claimDepthOf(st.Claim))
		if err != nil || !ok {
			fail("the engine plays %s, not a solution (%v)", best, err)
		}
		if st.Claim == "wins-material" {
			end, err := followMain(pos, root)
			if err != nil {
				fail("%v", err)
				return
			}
			gain := balance(end.FEN()) - balance(st.FEN)
			if pos.SideToMove() == engine.Black {
				gain = -gain
			}
			if gain < 2 {
				fail("after the main line the person is only %d up in material (%s)", gain, end.FEN())
			}
		}
	case "check1":
		for _, m := range pos.LegalMoves() {
			if givesCheck(pos, m) != slices.Contains(firsts, m) {
				fail("%s: check %v, solution %v", pos.SAN(m), givesCheck(pos, m), slices.Contains(firsts, m))
			}
		}
	case "stalemate":
		for _, m := range firsts {
			c := pos.Copy()
			c.Play(m)
			if o := c.Outcome(nil); !o.Over || o.Checkmate {
				fail("%s does not stalemate", pos.SAN(m))
			}
		}
	case "no-stalemate":
		some := false
		for _, m := range pos.LegalMoves() {
			c := pos.Copy()
			c.Play(m)
			stale := c.Outcome(nil).Over && !c.Outcome(nil).Checkmate
			some = some || stale
			if stale && slices.Contains(firsts, m) {
				fail("%s stalemates but is a solution", pos.SAN(m))
			}
		}
		if !some {
			fail("no move stalemates, so there is nothing to avoid")
		}
	case "defend-mate1":
		if len(mates(pos)) > 0 {
			fail("the person has a mate in one already")
		}
		for _, m := range pos.LegalMoves() {
			c := pos.Copy()
			c.Play(m)
			mated := !c.Outcome(nil).Over && len(mates(c)) > 0
			if mated == slices.Contains(firsts, m) {
				fail("%s: the opponent mates after it: %v, solution: %v", pos.SAN(m), mated, slices.Contains(firsts, m))
			}
		}
	case "promotes":
		for _, a := range root.Moves {
			if len(a.UCI) != 5 {
				fail("%s does not promote", a.SAN)
			}
		}
	case "capture":
		for _, m := range firsts {
			if !isCapture(pos, m) {
				fail("%s does not take a piece", pos.SAN(m))
			}
		}
	}
}

// inCheck says whether the side to move in the FEN has its king attacked. The engine's own
// SAN leaves out the check mark in positions it calls dead (a bare king against a king and a
// minor piece), so the claims use this instead.
func inCheck(fen string) bool {
	parts := strings.Fields(fen)
	var board [8][8]rune // board[rank][file], rank 0 is the 1st rank
	rank, file := 7, 0
	for _, r := range parts[0] {
		switch {
		case r == '/':
			rank, file = rank-1, 0
		case r >= '1' && r <= '8':
			file += int(r - '0')
		default:
			board[rank][file] = r
			file++
		}
	}
	white := parts[1] == "w"
	king := 'k'
	if white {
		king = 'K'
	}
	kr, kf := -1, -1
	for r := 0; r < 8; r++ {
		for f := 0; f < 8; f++ {
			if board[r][f] == king {
				kr, kf = r, f
			}
		}
	}
	if kr < 0 {
		return false
	}
	enemy := func(p rune) bool { // a piece of the side that attacks
		if p == 0 {
			return false
		}
		isWhite := p >= 'A' && p <= 'Z'
		return isWhite != white
	}
	at := func(r, f int) rune {
		if r < 0 || r > 7 || f < 0 || f > 7 {
			return 0
		}
		return board[r][f]
	}
	lower := func(p rune) rune { return p | 0x20 }
	// knights and kings
	for _, d := range [][2]int{{1, 2}, {2, 1}, {-1, 2}, {-2, 1}, {1, -2}, {2, -1}, {-1, -2}, {-2, -1}} {
		if p := at(kr+d[0], kf+d[1]); enemy(p) && lower(p) == 'n' {
			return true
		}
	}
	for dr := -1; dr <= 1; dr++ {
		for df := -1; df <= 1; df++ {
			if p := at(kr+dr, kf+df); (dr != 0 || df != 0) && enemy(p) && lower(p) == 'k' {
				return true
			}
		}
	}
	// pawns: a white pawn attacks up the board, so it sits one rank below the king it attacks
	pr := -1 // a white pawn below the black king attacks up
	if white {
		pr = 1 // a black pawn above the white king attacks down
	}
	for _, df := range []int{-1, 1} {
		if p := at(kr+pr, kf+df); enemy(p) && lower(p) == 'p' {
			return true
		}
	}
	// sliders
	for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
		for r, f := kr+d[0], kf+d[1]; r >= 0 && r < 8 && f >= 0 && f < 8; r, f = r+d[0], f+d[1] {
			p := board[r][f]
			if p == 0 {
				continue
			}
			if enemy(p) {
				straight := d[0] == 0 || d[1] == 0
				switch lower(p) {
				case 'q':
					return true
				case 'r':
					if straight {
						return true
					}
				case 'b':
					if !straight {
						return true
					}
				}
			}
			break
		}
	}
	return false
}

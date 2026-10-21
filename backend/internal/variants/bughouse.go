package variants

import (
	"errors"
	"fmt"
)

// Bughouse (Swedish chess), as on Chess.com: four players in two teams of two on two boards. A team has White
// on one board and Black on the other, so partners sit side by side with opposite colours; the piece a player
// takes goes at once to the pocket of the partner (a pawn if it was promoted), who may drop it instead of a
// move, as in Crazyhouse. Every player has a clock of their own. The game ends on the first of: a mate on
// either board, a flag of any player, a resignation; the team of the loser loses.
//
// This is not a State (a State has one board and two sides), so vplay plays it through its own four-seat game
// (vplay.BugGame). The rules live here, pure and testable: Bug holds the two boards (each a Game under
// bugRules) and moves the pieces between the pockets.
//
// Decisions the rules leave open (VARIANTS-PLAN.md, E.2):
//   - mate ends the game at once, even if a piece from the partner could still block the check;
//   - a player with no move and no check (stalemate) is not given a draw: the turn stays with them, their clock
//     runs, and a piece from their partner may free them; the position is never drawn by repetition;
//   - there is no draw by agreement, no fifty-move rule, no insufficient material (pockets bring material).

// Seats of a bughouse game: board 0 White, board 0 Black, board 1 White, board 1 Black.
const BugSeats = 4

// BugSeat is the seat of the player of `side` on the board (0 or 1), 0..3.
func BugSeat(board int, side Side) int {
	if side == Black {
		return board*2 + 1
	}
	return board * 2
}

// BugPlace is the board and the colour of a seat.
func BugPlace(seat int) (board int, side Side) {
	side = White
	if seat%2 == 1 {
		side = Black
	}
	return seat / 2, side
}

// BugPartner is the seat of the partner: the other board, the other colour.
func BugPartner(seat int) int {
	board, side := BugPlace(seat)
	return BugSeat(1-board, side.Opp())
}

// BugTeam is 0 for the team of White on board 0 (seats 0 and 3), 1 for the other (seats 1 and 2).
func BugTeam(seat int) int {
	if seat == 0 || seat == 3 {
		return 0
	}
	return 1
}

// BugResultFor is the result in which the team wins: "1-0" for team 0, "0-1" for team 1.
func BugResultFor(team int) string {
	if team == 0 {
		return ResultWhite
	}
	return ResultBlack
}

// bugRules are the rules of one board: Crazyhouse drops, no result of its own (the game ends by mate, flag or
// resignation, judged by Bug), and the pockets are filled by Bug when the other board takes a piece.
type bugRules struct{ ClassicRules }

func (bugRules) ExtraMoves(p *Position) []Move { return DropMoves(p) }

// Result: a board never ends the game by itself (a stalemated player waits), Bug judges mates.
func (bugRules) Result(*Game) (string, string, bool) { return "", "", false }

func (bugRules) Extra(g *Game, _ Side) any { return PocketView(g.Position()) }

// ErrBugBoard is the answer to a board number that does not exist.
var ErrBugBoard = errors.New("такой доски нет")

// Bug is a game of bughouse: two boards and the way pieces pass between them.
type Bug struct {
	boards [2]*Game
	over   bool
	result string
	reason string
}

// NewBug starts a game.
func NewBug(o Options) (*Bug, error) {
	b := &Bug{}
	for i := range b.boards {
		g, err := NewGame(bugRules{}, o)
		if err != nil {
			return nil, err
		}
		b.boards[i] = g
	}
	return b, nil
}

func (b *Bug) board(i int) (*Game, error) {
	if i < 0 || i > 1 {
		return nil, ErrBugBoard
	}
	return b.boards[i], nil
}

// Turn is who acts on the board now (NoSide for a board that does not exist).
func (b *Bug) Turn(board int) Side {
	g, err := b.board(board)
	if err != nil {
		return NoSide
	}
	return g.Turn()
}

// Legal lists the actions of the side to move on the board (none once the game is over).
func (b *Bug) Legal(board int) []string {
	g, err := b.board(board)
	if err != nil || b.over {
		return nil
	}
	return g.Legal()
}

// Apply plays an action of the side to move on the board: a move or a drop ("e2e4", "N@f3"). It returns the
// notation. A piece taken goes to the pocket of the mover's partner on the other board. A mate ends the game.
func (b *Bug) Apply(board int, action string) (string, error) {
	g, err := b.board(board)
	if err != nil {
		return "", err
	}
	if b.over {
		return "", ErrOver
	}
	mv, ok := Find(g.Rules(), g.Moves(), action)
	if !ok {
		return "", ErrIllegal
	}
	mover := g.Position().Turn
	taken := CapturedBy(g.Position(), mv)
	note := g.Play(mv)
	if !taken.Empty() {
		kind := PocketKind(taken)
		b.boards[1-board].Edit(func(p *Position) { p.Pocket[mover.Opp()][kind]++ })
	}
	if p := g.Position(); g.Rules().Check(p, p.Turn) && len(g.Moves()) == 0 {
		b.over, b.reason = true, ReasonCheckmate
		b.result = BugResultFor(BugTeam(BugSeat(board, mover)))
	}
	return note, nil
}

// Over reports the result: "1-0" team 0 wins, "0-1" team 1 wins.
func (b *Bug) Over() (result, reason string, over bool) { return b.result, b.reason, b.over }

// Finish ends the game by hand: a flag, a resignation (result of BugResultFor).
func (b *Bug) Finish(result, reason string) { b.result, b.reason, b.over = result, reason, true }

// View is the position of one board for a viewer of the side on it (NoSide: a spectator): only the player to
// move is told their legal actions.
func (b *Bug) View(board int, viewer Side) View {
	g, err := b.board(board)
	if err != nil {
		return View{}
	}
	v := g.View(viewer)
	if b.over {
		v.Legal = nil
	}
	return v
}

// Position is the position of a board (to read; Bug changes it).
func (b *Bug) Position(board int) *Position {
	g, err := b.board(board)
	if err != nil {
		return nil
	}
	return g.Position()
}

// BugAction is one action of a bughouse game as it is stored and replayed.
type BugAction struct {
	Board  int    `json:"board"`
	Action string `json:"action"`
}

// ReplayBug rebuilds a game from its actions, the only way one is restored.
func ReplayBug(o Options, actions []BugAction) (*Bug, error) {
	b, err := NewBug(o)
	if err != nil {
		return nil, err
	}
	for i, a := range actions {
		if _, err := b.Apply(a.Board, a.Action); err != nil {
			return nil, fmt.Errorf("bughouse: action %d (%s): %w", i+1, a.Action, err)
		}
	}
	return b, nil
}

// BugVariant is implemented by a Variant that is played by four people on two boards, not through State.
// vplay asks for it before it makes a game.
type BugVariant interface {
	NewBug(o Options) (*Bug, error)
}

type bughouse struct{}

func (bughouse) Info() Info {
	return Info{
		ID:    "bughouse",
		Name:  "Шведские шахматы (Bughouse)",
		Short: "Четверо играют командами по двое на двух досках: взятая вами фигура сразу достаётся партнёру, и он может выставить её на свою доску.",
	}
}

// New is refused: a bughouse game has two boards and four seats (see BugVariant).
func (bughouse) New(Options) (State, error) {
	return nil, errors.New("шведские шахматы играются на двух досках (вариант BugVariant)")
}

func (bughouse) NewBug(o Options) (*Bug, error) { return NewBug(o) }

func init() { Register(bughouse{}) }

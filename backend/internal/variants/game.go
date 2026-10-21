package variants

import (
	"errors"
)

// ErrIllegal is the answer to an action that is not among the legal ones.
var ErrIllegal = errors.New("такой ход невозможен")

// ErrOver is the answer to an action in a game that has ended.
var ErrOver = errors.New("партия окончена")

// View is a position as one viewer is allowed to see it: the part of a game
// snapshot that depends on the variant. The browser draws Board and offers
// Legal; Extra is the variant's own business.
type View struct {
	// Board maps a square to a piece code: {"e4": "wP", "g8": "bN", "c3": "xD"}.
	Board map[string]string `json:"board"`
	// Turn is who acts now (a duck game has two actions of the same side).
	Turn Side `json:"turn"`
	// Legal lists the actions of the viewer, when it is the viewer's turn and
	// the game is going; nobody else is told what could be played.
	Legal []string `json:"legal,omitempty"`
	// Last holds the squares of the last action ("e2", "e4"); a drop gives one.
	Last []string `json:"last,omitempty"`
	// Check holds the squares of the kings in check (none for variants without check).
	Check []string `json:"check,omitempty"`
	// Extra is free JSON of the variant: pockets, dice, squares hidden by fog.
	Extra any `json:"extra,omitempty"`
}

// State is a game of a variant as the server plays it. It is deterministic:
// the same Options and the same actions always give the same state, which is
// how a game is restored from the database (Replay) and why there is no
// Marshal. Randomness must come from Options.Rand().
//
// An action is a string. The base's are a move "e2e4" ("e7e8q" promotes), a
// drop "P@e4". A variant adds its own ("@e5": put the duck on e5) and
// documents them for its browser half.
type State interface {
	// Turn is who must act now. The clock of the server switches when the
	// value changes, so a turn of several actions of one side keeps it.
	Turn() Side
	// Legal lists every action the side to move may take at this step.
	Legal() []string
	// Apply performs one action and returns its notation for the list of moves.
	Apply(action string) (notation string, err error)
	// Over says whether the game has ended: result "1-0", "0-1", "1/2-1/2" and
	// a reason key (ReasonCheckmate, ...).
	Over() (result, reason string, over bool)
	// View is the position for the viewer: White, Black, or NoSide for a spectator.
	View(viewer Side) View
}

// Timeouter is implemented by a State whose result on a fallen flag is not the
// plain loss of the player who ran out of time.
type Timeouter interface {
	Timeout(loser Side) string
}

// Finisher is implemented by a State that wants to be told the game ended by
// something outside its rules (resignation, a fallen flag, an agreed draw), so
// that it stops offering moves and shows everything (fog of war). *Game
// implements it, and so does anything that embeds one.
type Finisher interface {
	Finish(result, reason string)
}

// MoveRedactor is implemented by a State that hides some of the moves from
// some viewers (fog of war). The server shows every viewer what this returns
// for the list of moves; after the end it should return them untouched.
type MoveRedactor interface {
	RedactMoves(viewer Side, moves []MoveRec) []MoveRec
}

// Game is a Position with its history, played under Rules, and a State. A
// variant whose turn is a single move is just a Game; a variant with a longer
// turn embeds or wraps one and uses these methods in its own State: Moves,
// Play, Edit, Finish.
type Game struct {
	rules Rules
	opts  Options
	pos   *Position
	keys  []string // the key of every position so far, the current one last

	legal   []Move
	legalOK bool
	last    []Sq

	over   bool
	result string
	reason string
}

// NewGame starts a game under the rules.
func NewGame(r Rules, o Options) (*Game, error) {
	pos, err := r.Start(o)
	if err != nil {
		return nil, err
	}
	g := &Game{rules: r, opts: o, pos: pos}
	g.keys = []string{positionKey(pos)}
	g.Judge()
	return g, nil
}

// Rules returns the rules the game is played under.
func (g *Game) Rules() Rules { return g.rules }

// Options returns the options the game was made with.
func (g *Game) Options() Options { return g.opts }

// Position is the current position. Read it; to change it use Edit.
func (g *Game) Position() *Position { return g.pos }

// Moves are the legal moves of the side to move, cached until the position changes.
func (g *Game) Moves() []Move {
	if !g.legalOK {
		g.legal = g.pos.LegalMoves(g.rules)
		g.legalOK = true
	}
	return g.legal
}

// Play plays one of Moves and returns its notation. It records the position
// for the repetition rule, remembers the squares for View.Last and judges the
// game (Rules.Result). It does not check that the move is legal or that the
// game is not over: that is Apply's job.
func (g *Game) Play(mv Move) string {
	legal := g.Moves()
	next := g.pos.Apply(g.rules, mv)
	note := g.rules.Notation(g, mv, legal, next)
	g.pos = next
	g.keys = append(g.keys, positionKey(next))
	g.legalOK = false
	g.last = g.last[:0]
	if mv.Drop == NoKind {
		g.last = append(g.last, mv.From)
	}
	g.last = append(g.last, mv.To)
	g.Judge()
	return note
}

// Edit changes the current position directly (put the duck down, give the
// move to the other side again) and judges the game afresh. The position's
// key in the history is replaced, not added.
func (g *Game) Edit(fn func(p *Position)) {
	fn(g.pos)
	g.legalOK = false
	g.keys[len(g.keys)-1] = positionKey(g.pos)
	g.Judge()
}

// SetLast sets the squares View.Last shows (Edit does not touch them).
func (g *Game) SetLast(squares ...Sq) { g.last = append(g.last[:0], squares...) }

// Judge asks the rules whether the game is over now.
func (g *Game) Judge() { g.result, g.reason, g.over = g.rules.Result(g) }

// Finish ends the game by hand, for a rule that lives outside Rules.Result.
func (g *Game) Finish(result, reason string) { g.result, g.reason, g.over = result, reason, true }

// Repetitions counts how many times the current position has occurred.
func (g *Game) Repetitions() int {
	n := 0
	cur := g.keys[len(g.keys)-1]
	for _, k := range g.keys {
		if k == cur {
			n++
		}
	}
	return n
}

// positionKey identifies a position for the repetition rule: the pieces, the
// side to move, the castling rights, the en passant square when a pawn could
// take, the pockets.
func positionKey(p *Position) string {
	b := make([]byte, 0, 64*2+16+24)
	for _, pc := range p.B {
		b = append(b, byte(pc.Kind), byte(pc.Side))
	}
	b = append(b, byte(p.Turn))
	for _, s := range [2]Side{White, Black} {
		b = append(b, byte(p.Castle[s][0]), byte(p.Castle[s][1]))
	}
	if p.epCapturable() {
		b = append(b, byte(p.EP))
	}
	for s := White; s <= Black; s++ {
		for _, n := range p.Pocket[s] {
			b = append(b, byte(n))
		}
	}
	return string(b)
}

// ------------------------------------------------------------ State

// Turn is the side to move.
func (g *Game) Turn() Side { return g.pos.Turn }

// Legal lists the legal actions of the side to move (none once the game is over).
func (g *Game) Legal() []string {
	if g.over {
		return nil
	}
	moves := g.Moves()
	out := make([]string, len(moves))
	for i, m := range moves {
		out[i] = Encode(g.rules, m)
	}
	return out
}

// Apply plays the action if it is legal.
func (g *Game) Apply(action string) (string, error) {
	if g.over {
		return "", ErrOver
	}
	mv, ok := Find(g.rules, g.Moves(), action)
	if !ok {
		return "", ErrIllegal
	}
	return g.Play(mv), nil
}

// Over reports the result of the game.
func (g *Game) Over() (string, string, bool) { return g.result, g.reason, g.over }

// Timeout is the result when the loser's flag falls.
func (g *Game) Timeout(loser Side) string { return g.rules.Timeout(g, loser) }

// View builds the view for the viewer, then lets the rules hide what is not theirs.
func (g *Game) View(viewer Side) View {
	v := View{Board: g.pos.Board(), Turn: g.pos.Turn, Extra: g.rules.Extra(g, viewer)}
	if !g.over && viewer != NoSide && viewer == g.pos.Turn {
		v.Legal = g.Legal()
	}
	for _, s := range g.last {
		v.Last = append(v.Last, s.String())
	}
	if g.rules.Check(g.pos, g.pos.Turn) {
		if k := g.pos.KingSq(g.pos.Turn); k != NoSq {
			v.Check = append(v.Check, k.String())
		}
	}
	g.rules.Redact(g, viewer, &v)
	return v
}

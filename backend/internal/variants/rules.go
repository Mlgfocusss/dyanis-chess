package variants

import (
	"math/rand"
)

// Options are what a game of a variant is created with. The server draws the
// Seed when the game is made and stores it with the game, so a game restored
// from the database (by replaying its actions) meets the same random choices:
// the starting setup of Chess960, the dice. Params are the choices a player
// made in the lobby (a variant checks them in OptionsChecker).
type Options struct {
	Seed   int64          `json:"seed,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

// Rand is a generator seeded with the options' seed. Every call returns a
// fresh generator at the start of the sequence; keep the one you take.
func (o Options) Rand() *rand.Rand { return rand.New(rand.NewSource(o.Seed)) }

// Int reads a numeric parameter (JSON numbers arrive as float64).
func (o Options) Int(name string, def int) int {
	switch v := o.Params[name].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return def
}

// String reads a text parameter.
func (o Options) String(name, def string) string {
	if v, ok := o.Params[name].(string); ok {
		return v
	}
	return def
}

// MoveInfo tells AfterMove what the base has just done.
type MoveInfo struct {
	Move Move
	Side Side // who moved (the position's Turn has already passed to the other side)
	// Piece is the piece that moved (before a promotion), or the piece dropped.
	Piece Piece
	// Captured is the piece taken, and CapturedAt the square it stood on (not
	// the target square for en passant). Captured is empty when nothing was taken.
	Captured   Piece
	CapturedAt Sq
}

// Rules are the places where a variant differs from the classical game. The
// base (movegen.go, game.go) asks the Rules for each of them; ClassicRules
// answers the classical way. A variant embeds ClassicRules (a struct with no
// fields) and defines only the methods it changes; the embedded methods stay
// reachable as r.ClassicRules.Method(...).
//
// Which hook serves which need:
//
//	(a) other setup, random starts    Start (uses Options.Seed)
//	(b) capture rules, side effects   CanCapture, AfterMove
//	(c) other ways to win             Result
//	(d) no check, king may be taken   Check (always false), CastleSafe, Result
//	(e) drops from a pocket           ExtraMoves, AfterMove (fills the pocket), Position.Pocket
//	(f) other castling                Start (rights), CastleAsKingTakesRook, CastleSafe
//	(g) hidden information            Redact (and Extra for the variant's own data)
//	    other pawn moves              PawnDoubleStep
//	    other text of a move          Notation
//
// The multi-action turn (d in the plan: the duck, three dice moves) is not a
// Rules matter: the variant writes a State that wraps a Game (see game.go).
type Rules interface {
	// Start returns the initial position for the options.
	Start(o Options) (*Position, error)

	// PawnDoubleStep: may the pawn on `from` move two squares (when both
	// squares ahead are empty), and may it then be taken en passant. The
	// classical answer is true,true from the second rank.
	PawnDoubleStep(p *Position, from Sq) (allowed, setsEP bool)

	// CanCapture: may the move (a pseudo-legal one, taking `victim`) capture. A
	// piece of your own side or the duck can never be taken whatever this says.
	CanCapture(p *Position, mv Move, victim Piece) bool

	// ExtraMoves are the pseudo-legal moves besides the board moves and the
	// castlings of the side to move: drops from the pocket.
	ExtraMoves(p *Position) []Move

	// CastleSafe: may the king of `side` stand on, or pass over, the square
	// during a castling. The classical answer is "it is not attacked". The
	// king's destination is not asked here; the check filter judges the final position.
	CastleSafe(p *Position, side Side, sq Sq) bool

	// CastleAsKingTakesRook: write a castling as the king moving onto its own
	// rook ("e1h1"), as Chess960 needs, and not as the king's two squares ("e1g1").
	CastleAsKingTakesRook() bool

	// Check says whether `side` is in check (shown on the board, and
	// the filter that refuses moves leaving the mover in check, and the "+" and
	// "#" of the notation).
	Check(p *Position, side Side) bool

	// Legal is an extra veto on a pseudo-legal move of p.Turn, given the
	// position `next` it leads to. The base has already refused every move after
	// which Check(next, mover) is true, so the classical answer is always true.
	// (To allow moves into check, make Check return false.)
	Legal(p *Position, mv Move, next *Position) bool

	// AfterMove may change the position after the base has played a move and
	// passed the turn: explosions, pieces into the pocket.
	AfterMove(next *Position, info MoveInfo)

	// Result judges the game after each action. result is "1-0", "0-1" or
	// "1/2-1/2"; reason is a short key ("checkmate", "stalemate", ...).
	Result(g *Game) (result, reason string, over bool)

	// Notation writes the move that is being played from g.Position() into
	// `next`; it is the text the list of moves shows.
	Notation(g *Game, mv Move, legal []Move, next *Position) string

	// Timeout is the result when `loser` runs out of time.
	Timeout(g *Game, loser Side) string

	// Extra is the variant's own JSON for the viewer (View.Extra): the
	// pockets, the dice, the hidden squares.
	Extra(g *Game, viewer Side) any

	// Redact removes from the view what the viewer must not see.
	Redact(g *Game, viewer Side, v *View)
}

// ClassicRules is classical chess: the answer of every hook. Embed it.
type ClassicRules struct{}

// Start is the initial position of chess.
func (ClassicRules) Start(Options) (*Position, error) { return ParseFEN(StartFEN) }

// PawnDoubleStep: from the second rank (the seventh for Black).
func (ClassicRules) PawnDoubleStep(p *Position, from Sq) (bool, bool) {
	home := 1
	if p.B[from].Side == Black {
		home = 6
	}
	return from.Rank() == home, from.Rank() == home
}

// CanCapture is true for anything but the duck.
func (ClassicRules) CanCapture(_ *Position, _ Move, victim Piece) bool { return victim.Kind != Duck }

// ExtraMoves: none.
func (ClassicRules) ExtraMoves(*Position) []Move { return nil }

// CastleSafe: not attacked.
func (ClassicRules) CastleSafe(p *Position, side Side, sq Sq) bool {
	return !p.Attacked(sq, side.Opp())
}

// CastleAsKingTakesRook is false: the king's two squares.
func (ClassicRules) CastleAsKingTakesRook() bool { return false }

// Check: the king is attacked.
func (ClassicRules) Check(p *Position, side Side) bool { return p.InCheck(side) }

// Legal vetoes nothing (the check filter is the base's).
func (ClassicRules) Legal(*Position, Move, *Position) bool { return true }

// AfterMove does nothing.
func (ClassicRules) AfterMove(*Position, MoveInfo) {}

// Result is ClassicResult.
func (ClassicRules) Result(g *Game) (string, string, bool) { return ClassicResult(g) }

// Notation is standard algebraic notation (SAN) with "+" and "#".
func (r ClassicRules) Notation(g *Game, mv Move, legal []Move, next *Position) string {
	return SAN(g.rules, g.pos, legal, mv, next)
}

// Timeout: the side that ran out loses, unless the other side could never mate.
func (ClassicRules) Timeout(g *Game, loser Side) string { return ClassicTimeout(g.pos, loser) }

// Extra: nothing.
func (ClassicRules) Extra(*Game, Side) any { return nil }

// Redact: nothing is hidden.
func (ClassicRules) Redact(*Game, Side, *View) {}

// Results.
const (
	ResultWhite = "1-0"
	ResultBlack = "0-1"
	ResultDraw  = "1/2-1/2"
)

// ResultFor is the result in which `winner` wins.
func ResultFor(winner Side) string {
	if winner == White {
		return ResultWhite
	}
	return ResultBlack
}

// Reasons a game ends, the keys of View-independent texts in the browser.
const (
	ReasonCheckmate    = "checkmate"
	ReasonStalemate    = "stalemate"
	ReasonInsufficient = "insufficient-material"
	ReasonFiftyMoves   = "fifty-moves"
	ReasonRepetition   = "repetition"
	ReasonKingCaptured = "king-captured"
	ReasonNoMoves      = "no-moves"
)

// ClassicResult judges a game by the classical rules: mate, stalemate, dead
// material, the fifty-move rule and the threefold repetition, in this order of
// strength (as the engine does).
func ClassicResult(g *Game) (string, string, bool) {
	p := g.pos
	if len(g.Moves()) == 0 {
		if g.rules.Check(p, p.Turn) {
			return ResultFor(p.Turn.Opp()), ReasonCheckmate, true
		}
		return ResultDraw, ReasonStalemate, true
	}
	if InsufficientMaterial(p) {
		return ResultDraw, ReasonInsufficient, true
	}
	if p.Half >= 100 {
		return ResultDraw, ReasonFiftyMoves, true
	}
	if g.Repetitions() >= 3 {
		return ResultDraw, ReasonRepetition, true
	}
	return "", "", false
}

// InsufficientMaterial: neither side can ever give mate. As the engine counts
// it: no pawn, rook or queen, and a lone minor piece against a king, or one
// bishop each on squares of one colour.
func InsufficientMaterial(p *Position) bool {
	var minors [3]int
	var bishop [3]Sq
	bishop[White], bishop[Black] = NoSq, NoSq
	for i, pc := range p.B {
		switch pc.Kind {
		case Pawn, Rook, Queen:
			return false
		case Knight:
			minors[pc.Side]++
		case Bishop:
			minors[pc.Side]++
			if bishop[pc.Side] == NoSq {
				bishop[pc.Side] = Sq(i)
			}
		}
	}
	w, b := minors[White], minors[Black]
	switch {
	case w == 0 && b == 0, w+b == 1:
		return true
	case w == 1 && b == 1 && bishop[White] != NoSq && bishop[Black] != NoSq:
		return (bishop[White].File()+bishop[White].Rank())%2 == (bishop[Black].File()+bishop[Black].Rank())%2
	}
	return false
}

// CanMate: may the side ever give mate (a pawn, rook or queen, or two minor pieces).
func CanMate(p *Position, side Side) bool {
	minors := 0
	for _, pc := range p.B {
		if pc.Side != side {
			continue
		}
		switch pc.Kind {
		case Pawn, Rook, Queen:
			return true
		case Knight, Bishop:
			minors++
		}
	}
	return minors >= 2
}

// ClassicTimeout: the loser's opponent wins, but a flag against a side that
// could never be mated... the other way round: when the opponent cannot mate
// at all, the game is a draw.
func ClassicTimeout(p *Position, loser Side) string {
	if !CanMate(p, loser.Opp()) {
		return ResultDraw
	}
	return ResultFor(loser.Opp())
}

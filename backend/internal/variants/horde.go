package variants

// Horde, as on Lichess.
//
//   - White has no king and 36 pawns: all of the first four ranks and b5, c5,
//     f5, g5. Black has the ordinary army and moves second.
//   - White pawns on the first rank may step two squares (when both squares are
//     empty); a pawn that does it may not be taken en passant. Pawns on the
//     second rank step two squares as usual, and may be taken en passant.
//   - White wins by checkmating the black king. Black wins by taking every
//     white piece. White has no king, so it is never in check, and when it has
//     no move (every pawn blocked) the game is a stalemate: a draw.
//   - No insufficient-material draw: Black can always win by taking the horde,
//     so as in Lichess the game is decided, or drawn by stalemate, the fifty-move
//     rule or repetition.
//
// A deviation from Lichess: when Black's flag falls, White wins unless it could
// never mate (see hordeCanMate); Lichess's table of such endings is longer.

// ReasonHordeDestroyed is the end of a horde game by the last white piece taken.
const ReasonHordeDestroyed = "horde-destroyed"

// HordeFEN is the initial position of Horde.
const HordeFEN = "rnbqkbnr/pppppppp/8/1PP2PP1/PPPPPPPP/PPPPPPPP/PPPPPPPP/PPPPPPPP w kq - 0 1"

type hordeRules struct{ ClassicRules }

func (hordeRules) Start(Options) (*Position, error) { return ParseFEN(HordeFEN) }

// PawnDoubleStep: the first rank for White (without en passant), the second
// rank for White and the seventh for Black as usual.
func (hordeRules) PawnDoubleStep(p *Position, from Sq) (bool, bool) {
	if p.B[from].Side == White {
		return from.Rank() <= 1, from.Rank() == 1
	}
	return from.Rank() == 6, from.Rank() == 6
}

// hordePieces counts the white pieces left.
func hordePieces(p *Position) int {
	n := 0
	for _, pc := range p.B {
		if pc.Side == White && !pc.Empty() {
			n++
		}
	}
	return n
}

func (hordeRules) Result(g *Game) (string, string, bool) {
	p := g.Position()
	if hordePieces(p) == 0 {
		return ResultBlack, ReasonHordeDestroyed, true
	}
	if len(g.Moves()) == 0 {
		if g.rules.Check(p, p.Turn) {
			return ResultFor(p.Turn.Opp()), ReasonCheckmate, true
		}
		return ResultDraw, ReasonStalemate, true
	}
	if p.Half >= 100 {
		return ResultDraw, ReasonFiftyMoves, true
	}
	if g.Repetitions() >= 3 {
		return ResultDraw, ReasonRepetition, true
	}
	return "", "", false
}

// Timeout: Black's flag wins for White only if White could ever mate.
func (hordeRules) Timeout(g *Game, loser Side) string {
	if loser == Black && !hordeCanMate(g.Position()) {
		return ResultDraw
	}
	return ResultFor(loser.Opp())
}

// hordeCanMate: can White, with what is left, ever mate. A pawn promotes, a
// rook or queen mates; minor pieces need a second one, or a black army that can
// shut its own king in.
func hordeCanMate(p *Position) bool {
	minors, otherBlack := 0, false
	for _, pc := range p.B {
		switch {
		case pc.Side == Black && pc.Kind != King && !pc.Empty():
			otherBlack = true
		case pc.Side == White:
			if pc.Kind == Pawn || pc.Kind == Rook || pc.Kind == Queen {
				return true
			}
			minors++
		}
	}
	return minors >= 2 || minors == 1 && otherBlack
}

// Extra tells the browser how big the horde still is.
func (hordeRules) Extra(g *Game, _ Side) any {
	return map[string]any{"horde": hordePieces(g.Position())}
}

type horde struct{}

func (horde) Info() Info {
	return Info{
		ID:    "horde",
		Name:  "Орда",
		Short: "У белых 36 пешек и нет короля; белые побеждают матом, чёрные, взяв все белые фигуры.",
	}
}

func (horde) New(o Options) (State, error) { return NewGame(hordeRules{}, o) }

func init() { Register(horde{}) }

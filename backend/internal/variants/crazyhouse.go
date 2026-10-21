package variants

// Crazyhouse, as on Lichess: a piece taken goes into the taker's pocket (a
// promoted piece comes back as a pawn) and the side to move may, instead of a
// move, drop a piece from its pocket on any empty square (a pawn not on the
// first or the last rank). A drop may answer a check and may give mate; castling
// and en passant are the usual.
//
// Decisions on the end of the game:
//   - mate and stalemate as in chess (a drop that blocks a check counts as an
//     escape; a stalemate is a draw, as on Lichess);
//   - the threefold repetition and the fifty-move rule are kept as in the base
//     (the pockets are part of the position, and a drop or a capture resets the
//     fifty-move clock, so that rule hardly ever fires);
//   - insufficient material does not exist: a pocket can always bring material
//     back, so a bare king is never "dead" (the base's InsufficientMaterial is
//     not asked) and a fallen flag always loses (never a draw against a bare king).
type crazyhouseRules struct{ ClassicRules }

func (crazyhouseRules) ExtraMoves(p *Position) []Move { return DropMoves(p) }

// AfterMove: the piece taken goes into the taker's pocket.
func (crazyhouseRules) AfterMove(next *Position, info MoveInfo) {
	if info.Captured.Empty() {
		return
	}
	next.Pocket[info.Side][PocketKind(info.Captured)]++
}

func (crazyhouseRules) Result(g *Game) (string, string, bool) {
	p := g.Position()
	if len(g.Moves()) == 0 {
		if g.Rules().Check(p, p.Turn) {
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

// Timeout: whoever has time left wins; nothing is "insufficient" with pockets.
func (crazyhouseRules) Timeout(_ *Game, loser Side) string { return ResultFor(loser.Opp()) }

func (crazyhouseRules) Extra(g *Game, _ Side) any { return PocketView(g.Position()) }

type crazyhouse struct{}

func (crazyhouse) Info() Info {
	return Info{
		ID:    "crazyhouse",
		Name:  "Crazyhouse",
		Short: "Взятые фигуры уходят в ваш резерв, и вместо хода их можно выставить обратно на любое свободное поле.",
	}
}

func (crazyhouse) New(o Options) (State, error) { return NewGame(crazyhouseRules{}, o) }

func init() { Register(crazyhouse{}) }

package variants

import "strings"

// Atomic chess, as on Lichess.
//
//   - A capture is an explosion centred on the square the capturing piece
//     lands on: the capturing piece, the captured piece and every piece on the
//     eight squares around (pawns excepted) disappear. A pawn next to the blast
//     survives; a pawn that takes, or is taken, goes.
//   - A king may never take (it would blow itself up).
//   - A move that blows up your own king is illegal. A move that blows up the
//     enemy king wins on the spot, even when your own king stands in check.
//   - Kings may stand next to each other. While they touch, nobody is in check:
//     taking a king next to the other king would destroy the taker's king too.
//   - Checkmate and stalemate as in chess (stalemate is a draw).
//
// Deviations from Lichess, recorded in VARIANTS-PLAN.md: the castling path is
// judged without the enemy king's attacks (it cannot take anyway); the
// insufficient-material rule is a conservative reading of Lichess's (see
// atomicCanWin); a capturing promotion is offered as a queen only, because the
// promoted piece explodes at once and the choice would mean nothing.

// ReasonKingExploded is the end of an atomic game by a king blown up.
const ReasonKingExploded = "king-exploded"

type atomicRules struct{ ClassicRules }

func kingsTouch(a, b Sq) bool {
	df, dr := a.File()-b.File(), a.Rank()-b.Rank()
	return df >= -1 && df <= 1 && dr >= -1 && dr <= 1
}

// Check: the king is attacked, unless the kings touch. A side whose king is
// gone counts as in check (every move of it is then refused, which is how a
// move that blows up your own king is outlawed); a move after which the enemy
// king is gone is never refused.
func (atomicRules) Check(p *Position, side Side) bool {
	mine, theirs := p.KingSq(side), p.KingSq(side.Opp())
	switch {
	case mine == NoSq:
		return true
	case theirs == NoSq:
		return false
	case kingsTouch(mine, theirs):
		return false
	}
	return p.Attacked(mine, side.Opp())
}

// CanCapture: everything but a king may take.
func (atomicRules) CanCapture(p *Position, mv Move, victim Piece) bool {
	return victim.Kind != Duck && p.B[mv.From].Kind != King
}

// Legal vetoes the promotions of a capture other than the queen: the piece
// explodes the moment it lands, so the four choices are one move.
func (atomicRules) Legal(p *Position, mv Move, next *Position) bool {
	if mv.Promo != NoKind && mv.Promo != Queen && !p.B[mv.To].Empty() {
		return false
	}
	return true
}

// CastleSafe: the path must not be attacked by anything that can take; the
// enemy king cannot, in this game.
func (atomicRules) CastleSafe(p *Position, side Side, sq Sq) bool {
	q := *p
	if k := q.KingSq(side.Opp()); k != NoSq {
		q.B[k] = Piece{}
	}
	return !q.Attacked(sq, side.Opp())
}

// AfterMove explodes a capture.
func (atomicRules) AfterMove(next *Position, info MoveInfo) {
	if info.Captured.Empty() {
		return
	}
	at := info.Move.To
	next.B[at] = Piece{}
	for df := -1; df <= 1; df++ {
		for dr := -1; dr <= 1; dr++ {
			if s := step(at, df, dr); s != NoSq && next.B[s].Kind != Pawn {
				next.B[s] = Piece{}
			}
		}
	}
	next.FixCastling()
}

// Result: a missing king loses, then mate and stalemate, then the material and
// the move counters.
func (atomicRules) Result(g *Game) (string, string, bool) {
	p := g.Position()
	for _, s := range [2]Side{White, Black} {
		if p.KingSq(s) == NoSq {
			return ResultFor(s.Opp()), ReasonKingExploded, true
		}
	}
	if len(g.Moves()) == 0 {
		if g.rules.Check(p, p.Turn) {
			return ResultFor(p.Turn.Opp()), ReasonCheckmate, true
		}
		return ResultDraw, ReasonStalemate, true
	}
	if !atomicCanWin(p, White) && !atomicCanWin(p, Black) {
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

// Notation is SAN without the "+" and "#" of a king that was blown up.
func (atomicRules) Notation(g *Game, mv Move, legal []Move, next *Position) string {
	s := SAN(g.rules, g.pos, legal, mv, next)
	if next.KingSq(next.Turn) == NoSq {
		s = strings.TrimRight(s, "+#")
	}
	return s
}

// Timeout: a flag wins unless the other side could never win.
func (atomicRules) Timeout(g *Game, loser Side) string {
	if !atomicCanWin(g.Position(), loser.Opp()) {
		return ResultDraw
	}
	return ResultFor(loser.Opp())
}

// atomicCanWin: can `color` still win by some (cooperative) sequence of moves.
//
//   - It cannot with no piece but the king.
//   - While the enemy has pieces besides the king there is always a chance to
//     blow one up next to the king, except in the end of bishops only, where
//     bishops of opposite square colours can never take each other.
//   - Against a bare king the side must mate: a queen or a pawn can; a lone
//     knight, bishop or rook cannot; two or more of knights, bishops and rooks
//     can, except bishops all on one colour.
func atomicCanWin(p *Position, color Side) bool {
	enemy := color.Opp()
	if p.KingSq(enemy) == NoSq {
		return true
	}
	if p.KingSq(color) == NoSq {
		return false
	}
	var mine, theirs, queensPawns, minorsRooks int
	onlyBishops, myOnlyBishops := true, true
	var bishopColours [3][2]bool // [side][colour of the square]
	for i, pc := range p.B {
		if pc.Kind == NoKind || pc.Kind == King {
			continue
		}
		if pc.Kind == Bishop {
			bishopColours[pc.Side][(Sq(i).File()+Sq(i).Rank())%2] = true
		} else {
			onlyBishops = false
			if pc.Side == color {
				myOnlyBishops = false
			}
		}
		if pc.Side != color {
			theirs++
			continue
		}
		mine++
		if pc.Kind == Queen || pc.Kind == Pawn {
			queensPawns++
		} else {
			minorsRooks++
		}
	}
	switch {
	case mine == 0:
		return false
	case theirs > 0 && onlyBishops:
		// bishops of opposite colours never take each other
		return bishopColours[color][0] && bishopColours[enemy][0] ||
			bishopColours[color][1] && bishopColours[enemy][1]
	case theirs > 0, queensPawns > 0:
		return true
	case minorsRooks < 2:
		return false
	}
	// two or more knights, bishops, rooks against a bare king: mate is possible
	// unless they are bishops that all stand on one colour
	return !(myOnlyBishops && !(bishopColours[color][0] && bishopColours[color][1]))
}

// ------------------------------------------------------------------- State

// blast is what the last capture destroyed, for the browser.
type blast struct {
	Center  string            `json:"center"`
	Squares []string          `json:"squares"`
	Pieces  map[string]string `json:"pieces"` // square -> code of the piece that stood there
}

// atomicGame is a Game that remembers the last explosion.
type atomicGame struct {
	*Game
	blast *blast
}

func (a *atomicGame) Apply(action string) (string, error) {
	before := a.Game.Position().B
	note, err := a.Game.Apply(action)
	if err != nil {
		return "", err
	}
	after := a.Game.Position().B
	count := func(b [64]Piece) (n int) {
		for _, pc := range b {
			if !pc.Empty() {
				n++
			}
		}
		return
	}
	a.blast = nil
	if count(after) < count(before) && len(a.Game.last) == 2 {
		from, to := a.Game.last[0], a.Game.last[1]
		bl := &blast{Center: to.String(), Pieces: map[string]string{}}
		for s := Sq(0); s < 64; s++ {
			if s != from && !before[s].Empty() && after[s].Empty() {
				bl.Squares = append(bl.Squares, s.String())
				bl.Pieces[s.String()] = before[s].Code()
			}
		}
		a.blast = bl
	}
	return note, nil
}

// View adds the last explosion to the common view.
func (a *atomicGame) View(viewer Side) View {
	v := a.Game.View(viewer)
	if a.blast != nil {
		v.Extra = map[string]any{"blast": a.blast}
	}
	return v
}

type atomic struct{}

func (atomic) Info() Info {
	return Info{
		ID:    "atomic",
		Name:  "Атомные шахматы",
		Short: "Каждое взятие взрывает всё вокруг, кроме пешек; побеждает тот, кто взорвал короля соперника.",
	}
}

func (atomic) New(o Options) (State, error) {
	g, err := NewGame(atomicRules{}, o)
	if err != nil {
		return nil, err
	}
	return &atomicGame{Game: g}, nil
}

func init() { Register(atomic{}) }

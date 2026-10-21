package variants

import "strings"

// Fog of war, as on Chess.com. There is no check and no mate: a king may walk
// into attack and the game ends when a king is taken. A player sees their own
// pieces, every square one of their pieces could move to (empty or not) and
// the enemy pieces standing on those squares; the rest of the board is fog and
// the enemy pieces in it do not exist for that player. The rules are the
// classical ones minus check (castling is allowed through attacked squares).
//
// What decides "sees", written once in fogVisible:
//   - own pieces;
//   - the target square of every move the side could make (a slider stops at the
//     first piece, so a hidden piece behind another is not seen; the piece it
//     stops at is on a capture square and is seen);
//   - the square of the pawn taken en passant;
//   - the one square ahead of a pawn and the second one when the first is empty
//     and the pawn may step two, whoever stands there: a blocked pawn sees its
//     blocker. The diagonals of a pawn show only as capture squares, that is
//     when an enemy stands there (Chess.com does the same);
//   - the square where the enemy piece has just taken one of yours, until you
//     move (you are told what took your piece).
//
// Nothing hidden may reach a player. The snapshot of a player is a function of
// what they see only (TestFogNoninterference proves it by changing the hidden
// pieces); the list of moves shows the opponent's moves only when their target
// square was in view, without the starting square.
//
// The spectator sees no board until the game ends. No automatic draws except
// the stalemate: repetition, fifty moves and dead material would tell what the
// fog hides. An agreed draw, resignation and the flag are the server's.
type fogRules struct{ ClassicRules }

func (fogRules) Check(*Position, Side) bool          { return false }
func (fogRules) CastleSafe(*Position, Side, Sq) bool { return true }

// Result: the side without a king has lost; a side that cannot move draws.
func (fogRules) Result(g *Game) (string, string, bool) {
	p := g.Position()
	for _, s := range [2]Side{White, Black} {
		if p.KingSq(s) == NoSq {
			return ResultFor(s.Opp()), ReasonKingCaptured, true
		}
	}
	if len(g.Moves()) == 0 {
		return ResultDraw, ReasonStalemate, true
	}
	return "", "", false
}

// Timeout: any piece can take a king, so the one who still has time wins.
func (fogRules) Timeout(_ *Game, loser Side) string { return ResultFor(loser.Opp()) }

// hiddenMove is what the list shows for a move the viewer did not see.
const hiddenMove = "…"

// fogPly is what the opponent may know of a move: the text they see (no
// starting square) and whether the target square was in their view.
type fogPly struct {
	short string
	seen  bool
}

// fogGameState is the State of the fog of war: a Game and the memory of what each
// move showed to the other side.
type fogGameState struct {
	*Game
	plies    []fogPly
	revealAt Sq // where the last move took a piece (the loser of the piece sees the taker); NoSq if it took nothing
}

type fog struct{}

func (fog) Info() Info {
	return Info{
		ID:    "fog",
		Name:  "Туман войны",
		Short: "Видны только ваши фигуры и поля, на которые они могут сходить; шаха нет, побеждает взявший короля.",
	}
}

func (fog) New(o Options) (State, error) {
	g, err := NewGame(fogRules{}, o)
	if err != nil {
		return nil, err
	}
	return &fogGameState{Game: g, revealAt: NoSq}, nil
}

func init() { Register(fog{}) }

// fogVisible lists the squares `side` sees in p. reveal is the extra square the
// side was shown (the taker of its piece), NoSq for none.
func fogVisible(r Rules, p *Position, side Side, reveal Sq) [64]bool {
	var vis [64]bool
	q := *p
	q.Turn = side
	if p.Turn != side {
		q.EP = NoSq // the en passant square belongs to the side to move
	}
	dr := 1
	if side == Black {
		dr = -1
	}
	for i := 0; i < 64; i++ {
		pc := q.B[i]
		if pc.Kind == NoKind || pc.Side != side {
			continue
		}
		vis[i] = true
		if pc.Kind != Pawn {
			continue
		}
		from := Sq(i)
		one := step(from, 0, dr)
		if one == NoSq {
			continue
		}
		vis[one] = true
		if q.B[one].Empty() {
			if ok, _ := r.PawnDoubleStep(&q, from); ok {
				if two := step(from, 0, 2*dr); two != NoSq {
					vis[two] = true
				}
			}
		}
	}
	for _, mv := range q.PseudoMoves(r) {
		vis[mv.To] = true
		if mv.EnPassant {
			vis[MakeSq(mv.To.File(), mv.From.Rank())] = true
		}
	}
	if reveal != NoSq {
		vis[reveal] = true
	}
	return vis
}

// seen is what `viewer` sees now.
func (f *fogGameState) seen(viewer Side) [64]bool {
	reveal := NoSq
	if f.revealAt != NoSq && f.Turn() == viewer { // the viewer has not answered the taking yet
		reveal = f.revealAt
	}
	return fogVisible(f.Game.rules, f.Game.Position(), viewer, reveal)
}

// Apply plays the action and remembers what it showed to the other side.
func (f *fogGameState) Apply(action string) (string, error) {
	g := f.Game
	if _, _, over := g.Over(); over {
		return "", ErrOver
	}
	mv, ok := Find(g.rules, g.Moves(), action)
	if !ok {
		return "", ErrIllegal
	}
	before := g.Position()
	mover := before.Turn
	piece := before.B[mv.From]
	taken := !before.B[mv.To].Empty() || mv.EnPassant
	note := g.Play(mv)
	f.revealAt = NoSq
	if taken {
		f.revealAt = mv.To
	}
	ply := fogPly{short: fogShort(piece, mv, taken)}
	if _, _, over := g.Over(); !over {
		ply.seen = f.seen(mover.Opp())[mv.To]
	}
	f.plies = append(f.plies, ply)
	return note, nil
}

// fogShort writes a move for the opponent: the piece, the capture and the
// target square, no starting square ("Nxe4", "d5", "xd5", "O-O", "e8=Q").
func fogShort(piece Piece, mv Move, taken bool) string {
	if mv.Castle {
		if mv.To.File() == 2 {
			return "O-O-O"
		}
		return "O-O"
	}
	var b strings.Builder
	if piece.Kind != Pawn {
		b.WriteByte(piece.Kind.Letter())
	}
	if taken {
		b.WriteByte('x')
	}
	b.WriteString(mv.To.String())
	if mv.Promo != NoKind {
		b.WriteByte('=')
		b.WriteByte(mv.Promo.Letter())
	}
	return b.String()
}

// View shows the viewer what they see. After the end everything is open; the
// spectator of a game in progress sees no board.
func (f *fogGameState) View(viewer Side) View {
	v := f.Game.View(viewer)
	if _, _, over := f.Game.Over(); over {
		return v
	}
	if viewer == NoSide {
		v.Board, v.Legal, v.Last, v.Check = map[string]string{}, nil, nil, nil
		all := make([]string, 64)
		for i := range all {
			all[i] = Sq(i).String()
		}
		v.Extra = map[string]any{"fog": all, "spectator": true}
		return v
	}
	vis := f.seen(viewer)
	board := make(map[string]string, len(v.Board))
	for name, code := range v.Board {
		if s, ok := ParseSq(name); ok && vis[s] {
			board[name] = code
		}
	}
	v.Board = board
	// the viewer's own move is theirs to see whole; the opponent's only where it is in view
	if f.Turn() == viewer {
		var last []string
		for _, name := range v.Last {
			if s, ok := ParseSq(name); ok && vis[s] {
				last = append(last, name)
			}
		}
		v.Last = last
	}
	v.Check = nil
	fogged := []string{}
	for i := 0; i < 64; i++ {
		if !vis[i] {
			fogged = append(fogged, Sq(i).String())
		}
	}
	v.Extra = map[string]any{"fog": fogged}
	return v
}

// RedactMoves hides the opponent's moves the viewer did not see: the starting
// square never shows, and a move whose target was out of view shows as "…".
func (f *fogGameState) RedactMoves(viewer Side, moves []MoveRec) []MoveRec {
	if _, _, over := f.Game.Over(); over {
		return moves
	}
	out := make([]MoveRec, len(moves))
	for i, m := range moves {
		if viewer == NoSide || m.Side != viewer {
			m.Action = ""
			if viewer != NoSide && i < len(f.plies) && f.plies[i].seen {
				m.SAN = f.plies[i].short
			} else {
				m.SAN = hiddenMove
			}
		}
		out[i] = m
	}
	return out
}

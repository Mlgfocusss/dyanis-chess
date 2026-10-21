package variants

// Pockets: what Crazyhouse and Bughouse share. A side holds the pieces it has
// taken (Crazyhouse) or been given by its partner (Bughouse) in Position.Pocket
// and may drop one of them on an empty square instead of making a move.

// pocketKinds are the kinds a pocket can hold, in the order the browser shows them.
var pocketKinds = [...]Kind{Queen, Rook, Bishop, Knight, Pawn}

// PocketExtra is the JSON of View.Extra for a variant with pockets: how many
// pieces of each kind (letters Q R B N P) each side holds, and the squares of
// promoted pieces (they go back into a pocket as pawns when taken).
type PocketExtra struct {
	White    map[string]int `json:"white"`
	Black    map[string]int `json:"black"`
	Promoted []string       `json:"promoted,omitempty"`
}

// PocketOf counts the pieces a side holds, by letter.
func PocketOf(p *Position, s Side) map[string]int {
	m := map[string]int{}
	for _, k := range pocketKinds {
		if n := p.Pocket[s][k]; n > 0 {
			m[string([]byte{k.Letter()})] = int(n)
		}
	}
	return m
}

// PocketView is the Extra of a position with pockets.
func PocketView(p *Position) PocketExtra {
	e := PocketExtra{White: PocketOf(p, White), Black: PocketOf(p, Black)}
	for i, pc := range p.B {
		if pc.Promoted && pc.Kind != NoKind {
			e.Promoted = append(e.Promoted, Sq(i).String())
		}
	}
	return e
}

// DropMoves are the drops of the side to move: every piece it holds on every
// empty square, a pawn not on the first or the last rank. (The check filter of
// the base is applied to them like to any move: a drop may block a check.)
func DropMoves(p *Position) []Move {
	var out []Move
	for _, k := range pocketKinds {
		if p.Pocket[p.Turn][k] <= 0 {
			continue
		}
		for s := Sq(0); s < 64; s++ {
			if !p.B[s].Empty() {
				continue
			}
			if k == Pawn && (s.Rank() == 0 || s.Rank() == 7) {
				continue
			}
			out = append(out, Move{From: NoSq, To: s, Drop: k, Rook: NoSq})
		}
	}
	return out
}

// PocketKind is the kind a captured piece becomes in the taker's hand: a
// promoted piece is a pawn again.
func PocketKind(captured Piece) Kind {
	if captured.Promoted {
		return Pawn
	}
	return captured.Kind
}

// CapturedBy tells what the move takes on p (the position before it), the empty
// piece for a move that takes nothing. It is the base's own reading of a move
// (en passant included), for a variant that has to know before it plays.
func CapturedBy(p *Position, mv Move) Piece {
	if mv.Drop != NoKind || mv.Castle {
		return Piece{}
	}
	at := mv.To
	if mv.EnPassant {
		at = MakeSq(mv.To.File(), mv.From.Rank())
	}
	v := p.B[at]
	if v.Side == p.Turn || v.Kind == Duck {
		return Piece{}
	}
	return v
}

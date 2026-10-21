package variants

import (
	"fmt"
	"strconv"
	"strings"
)

// StartFEN is the classical initial position.
const StartFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"

// Position is a board with everything that decides which moves exist. It is a
// plain value: Apply returns a new one, and copying is cheap.
type Position struct {
	B    [64]Piece
	Turn Side
	// Castle[side][0] is the square of the rook that may still castle king
	// side, Castle[side][1] queen side; NoSq when there is no right. The king is
	// wherever the side's king stands (the right is lost the moment it moves).
	Castle [3][2]Sq
	// EP is the square a pawn may take onto en passant, or NoSq.
	EP Sq
	// Half is the half-move clock for the fifty-move rule, Full the move number.
	Half, Full int
	// Pocket[side][kind] counts the pieces a side holds in hand (crazyhouse).
	Pocket [3][8]int8
}

// EmptyPosition is a board with no pieces, White to move.
func EmptyPosition() *Position {
	p := &Position{Turn: White, EP: NoSq, Full: 1}
	for s := range p.Castle {
		p.Castle[s] = [2]Sq{NoSq, NoSq}
	}
	return p
}

// NewPosition is the classical initial position.
func NewPosition() *Position {
	p, err := ParseFEN(StartFEN)
	if err != nil {
		panic(err)
	}
	return p
}

// Clone copies the position.
func (p *Position) Clone() *Position {
	c := *p
	return &c
}

// At returns the piece on a square.
func (p *Position) At(s Sq) Piece { return p.B[s] }

// KingSq is the square of the side's king, or NoSq when it has none (it may
// have been captured or exploded, or the side is the horde).
func (p *Position) KingSq(side Side) Sq {
	for i, pc := range p.B {
		if pc.Kind == King && pc.Side == side {
			return Sq(i)
		}
	}
	return NoSq
}

var (
	knightSteps = [8][2]int{{1, 2}, {2, 1}, {2, -1}, {1, -2}, {-1, -2}, {-2, -1}, {-2, 1}, {-1, 2}}
	kingSteps   = [8][2]int{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}
	rookDirs    = [4][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}}
	bishopDirs  = [4][2]int{{1, 1}, {-1, 1}, {-1, -1}, {1, -1}}
)

// step returns the square dFile, dRank away from s, or NoSq off the board.
func step(s Sq, df, dr int) Sq {
	f, r := s.File()+df, s.Rank()+dr
	if f < 0 || f > 7 || r < 0 || r > 7 {
		return NoSq
	}
	return MakeSq(f, r)
}

// Attacked says whether a piece of side `by` attacks the square, whatever
// stands on it (pieces of either side, and the duck, block sliders).
func (p *Position) Attacked(sq Sq, by Side) bool {
	// pawns: a white pawn attacks one rank up, so it stands one rank below sq
	dr := -1
	if by == Black {
		dr = 1
	}
	for _, df := range [2]int{-1, 1} {
		if s := step(sq, df, dr); s != NoSq && p.B[s] == (Piece{Kind: Pawn, Side: by}) {
			return true
		}
	}
	for _, d := range knightSteps {
		if s := step(sq, d[0], d[1]); s != NoSq && p.B[s].Kind == Knight && p.B[s].Side == by {
			return true
		}
	}
	for _, d := range kingSteps {
		if s := step(sq, d[0], d[1]); s != NoSq && p.B[s].Kind == King && p.B[s].Side == by {
			return true
		}
	}
	for _, d := range rookDirs {
		for s := step(sq, d[0], d[1]); s != NoSq; s = step(s, d[0], d[1]) {
			if pc := p.B[s]; !pc.Empty() {
				if pc.Side == by && (pc.Kind == Rook || pc.Kind == Queen) {
					return true
				}
				break
			}
		}
	}
	for _, d := range bishopDirs {
		for s := step(sq, d[0], d[1]); s != NoSq; s = step(s, d[0], d[1]) {
			if pc := p.B[s]; !pc.Empty() {
				if pc.Side == by && (pc.Kind == Bishop || pc.Kind == Queen) {
					return true
				}
				break
			}
		}
	}
	return false
}

// InCheck is the classical check: the side's king is attacked. A side without
// a king is never in check.
func (p *Position) InCheck(side Side) bool {
	k := p.KingSq(side)
	return k != NoSq && p.Attacked(k, side.Opp())
}

// FixCastling drops the castling rights whose king or rook is no longer where
// the right needs it (after a capture, an explosion, a drop).
func (p *Position) FixCastling() {
	for _, s := range [2]Side{White, Black} {
		k := p.KingSq(s)
		for f := 0; f < 2; f++ {
			r := p.Castle[s][f]
			if r == NoSq {
				continue
			}
			if k == NoSq || k.Rank() != r.Rank() || p.B[r].Kind != Rook || p.B[r].Side != s {
				p.Castle[s][f] = NoSq
			}
		}
	}
}

// ----------------------------------------------------------------- FEN

// ParseFEN reads a position. Castling may be written as KQkq (the outermost
// rook on that side of the king), or with the files of the rooks (AHah,
// Shredder-FEN), as Chess960 needs. Pockets are not part of the text.
func ParseFEN(fen string) (*Position, error) {
	f := strings.Fields(fen)
	if len(f) < 2 {
		return nil, fmt.Errorf("variants: FEN %q: too few fields", fen)
	}
	p := EmptyPosition()
	rows := strings.Split(f[0], "/")
	if len(rows) != 8 {
		return nil, fmt.Errorf("variants: FEN %q: want 8 ranks", fen)
	}
	for i, row := range rows {
		rank, file := 7-i, 0
		for _, c := range row {
			switch {
			case c >= '1' && c <= '8':
				file += int(c - '0')
			default:
				k := KindOf(byte(c))
				if k == NoKind || k == Duck || file > 7 {
					return nil, fmt.Errorf("variants: FEN %q: bad piece %q", fen, c)
				}
				side := White
				if c >= 'a' {
					side = Black
				}
				p.B[MakeSq(file, rank)] = Piece{Kind: k, Side: side}
				file++
			}
		}
		if file != 8 {
			return nil, fmt.Errorf("variants: FEN %q: rank %d has %d files", fen, rank+1, file)
		}
	}
	switch f[1] {
	case "w":
		p.Turn = White
	case "b":
		p.Turn = Black
	default:
		return nil, fmt.Errorf("variants: FEN %q: bad side to move", fen)
	}
	if len(f) > 2 && f[2] != "-" {
		for _, c := range f[2] {
			side := White
			home := 0
			if c >= 'a' && c <= 'z' {
				side, home = Black, 7
			}
			k := p.KingSq(side)
			if k == NoSq || k.Rank() != home {
				continue
			}
			var rook Sq = NoSq
			up := strings.ToUpper(string(c))[0]
			switch up {
			case 'K': // the outermost rook right of the king
				for fl := 7; fl > k.File(); fl-- {
					if q := MakeSq(fl, home); p.B[q] == (Piece{Kind: Rook, Side: side}) {
						rook = q
						break
					}
				}
			case 'Q':
				for fl := 0; fl < k.File(); fl++ {
					if q := MakeSq(fl, home); p.B[q] == (Piece{Kind: Rook, Side: side}) {
						rook = q
						break
					}
				}
			default:
				if up >= 'A' && up <= 'H' {
					if q := MakeSq(int(up-'A'), home); p.B[q] == (Piece{Kind: Rook, Side: side}) {
						rook = q
					}
				}
			}
			if rook == NoSq {
				continue
			}
			if rook.File() > k.File() {
				p.Castle[side][0] = rook
			} else {
				p.Castle[side][1] = rook
			}
		}
	}
	if len(f) > 3 && f[3] != "-" {
		if s, ok := ParseSq(f[3]); ok {
			p.EP = s
		}
	}
	if len(f) > 4 {
		p.Half, _ = strconv.Atoi(f[4])
	}
	p.Full = 1
	if len(f) > 5 {
		if n, err := strconv.Atoi(f[5]); err == nil && n > 0 {
			p.Full = n
		}
	}
	return p, nil
}

// Placement is the first FEN field: the pieces. The duck is written as 'D'.
func (p *Position) Placement() string {
	var b strings.Builder
	for rank := 7; rank >= 0; rank-- {
		empty := 0
		for file := 0; file < 8; file++ {
			pc := p.B[MakeSq(file, rank)]
			if pc.Empty() {
				empty++
				continue
			}
			if empty > 0 {
				b.WriteByte(byte('0' + empty))
				empty = 0
			}
			b.WriteByte(pc.fenChar())
		}
		if empty > 0 {
			b.WriteByte(byte('0' + empty))
		}
		if rank > 0 {
			b.WriteByte('/')
		}
	}
	return b.String()
}

// castlingField writes the rights: KQkq when the right belongs to the
// outermost rook, otherwise the file of the rook.
func (p *Position) castlingField() string {
	var b strings.Builder
	for _, side := range [2]Side{White, Black} {
		k := p.KingSq(side)
		for f := 0; f < 2; f++ {
			r := p.Castle[side][f]
			if r == NoSq || k == NoSq {
				continue
			}
			outer := true
			for fl := 0; fl < 8; fl++ {
				q := MakeSq(fl, r.Rank())
				if p.B[q] != (Piece{Kind: Rook, Side: side}) {
					continue
				}
				if f == 0 && fl > r.File() && fl > k.File() || f == 1 && fl < r.File() && fl < k.File() {
					outer = false
				}
			}
			var c byte
			if outer {
				c = "KQ"[f]
			} else {
				c = byte('A' + r.File())
			}
			if side == Black {
				c += 'a' - 'A'
			}
			b.WriteByte(c)
		}
	}
	if b.Len() == 0 {
		return "-"
	}
	return b.String()
}

// epCapturable: a pawn of the side to move stands beside the en passant square.
func (p *Position) epCapturable() bool {
	if p.EP == NoSq {
		return false
	}
	dr := -1
	if p.Turn == Black {
		dr = 1
	}
	for _, df := range [2]int{-1, 1} {
		if s := step(p.EP, df, dr); s != NoSq && p.B[s] == (Piece{Kind: Pawn, Side: p.Turn}) {
			return true
		}
	}
	return false
}

// FEN writes the position (the en passant square only when a pawn could take).
func (p *Position) FEN() string {
	side := "w"
	if p.Turn == Black {
		side = "b"
	}
	ep := "-"
	if p.epCapturable() {
		ep = p.EP.String()
	}
	return fmt.Sprintf("%s %s %s %s %d %d", p.Placement(), side, p.castlingField(), ep, p.Half, p.Full)
}

// Board is the position as the browser takes it: square to piece code.
func (p *Position) Board() map[string]string {
	m := make(map[string]string, 32)
	for i, pc := range p.B {
		if !pc.Empty() {
			m[Sq(i).String()] = pc.Code()
		}
	}
	return m
}

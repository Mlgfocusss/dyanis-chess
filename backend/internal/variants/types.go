// Package variants holds the rules of chess and of its variants for the site's
// "Другие шахматы" section. It does not use the engine (which knows classical
// chess only): the board, the move generator and the rules live here, so that a
// variant can change them. The engine facade appears only in this package's
// tests, as the reference the base rules are compared with.
//
// Layers, from bottom to top:
//
//   - Position (position.go, movegen.go): the board, whose move, castling
//     rights, en passant, pockets of pieces in hand. Plain data; moves are
//     generated and played under a Rules value.
//   - Rules (rules.go): the hooks a variant overrides. ClassicRules is the
//     complete classical behaviour; a variant embeds it and replaces a few
//     methods.
//   - Game (game.go): a Position plus history under Rules. It implements State,
//     so a variant whose turn is one move needs nothing else.
//   - State (variant.go): what the server (internal/vplay) plays with. A
//     variant with a stranger turn (the duck, three dice moves) writes its own
//     State on top of a Game.
//   - Registry (registry.go): every variant registers itself in init().
//
// The contract is written out in VARIANTS-PLAN.md, section 5.
package variants

import (
	"fmt"
	"strings"
)

// Side is a colour. NoSide is the spectator, or the neutral owner of the duck.
type Side uint8

// The sides.
const (
	NoSide Side = iota
	White
	Black
)

// Opp is the other colour (NoSide stays NoSide).
func (s Side) Opp() Side {
	switch s {
	case White:
		return Black
	case Black:
		return White
	}
	return NoSide
}

func (s Side) String() string {
	switch s {
	case White:
		return "white"
	case Black:
		return "black"
	}
	return ""
}

// MarshalText makes a Side travel as "white", "black" or "" in JSON.
func (s Side) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// UnmarshalText reads what MarshalText wrote.
func (s *Side) UnmarshalText(b []byte) error {
	switch string(b) {
	case "white":
		*s = White
	case "black":
		*s = Black
	case "":
		*s = NoSide
	default:
		return fmt.Errorf("variants: unknown side %q", b)
	}
	return nil
}

// Kind is a kind of piece.
type Kind uint8

// The kinds. Duck is the neutral blocker of the duck variant.
const (
	NoKind Kind = iota
	Pawn
	Knight
	Bishop
	Rook
	Queen
	King
	Duck
)

var kindLetters = [...]byte{' ', 'P', 'N', 'B', 'R', 'Q', 'K', 'D'}

// Letter is the upper-case letter of the kind: P N B R Q K D.
func (k Kind) Letter() byte {
	if int(k) < len(kindLetters) {
		return kindLetters[k]
	}
	return '?'
}

// KindOf reads a letter (any case) into a Kind; NoKind for anything else.
func KindOf(letter byte) Kind {
	for k := Pawn; k <= Duck; k++ {
		if strings.EqualFold(string(kindLetters[k]), string(letter)) {
			return k
		}
	}
	return NoKind
}

// Piece is what stands on a square. The zero value is an empty square.
type Piece struct {
	Kind Kind
	Side Side
	// Promoted marks a piece that came from a pawn by promotion (crazyhouse
	// turns it back into a pawn when taken). The base sets it on promotion.
	Promoted bool
}

// Empty says whether there is no piece.
func (p Piece) Empty() bool { return p.Kind == NoKind }

// Code is the piece as the browser knows it: side letter and kind letter,
// "wP", "bN", and "xD" for the duck.
func (p Piece) Code() string {
	if p.Empty() {
		return ""
	}
	c := byte('x')
	switch p.Side {
	case White:
		c = 'w'
	case Black:
		c = 'b'
	}
	return string([]byte{c, p.Kind.Letter()})
}

// ParseCode reads what Code writes.
func ParseCode(s string) (Piece, bool) {
	if len(s) != 2 {
		return Piece{}, false
	}
	k := KindOf(s[1])
	if k == NoKind {
		return Piece{}, false
	}
	switch s[0] {
	case 'w':
		return Piece{Kind: k, Side: White}, true
	case 'b':
		return Piece{Kind: k, Side: Black}, true
	case 'x':
		return Piece{Kind: k}, true
	}
	return Piece{}, false
}

// fenChar is the FEN letter: upper case for White, lower for Black.
func (p Piece) fenChar() byte {
	c := p.Kind.Letter()
	if p.Side == Black {
		c += 'a' - 'A'
	}
	return c
}

// Sq is a square, 0 (a1) to 63 (h8); file = sq%8, rank = sq/8.
type Sq int8

// NoSq is "no square".
const NoSq Sq = -1

// MakeSq builds a square from a file and rank, both 0 to 7.
func MakeSq(file, rank int) Sq { return Sq(rank*8 + file) }

// File is 0 for a, 7 for h.
func (s Sq) File() int { return int(s) % 8 }

// Rank is 0 for the first rank, 7 for the eighth.
func (s Sq) Rank() int { return int(s) / 8 }

// OK says that s is on the board.
func (s Sq) OK() bool { return s >= 0 && s < 64 }

func (s Sq) String() string {
	if !s.OK() {
		return "-"
	}
	return string([]byte{byte('a' + s.File()), byte('1' + s.Rank())})
}

// ParseSq reads "e4".
func ParseSq(s string) (Sq, bool) {
	if len(s) != 2 || s[0] < 'a' || s[0] > 'h' || s[1] < '1' || s[1] > '8' {
		return NoSq, false
	}
	return MakeSq(int(s[0]-'a'), int(s[1]-'1')), true
}

// Move is one move or drop of a position.
type Move struct {
	From, To Sq
	Promo    Kind // the piece a pawn becomes
	Drop     Kind // set for a drop from the pocket: From is NoSq
	Castle   bool // To is the king's square after castling; Rook is the rook's square before
	Rook     Sq
	// EnPassant: a pawn taking en passant (the victim stands beside From, not on To).
	EnPassant bool
	// SetsEP: a pawn double step after which the pawn may be taken en passant.
	SetsEP bool
}

// UCI is the standard text of the move: "e2e4", "e7e8q", "e1g1" for a castling
// (the king's two squares), "P@e4" for a drop.
func (m Move) UCI() string {
	if m.Drop != NoKind {
		return string([]byte{m.Drop.Letter(), '@'}) + m.To.String()
	}
	s := m.From.String() + m.To.String()
	if m.Promo != NoKind {
		s += strings.ToLower(string(m.Promo.Letter()))
	}
	return s
}

// MoveRec is one action of a game as the list of moves records it.
type MoveRec struct {
	Side   Side   `json:"side"`
	Action string `json:"action"`
	SAN    string `json:"san"` // the notation State.Apply returned
}

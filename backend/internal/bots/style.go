package bots

import (
	"strconv"
	"strings"

	"dyanis-chess/engine"
)

// A bot that judges moves by a shallow search sees almost no difference
// between the moves of the opening: every one of them evaluates within a few
// centipawns, so a soft choice among them is nearly a coin toss and a3 or h4 is
// played as often as e4. People of any club strength do not play like that:
// they take the centre, bring out the knights and bishops, castle, and leave
// the edge pawns and the queen alone. openingPrior is that habit as a bonus (in
// centipawns, added to the bot's score of a move) for the first moves of a game.
// The bonuses are small beside a lost piece, so they only break ties; they
// matter most for the bots with a low Temp, whose choice they decide, and not
// at all for the weakest, whose Temp swamps them.

// openingPlies is how many half-moves the habit lasts.
const openingPlies = 20

// openingPrior returns the bonus for playing m in pos, positive for a move
// a person would be glad to find. Captures and positions past the opening get 0:
// there the search decides.
func openingPrior(pos *engine.Position, m engine.Move) float64 {
	sq, black, ply := parseFEN(pos.FEN())
	if ply >= openingPlies {
		return 0
	}
	u := m.String()
	from := int(u[0]-'a') + 8*int(u[1]-'1')
	to := int(u[2]-'a') + 8*int(u[3]-'1')
	piece := sq[from]
	if piece == 0 || sq[to] != 0 {
		return 0 // a capture
	}
	f, r := to%8, to/8 // file, and rank counted from the mover's side
	fromRank := from / 8
	if black {
		r, fromRank = 7-r, 7-fromRank
	}
	fromFile := from % 8

	switch piece {
	case 'P':
		if fromFile != f {
			return 0 // en passant
		}
		switch {
		case f == 3 || f == 4: // d, e
			switch r {
			case 3:
				return 50
			case 2:
				return 15
			default:
				return 10
			}
		case f == 2: // c
			if r == 3 {
				return 25
			}
			return 5
		case f == 5: // f: weakens the king
			return -50
		case f == 1 || f == 6: // b, g
			if r == 2 {
				return -5 // a fianchetto
			}
			return -60
		default: // a, h
			return -110
		}
	case 'N':
		bonus := 0.0
		if fromRank == 0 {
			bonus += 10 // a piece comes out
		} else {
			bonus -= 15 // the same piece again
		}
		if f == 0 || f == 7 {
			bonus -= 60 // a knight on the rim is dim
		} else if r == 2 && (f == 2 || f == 5) {
			bonus += 25 // c3 / f3
		}
		return bonus
	case 'B':
		if fromRank == 0 {
			return 25
		}
		if ply < 14 {
			return -15
		}
		return 0
	case 'Q':
		if ply < 12 {
			return -50
		}
		return 0
	case 'R':
		if ply < 16 {
			return -25
		}
		return 0
	case 'K':
		if abs(f-fromFile) == 2 {
			return 50 // castling
		}
		return -50
	}
	return 0
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// parseFEN reads a FEN into: the piece letters on the squares (square 0 is
// a1; black pieces are folded to upper case too, so that the rules above are
// written once for both sides), whether Black is to move, and how many
// half-moves have been played.
func parseFEN(fen string) (sq [64]byte, black bool, ply int) {
	f := strings.Fields(fen)
	if len(f) < 2 {
		return sq, false, 0
	}
	rank := 7
	file := 0
	for _, c := range f[0] {
		switch {
		case c == '/':
			rank--
			file = 0
		case c >= '1' && c <= '8':
			file += int(c - '0')
		default:
			if file < 8 && rank >= 0 {
				sq[file+8*rank] = byte(strings.ToUpper(string(c))[0])
			}
			file++
		}
	}
	black = f[1] == "b"
	move := 1
	if len(f) >= 6 {
		if n, err := strconv.Atoi(f[5]); err == nil && n > 0 {
			move = n
		}
	}
	ply = (move - 1) * 2
	if black {
		ply++
	}
	return sq, black, ply
}

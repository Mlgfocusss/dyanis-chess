package variants

import "strings"

// SAN writes a move in standard algebraic notation: "Nf3", "exd5", "e8=Q",
// "O-O", and for a drop "P@e4". mv is one of `legal`, the legal moves of p
// (they decide how much to disambiguate); `next` is the position after it, from
// which "+" and "#" come (Rules.Check, and whether the other side has a move).
func SAN(r Rules, p *Position, legal []Move, mv Move, next *Position) string {
	var s string
	switch {
	case mv.Drop != NoKind:
		s = string([]byte{mv.Drop.Letter(), '@'}) + mv.To.String()
	case mv.Castle:
		if mv.To.File() == 2 {
			s = "O-O-O"
		} else {
			s = "O-O"
		}
	default:
		pc := p.B[mv.From]
		capture := !p.B[mv.To].Empty() || mv.EnPassant
		if pc.Kind == Pawn {
			if capture {
				s = string(rune('a'+mv.From.File())) + "x"
			}
			s += mv.To.String()
			if mv.Promo != NoKind {
				s += "=" + string(rune(mv.Promo.Letter()))
			}
		} else {
			s = string(rune(pc.Kind.Letter()))
			fileTwin, rankTwin, twin := false, false, false
			for _, o := range legal {
				if o == mv || o.Drop != NoKind || o.Castle || o.To != mv.To || p.B[o.From].Kind != pc.Kind || o.From == mv.From {
					continue
				}
				twin = true
				if o.From.File() == mv.From.File() {
					fileTwin = true
				}
				if o.From.Rank() == mv.From.Rank() {
					rankTwin = true
				}
			}
			switch {
			case !twin:
			case !fileTwin:
				s += string(rune('a' + mv.From.File()))
			case !rankTwin:
				s += string(rune('1' + mv.From.Rank()))
			default:
				s += mv.From.String()
			}
			if capture {
				s += "x"
			}
			s += mv.To.String()
		}
	}
	if r.Check(next, next.Turn) {
		if len(next.LegalMoves(r)) == 0 {
			s += "#"
		} else {
			s += "+"
		}
	}
	return s
}

// Encode is the text of a move as the actions of a game carry it: UCI, with
// the castling written as the king taking its rook when the rules ask for it.
func Encode(r Rules, mv Move) string {
	if mv.Castle && r.CastleAsKingTakesRook() {
		return mv.From.String() + mv.Rook.String()
	}
	return mv.UCI()
}

// Find returns the move of the list that the action names.
func Find(r Rules, moves []Move, action string) (Move, bool) {
	action = strings.TrimSpace(action)
	for _, m := range moves {
		if Encode(r, m) == action {
			return m, true
		}
	}
	return Move{}, false
}

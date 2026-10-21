package variants

// promotion pieces, in the order they are generated
var promoKinds = [4]Kind{Queen, Rook, Bishop, Knight}

// PseudoMoves are the moves of the side to move that obey how the pieces
// move, with no regard for check: the base of LegalMoves. The Rules decide what
// may be captured, how pawns start, which extra moves (drops) exist.
func (p *Position) PseudoMoves(r Rules) []Move {
	me := p.Turn
	out := make([]Move, 0, 48)
	for i := 0; i < 64; i++ {
		pc := p.B[i]
		if pc.Kind == NoKind || pc.Side != me {
			continue
		}
		from := Sq(i)
		switch pc.Kind {
		case Pawn:
			out = p.pawnMoves(r, from, out)
		case Knight:
			for _, d := range knightSteps {
				out = p.addTarget(r, from, step(from, d[0], d[1]), out)
			}
		case King:
			for _, d := range kingSteps {
				out = p.addTarget(r, from, step(from, d[0], d[1]), out)
			}
		case Bishop, Rook, Queen:
			if pc.Kind != Bishop {
				out = p.slide(r, from, rookDirs[:], out)
			}
			if pc.Kind != Rook {
				out = p.slide(r, from, bishopDirs[:], out)
			}
		}
	}
	out = p.castlingMoves(r, out)
	return append(out, r.ExtraMoves(p)...)
}

// addTarget adds a move to `to` when it is empty or holds a piece that may be taken.
func (p *Position) addTarget(r Rules, from, to Sq, out []Move) []Move {
	if to == NoSq {
		return out
	}
	t := p.B[to]
	mv := Move{From: from, To: to, Rook: NoSq}
	switch {
	case t.Empty():
		return append(out, mv)
	case t.Side == p.Turn || t.Kind == Duck: // blocked
		return out
	case r.CanCapture(p, mv, t):
		return append(out, mv)
	}
	return out
}

func (p *Position) slide(r Rules, from Sq, dirs [][2]int, out []Move) []Move {
	for _, d := range dirs {
		for to := step(from, d[0], d[1]); to != NoSq; to = step(to, d[0], d[1]) {
			out = p.addTarget(r, from, to, out)
			if !p.B[to].Empty() {
				break
			}
		}
	}
	return out
}

func (p *Position) pawnMoves(r Rules, from Sq, out []Move) []Move {
	me := p.Turn
	dr, last := 1, 7
	if me == Black {
		dr, last = -1, 0
	}
	add := func(mv Move) {
		mv.Rook = NoSq
		if mv.To.Rank() == last {
			for _, k := range promoKinds {
				m := mv
				m.Promo = k
				out = append(out, m)
			}
			return
		}
		out = append(out, mv)
	}
	if one := step(from, 0, dr); one != NoSq && p.B[one].Empty() {
		add(Move{From: from, To: one})
		if ok, ep := r.PawnDoubleStep(p, from); ok {
			if two := step(from, 0, 2*dr); two != NoSq && p.B[two].Empty() {
				out = append(out, Move{From: from, To: two, SetsEP: ep, Rook: NoSq})
			}
		}
	}
	for _, df := range [2]int{-1, 1} {
		to := step(from, df, dr)
		if to == NoSq {
			continue
		}
		t := p.B[to]
		switch {
		case t.Empty():
			if to == p.EP {
				victim := p.B[MakeSq(to.File(), from.Rank())]
				if victim.Kind == Pawn && victim.Side == me.Opp() && r.CanCapture(p, Move{From: from, To: to, EnPassant: true}, victim) {
					add(Move{From: from, To: to, EnPassant: true})
				}
			}
		case t.Side != me && t.Kind != Duck && r.CanCapture(p, Move{From: from, To: to}, t):
			add(Move{From: from, To: to})
		}
	}
	return out
}

// castlingMoves adds the castlings of the side to move, by the rules of
// Chess960 (which contain the classical ones): the king ends on the g or c
// file and the rook on the f or d file of their rank; every square between
// them and their destinations is empty but for the two; the king's start and
// the squares it passes are safe (Rules.CastleSafe).
func (p *Position) castlingMoves(r Rules, out []Move) []Move {
	me := p.Turn
	king := p.KingSq(me)
	if king == NoSq {
		return out
	}
	for flank := 0; flank < 2; flank++ {
		rook := p.Castle[me][flank]
		if rook == NoSq || rook.Rank() != king.Rank() || p.B[rook].Kind != Rook || p.B[rook].Side != me {
			continue
		}
		if flank == 0 && rook.File() < king.File() || flank == 1 && rook.File() > king.File() {
			continue
		}
		kf, rf := 6, 5
		if flank == 1 {
			kf, rf = 2, 3
		}
		rank := king.Rank()
		kingTo, rookTo := MakeSq(kf, rank), MakeSq(rf, rank)
		lo, hi := min(int(king), int(rook), int(kingTo), int(rookTo)), max(int(king), int(rook), int(kingTo), int(rookTo))
		free := true
		for s := lo; s <= hi; s++ {
			if Sq(s) != king && Sq(s) != rook && !p.B[s].Empty() {
				free = false
				break
			}
		}
		if !free {
			continue
		}
		safe := true
		a, b := king.File(), kf
		if a > b {
			a, b = b, a
		}
		for f := a; f <= b; f++ {
			if f == kf && kf != king.File() {
				continue // the destination is judged on the board after the move
			}
			if !r.CastleSafe(p, me, MakeSq(f, rank)) {
				safe = false
				break
			}
		}
		if safe {
			out = append(out, Move{From: king, To: kingTo, Castle: true, Rook: rook})
		}
	}
	return out
}

// Apply plays a pseudo-legal move of the side to move and returns the new
// position; p is not changed. The base moves the pieces, takes (en passant
// too), promotes, castles, drops, keeps the castling rights, the en passant
// square and the clocks, and passes the turn; Rules.AfterMove may then add to it.
func (p *Position) Apply(r Rules, mv Move) *Position {
	n := *p
	me := p.Turn
	n.EP = NoSq
	info := MoveInfo{Move: mv, Side: me, CapturedAt: NoSq}
	switch {
	case mv.Drop != NoKind:
		info.Piece = Piece{Kind: mv.Drop, Side: me}
		n.B[mv.To] = info.Piece
		if n.Pocket[me][mv.Drop] > 0 {
			n.Pocket[me][mv.Drop]--
		}
		n.Half = 0
	case mv.Castle:
		info.Piece = n.B[mv.From]
		rank := mv.From.Rank()
		rookTo := MakeSq(5, rank)
		if mv.To.File() == 2 {
			rookTo = MakeSq(3, rank)
		}
		king, rook := n.B[mv.From], n.B[mv.Rook]
		n.B[mv.From], n.B[mv.Rook] = Piece{}, Piece{}
		n.B[mv.To], n.B[rookTo] = king, rook
		n.Castle[me] = [2]Sq{NoSq, NoSq}
		n.Half++
	default:
		piece := n.B[mv.From]
		info.Piece = piece
		capAt := mv.To
		if mv.EnPassant {
			capAt = MakeSq(mv.To.File(), mv.From.Rank())
		}
		if v := n.B[capAt]; !v.Empty() {
			info.Captured, info.CapturedAt = v, capAt
		}
		n.B[capAt] = Piece{}
		n.B[mv.From] = Piece{}
		if mv.Promo != NoKind {
			piece = Piece{Kind: mv.Promo, Side: me, Promoted: true}
		}
		n.B[mv.To] = piece
		if mv.SetsEP {
			n.EP = MakeSq(mv.From.File(), (mv.From.Rank()+mv.To.Rank())/2)
		}
		if info.Piece.Kind == King {
			n.Castle[me] = [2]Sq{NoSq, NoSq}
		}
		for f := 0; f < 2; f++ {
			if n.Castle[me][f] == mv.From {
				n.Castle[me][f] = NoSq
			}
			if !info.Captured.Empty() && n.Castle[me.Opp()][f] == info.CapturedAt {
				n.Castle[me.Opp()][f] = NoSq
			}
		}
		if info.Piece.Kind == Pawn || !info.Captured.Empty() {
			n.Half = 0
		} else {
			n.Half++
		}
	}
	n.Turn = me.Opp()
	if me == Black {
		n.Full++
	}
	r.AfterMove(&n, info)
	return &n
}

// LegalMoves are the moves the rules allow: pseudo-legal ones after which the
// mover is not in check (Rules.Check) and that Rules.Legal does not veto.
func (p *Position) LegalMoves(r Rules) []Move {
	pseudo := p.PseudoMoves(r)
	out := pseudo[:0:0]
	for _, mv := range pseudo {
		next := p.Apply(r, mv)
		if r.Check(next, p.Turn) || !r.Legal(p, mv, next) {
			continue
		}
		out = append(out, mv)
	}
	return out
}

// Perft counts the leaves of the tree of legal moves to the depth; the usual
// way to prove a move generator right.
func Perft(r Rules, p *Position, depth int) uint64 {
	if depth == 0 {
		return 1
	}
	moves := p.LegalMoves(r)
	if depth == 1 {
		return uint64(len(moves))
	}
	var n uint64
	for _, mv := range moves {
		n += Perft(r, p.Apply(r, mv), depth-1)
	}
	return n
}

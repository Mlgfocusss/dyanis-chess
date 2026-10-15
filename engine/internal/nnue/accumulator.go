package nnue

import (
	"math/bits"

	"dyanis-chess/engine/internal/board"
)

// Accumulator is the hidden layer before activation, for both
// perspectives: v[board.White] and v[board.Black]. It lives outside
// board.Board (Board.Copy copies the struct, and slices inside it would be
// shared between copies); a search keeps one per ply and builds each
// child from its parent with Update, so unmaking a move is just going back
// a ply.
type Accumulator struct {
	v [2][]int16
	// ks is each perspective's own king square as that perspective sees it
	// (flipped for Black, before any mirroring): it picks the king bucket
	// and the mirroring the features of that perspective use.
	ks [2]uint8
}

// NewAccumulator returns an accumulator sized for n. Its contents are
// meaningless until Refresh or Update fills it.
func (n *Network) NewAccumulator() *Accumulator {
	return &Accumulator{v: [2][]int16{make([]int16, n.Hidden), make([]int16, n.Hidden)}}
}

// CopyFrom makes a equal to src; both must belong to the same network.
// A null move's child uses it: no piece moved.
func (a *Accumulator) CopyFrom(src *Accumulator) {
	copy(a.v[0], src.v[0])
	copy(a.v[1], src.v[1])
	a.ks = src.ks
}

// Equal reports whether a and b hold the same values.
func (a *Accumulator) Equal(b *Accumulator) bool {
	if a.ks != b.ks {
		return false
	}
	for p := range a.v {
		if len(a.v[p]) != len(b.v[p]) {
			return false
		}
		for i, x := range a.v[p] {
			if b.v[p][i] != x {
				return false
			}
		}
	}
	return true
}

// relSquare is sq as persp sees the board: Black sees it flipped
// vertically, so both perspectives look at "their" pieces the same way.
func relSquare(persp board.Color, sq board.Square) uint8 {
	if persp == board.Black {
		return uint8(sq) ^ 56
	}
	return uint8(sq)
}

// kingState is what a perspective's features depend on besides the piece
// itself: the king bucket (n.KingMap of the possibly mirrored king square)
// and whether the board is mirrored (the king stands on files e–h).
func (n *Network) kingState(ks uint8) (bucket int, mirror bool) {
	if n.Mirror && ks&7 >= 4 {
		return int(n.KingMap[ks^7]), true
	}
	return int(n.KingMap[ks]), false
}

// featureIndex is the input a piece on a square activates, seen from
// persp whose own king stands on ks (relSquare form): pieces of persp's own
// color come first.
//
//	index = bucket · 768 + (relative color · 6 + (type − 1)) · 64 + square
//	relative color: 0 = persp's own piece, 1 = the opponent's
//	square: a1 = 0 … h8 = 63, XOR 56 (rank flip) when persp is Black,
//	        XOR 7 (file flip) when the network mirrors and the king is on e–h
func (n *Network) featureIndex(persp board.Color, p board.Piece, sq board.Square, ks uint8) int {
	rel := 0
	if p.Color() != persp {
		rel = 1
	}
	s := int(relSquare(persp, sq))
	bucket, mirror := n.kingState(ks)
	if mirror {
		s ^= 7
	}
	return bucket*Inputs + (rel*6+int(p.Type())-1)*64 + s
}

func (n *Network) row(f int) []int16 {
	return n.W1[f*n.Hidden : (f+1)*n.Hidden]
}

// refreshPersp computes one perspective of acc from scratch.
func (n *Network) refreshPersp(acc *Accumulator, persp board.Color, pieces *[12]board.Bitboard) {
	// PieceBitboards layout: (type-1)*2, +1 for White.
	kingIdx := (int(board.King)-1)*2 + 0
	if persp == board.White {
		kingIdx++
	}
	ks := relSquare(persp, board.Square(bits.TrailingZeros64(uint64(pieces[kingIdx]))))
	acc.ks[persp] = ks
	v := acc.v[persp]
	copy(v, n.B1)
	for idx, bb := range pieces {
		color := board.Black
		if idx%2 == 1 {
			color = board.White
		}
		p := board.MakePiece(board.PieceType(idx/2+1), color)
		for rest := uint64(bb); rest != 0; rest &= rest - 1 {
			addRow(v, n.row(n.featureIndex(persp, p, board.Square(bits.TrailingZeros64(rest)), ks)))
		}
	}
}

// Refresh computes acc from scratch for position b.
func (n *Network) Refresh(acc *Accumulator, b *board.Board) {
	pieces := b.PieceBitboards()
	for persp := board.White; persp <= board.Black; persp++ {
		n.refreshPersp(acc, persp, &pieces)
	}
}

// KingRefresh reports whether the move just made (u from b.MakeMove(m))
// moved a king into a different king bucket or across the mirror line, so
// that Update needs the piece bitboards of the position after the move.
func (n *Network) KingRefresh(m board.Move, u board.Undo) bool {
	if u.MovedPiece.Type() != board.King || (n.KingBuckets == 1 && !n.Mirror) {
		return false
	}
	us := u.MovedPiece.Color()
	b0, m0 := n.kingState(relSquare(us, m.From))
	b1, m1 := n.kingState(relSquare(us, m.To))
	return b0 != b1 || m0 != m1
}

// feature is a piece on a square, the unit Update adds and removes.
type feature struct {
	p  board.Piece
	sq board.Square
}

// Update sets dst to src after move m, where u is what b.MakeMove(m)
// returned. dst and src must be different accumulators. A null move
// changes no piece: the child simply uses its parent's accumulator.
// pieces is the piece bitboards of the position after the move; it is read
// only when KingRefresh(m, u) is true and may be nil otherwise.
func (n *Network) Update(dst, src *Accumulator, m board.Move, u board.Undo, pieces *[12]board.Bitboard) {
	var add, sub [2]feature
	nAdd, nSub := 0, 0
	us := u.MovedPiece.Color()

	sub[nSub], nSub = feature{u.MovedPiece, m.From}, nSub+1
	placed := u.MovedPiece
	if m.Flag == board.Promotion || m.Flag == board.PromotionCapture {
		placed = board.MakePiece(m.Promotion, us)
	}
	add[nAdd], nAdd = feature{placed, m.To}, nAdd+1

	switch {
	case u.CapturedSquare != board.NoSquare:
		sub[nSub], nSub = feature{u.CapturedPiece, u.CapturedSquare}, nSub+1
	case m.Flag == board.CastleKingside || m.Flag == board.CastleQueenside:
		rank := m.From.Rank()
		rook := board.MakePiece(board.Rook, us)
		fromFile, toFile := 7, 5
		if m.Flag == board.CastleQueenside {
			fromFile, toFile = 0, 3
		}
		sub[nSub], nSub = feature{rook, board.MakeSquare(fromFile, rank)}, nSub+1
		add[nAdd], nAdd = feature{rook, board.MakeSquare(toFile, rank)}, nAdd+1
	}

	kingMoved := u.MovedPiece.Type() == board.King
	for persp := board.White; persp <= board.Black; persp++ {
		ks := src.ks[persp]
		if kingMoved && persp == us {
			if n.KingRefresh(m, u) {
				n.refreshPersp(dst, persp, pieces)
				continue
			}
			ks = relSquare(us, m.To)
		}
		dst.ks[persp] = ks
		d, s := dst.v[persp], src.v[persp]
		a0 := n.row(n.featureIndex(persp, add[0].p, add[0].sq, ks))
		s0 := n.row(n.featureIndex(persp, sub[0].p, sub[0].sq, ks))
		addSubRows(d, s, a0, s0)
		if nAdd > 1 {
			addRow(d, n.row(n.featureIndex(persp, add[1].p, add[1].sq, ks)))
		}
		if nSub > 1 {
			subRow(d, n.row(n.featureIndex(persp, sub[1].p, sub[1].sq, ks)))
		}
	}
}

// The row and dot-product helpers below run the AVX2 versions from
// simd_amd64.go when the build and the CPU have them (haveSIMD), and these
// plain loops otherwise; both give identical results.

func addRow(v, w []int16) {
	if haveSIMD {
		addRowSIMD(v, w)
		return
	}
	w = w[:len(v)]
	for i := range v {
		v[i] += w[i]
	}
}

func subRow(v, w []int16) {
	if haveSIMD {
		subRowSIMD(v, w)
		return
	}
	w = w[:len(v)]
	for i := range v {
		v[i] -= w[i]
	}
}

// addSubRows sets d = s + a - b, the whole update of a quiet move.
func addSubRows(d, s, a, b []int16) {
	if haveSIMD {
		addSubRowsSIMD(d, s, a, b)
		return
	}
	s, a, b = s[:len(d)], a[:len(d)], b[:len(d)]
	for i := range d {
		d[i] = s[i] + a[i] - b[i]
	}
}

// Evaluate returns the network's score for the position acc describes, in
// centipawns from stm's point of view (the convention of eval.Evaluate).
// pieces is the number of pieces on the board, which picks the output
// bucket.
func (n *Network) Evaluate(acc *Accumulator, stm board.Color, pieces int) int {
	us, them := acc.v[stm], acc.v[stm^1]
	k := n.Bucket(pieces)
	w := n.W2[k*2*n.Hidden : (k+1)*2*n.Hidden]
	qa := n.QA
	crelu, screlu := creluDot, screluDot
	if haveSIMD {
		crelu, screlu = creluDotSIMD, screluDotSIMD
	}
	var sum int64
	switch n.Activation {
	case CReLU:
		sum = crelu(us, w[:n.Hidden], qa) + crelu(them, w[n.Hidden:], qa)
	case SCReLU:
		sum = (screlu(us, w[:n.Hidden], qa) + screlu(them, w[n.Hidden:], qa)) / int64(qa)
	}
	sum += int64(n.B2[k])
	return int(sum * int64(n.Scale) / (int64(qa) * int64(n.QB)))
}

// The dot products sum in int32 over blocks of dotBlock elements and in
// int64 across blocks. With QA ≤ 255 and |W2| ≤ MaxOutputWeight an SCReLU
// term is below 8.3·10⁶, so a block stays below 5.3·10⁸ — no overflow,
// whatever the hidden size. Four partial sums let the CPU overlap the
// multiplications.
const dotBlock = 64

func clampQA(x int16, qa int32) int32 {
	return min(max(int32(x), 0), qa)
}

func creluDot(v, w []int16, qa int32) int64 {
	w = w[:len(v)]
	var total int64
	i := 0
	for ; i+dotBlock <= len(v); i += dotBlock {
		vb, wb := (*[dotBlock]int16)(v[i:]), (*[dotBlock]int16)(w[i:])
		var s0, s1, s2, s3 int32
		for j := 0; j < dotBlock; j += 4 {
			s0 += clampQA(vb[j], qa) * int32(wb[j])
			s1 += clampQA(vb[j+1], qa) * int32(wb[j+1])
			s2 += clampQA(vb[j+2], qa) * int32(wb[j+2])
			s3 += clampQA(vb[j+3], qa) * int32(wb[j+3])
		}
		total += int64(s0 + s1 + s2 + s3)
	}
	for ; i < len(v); i++ {
		total += int64(clampQA(v[i], qa) * int32(w[i]))
	}
	return total
}

func screluDot(v, w []int16, qa int32) int64 {
	w = w[:len(v)]
	var total int64
	i := 0
	for ; i+dotBlock <= len(v); i += dotBlock {
		vb, wb := (*[dotBlock]int16)(v[i:]), (*[dotBlock]int16)(w[i:])
		var s0, s1, s2, s3 int32
		for j := 0; j < dotBlock; j += 4 {
			c0, c1, c2, c3 := clampQA(vb[j], qa), clampQA(vb[j+1], qa), clampQA(vb[j+2], qa), clampQA(vb[j+3], qa)
			s0 += c0 * c0 * int32(wb[j])
			s1 += c1 * c1 * int32(wb[j+1])
			s2 += c2 * c2 * int32(wb[j+2])
			s3 += c3 * c3 * int32(wb[j+3])
		}
		total += int64(s0 + s1 + s2 + s3)
	}
	for ; i < len(v); i++ {
		c := clampQA(v[i], qa)
		total += int64(c * c * int32(w[i]))
	}
	return total
}

package search

import "dyanis-chess/engine/internal/board"

// History tables beyond the plain from/to one: the main table is split by
// side to move, continuation tables score a quiet move by the move one and
// two plies earlier in the same line (piece+destination pairs), and the
// capture table scores captures by moving piece, destination and victim.
// All values are int16 under the same ±maxHistory gravity as before.

const (
	pieceKeys = 13 * 64 // (piece+6)*64 + to; index 6 (no piece) is never used
	histMain  = 2 * 4096
)

type histTables struct {
	main  [histMain]int16
	cont1 []int16 // [prevKey*pieceKeys + curKey], previous move of the opponent
	cont2 []int16 // same, our own move two plies back
	capt  [pieceKeys * 7]int16

	corrPawn, corrNPW, corrNPB [2 * corrSize]int16
}

func newHistTables() *histTables {
	return &histTables{
		cont1: make([]int16, pieceKeys*pieceKeys),
		cont2: make([]int16, pieceKeys*pieceKeys),
	}
}

func (h *histTables) clear() {
	h.main = [histMain]int16{}
	h.capt = [pieceKeys * 7]int16{}
	h.corrPawn, h.corrNPW, h.corrNPB = [2 * corrSize]int16{}, [2 * corrSize]int16{}, [2 * corrSize]int16{}
	clear(h.cont1)
	clear(h.cont2)
}

// moveKey identifies a made move for the continuation tables.
func moveKey(p board.Piece, to board.Square) int16 {
	return int16((int(p)+6)*64 + int(to))
}

func mainIndex(p board.Piece, m board.Move) int {
	side := 0
	if p < 0 {
		side = 1
	}
	return side*4096 + int(m.From)*64 + int(m.To)
}

func captIndex(p board.Piece, m board.Move, victim board.PieceType) int {
	return (int(p)+6)*64*7 + int(m.To)*7 + int(victim)
}

func gravity(h *int16, bonus int) {
	abs := bonus
	if abs < 0 {
		abs = -abs
	}
	v := int(*h)
	v += bonus - v*abs/maxHistory
	*h = int16(v)
}

// contBases returns the row offsets of the two continuation tables for a
// node at ply, or -1 where the earlier move is unknown (root, null move).
func (tt *TranspositionTable) contBases(ply int) (b1, b2 int) {
	b1, b2 = -1, -1
	if ply >= 1 && ply-1 < len(tt.moveKeys) {
		if k := tt.moveKeys[ply-1]; k >= 0 {
			b1 = int(k) * pieceKeys
		}
	}
	if ply >= 2 && ply-2 < len(tt.moveKeys) {
		if k := tt.moveKeys[ply-2]; k >= 0 {
			b2 = int(k) * pieceKeys
		}
	}
	return
}

// setMoveKey records the move made at ply (key -1 for a null move).
func (tt *TranspositionTable) setMoveKey(ply int, key int16) {
	if tt == nil {
		return
	}
	if ply < len(tt.moveKeys) {
		tt.moveKeys[ply] = key
	}
}

// quietHist is the combined ordering/pruning score of a quiet move by
// piece p (the mover), given the continuation bases of the node.
func (tt *TranspositionTable) quietHist(p board.Piece, m board.Move, b1, b2 int) int {
	h := tt.hist
	s := int(h.main[mainIndex(p, m)])
	cur := int(moveKey(p, m.To))
	if b1 >= 0 {
		s += int(h.cont1[b1+cur])
	}
	if b2 >= 0 {
		s += int(h.cont2[b2+cur])
	}
	return s
}

func (tt *TranspositionTable) updateQuietHist(p board.Piece, m board.Move, b1, b2, bonus int) {
	h := tt.hist
	gravity(&h.main[mainIndex(p, m)], bonus)
	cur := int(moveKey(p, m.To))
	if b1 >= 0 {
		gravity(&h.cont1[b1+cur], bonus)
	}
	if b2 >= 0 {
		gravity(&h.cont2[b2+cur], bonus)
	}
}

func (tt *TranspositionTable) captHistScore(p board.Piece, m board.Move, victim board.PieceType) int {
	return int(tt.hist.capt[captIndex(p, m, victim)])
}

func (tt *TranspositionTable) updateCaptHist(p board.Piece, m board.Move, victim board.PieceType, bonus int) {
	gravity(&tt.hist.capt[captIndex(p, m, victim)], bonus)
}

// capturedType is the victim of a capture move on b (before it is made).
func capturedType(b *board.Board, m board.Move) board.PieceType {
	if m.Flag == board.EnPassantCapture {
		return board.Pawn
	}
	return b.PieceAt(m.To).Type()
}

// Correction history: the static eval of a node is corrected by how far the
// search result of earlier nodes with the same pawn structure (and the same
// non-pawn pieces of each side) landed from their static eval. Entries are
// cp-like values bounded by corrLimit, updated with the same gravity rule as
// the move histories; they live as long as the other history tables.
const (
	corrBits  = 14
	corrSize  = 1 << corrBits
	corrLimit = 1024
)

type corrIdx struct{ stm, pawn, npw, npb int }

func mix64(x uint64) int {
	x *= 0x9E3779B97F4A7C15
	x ^= x >> 29
	x *= 0xBF58476D1CE4E5B9
	return int(x >> (64 - corrBits))
}

func nonPawnKey(b *board.Board, c board.Color) uint64 {
	return uint64(b.Pieces(board.Knight, c))*0x9E3779B97F4A7C15 ^
		uint64(b.Pieces(board.Bishop, c))*0xC2B2AE3D27D4EB4F ^
		uint64(b.Pieces(board.Rook, c))*0x165667B19E3779F9 ^
		uint64(b.Pieces(board.Queen, c))*0x27D4EB2F165667C5
}

func corrIndex(b *board.Board) corrIdx {
	stm := 0
	if b.SideToMove == board.Black {
		stm = 1
	}
	pk := uint64(b.Pieces(board.Pawn, board.White))*0x9E3779B97F4A7C15 ^
		uint64(b.Pieces(board.Pawn, board.Black))*0xC2B2AE3D27D4EB4F
	return corrIdx{
		stm:  stm,
		pawn: stm*corrSize + mix64(pk),
		npw:  stm*corrSize + mix64(nonPawnKey(b, board.White)),
		npb:  stm*corrSize + mix64(nonPawnKey(b, board.Black)),
	}
}

// corrValue is the correction, in centipawns, to add to the raw static eval.
func (tt *TranspositionTable) corrValue(ci corrIdx) int {
	h := tt.hist
	return (corrPawnW*int(h.corrPawn[ci.pawn]) + corrNonPawnW*(int(h.corrNPW[ci.npw])+int(h.corrNPB[ci.npb]))) / 131072
}

func (tt *TranspositionTable) updateCorr(ci corrIdx, diff, depth int) {
	bonus := max(-corrLimit/4, min(diff*depth/8, corrLimit/4))
	h := tt.hist
	corrGravity(&h.corrPawn[ci.pawn], bonus)
	corrGravity(&h.corrNPW[ci.npw], bonus)
	corrGravity(&h.corrNPB[ci.npb], bonus)
}

func corrGravity(e *int16, bonus int) {
	abs := bonus
	if abs < 0 {
		abs = -abs
	}
	v := int(*e)
	v += bonus - v*abs/corrLimit
	*e = int16(v)
}

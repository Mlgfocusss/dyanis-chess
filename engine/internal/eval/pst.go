package eval

import "dyanis-chess/engine/internal/board"

// -----------------------------------------------------------------------
// Piece-square tables.
//
// Indexed exactly like board.Board.Squares: index = rank*8+file, 0 = a1,
// 63 = h8, i.e. rank 1 is the FIRST row below and rank 8 is the LAST.
// That's the opposite order from how these tables are usually printed
// in engine write-ups (rank 8 on top, rank 1 on the bottom) — the rows
// below are that classic layout flipped vertically to match our index
// convention. All tables are written from White's point of view.
//
// For Black, don't build a second set of tables: mirror the square
// with sq^56, which flips the rank (0<->7, 1<->6, ...) while leaving
// the file untouched, and reuse the same White-oriented table. That's
// exactly the "same square relative to your own side" a mirrored
// table would give you.
//
// pawnPST/knightPST/bishopPST/rookPST/queenPST are Texel-tuned (see
// plan.md, "тюнинг эвала" — cmd/tune's coordinate descent, converged
// at epoch 76 against 725,000 labeled positions). Unlike before
// tuning, these are no longer hand-symmetric left-right or
// rank-graduated by eye — the tuner has no reason to preserve a
// symmetry a human found aesthetically obvious, only to lower MSE
// against real game outcomes, so don't be surprised the numbers look
// "noisier" than a hand-authored table would. They're still a SINGLE
// table per piece, not phase-tapered (unlike kingMidgamePST/
// kingEndgamePST below, which are the only tables that blend by
// `phase`) — queenPST in particular has to describe both "don't bring
// the queen out early" (midgame) and "an active queen near the enemy
// king is strong" (endgame) in one shared table, which is a real
// limitation of the tuned result worth keeping in mind, not something
// this tuning run fixed.
// -----------------------------------------------------------------------

var pawnPST = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0, // rank 1
	1, 0, -5, -17, -12, 10, 6, -9, // rank 2
	0, 0, 0, -5, 3, 2, 3, -6, // rank 3
	4, 2, 0, 5, 5, -2, -2, -6, // rank 4
	17, 14, 6, 2, 6, 6, 10, 7, // rank 5
	41, 35, 15, -12, -9, 19, 27, 27, // rank 6
	114, 110, 76, 62, 65, 74, 82, 105, // rank 7
	0, 0, 0, 0, 0, 0, 0, 0, // rank 8
}

var knightPST = [64]int{
	-30, -3, -15, 1, 6, 2, -3, -32, // rank 1
	-15, -13, 7, 22, 24, 20, 2, -1, // rank 2
	-12, 8, 23, 29, 30, 31, 26, -4, // rank 3
	2, 13, 29, 36, 38, 35, 31, 10, // rank 4
	2, 24, 34, 50, 43, 46, 29, 16, // rank 5
	-14, 23, 27, 37, 36, 39, 28, -5, // rank 6
	-26, 4, 18, 19, 16, 12, 1, -21, // rank 7
	-100, -40, -3, -16, 1, -40, -39, -105, // rank 8
}

var bishopPST = [64]int{
	0, 18, 6, 14, 15, 9, 10, -1, // rank 1
	16, 22, 26, 18, 27, 24, 36, 11, // rank 2
	11, 25, 28, 29, 28, 30, 20, 13, // rank 3
	12, 21, 23, 35, 30, 17, 17, 16, // rank 4
	11, 18, 24, 34, 30, 30, 18, 17, // rank 5
	6, 20, 23, 21, 20, 24, 26, 9, // rank 6
	1, 13, 8, 2, 16, 32, 13, -19, // rank 7
	-17, 1, -6, 2, 7, 5, 8, -3, // rank 8
}

var rookPST = [64]int{
	-6, 4, 13, 15, 13, 8, -10, -8, // rank 1
	-14, 2, 4, 9, 5, 8, 0, -28, // rank 2
	-11, 6, 3, 5, 4, 4, 5, -16, // rank 3
	1, 9, 13, 10, 8, 8, 12, -2, // rank 4
	9, 10, 19, 14, 15, 19, 10, 10, // rank 5
	14, 20, 16, 20, 10, 18, 20, 10, // rank 6
	24, 23, 29, 28, 21, 23, 19, 22, // rank 7
	18, 17, 17, 18, 20, 17, 16, 13, // rank 8
}

var queenPST = [64]int{
	3, -2, 6, 26, 4, -14, -23, -37, // rank 1
	-12, -3, 15, 12, 20, 12, -9, 6, // rank 2
	-7, 3, 5, 6, 7, 14, 18, 12, // rank 3
	-7, -11, 0, 1, 12, 13, 21, 13, // rank 4
	-15, -15, -8, 1, 21, 24, 26, 16, // rank 5
	-20, -8, -3, 20, 39, 54, 53, 42, // rank 6
	-30, -33, 0, 23, 20, 43, 25, 38, // rank 7
	-26, 4, 14, 15, 33, 26, 29, 39, // rank 8
}

// King tables — kingMidgamePST/kingEndgamePST — are the ORIGINAL
// hand-authored values, and now permanently so: King PST is no
// longer even in cmd/tune's tunable scope (see eval/tunable.go's
// TuneParams — there's no field for it at all), not merely "tuned but
// its output discarded". That followed TWO separate runs, both
// starting from these same original tables, both converging to the
// same extreme, incoherent pattern: swings like -126 to +158 on
// single cells, three to five times larger than anything in the other
// tuned tables, with no spatial coherence (no smoothness toward the
// center/corners the way even the noisiest of the other tuned tables
// still has). The reason is structural, not bad luck: the king only
// ever occupies a handful of squares in any real midgame (mostly
// g1/c1/g8/c8 after castling, or e1/e8 before it), so most cells in
// kingMidgamePST have only a handful of supporting positions in the
// dataset, sometimes close to none — coordinate descent, as built
// here, doesn't distinguish "real signal" from "fit to noise from 3
// stray positions", it just lowers MSE either way. Confirming that
// twice, from the same starting point, ruled out "one unlucky run" —
// this needs per-cell sample-count awareness or regularization before
// it could be trusted, a separate, harder problem this project hasn't
// taken on. See eval.go's own tuning history note for the numbers
// from every run that DID feed into the tables below.
var kingMidgamePST = [64]int{
	20, 30, 10, 0, 0, 10, 30, 20, // rank 1
	20, 20, 0, 0, 0, 0, 20, 20, // rank 2
	-10, -20, -20, -20, -20, -20, -20, -10, // rank 3
	-20, -30, -30, -40, -40, -30, -30, -20, // rank 4
	-30, -40, -40, -50, -50, -40, -40, -30, // rank 5
	-30, -40, -40, -50, -50, -40, -40, -30, // rank 6
	-30, -40, -40, -50, -50, -40, -40, -30, // rank 7
	-30, -40, -40, -50, -50, -40, -40, -30, // rank 8
}

var kingEndgamePST = [64]int{
	-50, -30, -30, -30, -30, -30, -30, -50, // rank 1
	-30, -30, 0, 0, 0, 0, -30, -30, // rank 2
	-30, -10, 20, 30, 30, 20, -10, -30, // rank 3
	-30, -10, 30, 40, 40, 30, -10, -30, // rank 4
	-30, -10, 30, 40, 40, 30, -10, -30, // rank 5
	-30, -10, 20, 30, 30, 20, -10, -30, // rank 6
	-30, -20, -10, 0, 0, -10, -20, -30, // rank 7
	-50, -40, -30, -20, -20, -30, -40, -50, // rank 8
}

// mirror flips a square's rank while keeping its file, turning a
// White-oriented PST lookup into the equivalent square for Black.
func mirror(sq board.Square) board.Square {
	return board.Square(int(sq) ^ 56)
}

// init registers this package's real material+PST values as
// board.Board's incremental materialPST hook (see
// board/material_hook.go) — the one piece of wiring that lets Board
// track material+PST incrementally through MakeMove/UnmakeMove/
// SetSquare without board itself knowing what a piece is "worth".
//
// Returns 0 for King: its positional value depends on `phase`, a
// board-wide quantity that changes on every non-pawn capture or
// promotion anywhere on the board, not just when the king itself
// moves — see PieceValue's own doc comment in material_hook.go for
// why that rules out a per-square incremental hook for it.
// materialAndPstScore below computes the king's own term itself,
// separately, directly from board.Board.KingSquare (already O(1), no
// scan needed) and the CURRENT phase at evaluation time.
func init() {
	// The hook runs on every MakeMove/UnmakeMove, so it reads a table built
	// once here instead of going through pieceValue/pstValue each time.
	// Indexed by Piece+6 (Piece is -6..6), then square.
	for p := board.BK; p <= board.WK; p++ {
		if p == board.None || p.Type() == board.King {
			continue // empty square and king: 0 (see above)
		}
		for sq := board.Square(0); sq < 64; sq++ {
			// phase=0 is safe: every pstValue branch except King's ignores it.
			pieceSquareValue[p+6][sq] = pieceValue(p.Type()) + pstValue(p, sq, 0)
		}
	}
	for phase := 0; phase <= maxPhase; phase++ {
		for sq := 0; sq < 64; sq++ {
			kingTaper[phase][sq] = (kingMidgamePST[sq]*phase + kingEndgamePST[sq]*(maxPhase-phase)) / maxPhase
		}
	}
	board.SetPieceValueHook(func(p board.Piece, sq board.Square) int {
		return pieceSquareValue[p+6][sq]
	})
}

// pieceSquareValue[p+6][sq] is material + PST for every non-king piece
// (kings and None stay 0), the table behind the incremental materialPST hook.
var pieceSquareValue [13][64]int

// kingTaper[phase][idx] is the king PST blended by phase, precomputed with
// exactly pstValue's formula (idx is already color-mirrored).
var kingTaper [maxPhase + 1][64]int

// pstValue looks up a piece's positional bonus/penalty on sq. phase is
// the value returned by gamePhase, passed in so it's only computed
// once per Evaluate call rather than once per piece.
func pstValue(p board.Piece, sq board.Square, phase int) int {
	idx := sq
	if p.Color() == board.Black {
		idx = mirror(sq)
	}
	switch p.Type() {
	case board.Pawn:
		return pawnPST[idx]
	case board.Knight:
		return knightPST[idx]
	case board.Bishop:
		return bishopPST[idx]
	case board.Rook:
		return rookPST[idx]
	case board.Queen:
		return queenPST[idx]
	case board.King:
		// Linear taper: phase == maxPhase -> pure mg, phase == 0 -> pure eg
		// (precomputed in init).
		return kingTaper[phase][idx]
	default:
		return 0
	}
}

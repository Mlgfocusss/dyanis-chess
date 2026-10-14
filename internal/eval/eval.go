// Package eval scores a position from the perspective of the side to
// move: positive means the side to move is better off.
//
// Step 5 of the plan: material (step 3) and piece-square tables (step
// 4), plus five more positional terms, each in its own file:
//   - mobility        (mobility.go)
//   - pawn structure  (pawns.go)
//   - king safety     (king_safety.go)
//   - the bishop pair (bishops.go)
//   - mop-up          (mopup.go) — king-hunting technique once a
//     material advantage is already decisive, so search has something
//     to converge on instead of shuffling a piece forever in a
//     trivially won position (e.g. K+2Q+R vs K)
//
// Every term below is computed as a single White-minus-Black number,
// not "side to move minus other side". Evaluate sums all the terms
// and flips the sign exactly once at the end if Black is to move,
// instead of every term re-deriving "am I SideToMove or not" for
// every piece — cheaper, and it means a term's own tests can just
// think in absolute White/Black terms.
package eval

import "dyanis-chess/internal/board"

// Material values in centipawns, the conventional unit (1 pawn = 100).
//
// Texel-tuned (see plan.md, "тюнинг эвала"). Four runs so far:
//   - material+PST only: converged epoch 76, train MSE 0.064826 ->
//     0.061244.
//   - + mobility/pawn-structure/king-safety: converged epoch 33,
//     train MSE -> 0.060733.
//   - + King PST (later fully removed from scope again — two separate
//     runs reproduced the same overfit pattern on it; see pst.go's
//     kingMidgamePST note): converged epoch 48, train MSE -> 0.060262.
//   - + bishop pair, King PST removed from scope for good: converged
//     epoch 19, train MSE -> 0.060711, held-out 0.061195. Removing
//     King PST from the tuning loop barely moved anything else (a
//     couple of centipawns here and there) — reassuring: the other
//     groups weren't quietly leaning on King PST's overfit signal.
//
// Knight and Bishop are unequal in every run (currently 317/324)
// rather than the 300/300 this project started with — the data
// consistently pulls the bishop slightly ahead. Whether that gap (and
// the similarly sized shifts in mobility/king-safety) is genuine
// signal or an artifact of these tables/weights not being
// phase-tapered isn't settled by MSE alone — the tactical test-suite
// result after each change is what actually answers that.
const (
	PawnValue   = 91
	KnightValue = 317
	BishopValue = 324
	RookValue   = 541
	QueenValue  = 1079
)

func pieceValue(t board.PieceType) int {
	switch t {
	case board.Pawn:
		return PawnValue
	case board.Knight:
		return KnightValue
	case board.Bishop:
		return BishopValue
	case board.Rook:
		return RookValue
	case board.Queen:
		return QueenValue
	default: // King: material-count doesn't need to value it, it's never captured
		return 0
	}
}

// materialAndPstScore returns material + piece-square value, White
// minus Black. matPST is the material+PST sum over every piece except
// the king (in practice always board.Board.MaterialPST() — see this
// function's one call site in Evaluate below); taking it as a
// parameter rather than reading b.MaterialPST() directly keeps this
// function correct for any future caller that doesn't have that
// incremental value available to read (board.Board.MaterialPST() is
// seeded once by FromFEN/NewInitialBoard and kept in sync by
// MakeMove/UnmakeMove — it can never reflect a hypothetical set of
// material/PST weights someone is probing offline, e.g. a Texel-
// tuning harness recomputing it from SquaresArray() on each probed
// weight). Evaluate's own behavior is unchanged: it still passes
// b.MaterialPST() here, exactly as this function itself used to read
// internally before this value became a parameter.
//
// Only the king's own term still gets computed fresh on every call,
// since its value blends between midgame/endgame PSTs based on
// `phase` — a board-wide quantity that changes on every non-pawn
// capture or promotion anywhere, not just when the king itself moves,
// so it can never be folded into the incremental matPST sum. Still
// O(1) either way: two table lookups off board.Board.KingSquare
// (itself already cached), not a scan.
func materialAndPstScore(b *board.Board, phase int, matPST int) int {
	score := matPST

	if wk := b.KingSquare(board.White); wk != board.NoSquare {
		score += pstValue(board.MakePiece(board.King, board.White), wk, phase)
	}
	if bk := b.KingSquare(board.Black); bk != board.NoSquare {
		score -= pstValue(board.MakePiece(board.King, board.Black), bk, phase)
	}
	return score
}

// phaseWeight is how much each piece type counts toward "how much
// midgame material is still on the board". Values follow the common
// convention: N/B = 1, R = 2, Q = 4. A full starting set of minor/major
// pieces (4N+4B+4R+2Q) sums to maxPhase = 24; an endgame with just
// kings and pawns sums to 0.
func phaseWeight(t board.PieceType) int {
	switch t {
	case board.Knight, board.Bishop:
		return 1
	case board.Rook:
		return 2
	case board.Queen:
		return 4
	default:
		return 0
	}
}

const maxPhase = 24

// gamePhase counts how much non-pawn material remains on the board,
// clamped to [0, maxPhase]. maxPhase means "still midgame material
// out there", 0 means "no minor/major pieces left, deep endgame".
// One Count() popcount per (piece type, color) pair instead of a
// 64-square scan.
func gamePhase(b *board.Board) int {
	phase := 0
	for _, color := range [2]board.Color{board.White, board.Black} {
		for pt := board.Knight; pt <= board.Queen; pt++ {
			phase += phaseWeight(pt) * b.Pieces(pt, color).Count()
		}
	}
	if phase > maxPhase {
		phase = maxPhase
	}
	return phase
}

// Evaluate returns a centipawn score from the perspective of the side
// to move: positive means the side to move is ahead. cache, when
// non-nil, is used for pawn-structure scoring — see PawnCache's own
// doc comment for what it's for and why it's threaded in here rather
// than kept as a package-level global. Passing nil (as every existing
// eval package test does) just means pawn structure gets recomputed
// from scratch every call, same as before caching existed — never
// wrong, only slower.
//
// TODO(later steps): real king danger from actual enemy attackers
// (not just open/semi-open files), space, outposts.
func Evaluate(b *board.Board, cache *PawnCache) int {
	phase := gamePhase(b)

	score := materialAndPstScore(b, phase, b.MaterialPST())
	score += mobilityScore(b)
	score += pawnStructureScore(b, cache)
	score += kingSafetyScore(b, phase)
	score += bishopPairScore(b)
	score += mopupScore(b, phase)

	if b.SideToMove == board.Black {
		score = -score
	}
	return score
}

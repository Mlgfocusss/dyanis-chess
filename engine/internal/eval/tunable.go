package eval

import "dyanis-chess/engine/internal/board"

// MaterialPSTParams is every material+PST weight the "тюнинг эвала"
// plan step (plan.md) tunes, made explicit instead of living only as
// package-level constants (PawnValue etc., eval.go) and tables
// (pawnPST etc., pst.go). King is deliberately absent: it's already
// excluded from board.Board.MaterialPST() itself (see
// material_hook.go's PieceValue doc comment — its value depends on
// phase, a board-wide quantity, so it can never be a per-square
// incremental/offline sum) and stays computed the same way it always
// has, inside materialAndPstScore's own king term.
type MaterialPSTParams struct {
	PawnValue, KnightValue, BishopValue, RookValue, QueenValue int
	PawnPST, KnightPST, BishopPST, RookPST, QueenPST           [64]int
}

// DefaultMaterialPSTParams is today's actual production weights
// (eval.go's PawnValue/KnightValue/... and pst.go's pawnPST/
// knightPST/...) collected into one value. Computing
// ComputeMaterialPST with this reproduces board.Board.MaterialPST()
// exactly for every position — cmd/tune's baseline-MSE check is what
// confirms that, not a unit test (internal/eval intentionally has
// none beyond Evaluate() integration tests — see plan.md).
var DefaultMaterialPSTParams = MaterialPSTParams{
	PawnValue: PawnValue, KnightValue: KnightValue, BishopValue: BishopValue,
	RookValue: RookValue, QueenValue: QueenValue,
	PawnPST: pawnPST, KnightPST: knightPST, BishopPST: bishopPST,
	RookPST: rookPST, QueenPST: queenPST,
}

// pieceValue mirrors package-level pieceValue(t) in eval.go, reading
// from params instead of the package constants — King intentionally
// falls to the same "0" default as that function, via the switch's
// default case.
func (p *MaterialPSTParams) pieceValue(t board.PieceType) int {
	switch t {
	case board.Pawn:
		return p.PawnValue
	case board.Knight:
		return p.KnightValue
	case board.Bishop:
		return p.BishopValue
	case board.Rook:
		return p.RookValue
	case board.Queen:
		return p.QueenValue
	default:
		return 0
	}
}

// pstValue mirrors pstValue(p, sq, phase) in pst.go for the five
// non-king piece types, reading from params' tables instead of the
// package-level ones. idx is expected pre-mirrored for Black by the
// caller (ComputeMaterialPST below), same convention pstValue itself
// uses.
func (p *MaterialPSTParams) pstValue(t board.PieceType, idx board.Square) int {
	switch t {
	case board.Pawn:
		return p.PawnPST[idx]
	case board.Knight:
		return p.KnightPST[idx]
	case board.Bishop:
		return p.BishopPST[idx]
	case board.Rook:
		return p.RookPST[idx]
	case board.Queen:
		return p.QueenPST[idx]
	default:
		return 0
	}
}

// ComputeMaterialPST recomputes the White-minus-Black material+PST
// sum from scratch off b.SquaresArray() — the offline counterpart to
// board.Board.MaterialPST(), which is only ever seeded once (FromFEN/
// NewInitialBoard) and kept incrementally in sync by MakeMove/
// UnmakeMove for ONE fixed set of weights (board.PieceValue, wired up
// once in pst.go's init()). A tuning harness needs to ask "what would
// material+PST be under a DIFFERENT, hypothetical set of weights?" —
// something the incremental value can never answer — without mutating
// or re-deriving the board itself. Same O(64) cost as
// board.rebuildMaterialPST (material_hook.go), which this is the
// eval-package equivalent of, just parameterized by params instead of
// hard-wired to the one registered board.PieceValue hook.
//
// King is skipped (see MaterialPSTParams' own doc comment for why) —
// exactly like board.PieceValue already does for the production path.
func ComputeMaterialPST(b *board.Board, params MaterialPSTParams) int {
	total := 0
	squares := b.SquaresArray()
	for i, p := range squares {
		if p.IsNone() || p.Type() == board.King {
			continue
		}
		idx := board.Square(i)
		if p.Color() == board.Black {
			idx = mirror(idx)
		}
		v := params.pieceValue(p.Type()) + params.pstValue(p.Type(), idx)
		if p.Color() == board.White {
			total += v
		} else {
			total -= v
		}
	}
	return total
}

// NonMaterialScore is everything Evaluate computes that the CURRENT
// tuning scope (TuneParams, below) doesn't cover: the king's own PST
// term (materialAndPstScore with matPST=0 isolates just that) and
// mop-up. phase is computed internally (gamePhase is unexported —
// same reasoning as ComputeKingSafetyScore below).
//
// King PST was briefly IN the tuned scope (a run did include it,
// converged, and its own result was rejected — see pst.go's note on
// kingMidgamePST for why). This function's narrowing was reverted
// back out for it specifically, not just its printed output: the
// same run tuned again afterward (to add bishop pair) reproduced the
// identical extreme, incoherent pattern on King PST, starting from
// the SAME original hand-authored tables — confirming it wasn't a
// one-off fluke of that particular run, but this project's
// unregularized coordinate descent reliably overfitting a term with
// very sparse per-square support. So King PST is out of TuneParams
// entirely now, not merely "tuned but not pasted" — leaving it in
// scope while distrusting its output was the actual bug, not King PST
// itself.
func NonMaterialScore(b *board.Board) int {
	phase := gamePhase(b)
	score := materialAndPstScore(b, phase, 0) // matPST=0: king term only
	score += mopupScore(b, phase)
	return score
}

// EvaluateWithMaterialPST is Evaluate's tuning-time counterpart:
// identical to Evaluate, except the material+PST term comes from
// ComputeMaterialPST(b, params) instead of the board's own
// incrementally-tracked b.MaterialPST(). Passing
// DefaultMaterialPSTParams must reproduce Evaluate(b, cache) exactly,
// position for position — that equality (checked via cmd/tune's
// baseline MSE staying bit-identical, not a unit test — see this
// file's own top-of-file note) is what proves this function is
// correct before it's ever used to probe a weight that differs from
// production.
//
// Evaluate itself is untouched by this: this function exists purely
// so a tuning harness can ask "what would the score be under
// different material+PST weights?" without ever mutating or
// re-deriving a board.Board's own live state.
func EvaluateWithMaterialPST(b *board.Board, cache *PawnCache, params MaterialPSTParams) int {
	phase := gamePhase(b)
	matPST := ComputeMaterialPST(b, params)

	score := materialAndPstScore(b, phase, matPST)
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

// MobilityParams mirrors mobility.go's four per-piece-type weights
// (knightMobilityWeight/bishopMobilityWeight/rookMobilityWeight/
// queenMobilityWeight) — see ComputeMobilityScore.
type MobilityParams struct {
	Knight, Bishop, Rook, Queen int
}

// PawnStructureParams mirrors pawns.go's doubled/isolated penalties
// and the passed-pawn bonus table. PassedBonus[0] and PassedBonus[7]
// are never read by ComputePawnStructureScore below — same as the
// production passedPawnBonus table (see its own doc comment in
// pawns.go: a pawn can't stand on its own back rank or the promotion
// rank) — so cmd/tune's paramSlots deliberately skips those two
// indices rather than spending coordinate-descent passes on values
// nothing ever looks at.
type PawnStructureParams struct {
	DoubledPenalty, IsolatedPenalty int
	PassedBonus                     [8]int
}

// KingSafetyParams mirrors king_safety.go's three constants
// (shieldPawnBonus/openFileKingPenalty/semiOpenFileKingPenalty).
type KingSafetyParams struct {
	ShieldPawnBonus, OpenFilePenalty, SemiOpenFilePenalty int
}

// TuneParams is every weight the current tuning scope covers:
// material+PST (MaterialPSTParams), mobility, pawn structure, king
// safety, and bishop pair. King PST and mop-up are deliberately
// absent — King PST was tried and its result rejected as overfit to
// sparse per-square data (see NonMaterialScore's own doc comment
// above for the full story); mop-up is a gated threshold term (see
// mopupEligible in mopup.go), not a smooth function coordinate
// descent's "try +step/-step, keep whichever lowers MSE" logic is
// built for. BishopPair is a bare int, not a wrapper struct — one
// scalar doesn't need one, unlike the other groups.
type TuneParams struct {
	MaterialPST MaterialPSTParams
	Mobility    MobilityParams
	Pawns       PawnStructureParams
	KingSafety  KingSafetyParams
	BishopPair  int
}

// DefaultTuneParams is today's production weights across all groups,
// collected the same way DefaultMaterialPSTParams already is. Note
// MaterialPST/Mobility/Pawns/KingSafety here are ALREADY the
// Texel-tuned values from previous runs — this run starts warm on
// everything already tuned and cold only on the one newly-added
// BishopPair parameter. King PST is intentionally not part of this
// struct at all (see TuneParams' own doc comment).
var DefaultTuneParams = TuneParams{
	MaterialPST: DefaultMaterialPSTParams,
	Mobility: MobilityParams{
		Knight: knightMobilityWeight, Bishop: bishopMobilityWeight,
		Rook: rookMobilityWeight, Queen: queenMobilityWeight,
	},
	Pawns: PawnStructureParams{
		DoubledPenalty: doubledPawnPenalty, IsolatedPenalty: isolatedPawnPenalty,
		PassedBonus: passedPawnBonus,
	},
	KingSafety: KingSafetyParams{
		ShieldPawnBonus: shieldPawnBonus, OpenFilePenalty: openFileKingPenalty,
		SemiOpenFilePenalty: semiOpenFileKingPenalty,
	},
	BishopPair: bishopPairBonus,
}

// ComputeMobilityScore mirrors mobility.go's mobilityScore exactly,
// reading weights from params instead of the package-level
// knightMobilityWeight/bishopMobilityWeight/rookMobilityWeight/
// queenMobilityWeight constants. Reuses mobility.go's own unexported
// helpers (pawnAttackSquares/knightMobility/slidingMobility) — same
// package, so only the "multiply by weight and sum" shell is
// duplicated here, not the actual attack-counting logic.
func ComputeMobilityScore(b *board.Board, params MobilityParams) int {
	whitePawnAtk, blackPawnAtk := pawnAttackSquares(b)
	occ := b.Occupied()

	score := 0
	for _, color := range [2]board.Color{board.White, board.Black} {
		sign := 1
		enemyPawnAtk := blackPawnAtk
		if color == board.Black {
			sign = -1
			enemyPawnAtk = whitePawnAtk
		}

		knights := b.Pieces(board.Knight, color)
		for knights != 0 {
			var sq board.Square
			sq, knights = knights.PopLSB()
			score += sign * params.Knight * knightMobility(b, sq, color, enemyPawnAtk)
		}
		bishops := b.Pieces(board.Bishop, color)
		for bishops != 0 {
			var sq board.Square
			sq, bishops = bishops.PopLSB()
			score += sign * params.Bishop * slidingMobility(b, board.BishopAttacks(sq, occ), color, enemyPawnAtk)
		}
		rooks := b.Pieces(board.Rook, color)
		for rooks != 0 {
			var sq board.Square
			sq, rooks = rooks.PopLSB()
			score += sign * params.Rook * slidingMobility(b, board.RookAttacks(sq, occ), color, enemyPawnAtk)
		}
		queens := b.Pieces(board.Queen, color)
		for queens != 0 {
			var sq board.Square
			sq, queens = queens.PopLSB()
			score += sign * params.Queen * slidingMobility(b, board.QueenAttacks(sq, occ), color, enemyPawnAtk)
		}
	}
	return score
}

// ComputePawnStructureScore mirrors pawns.go's
// computePawnStructureScore, reading weights from params instead of
// doubledPawnPenalty/isolatedPawnPenalty/passedPawnBonus. Same
// signature shape as computePawnStructureScore itself (two pawn
// bitboards, not *board.Board or a cache) — that's all this ever
// needed.
func ComputePawnStructureScore(whitePawns, blackPawns board.Bitboard, params PawnStructureParams) int {
	whiteFiles := fileCounts(whitePawns)
	blackFiles := fileCounts(blackPawns)

	score := 0
	for f := 0; f < 8; f++ {
		if whiteFiles[f] > 1 {
			score -= params.DoubledPenalty * (whiteFiles[f] - 1)
		}
		if blackFiles[f] > 1 {
			score += params.DoubledPenalty * (blackFiles[f] - 1)
		}
		if whiteFiles[f] > 0 && isIsolated(whiteFiles, f) {
			score -= params.IsolatedPenalty * whiteFiles[f]
		}
		if blackFiles[f] > 0 && isIsolated(blackFiles, f) {
			score += params.IsolatedPenalty * blackFiles[f]
		}
	}

	wp := whitePawns
	for wp != 0 {
		var sq board.Square
		sq, wp = wp.PopLSB()
		if isPassed(blackPawns, sq, board.White) {
			score += params.PassedBonus[sq.Rank()]
		}
	}
	bp := blackPawns
	for bp != 0 {
		var sq board.Square
		sq, bp = bp.PopLSB()
		if isPassed(whitePawns, sq, board.Black) {
			score -= params.PassedBonus[7-sq.Rank()]
		}
	}
	return score
}

// ComputeKingSafetyScore mirrors king_safety.go's kingSafetyScore,
// reading weights from params instead of shieldPawnBonus/
// openFileKingPenalty/semiOpenFileKingPenalty. Computes phase
// internally (gamePhase is unexported — this is the one entry point
// external callers like cmd/tune have, so it owns computing its own
// phase rather than requiring the caller to reach into an unexported
// function, the same reasoning NonMaterialScore above already uses).
func ComputeKingSafetyScore(b *board.Board, params KingSafetyParams) int {
	phase := gamePhase(b)
	if phase == 0 {
		return 0
	}

	whitePawns := b.Pieces(board.Pawn, board.White)
	blackPawns := b.Pieces(board.Pawn, board.Black)

	raw := 0
	if wk := b.KingSquare(board.White); wk != board.NoSquare {
		raw += kingSafetyTermParam(wk, board.White, whitePawns, blackPawns, params)
	}
	if bk := b.KingSquare(board.Black); bk != board.NoSquare {
		raw -= kingSafetyTermParam(bk, board.Black, blackPawns, whitePawns, params)
	}
	return raw * phase / maxPhase
}

// kingSafetyTermParam mirrors king_safety.go's kingSafetyTerm,
// reading params instead of the package-level constants. Shares
// fileMask with pawns.go/king_safety.go (same package).
func kingSafetyTermParam(kingSq board.Square, color board.Color, ownPawns, enemyPawns board.Bitboard, params KingSafetyParams) int {
	file, rank := kingSq.File(), kingSq.Rank()
	score := 0

	shieldRank := rank + 1
	if color == board.Black {
		shieldRank = rank - 1
	}
	if shieldRank >= 0 && shieldRank < 8 {
		for f := file - 1; f <= file+1; f++ {
			if f < 0 || f > 7 {
				continue
			}
			if ownPawns.Test(board.MakeSquare(f, shieldRank)) {
				score += params.ShieldPawnBonus
			}
		}
	}

	for f := file - 1; f <= file+1; f++ {
		if f < 0 || f > 7 {
			continue
		}
		ownHere := ownPawns&fileMask[f] != 0
		enemyHere := enemyPawns&fileMask[f] != 0
		switch {
		case !ownHere && !enemyHere:
			score -= params.OpenFilePenalty
		case !ownHere && enemyHere:
			score -= params.SemiOpenFilePenalty
		}
	}
	return score
}

// ComputeBishopPairScore mirrors bishops.go's bishopPairScore,
// reading the bonus from a parameter instead of the package-level
// bishopPairBonus constant.
func ComputeBishopPairScore(b *board.Board, bonus int) int {
	white := b.Pieces(board.Bishop, board.White).Count()
	black := b.Pieces(board.Bishop, board.Black).Count()

	score := 0
	if white >= 2 {
		score += bonus
	}
	if black >= 2 {
		score -= bonus
	}
	return score
}

// EvaluateWithTuneParams is Evaluate's tuning-time counterpart at the
// CURRENT tuning scope (material+PST, mobility, pawn structure, king
// safety, bishop pair — TuneParams). Passing DefaultTuneParams must
// reproduce Evaluate(b, cache) exactly — verified the same way as
// before, via cmd/tune's -mode=verify at shifted weights, not a unit
// test (see tunable.go's earlier note on why: plan.md, "тестов на
// internal/eval сознательно нет").
//
// Deliberately has NO *PawnCache parameter, unlike Evaluate/
// EvaluateWithMaterialPST. PawnCache's key is just the two pawn
// bitboards (pawn_cache.go) — it has no idea WHICH weights produced a
// cached score. Now that pawn structure is tunable, reusing a shared
// cache across different probed Pawns params would silently hand back
// a score computed under a DIFFERENT parameter set than the one
// actually being probed — a correctness bug, not just a wasted
// optimization. Every existing call site already passed nil here
// anyway (cmd/tune's whiteScoreFull never had a real cache to share),
// so removing the parameter just makes that the only option instead
// of merely the convention.
//
// Calls materialAndPstScore for the king's own term, same as
// production Evaluate does — King PST isn't tunable (see
// NonMaterialScore's doc comment above), so there's no separate
// params field to plug in here; the package-level kingMidgamePST/
// kingEndgamePST are exactly what should be read.
func EvaluateWithTuneParams(b *board.Board, params TuneParams) int {
	phase := gamePhase(b)
	matPST := ComputeMaterialPST(b, params.MaterialPST)

	score := materialAndPstScore(b, phase, matPST)
	score += ComputeMobilityScore(b, params.Mobility)

	whitePawns := b.Pieces(board.Pawn, board.White)
	blackPawns := b.Pieces(board.Pawn, board.Black)
	score += ComputePawnStructureScore(whitePawns, blackPawns, params.Pawns)

	score += ComputeKingSafetyScore(b, params.KingSafety)
	score += ComputeBishopPairScore(b, params.BishopPair)
	score += mopupScore(b, phase)

	if b.SideToMove == board.Black {
		score = -score
	}
	return score
}

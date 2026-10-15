package eval

import "dyanis-chess/engine/internal/board"

// Mobility weight per available destination square, in centipawns.
//
// Texel-tuned (see plan.md, "тюнинг эвала" — second cmd/tune run,
// scope extended from material+PST to include mobility/pawn-
// structure/king-safety; converged epoch 33, train MSE 0.060733,
// held-out 0.061214). Originally this project set these by hand with
// knight/bishop given the LARGEST per-square weight and rook/queen
// deliberately the smallest, on the reasoning that a rook or queen
// usually has some mobility almost everywhere just from how far they
// reach, so a raw square count says less about the piece's quality
// for them than for a knight or bishop. The tuned result contradicts
// that: rook and queen came out with mobility weights close to (queen
// even matching) knight's, not below it. That could mean the original
// reasoning was simply wrong, or it could be this term picking up
// slack from some other term that's still hand-tuned (the split
// between "how much of a rook's value is mobility vs. open-file/7th-
// rank PST bonuses" isn't something this tuning step isolates) — MSE
// alone doesn't distinguish those, which is why this result is worth
// double-checking against the tactical test-suite and cmd/profile
// rather than trusted on the number alone.
const (
	knightMobilityWeight = 4
	bishopMobilityWeight = 4
	rookMobilityWeight   = 5
	queenMobilityWeight  = 4
)

const (
	fileABits board.Bitboard = 0x0101010101010101
	fileHBits board.Bitboard = 0x8080808080808080
)

// pawnAttackSquares reports, for each color, the full bitboard of
// squares that color's pawns attack — every pawn's board.PawnAttacks
// ORed together, computed with two shifts per color instead of a loop
// over the pawns. Computed once per Evaluate call (like gamePhase) and
// shared by every piece's mobility count below instead of being
// re-derived per piece.
func pawnAttackSquares(b *board.Board) (white, black board.Bitboard) {
	return pawnAttacksOf(b.Pieces(board.Pawn, board.White), b.Pieces(board.Pawn, board.Black))
}

func pawnAttacksOf(wp, bp board.Bitboard) (white, black board.Bitboard) {
	white = (wp&^fileABits)<<7 | (wp&^fileHBits)<<9
	black = (bp&^fileABits)>>9 | (bp&^fileHBits)>>7
	return white, black
}

// sideMobility sums the mobility weights of one side's knights, bishops,
// rooks and queens. mask is every square that counts as reachable: not
// occupied by the mover's own pieces and not attacked by an enemy pawn
// (a knight that's only "mobile" into a square it gets immediately traded
// off of isn't really mobile). Sliding attacks are already blocker-aware
// (board.BishopAttacks/RookAttacks stop at the first occupied square, same
// as a real move would); an enemy-pawn-attacked empty square isn't a
// blocker, so it is just masked out of the count without truncating the ray.
func sideMobility(knights, bishops, rooks, queens, occ, mask board.Bitboard) int {
	score := 0
	for knights != 0 {
		var sq board.Square
		sq, knights = knights.PopLSB()
		score += knightMobilityWeight * (board.KnightAttacks(sq) & mask).Count()
	}
	for bishops != 0 {
		var sq board.Square
		sq, bishops = bishops.PopLSB()
		score += bishopMobilityWeight * (board.BishopAttacks(sq, occ) & mask).Count()
	}
	for rooks != 0 {
		var sq board.Square
		sq, rooks = rooks.PopLSB()
		score += rookMobilityWeight * (board.RookAttacks(sq, occ) & mask).Count()
	}
	for queens != 0 {
		var sq board.Square
		sq, queens = queens.PopLSB()
		score += queenMobilityWeight * (board.QueenAttacks(sq, occ) & mask).Count()
	}
	return score
}

// mobilityScore returns White-minus-Black mobility bonus in
// centipawns for knights, bishops, rooks, and queens. Pawns aren't
// scored here (their advance is covered by pawnStructureScore's
// passed-pawn bonus), and neither is the king (huddling near safety
// is often correct, so a raw square count would send the wrong
// signal — king safety gets its own term instead).
func mobilityScore(b *board.Board) int {
	pc := b.PieceBitboards() // index (type-1)*2 + 1 for White, +0 for Black
	whitePawnAtk, blackPawnAtk := pawnAttacksOf(pc[(board.Pawn-1)*2+1], pc[(board.Pawn-1)*2])
	occ := b.Occupied()

	whiteMask := ^(b.OccupiedBy(board.White) | blackPawnAtk)
	blackMask := ^(b.OccupiedBy(board.Black) | whitePawnAtk)

	return sideMobility(pc[(board.Knight-1)*2+1], pc[(board.Bishop-1)*2+1], pc[(board.Rook-1)*2+1], pc[(board.Queen-1)*2+1], occ, whiteMask) -
		sideMobility(pc[(board.Knight-1)*2], pc[(board.Bishop-1)*2], pc[(board.Rook-1)*2], pc[(board.Queen-1)*2], occ, blackMask)
}

// knightMobility counts squares a knight on s could move or capture
// to, excluding squares occupied by its own side and squares an enemy
// pawn attacks — a knight that's only "mobile" into a square it gets
// immediately traded off of isn't really mobile. board.KnightAttacks
// already gives the full destination set as one table lookup; this is
// just that set with the two exclusions masked off and popcounted.
func knightMobility(b *board.Board, s board.Square, color board.Color, enemyPawnAtk board.Bitboard) int {
	targets := board.KnightAttacks(s) &^ b.OccupiedBy(color) &^ enemyPawnAtk
	return targets.Count()
}

// slidingMobility counts reachable squares from a bishop/rook/queen's
// attack bitboard (already blocker-aware — see board.BishopAttacks/
// RookAttacks/QueenAttacks, which stop at the first occupied square
// same as a real move would), excluding the mover's own pieces and
// squares an enemy pawn attacks (same "safe mobility" idea as
// knightMobility). Unlike the old ray-walk version, there's no
// separate "count this square but keep going" step needed: an
// enemy-pawn-attacked square that's otherwise empty still isn't a
// blocker, so it's already included in attacks and simply gets masked
// out of the count here without truncating the ray — exactly the
// original behavior, just derived from one magic lookup instead of a
// hand-walked loop.
func slidingMobility(b *board.Board, attacks board.Bitboard, color board.Color, enemyPawnAtk board.Bitboard) int {
	targets := attacks &^ b.OccupiedBy(color) &^ enemyPawnAtk
	return targets.Count()
}


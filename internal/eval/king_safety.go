package eval

import "dyanis-chess/internal/board"

// Texel-tuned (see plan.md, "тюнинг эвала" — latest cmd/tune run,
// converged epoch 19, train MSE 0.060711, held-out 0.061195; King PST
// removed from the tuning loop entirely as of this run, not just its
// output discarded — see pst.go's kingMidgamePST note).
//
// This term is NOT phase-tapered (unlike the king's own PST, see
// pst.go's kingMidgamePST/kingEndgamePST) — kingSafetyScore below
// already scales its raw total by phase/maxPhase itself, so the
// weights here are shared across the whole game, middlegame attacks
// and pawn-only endgames alike, even though shield pawns and open
// files plausibly matter less once most attackers are off the board.
// Going into the run that first added these to tuning, the concern
// was that averaging over a dataset mixing both phases might wash
// these weights down toward zero. That hasn't happened across three
// runs now: openFileKingPenalty and semiOpenFileKingPenalty stayed
// large; shieldPawnBonus keeps drifting slightly (6->7 this time).
// Whether the file penalties reflect a real, undervalued danger or
// are compensating for the missing phase split some other way isn't
// something MSE settles — same caveat as mobility.go's own note.
const (
	shieldPawnBonus         = 7
	openFileKingPenalty     = 48
	semiOpenFileKingPenalty = 22
)

// kingSafetyScore returns a White-minus-Black king safety adjustment,
// scaled by phase — full weight in the middlegame, fading to zero as
// material comes off the board — the same taper the king PST already
// uses: a king stuck on an open file is a real liability when enemy
// rooks and queens are still around to use it, and much less of one
// once they aren't. At phase 0 the term is skipped entirely rather
// than computed and multiplied by zero.
func kingSafetyScore(b *board.Board, phase int) int {
	if phase == 0 {
		return 0
	}

	whitePawns := b.Pieces(board.Pawn, board.White)
	blackPawns := b.Pieces(board.Pawn, board.Black)

	raw := 0
	if wk := b.KingSquare(board.White); wk != board.NoSquare {
		raw += kingSafetyTerm(wk, board.White, whitePawns, blackPawns)
	}
	if bk := b.KingSquare(board.Black); bk != board.NoSquare {
		raw -= kingSafetyTerm(bk, board.Black, blackPawns, whitePawns)
	}

	return raw * phase / maxPhase
}

// kingSafetyTerm scores one king in isolation: a bonus for each
// friendly pawn still standing on the shield squares directly in
// front of it, and a penalty for each nearby file (its own plus the
// two beside it) that's open or semi-open — a lane an enemy rook or
// queen could use without a pawn ever getting in the way.
func kingSafetyTerm(kingSq board.Square, color board.Color, ownPawns, enemyPawns board.Bitboard) int {
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
				score += shieldPawnBonus
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
			score -= openFileKingPenalty
		case !ownHere && enemyHere:
			score -= semiOpenFileKingPenalty
		}
	}

	return score
}

package eval

import "dyanis-chess/engine/internal/board"

// bishopPairBonus rewards having both bishops: together they cover
// both square colors, which no single bishop can. It's a flat bonus
// tied to the piece set as a whole rather than to any one square, so
// it doesn't fit naturally into the piece-square tables.
//
// Texel-tuned (see plan.md, "тюнинг эвала"): converged to 43 in TWO
// separate cmd/tune runs — once with King PST still in the tuning
// loop, once after King PST was removed entirely — landing on the
// exact same value both times. That's a good sign for this
// particular parameter specifically: unlike King PST's sparse,
// per-square support, "does this side have both bishops" is a
// well-populated, roughly binary signal across the whole dataset, so
// there was never much room for it to quietly ride along on King
// PST's overfit noise the way the earlier entanglement worry
// (discussed before this run) raised as a possibility.
const bishopPairBonus = 43

func bishopPairScore(b *board.Board) int {
	white := b.Pieces(board.Bishop, board.White).Count()
	black := b.Pieces(board.Bishop, board.Black).Count()

	score := 0
	if white >= 2 {
		score += bishopPairBonus
	}
	if black >= 2 {
		score -= bishopPairBonus
	}
	return score
}

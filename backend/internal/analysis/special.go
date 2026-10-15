package analysis

import (
	"dyanis-chess/engine"
)

// Miss is the opposite of grabbing a gift: the opponent has just made a
// mistake, the mover stands clearly better, and the move played (an
// inaccuracy or a mistake) lets much of it go. A blunder stays a blunder.
//
// The two classes above Best, Great and Brilliant, are not a matter of how
// little a move lost (a Best move lost nothing) but of what it was:
//
//   - Great: the only move. The played move is the engine's choice and
//     every alternative is much worse (the best of them is at least
//     GreatGap points of win probability behind), in a position that was
//     still open. Finding it needed more than playing a natural move.
//   - Brilliant: a sacrifice that works. The move gives up material (the
//     engine's own line leaves the mover at least BrilliantSacrifice pawns
//     down once the captures settle) and yet loses nothing, in a position
//     that was not already won and leaves the mover no worse than roughly
//     equal.
//
// Both are heuristics of this project, not a copy of any site's rules. The
// engine has no multi-line search, so "every alternative is worse" is
// measured the plain way: each alternative is played and the position
// searched a few plies. All the numbers are variables below, to be adjusted
// by looking at real games.
const (
	// GreatGap is the win-probability lead of the played move over the
	// second best, in points, that makes it the "only move".
	GreatGap = 15.0
	// GreatMinPlayed is the least win probability the mover must keep after
	// the move: finding the only move in a position that is lost anyway is
	// not great.
	GreatMinPlayed = 25.0
	// GreatMaxBefore and GreatMinBefore bound the mover's win probability
	// before the move: a position already decided either way is not a test.
	GreatMaxBefore = 85.0
	GreatMinBefore = 8.0
	// GreatMaxPrevLoss: when the opponent's last move lost this much or more
	// (a mistake), reacting to it is not great.
	GreatMaxPrevLoss = 10.0

	// BrilliantSacrifice is the material, in pawns, the mover gives up.
	BrilliantSacrifice = 2
	// BrilliantMaxBefore: if the mover was already this likely to win,
	// the sacrifice proves nothing.
	BrilliantMaxBefore = 85.0
	// BrilliantMinAfter: after the sacrifice the mover must still stand
	// at least about equal.
	BrilliantMinAfter = 40.0
	// MissMinPrevLoss and MissMinBefore: the opponent's last move lost at least
	// this much (win-probability points) and left the mover at least this
	// likely to win: a chance was offered.
	MissMinPrevLoss = 10.0
	MissMinBefore   = 60.0
	// maxTraceLen is how many plies of the engine's line are followed to
	// see where the captures end.
	maxTraceLen = 8
)

// Standard piece values in pawns.
var pieceValue = map[rune]int{'p': 1, 'n': 3, 'b': 3, 'r': 5, 'q': 9}

// material returns White's material minus Black's, in pawns, and the number
// of pieces (kings included) on the board, read from a FEN.
func material(fen string) (balance, pieces int) {
	for _, r := range fen {
		if r == ' ' {
			break
		}
		lower := r
		if r >= 'A' && r <= 'Z' {
			lower = r + ('a' - 'A')
		}
		switch {
		case lower >= 'a' && lower <= 'z':
			pieces++
			v := pieceValue[lower]
			if r >= 'A' && r <= 'Z' {
				balance += v
			} else {
				balance -= v
			}
		}
	}
	return balance, pieces
}

// pvTrace replays a principal variation from b and records, after each ply,
// the material balance (White minus Black) and whether that ply was a
// capture. It stops at the first move that is not legal.
func pvTrace(b *engine.Position, pv []engine.Move) (bal []int, captured []bool) {
	cur := b.Copy()
	_, pieces := material(cur.FEN())
	for i, m := range pv {
		if i >= maxTraceLen {
			break
		}
		legal := false
		for _, lm := range cur.LegalMoves() {
			if lm == m {
				legal = true
				break
			}
		}
		if !legal {
			break
		}
		cur.Play(m)
		bl, n := material(cur.FEN())
		bal = append(bal, bl)
		captured = append(captured, n < pieces)
		pieces = n
	}
	return bal, captured
}

// sacrificed returns how many pawns of material the mover (sign +1 for
// White, -1 for Black) gives up by a move, following the engine's best
// reply line from the position after the move until a ply that is not a
// capture: the exchanges are then over and the material counted.
//
// before is the position the move was played in, after the one it led to.
func sacrificed(before, after PositionEval, sign int) int {
	start := sign * before.bal
	end := sign * after.bal
	for j := range after.pvBal {
		end = sign * after.pvBal[j]
		if !after.pvCap[j] {
			break
		}
	}
	return start - end
}

// isBrilliant decides the Brilliant class for a move that is already known
// to have lost (almost) nothing. moverBefore and moverAfter are the mover's
// win probabilities around the move.
//
// A sacrifice in a position that was already won proves nothing, unless it
// is the way to the win: a forced mate for the mover after the sacrifice
// (a queen given up for mate in two) is brilliant however large the
// advantage was.
func isBrilliant(before, after PositionEval, sign int, moverBefore, moverAfter float64, promotion bool) bool {
	if promotion || moverAfter < BrilliantMinAfter {
		return false
	}
	matesNow := after.Mate != 0 && (after.Mate > 0) == (sign > 0)
	if moverBefore > BrilliantMaxBefore && !matesNow {
		return false
	}
	return sacrificed(before, after, sign) >= BrilliantSacrifice
}

// isGreat decides the Great class for a move that is the engine's choice:
// gap is how far the best alternative is behind it (see onlyMoveGap).
//
// prevLoss is what the opponent's previous move cost them: after a real
// mistake the only move is usually just taking what they left hanging,
// which is not a find.
func isGreat(gap, playedPct, moverBefore, prevLoss float64) bool {
	return gap >= GreatGap && playedPct >= GreatMinPlayed &&
		moverBefore <= GreatMaxBefore && moverBefore >= GreatMinBefore &&
		prevLoss < GreatMaxPrevLoss
}

// specialDepth is the search depth used for the alternatives of a move: a
// few plies less than the main search (it is repeated for every legal move).
func specialDepth(opt *Options) int {
	if opt.Depth > 0 {
		return min(9, max(5, opt.Depth-5))
	}
	return 7
}

// onlyMoveGap plays every legal move of pos, searches the result and
// returns the win probability (for the mover) of the played move and how far
// it is ahead of the best other move. If the shallow search prefers another
// move to the played one the gap is 0: the move is not the clear only move.
func onlyMoveGap(pos *engine.Position, hist []uint64, played engine.Move, opt *Options) (playedPct, gap float64) {
	depth := specialDepth(opt)
	moves := pos.LegalMoves()
	if len(moves) < 3 {
		return 0, 0
	}
	best, second := -1.0, -1.0
	var playedVal float64
	for _, m := range moves {
		child := pos.Copy()
		child.Play(m)
		h := append(hist[:len(hist):len(hist)], child.Hash())
		var pct float64
		if out := child.Outcome(h); out.Over {
			if out.Checkmate {
				pct = 100
			} else {
				pct = 50
			}
		} else {
			res, err := engine.Search(child, h, engine.Limits{Depth: depth, HashMB: 1})
			if err != nil {
				continue
			}
			pct = WinPercent(-res.Score, opt.WinK)
		}
		if m == played {
			playedVal = pct
		}
		if pct > best {
			best, second = pct, best
		} else if pct > second {
			second = pct
		}
	}
	if second < 0 || playedVal < best { // not the clear first choice
		return playedVal, 0
	}
	return playedVal, playedVal - second
}

// isMiss decides the Miss class for a move already judged class: an inaccuracy
// or a mistake played right after the opponent's mistake, in a position the
// mover was clearly winning.
func isMiss(class Class, prevLoss, moverBefore float64) bool {
	return (class == Inaccuracy || class == Mistake) && prevLoss >= MissMinPrevLoss && moverBefore >= MissMinBefore
}

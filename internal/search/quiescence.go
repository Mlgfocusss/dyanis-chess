package search

import (
	"dyanis-chess/internal/board"
	"dyanis-chess/internal/movegen"
)

// --- Quiescence search -------------------------------------------------

// maxQuiescenceCheckExtensions bounds how many plies of "must respond
// to check" quiescence will follow down a single line before it's
// willing to fall back to evaluating the position anyway. Without
// this cap, a long forced-check sequence (each side giving check,
// neither side capturing anything) could make quiescence recurse far
// deeper than intended, since "in check" moves aren't narrowed down
// to captures the way normal quiescence nodes are — see
// quiescenceSearch. Six plies is a fairly generous allowance for a
// real check sequence while still being a hard ceiling on the
// pathological case.
const maxQuiescenceCheckExtensions = 6

// quiescence is negamax's leaf-node evaluation, but instead of
// statically scoring whatever position depth 0 happens to land on, it
// keeps searching through "noisy" moves (captures, and check
// evasions) until the position settles down or the check-extension
// budget runs out. This is what fixes the horizon effect: plain
// negamax stopping exactly at depth 0 mid-capture-exchange would
// score "I just won a pawn" as good even when the very next move (one
// ply past the search horizon) recaptures a whole piece back.
func quiescence(b *board.Board, ply, alpha, beta int, tt *TranspositionTable) int {
	return quiescenceSearch(b, ply, alpha, beta, maxQuiescenceCheckExtensions, tt)
}

func quiescenceSearch(b *board.Board, ply, alpha, beta, checkExtLeft int, tt *TranspositionTable) int {
	if tt != nil {
		tt.nodes++
		if tt.shouldStop() {
			return 0
		}

		// Same repetition check as negamax's, and for the same reason
		// — see isRepetition's own comment. This matters here
		// specifically because a perpetual check plays out entirely
		// inside quiescence's own "in check" recursion (see below),
		// never touching negamax's move loop at all — without this,
		// exactly that pattern would be invisible to repetition
		// detection. Shares negamax's own tt.searchPath (same ply
		// numbering across both), so a cycle that starts in the main
		// search and finishes in quiescence, or vice versa, is caught
		// too.
		hash := b.Hash()
		if tt.isRepetition(ply, hash, b.HalfmoveClock) {
			return 0
		}
		tt.recordPathHash(ply, hash)

		// Transposition table, same bound logic as negamax's probe. Any
		// stored entry is at least as deep as quiescence (depth 0), so
		// there's no depth test; entries from quiescence itself carry
		// no move (see the store below) — only their score/bound.
		entry, found := tt.probe(hash)
		if found {
			s := readAdjustMateScore(entry.score, ply)
			switch entry.bound {
			case exactBound:
				return s
			case lowerBound:
				if s >= beta {
					return s
				}
			case upperBound:
				if s <= alpha {
					return s
				}
			}
		}

		origAlpha := alpha
		score := quiescenceBody(b, ply, alpha, beta, checkExtLeft, tt)
		// An aborted search's score is garbage, and a deeper entry for
		// this same position (from negamax) must not be overwritten by
		// this shallower, move-less one.
		if !tt.aborted() && !(found && entry.depth > 0) {
			resultBound := exactBound
			if score <= origAlpha {
				resultBound = upperBound
			} else if score >= beta {
				resultBound = lowerBound
			}
			tt.store(hash, ttEntry{depth: 0, score: storeAdjustMateScore(score, ply), bound: resultBound})
		}
		return score
	}
	return quiescenceBody(b, ply, alpha, beta, checkExtLeft, tt)
}

// quiescenceBody is quiescenceSearch's actual search, after the node
// bookkeeping (node count, stop check, repetition, TT) done there.
func quiescenceBody(b *board.Board, ply, alpha, beta, checkExtLeft int, tt *TranspositionTable) int {
	inCheck := movegen.InCheck(b)

	// In check: every legal reply matters (block, capture, king move,
	// and — critically — "none of the above" means checkmate), so
	// this path still needs the full legal-move list, same as before
	// this function was split. Checks are the minority of quiescence
	// nodes; this is not the path the GenerateLegalCaptures change
	// below is aimed at.
	if inCheck {
		legal := movegen.GenerateLegalMoves(b)
		if len(legal) == 0 {
			return ply - MateScore
		}
		if b.HalfmoveClock >= 100 || movegen.InsufficientMaterial(b) {
			return 0
		}

		extendForCheck := checkExtLeft > 0
		var candidates []board.Move
		if extendForCheck {
			candidates = legal
		} else {
			// Check-extension budget spent: degrade to the same
			// captures-only handling the not-in-check path below uses,
			// filtered from the legal list already in hand (no second
			// generation needed — legal was already the full list).
			standPat := tt.evaluate(b, ply)
			if standPat >= beta {
				return beta
			}
			if standPat > alpha {
				alpha = standPat
			}
			for _, m := range legal {
				if m.IsCapture() && !deltaPrune(b, m, standPat, alpha) && SEE(b, m) >= 0 {
					candidates = append(candidates, m)
				}
			}
			sortCandidatesBySEE(b, candidates)
		}

		nextCheckExtLeft := checkExtLeft
		if extendForCheck {
			nextCheckExtLeft--
		}
		return quiescenceLoop(b, ply, alpha, beta, nextCheckExtLeft, candidates, false, tt)
	}

	// Not in check — the overwhelming majority of quiescence nodes.
	// GenerateLegalCaptures (movegen.go) generates only captures
	// straight from bitboards, skipping the quiet-move generation
	// GenerateLegalMoves would otherwise do for no benefit here (the
	// standPat/SEE>=0 filtering immediately below only ever keeps
	// captures anyway). This used to be a measured hot spot —
	// quiescence nodes vastly outnumber ordinary search nodes, so
	// paying full move-generation cost on every one of them was a
	// large fraction of total search time (see TECHNICAL.md).
	//
	// Trade-off, deliberate: this does NOT distinguish "no captures,
	// but other legal moves exist" from "actually stalemate, zero
	// legal moves at all" the way the len(legal)==0 check above does
	// for the in-check path — a stalemate landing exactly on a
	// not-in-check quiescence node (already a rare event this deep in
	// a search) falls through to standPat below instead of the
	// correct 0. negamax's own GameStatusFromMoves check at every real
	// search node remains the authoritative, fully-correct terminal
	// detection; this only affects the leaf value quiescence falls
	// back to on the rare position that happens to be stalemate.
	if b.HalfmoveClock >= 100 || movegen.InsufficientMaterial(b) {
		return 0
	}

	standPat := tt.evaluate(b, ply)
	if standPat >= beta {
		return beta
	}
	if standPat > alpha {
		alpha = standPat
	}

	// Stack buffers (a position rarely has more than a few captures; more
	// just spills to the heap). SEE is evaluated once per capture and the
	// value reused for the sort.
	var capBuf, candBuf [32]board.Move
	var seeBuf [32]int
	candidates, sees := candBuf[:0], seeBuf[:0]
	for _, m := range movegen.AppendPseudoLegalCaptures(b, capBuf[:0]) {
		if deltaPrune(b, m, standPat, alpha) {
			continue
		}
		if s := SEE(b, m); s >= 0 {
			candidates = append(candidates, m)
			sees = append(sees, s)
		}
	}
	insertionSortByScoreDesc(candidates, sees)

	return quiescenceLoop(b, ply, alpha, beta, checkExtLeft, candidates, true, tt)
}

// sortCandidatesBySEE sorts capture candidates by their real SEE
// value, descending — not MVV-LVA: MVV-LVA is a cheap PROXY for
// "which capture is probably good" (biggest victim, cheapest
// attacker) that breaks down on defended pieces — pawn-takes-
// defended-queen ranks above knight-takes-hanging-rook under MVV-LVA
// even though the queen capture nets nothing once the recapture is
// accounted for.
//
// SEE is computed exactly once per candidate, into seeValues, rather
// than inside the sort comparator: sort.Slice's Less gets called
// roughly O(n log n) times, and SEE isn't a cheap lookup — it
// simulates the full capture exchange (leastValuableAttacker scans
// the target square repeatedly as pieces get removed). Computing it
// once per candidate up front and sorting the cached values via
// insertionSortByScoreDesc (see insertionSortByScoreDesc's comment for why that's
// preferred over sort.Sort/sort.Slice at this list size) turns that
// back into O(n) SEE calls.
func sortCandidatesBySEE(b *board.Board, candidates []board.Move) {
	if len(candidates) < 2 {
		return
	}
	seeValues := make([]int, len(candidates))
	for i, m := range candidates {
		seeValues[i] = SEE(b, m)
	}
	insertionSortByScoreDesc(candidates, seeValues)
}

// quiescenceLoop is quiescenceSearch's shared move-trying loop —
// standard negamax-style alpha-beta over whatever candidate list the
// caller (in-check or not-in-check path above) already built.
// checkLegal is true when candidates are pseudo-legal (not-in-check path): each move is
// made anyway, so a move that leaves our king attacked is rejected right after MakeMove.
func quiescenceLoop(b *board.Board, ply, alpha, beta, nextCheckExtLeft int, candidates []board.Move, checkLegal bool, tt *TranspositionTable) int {
	us := b.SideToMove
	for _, m := range candidates {
		undo := b.MakeMove(m)
		if checkLegal && movegen.IsSquareAttacked(b, b.KingSquare(us), b.SideToMove) {
			b.UnmakeMove(m, undo)
			continue
		}
		tt.nnuePush(ply+1, m, undo, b)
		score := -quiescenceSearch(b, ply+1, -beta, -alpha, nextCheckExtLeft, tt)
		b.UnmakeMove(m, undo)
		if tt.aborted() {
			return 0
		}
		if score >= beta {
			return beta
		}
		if score > alpha {
			alpha = score
		}
	}
	return alpha
}

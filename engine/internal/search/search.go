// Package search implements move search: negamax with alpha-beta
// pruning (step 3), a transposition table keyed by board.Board.Hash()
// for both pruning and move ordering (TT move / MVV-LVA / killers /
// history — see the Move ordering section in search.go), quiescence
// search at leaf nodes to avoid the horizon effect, null-move pruning
// to skip subtrees that are fine even if the side to move does
// nothing, and iterative deepening with a time budget (step 6)
// layered on top of the same negamax. An opening book lookup runs in
// front of all of this — see opening_book.go.
//
// Step 10 adds four more tricks, three of them (LMR, PVS, aspiration
// windows) aimed at proving the SAME answer with a narrower alpha-beta
// window — never at changing which move is best — and the fourth (SEE)
// aimed at making capture ordering and quiescence pruning more accurate
// than MVV-LVA alone can be:
//   - Late Move Reductions (LMR): late, quiet, non-critical moves get
//     a cheap reduced-depth probe first, and only earn a full-depth
//     re-search if that probe suggests they might actually matter.
//   - Principal Variation Search (PVS/NegaScout): every move after
//     the first (expected, ordering-wise, to already be the best one)
//     gets a cheap null-window "is this even better than what we
//     have" probe before any move earns a full-window search.
//   - Aspiration windows: each iterative-deepening pass after the
//     first starts with a narrow window centered on the previous
//     pass's score instead of (-Infinity, Infinity), re-searching
//     with the full window only on the rare pass where the score
//     actually moved outside it.
//   - Static Exchange Evaluation (SEE, see.go): simulates a full
//     capture sequence on a square to estimate its true material
//     outcome, unlike MVV-LVA's "biggest victim, cheapest attacker"
//     heuristic which can't tell a winning capture from one that just
//     loses a piece to a defended square. Used by quiescenceSearch
//     below to skip clearly-losing captures and to order the rest by
//     their real expected value instead of the MVV-LVA proxy.
package search

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"dyanis-chess/engine/internal/board"
	"dyanis-chess/engine/internal/eval"
	"dyanis-chess/engine/internal/movegen"
)

// Infinity is a value larger than any realistic evaluation score,
// used as the initial alpha-beta window bound.
const Infinity = 1_000_000

// MateScore is returned for a checkmated position — deliberately far
// larger than any material score so mate is always preferred over
// material gain, but still comfortably below Infinity so alpha-beta
// bounds don't collide with it.
//
// A raw checkmate leaf returns ply-MateScore, not a flat -MateScore
// (see negamax) — this is mate-distance adjustment: a mate found
// closer to the root (smaller ply) scores CLOSER to ±MateScore than
// one found deeper, so alpha-beta correctly prefers a faster mate
// over a slower one, and correctly prefers delaying an unavoidable
// mate as long as possible when on the losing side. Every non-leaf
// node just negates its child's score unchanged, so this ply-relative
// value propagates all the way to the root intact.
const MateScore = 900_000

// mateThreshold is the boundary used to recognize "this score
// represents some mate distance, not a normal material/positional
// eval" — anything at or beyond it is treated as a mate score by
// storeAdjustMateScore/readAdjustMateScore below. Comfortably below
// MateScore itself: even an absurdly deep search (thousands of plies)
// couldn't push a genuine mate-in-N score below this line, while no
// plausible eval.Evaluate output gets anywhere close to it.
const mateThreshold = MateScore - 1000

// --- Search -----------------------------------------------------------

// SearchInfo summarizes one completed pass of search — either the
// single call BestMove/BestMoveInfo make, or one depth of
// BestMoveTimed/BestMoveTimedInfo's iterative deepening — for callers
// that want to report progress as it happens rather than only ever
// seeing the final move. Its one real consumer today is the UCI
// package's "info depth ... score ... nodes ... pv ..." line (see
// internal/uci/uci.go), but nothing here is UCI-specific; it's plain
// search-progress data.
type SearchInfo struct {
	Depth int
	Score int
	// PV is the principal variation found at this depth: the best
	// move at the root, then the best reply to that, and so on — see
	// principalVariation below for how it's reconstructed.
	PV []board.Move
	// Nodes is the running total of negamax/quiescence calls made so
	// far in this whole search (see TranspositionTable.nodes) — not
	// reset between depths within one BestMoveTimedInfo call.
	Nodes int64
}

// ScoreToUCI renders a raw negamax score (root-relative centipawns, or
// a mate score per MateScore's comment) the way UCI expects it: "cp N"
// for an ordinary evaluation, or "mate N" once the score represents a
// forced mate found somewhere in the tree — N moves until mate is
// delivered (not plies: UCI counts full moves, matching how a human
// would say "mate in 2"), positive if the side to move at the root
// delivers it, negative if the root's side is the one getting mated.
func ScoreToUCI(score int) string {
	if moves, isMate := mateDistanceInMoves(score); isMate {
		return fmt.Sprintf("mate %d", moves)
	}
	return fmt.Sprintf("cp %d", score)
}

// mateDistanceInMoves inverts the ply-MateScore construction described
// in MateScore's own comment: a score of magnitude >= mateThreshold
// encodes "mate found at this many plies from the root" as
// MateScore-ply, so recovering ply back out of the score and rounding
// half-moves up to full moves (ply 1 and 2 are both "mate in 1": White
// mates on ply 1, or is mated by Black's ply-2 reply to a move that
// already lost) gives the number UCI wants.
func mateDistanceInMoves(score int) (moves int, isMate bool) {
	abs := score
	if abs < 0 {
		abs = -abs
	}
	if abs < mateThreshold {
		return 0, false
	}
	plyAtMate := MateScore - abs
	moves = (plyAtMate + 1) / 2
	if score < 0 {
		moves = -moves
	}
	return moves, true
}

// principalVariation reconstructs the best line found from position b
// by repeatedly looking up the best move the transposition table
// recorded for the current position, applying it, and looking up the
// resulting position in turn — the same tt the search itself just
// populated, so this comes essentially for free instead of needing a
// dedicated triangular PV table threaded through negamax's recursion.
//
// Stops early, rather than risk showing a wrong or nonsensical line,
// if: there's no TT entry for the current position (most commonly
// because the line has run past negamax's own depth into territory
// only quiescence looked at, which doesn't write to the TT), the
// recorded move isn't actually legal there (a defensive check against
// the vanishingly rare 64-bit Zobrist hash collision — see Hash's
// comment in zobrist.go — silently walking a corrupted PV would be a
// worse outcome than just truncating it early), the same position
// would be visited a second time (a defensive guard against an actual
// cycle, which a correct search shouldn't produce but a bug or hash
// collision might), or maxLen moves have already been collected.
func principalVariation(b *board.Board, tt *TranspositionTable, maxLen int) []board.Move {
	if tt == nil {
		return nil
	}

	var pv []board.Move
	// b itself must not be mutated — it's the caller's actual search
	// root (or, one level up, the receiver negamax was handed) — so
	// this walk works on a disposable copy instead. Nothing here needs
	// to unmake anything: the copy is discarded once the loop ends.
	cur := b.Copy()
	seen := map[uint64]bool{}
	for len(pv) < maxLen {
		hash := cur.Hash()
		if seen[hash] {
			break
		}
		seen[hash] = true

		entry, ok := tt.probe(hash)
		if !ok || !isLegalMove(cur, entry.move) {
			break
		}
		pv = append(pv, entry.move)
		cur.MakeMove(entry.move)
	}
	return pv
}

func isLegalMove(b *board.Board, m board.Move) bool {
	for _, legal := range movegen.GenerateLegalMoves(b) {
		if legal == m {
			return true
		}
	}
	return false
}

// BestMove searches to a fixed depth (in plies) using negamax with
// alpha-beta pruning and a fresh transposition table, and returns the
// best move found for the side to move. depth 1 already means "look at
// every legal move and check whether it immediately delivers
// checkmate" (the terminal check happens before the depth==0 cutoff),
// so a mate-in-1 is found even at depth 1.
func BestMove(b *board.Board, depth int) (board.Move, error) {
	return BestMoveInfo(b, depth, nil)
}

// BestMoveInfo is BestMove with an added onInfo callback, invoked
// exactly once after the (single, fixed-depth) search completes —
// onInfo may be nil, in which case this behaves exactly like BestMove.
func BestMoveInfo(b *board.Board, depth int, onInfo func(SearchInfo)) (board.Move, error) {
	return BestMoveInfoTT(b, depth, NewTranspositionTable(), onInfo)
}

// BestMoveInfoTT is BestMoveInfo but searches using the given tt
// instead of constructing a fresh one — see
// TranspositionTable.NewSearch's own comment for why a caller would
// want this (persisting a hash table across several separate
// BestMove*-style calls, e.g. across a whole game's worth of moves).
// tt must not be nil.
func BestMoveInfoTT(b *board.Board, depth int, tt *TranspositionTable, onInfo func(SearchInfo)) (board.Move, error) {
	if len(movegen.GenerateLegalMoves(b)) == 0 {
		return board.Move{}, errors.New("search.BestMove: no legal moves in this position")
	}
	tt.nnueReset(b)
	score, bestMove := negamax(b, depth, 0, -Infinity, Infinity, true, tt, maxSearchCheckExtensions)
	if onInfo != nil {
		onInfo(SearchInfo{Depth: depth, Score: score, PV: principalVariation(b, tt, depth), Nodes: tt.nodes})
	}
	return bestMove, nil
}

// aspirationDelta is the initial half-width of the window placed
// around the previous iteration's score, in centipawns. Narrow enough
// that most iterations actually benefit (fewer nodes explored before
// either bound proves out), wide enough that only a real swing in
// evaluation — a genuine tactic appearing at the new depth — triggers
// the full-window re-search below.
var aspirationDelta = 41

// aspirationMinDepth is the shallowest depth aspiration windows are
// attempted at. Depth 1 has no previous score to center a window on,
// and very shallow depths swing enough between iterations that a
// narrow window would just cause repeated fail-high/fail-low
// re-searches instead of saving any work.
const aspirationMinDepth = 4

// BestMoveTimed runs iterative deepening: it searches depth 1, then 2,
// then 3, and so on (sharing one transposition table across all of
// them, so each deeper pass benefits from move ordering learned by the
// previous one), stopping and returning the last FULLY completed
// depth's move once budget has elapsed or maxDepth is reached.
//
// From aspirationMinDepth onward, each pass first tries a narrow
// window centered on the PREVIOUS pass's score (see aspirationDelta)
// instead of the full (-Infinity, Infinity) — most of the time the
// score doesn't move much between one depth and the next, so the
// narrow window still contains the true score and proves it with
// fewer nodes. A score landing exactly on either edge of that window
// is only a bound, not the real value (alpha-beta never proves an
// exact score right at a window boundary), so that case re-searches
// the SAME depth with the full window before moving on — correctness
// never depends on the narrow window succeeding, only speed does.
//
// The deadline is checked between depths AND inside the tree (every
// 2048 nodes — see TranspositionTable.shouldStop): an iteration still
// running when the budget expires is abandoned and the last completed
// depth's move is returned. Depth 1 always runs to completion
// regardless of budget, so this never returns a zero-value move even if
// budget is very small (or zero).
func BestMoveTimed(b *board.Board, maxDepth int, budget time.Duration) (board.Move, error) {
	return BestMoveTimedInfo(b, maxDepth, budget, nil)
}

// runIterativeDeepening runs the depth-by-depth, aspiration-windowed
// loop described on BestMoveTimedInfo above, against a
// caller-constructed tt and a caller-computed deadline, rather than
// building its own table or measuring its own budget. Factored out
// of BestMoveTimedInfo specifically so BestMoveTimedMT (Lazy SMP —
// plan.md item 3) can run this SAME loop concurrently across several
// threads: each thread passes its own TranspositionTable (its own
// killers/history/searchPath/pawnCache/nodes — see those fields' own
// comments on TranspositionTable) but, in the multi-threaded case,
// every thread's tt.shared points at the same underlying hash table.
// This function itself has no idea whether it's the only caller of
// tt.shared or one of several running at once — that distinction is
// entirely the caller's concern.
//
// The deadline check uses !time.Now().Before(deadline) rather than
// .After(deadline) specifically to match elapsed >= budget (not
// elapsed > budget): BestMoveTimedInfo's very first version measured
// time.Since(start) >= budget, and this preserves that exactly rather
// than introducing an off-by-an-instant behavior change.
func runIterativeDeepening(b *board.Board, maxDepth int, deadline time.Time, tt *TranspositionTable, onInfo func(SearchInfo)) (board.Move, SearchInfo) {
	var bestMove board.Move
	var lastInfo SearchInfo
	prevScore := 0
	// Mid-search cutoff (see TranspositionTable.shouldStop): armed only
	// from depth 2 on, so depth 1 always completes and there is always a
	// move to return. An iteration that gets cut off is discarded whole —
	// its partial result is never used — and the last COMPLETED depth's
	// move stands. tt is left disarmed on exit, since callers may reuse it.
	tt.stopped = false
	defer func() { tt.timed, tt.stopped = false, false }()
	tt.nnueReset(b)
	for depth := 1; depth <= maxDepth; depth++ {
		if depth > 1 && !time.Now().Before(deadline) {
			break
		}
		if depth == 2 {
			tt.deadline, tt.timed = deadline, true
		}

		alpha, beta := -Infinity, Infinity
		delta := aspirationDelta
		if depth >= aspirationMinDepth {
			alpha, beta = prevScore-delta, prevScore+delta
		}

		var score int
		var m board.Move
		for {
			tt.rootPartialOK = false
			score, m = negamax(b, depth, 0, alpha, beta, true, tt, maxSearchCheckExtensions)
			if tt.stopped {
				break
			}
			if score > alpha && score < beta {
				break
			}
			if alpha <= -Infinity && beta >= Infinity {
				break
			}
			// Fail-low or fail-high: the narrow window didn't contain the
			// true score, so the move/score above can't be trusted as-is.
			if !swAspWiden {
				alpha, beta = -Infinity, Infinity
				continue
			}
			delta += delta / 2
			if score <= alpha {
				beta = (alpha + beta) / 2
				alpha = score - delta
			} else {
				beta = score + delta
			}
			if delta > 600 || alpha < -2000 || beta > 2000 {
				alpha, beta = -Infinity, Infinity
			}
		}
		if tt.stopped {
			if swPartial && depth > 1 && tt.rootPartialOK {
				bestMove = tt.rootPartial
			}
			break
		}

		bestMove = m
		prevScore = score
		lastInfo = SearchInfo{Depth: depth, Score: score, PV: principalVariation(b, tt, depth), Nodes: tt.nodes}
		if onInfo != nil {
			onInfo(lastInfo)
		}
		// Our own mate is found and the search is deep enough to trust it
		// (twice its length, margin for pruning that skips some lines):
		// going deeper only burns time. Only for OUR mate — when we are
		// being mated, a deeper search can still find a defence.
		if score >= mateThreshold && depth >= 2*(MateScore-score) {
			break
		}
	}
	return bestMove, lastInfo
}

// BestMoveTimedInfo is BestMoveTimed with an added onInfo callback,
// invoked once after EVERY completed depth (not just the last one) —
// this is what lets a caller like the UCI loop print a progressively
// improving "info depth N score ... pv ..." line as the search goes,
// rather than only learning the outcome once iterative deepening stops
// altogether. onInfo may be nil, in which case this behaves exactly
// like BestMoveTimed.
func BestMoveTimedInfo(b *board.Board, maxDepth int, budget time.Duration, onInfo func(SearchInfo)) (board.Move, error) {
	return BestMoveTimedInfoTT(b, maxDepth, budget, NewTranspositionTable(), onInfo)
}

// BestMoveTimedInfoTT is BestMoveTimedInfo but searches using the
// given tt instead of constructing a fresh one — see
// BestMoveInfoTT's own comment for why a caller would want this. tt
// must not be nil.
func BestMoveTimedInfoTT(b *board.Board, maxDepth int, budget time.Duration, tt *TranspositionTable, onInfo func(SearchInfo)) (board.Move, error) {
	if len(movegen.GenerateLegalMoves(b)) == 0 {
		return board.Move{}, errors.New("search.BestMoveTimed: no legal moves in this position")
	}

	deadline := time.Now().Add(budget)
	bestMove, _ := runIterativeDeepening(b, maxDepth, deadline, tt, onInfo)
	return bestMove, nil
}

// mtResult is one Lazy SMP thread's outcome — see BestMoveTimedMT.
type mtResult struct {
	move board.Move
	info SearchInfo
}

// BestMoveTimedMT runs Lazy SMP (plan.md item 3): `threads` goroutines
// each run runIterativeDeepening independently and concurrently, on
// their OWN copy of the position (b.Copy() — negamax mutates the
// board in place via MakeMove/UnmakeMove, so sharing one *board.Board
// across goroutines would corrupt it, not just race on it) and their
// own TranspositionTable (so each has its own
// killers/history/searchPath/pawnCache/nodes, exactly as those
// fields' own comments already call for), but every thread's
// TranspositionTable points at ONE shared *sharedTT — so one thread's
// work can still help another's through shared TT entries, which is
// the entire point of Lazy SMP over just running independent searches.
//
// threads <= 1 falls back to plain BestMoveTimedInfo exactly — no
// shared table, no goroutines, byte-for-byte the same code path as
// before this function existed, so nothing single-threaded (including
// every existing test) is affected by BestMoveTimedMT merely being
// present in the package.
//
// Picking a result among threads that finished at different depths or
// with different answers: the thread that reached the GREATEST
// COMPLETED depth wins outright — more search is essentially always
// more trustworthy, regardless of which thread produced it. A tie
// (more than one thread completing at the same max depth — a real
// possibility, since threads only share TT entries, not identical
// search order, so nothing guarantees they progress in lockstep) is
// broken by the higher reported score — both threads searched the
// SAME root position with the SAME side to move, so comparing scores
// directly is meaningful, not an apples-to-oranges comparison. Still
// tied after that (identical depth AND score): the lowest thread
// index wins, purely so the result is deterministic rather than
// depending on goroutine scheduling.
//
// SearchInfo.Nodes on the returned info is the SUM of every thread's
// node count, not just the winning thread's — matching what UCI GUIs
// and "nodes per second" displays expect from a multi-threaded
// search: the total work actually done, not one thread's slice of it.
// onInfo, if non-nil, is called exactly ONCE, with that combined
// info, after all threads finish — not once per depth per thread,
// which would both race on onInfo (called concurrently from several
// goroutines) and interleave unrelated threads' depths in whatever
// order they happened to finish. Progressive, depth-by-depth
// reporting for a multi-threaded search is left to plan.md item D,
// when this gets wired into UCI.
func BestMoveTimedMT(b *board.Board, maxDepth int, budget time.Duration, threads int, onInfo func(SearchInfo)) (board.Move, error) {
	if threads <= 1 {
		return BestMoveTimedInfo(b, maxDepth, budget, onInfo)
	}
	if len(movegen.GenerateLegalMoves(b)) == 0 {
		return board.Move{}, errors.New("search.BestMoveTimedMT: no legal moves in this position")
	}
	return bestMoveTimedMT(b, maxDepth, budget, threads, newSharedTT(ttSize), nil, onInfo)
}

// BestMoveTimedMTShared is BestMoveTimedMT, but every thread shares
// persistentTT's underlying hash table instead of a table built fresh
// for just this one call — the Lazy SMP counterpart to
// TranspositionTable.NewSearch/BestMoveInfoTT/BestMoveTimedInfoTT:
// pass the SAME *TranspositionTable across repeated calls (e.g. one
// call per move across a whole UCI game) to keep that hash table warm
// across MOVES, layered on top of the sharing threads already do with
// each other WITHIN one call. persistentTT must not be nil.
//
// threads<=1 still gets real benefit here, unlike BestMoveTimedMT's
// own threads<=1 fallback (a fully independent, non-shared search):
// it runs single-threaded via BestMoveTimedInfoTT against
// persistentTT.NewSearch(), so even a single-threaded caller gets
// cross-move persistence through this entry point.
func BestMoveTimedMTShared(b *board.Board, maxDepth int, budget time.Duration, threads int, persistentTT *TranspositionTable, onInfo func(SearchInfo)) (board.Move, error) {
	if threads <= 1 {
		return BestMoveTimedInfoTT(b, maxDepth, budget, persistentTT.NewSearch(), onInfo)
	}
	if len(movegen.GenerateLegalMoves(b)) == 0 {
		return board.Move{}, errors.New("search.BestMoveTimedMTShared: no legal moves in this position")
	}
	return bestMoveTimedMT(b, maxDepth, budget, threads, persistentTT.shared, persistentTT.gameHistory, onInfo)
}

// bestMoveTimedMT is BestMoveTimedMT/BestMoveTimedMTShared's shared
// implementation once threads>1 and a legal-move check has already
// passed — the only thing that differs between "fresh, single-call"
// Lazy SMP and "shared across moves" Lazy SMP is which *sharedTT every
// thread's own TranspositionTable points at, so that's the one thing
// this takes as a parameter rather than constructing itself (plus the
// game history every thread needs for repetitions — see SetGameHistory).
func bestMoveTimedMT(b *board.Board, maxDepth int, budget time.Duration, threads int, shared *sharedTT, gameHistory []uint64, onInfo func(SearchInfo)) (board.Move, error) {
	deadline := time.Now().Add(budget)
	shared.nextGeneration()

	results := make([]mtResult, threads)
	var wg sync.WaitGroup
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func(thread int) {
			defer wg.Done()
			threadBoard := b.Copy()
			tt := &TranspositionTable{
				shared:      shared,
				hist:        newHistTables(),
				pawnCache:   eval.NewPawnCache(),
				gameHistory: gameHistory,
			}
			move, info := runIterativeDeepening(threadBoard, maxDepth, deadline, tt, nil)
			results[thread] = mtResult{move: move, info: info}
		}(i)
	}
	wg.Wait()

	best := results[0]
	totalNodes := results[0].info.Nodes
	for i := 1; i < threads; i++ {
		r := results[i]
		totalNodes += r.info.Nodes
		if r.info.Depth > best.info.Depth ||
			(r.info.Depth == best.info.Depth && r.info.Score > best.info.Score) {
			best = r
		}
	}
	best.info.Nodes = totalNodes

	if onInfo != nil {
		onInfo(best.info)
	}
	return best.move, nil
}

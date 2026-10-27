package search

import (
	"math"

	"dyanis-chess/engine/internal/board"
	"dyanis-chess/engine/internal/movegen"
)

// nullMoveReduction (R) is how many EXTRA plies, beyond the mandatory
// one, the null-move probe skips compared to a real move — i.e. it
// searches to depth-1-nullMoveReduction instead of depth-1. 2 is the
// conventional starting value: aggressive enough to matter, modest
// enough to stay safe without a verification search on top.
const nullMoveReduction = 2

// nullMoveMinDepth is the shallowest depth NMP bothers attempting at
// — must be at least nullMoveReduction+1 so depth-1-nullMoveReduction
// never goes negative.
const nullMoveMinDepth = 3

// lmrMinDepth is the shallowest remaining depth Late Move Reductions
// bothers attempting at — below this there's barely anything left to
// shave off, so the extra branching (probe, maybe re-search) isn't
// worth it. Kept equal to nullMoveMinDepth, with headroom to raise
// lmrReduction later without redoing this arithmetic.
const lmrMinDepth = 3

// lmrFullDepthMoves is how many moves at the front of the (already
// TT/MVV-LVA/killer/history-ordered) move list are always searched at
// full depth before LMR starts reducing — move ordering has already
// spent its best guesses on exactly these moves, so they're the ones
// least worth gambling a reduced search on.
const lmrFullDepthMoves = 4

// lmrFullMovesFor is lmrFullDepthMoves, or with SwLmrFull separate counts for
// PV and non-PV nodes.
func lmrFullMovesFor(pv bool) int {
	if !swLmrFull {
		return lmrFullDepthMoves
	}
	if pv {
		return lmrFullPV
	}
	return lmrFullNonPV
}

// lmrReduction is how many extra plies, beyond the normal depth-1, a
// reduced move's initial probe skips. 1 is the conventional
// conservative starting value — aggressive enough to matter across a
// whole tree of late quiet moves, modest enough that a genuinely good
// move reliably still beats alpha at the reduced depth and triggers
// the full-depth re-search below rather than getting missed outright.
const lmrReduction = 1

// lmrTable[depth][moveIndex] is the logarithmic LMR reduction: later
// moves at higher remaining depth are reduced more. Replaces the flat
// lmrReduction for the first reduced probe (see lmrReductionFor).
var lmrTable [64][64]int

func init() { initLMR() }

// initLMR fills lmrTable from lmrBaseX100/lmrDivX100 (see tunables.go).
func initLMR() {
	for d := 1; d < 64; d++ {
		for i := 1; i < 64; i++ {
			lmrTable[d][i] = int(float64(lmrBaseX100)/100 + math.Log(float64(d))*math.Log(float64(i))/(float64(lmrDivX100)/100))
		}
	}
}

// lmrReductionFor returns the reduction, in plies beyond the normal
// depth-1, for the move at index i in a node with `depth` remaining.
// At least 1 (the old flat value) and never so large that the probe
// drops below depth 1.
func lmrReductionFor(depth, i int) int {
	if depth > 63 {
		depth = 63
	}
	if i > 63 {
		i = 63
	}
	r := lmrTable[depth][i]
	if r < lmrReduction {
		r = lmrReduction
	}
	if r > depth-2 {
		r = depth - 2
	}
	if r < 1 {
		r = 1
	}
	return r
}

// futilityMaxDepth is the deepest remaining depth futility pruning
// bothers attempting at — beyond this, too many plies remain for a
// static snapshot plus a fixed margin to say anything reliable about
// whether a quiet move could still matter; only very close to the
// leaves does "even a generous swing can't reach alpha" become a
// trustworthy signal.
const futilityMaxDepth = 3

// futilityMargin[depth] is how many centipawns of swing a quiet move
// is given the benefit of the doubt for, at that many plies of
// remaining depth, before being judged futile (index 0 unused: depth
// 0 hands off to quiescence first). Same centipawn scale as eval.go's
// piece values (a pawn is 100); deeper remaining depth gets a wider
// margin.
var futilityMargin = [futilityMaxDepth + 1]int{0, 135, 245, 571}

// razorMaxDepth is the deepest remaining depth razoring bothers
// attempting at — same reasoning as futilityMaxDepth's own comment:
// only very close to the leaves does a static snapshot plus a fixed
// margin say anything reliable, here about the WHOLE node rather than
// a single move.
const razorMaxDepth = 3

// razorMargin[depth] is how many centipawns below alpha the static
// eval is allowed to sit, at that many plies of remaining depth,
// before the node is judged not worth a full search at all (index 0
// unused, same convention as futilityMargin — depth 0 already hands
// off to quiescence before this is ever consulted). Wider than
// futilityMargin at every depth: razoring is judging whether ANY move
// at this node can make up the gap over a full remaining subtree,
// not just whether one specific quiet move can, so it only fires on a
// much larger shortfall.
var razorMargin = [razorMaxDepth + 1]int{0, 235, 602, 943}

// lmpMaxDepth is the deepest remaining depth Late Move Pruning
// bothers attempting at — same "too many plies remain for a cheap
// shortcut to be trustworthy" reasoning as futilityMaxDepth/
// razorMaxDepth's own comments.
const lmpMaxDepth = 3

// lmpMoveCountThreshold[depth] is how many moves are tried in full
// before LMP starts skipping the rest of the (quiet, non-check)
// candidates outright, at that many plies of remaining depth (index 0
// unused, same convention as futilityMargin/razorMargin — depth 0
// already hands off to quiescence before this is ever consulted).
// Move ordering has already spent its best guesses on everything
// tried before this count is reached (TT move, captures, killers,
// history-ranked quiet moves — see the Move ordering section below):
// a quiet move THIS far down an already-good ordering, with this
// little depth left to prove itself, essentially never turns out to
// matter. Grows with depth, same shape as futilityMargin growing with
// depth: one more ply of remaining search can still make a slightly
// later move worth checking.
var lmpMoveCountThreshold = [lmpMaxDepth + 1]int{0, 5, 11, 16}

// iidMinDepth is the shallowest remaining depth Internal Iterative
// Deepening bothers attempting at — below this, a reduced-depth
// search just to guess an ordering move costs more than the ordering
// benefit it could realistically buy back. Deliberately on the
// opposite end of the depth spectrum from futility/razoring/LMP's own
// *MaxDepth constants: those all stop mattering once too much depth
// remains for a cheap shortcut to be trustworthy, while IID only
// starts being worth its own cost once there's enough depth left for
// a better-ordered real search to actually pay that cost back.
const iidMinDepth = 5

// iidReduction is how many plies below the current depth IID's own
// probe search goes — deep enough (depth-iidReduction is still at
// least iidMinDepth-iidReduction = 3 plies, given the guard above) to
// produce a genuinely useful best move, shallow enough to stay
// meaningfully cheaper than the real search about to happen at this
// exact node anyway.
const iidReduction = 2

// seePruneMaxDepth/seePruneMargin: see the SEE pruning comment in the
// move loop. Starting values, tuned only by matches.
const seePruneMaxDepth = 6

// iirMinDepth: with the SwIIR switch, a node without a TT move searches one
// ply shallower instead of running an IID probe.
const iirMinDepth = 4

const histPruneMaxDepth = 4

// pruneMaxDepth is how deep futility and LMP reach: the old 3, or with the
// SwDeepPrune switch extendedPruneDepth, extrapolating the margins linearly.
const extendedPruneDepth = 6

func pruneDepthCap() int {
	if swDeepPrune {
		return extendedPruneDepth
	}
	return futilityMaxDepth
}

// seeQuietMaxDepth is how deep the quiet-move SEE pruning reaches.
const seeQuietMaxDepth = 6

// futDepth is the depth futility pruning takes its margin for: the node's own,
// or with SwFutLmr the move's reduced depth (at least 1).
func futDepth(depth, lmrD int) int {
	if swFutLmr {
		return max(min(lmrD, depth), 1)
	}
	return depth
}

func futilityMarginAt(depth int) int {
	if depth <= futilityMaxDepth {
		return futilityMargin[depth]
	}
	return futilityMargin[futilityMaxDepth] + (depth-futilityMaxDepth)*futExtStep
}

func lmpThresholdAt(depth int) int {
	if depth <= lmpMaxDepth {
		return lmpMoveCountThreshold[depth]
	}
	return lmpMoveCountThreshold[lmpMaxDepth] + (depth-lmpMaxDepth)*lmpExtStep
}

var seePruneMargin = 94

// rfpMaxDepth/rfpMargin: reverse futility pruning applies at remaining
// depth <= rfpMaxDepth, cutting when staticEval - rfpMargin*depth >=
// beta. Starting values, tuned only by matches (cmd/match).
const rfpMaxDepth = 6

var rfpMargin = 95

// hasNonPawnMaterial reports whether color c has any piece besides
// pawns and the king — the standard null-move pruning guard against
// zugzwang. In king-and-pawn endgames "doing nothing" is often
// exactly what the side to move would want but structurally can't (a
// real move must be played, and every one may worsen the position),
// so a null-move probe's "even doing nothing is fine" conclusion
// isn't trustworthy there.
func hasNonPawnMaterial(b *board.Board, c board.Color) bool {
	return b.Pieces(board.Knight, c)|b.Pieces(board.Bishop, c)|b.Pieces(board.Rook, c)|b.Pieces(board.Queen, c) != 0
}

// negamax evaluates a position from the perspective of the side to
// move, exploring `depth` more plies, and returns both the score and
// the best move found at this node (callers above the leaves use the
// move; leaves return the zero value, which is never read since a
// leaf's caller only wants its score).
//
// Alpha-beta pruning: once we find a move so good the opponent would
// never let us reach this position (score >= beta), we stop looking at
// this node's remaining moves — the opponent has a better alternative
// earlier in the tree that avoids this position entirely, so its exact
// value doesn't matter.
//
// tt may be nil, in which case this behaves as plain negamax with no
// caching or move-ordering help — kept possible mainly so the table
// isn't a hidden requirement threaded through every call site, though
// in practice every current caller passes one.
//
// isRoot must be true only for the top-level call (from BestMove/
// BestMoveTimed) and false for every recursive call — see the NMP
// guard below for why this matters.
// ply is how many plies deep from the search ROOT this call is (0 at
// the root, incremented by 1 on every recursive call — including the
// null-move probe). It exists purely for mate-distance adjustment —
// see MateScore's comment and storeAdjustMateScore/
// readAdjustMateScore above — and plays no other role in the search.
// maxSearchCheckExtensions bounds how many plies of "search one ply
// deeper because this move gives check" can stack up along any single
// line — the main-search counterpart to quiescence's own
// maxQuiescenceCheckExtensions (see its comment below), same
// reasoning: a position with a long forcing sequence of checks (a
// king hunt, a perpetual-check line) could otherwise keep extending
// indefinitely, since each check along the way looks individually
// worth resolving fully rather than cutting off at the horizon. A
// budget threaded through the recursion, decremented only when an
// extension actually fires, caps the total worst case without
// disabling the technique on ordinary, short forcing sequences (a
// two- or three-move mating net, a check that wins material) — which
// is exactly the case check extensions exist for: negamax otherwise
// has no way to tell "the search happened to stop right before this
// forced sequence resolved" apart from "this position is genuinely
// fine," and can misjudge a position specifically because the horizon
// landed mid-check.
const maxSearchCheckExtensions = 6

func negamax(b *board.Board, depth, ply, alpha, beta int, isRoot bool, tt *TranspositionTable, checkExtLeft int) (score int, bestMove board.Move) {
	if tt != nil {
		tt.nodes++
		if tt.shouldStop() {
			return 0, board.Move{}
		}
	}
	origAlpha := alpha
	hash := b.Hash()
	// excl: with a singular-extension verification search running at this
	// ply, the move it must leave out (zero value: none). Such a node takes
	// no TT cutoff, makes no null move and writes nothing to the TT or the
	// correction tables, since its result is about the position minus a move.
	var excl board.Move
	if tt != nil {
		excl = tt.excl[ply]
	}
	// cut: the parent expects this node to fail high (a "cut node"): set by
	// the parent through tt.cut before the call. isPVNode: open window.
	cut := tt != nil && tt.cut[ply]
	isPVNode := beta-alpha > 1
	// childCut is what the move at index i hands its child: in a PV node every
	// move after the first is probed with a null window and expected to fail
	// low, so its child is a cut node; below a non-PV node the flag alternates.
	childCut := func(i int) bool {
		if isPVNode {
			return i > 0
		}
		return !cut
	}

	if tt != nil {
		// Repetition detection: checked BEFORE the TT probe on purpose
		// — a TT entry for this exact hash may have been computed
		// along a DIFFERENT search line, where reaching this hash
		// wasn't a repetition at all, and its cached score would say
		// nothing about the actual forced draw sitting on THIS line.
		// See isRepetition's own comment for the full reasoning and
		// why returning 0 here can never mask a real checkmate: if
		// this exact position already occurred once on this line and
		// the search continued past it then, it wasn't checkmate (or
		// stalemate) then, and — being the identical position — it
		// still isn't now.
		if tt.isRepetition(ply, hash, b.HalfmoveClock) {
			return 0, board.Move{}
		}
		tt.recordPathHash(ply, hash)

		// PV nodes (open window) take no cutoff or narrowing from the
		// table: a bound-narrowed window made origAlpha stale, so an
		// upper bound could be stored as exact, and PV scores should
		// come from a real search anyway.
		if entry, ok := tt.probe(hash); ok && entry.depth >= depth && (!swTTPV || beta-alpha == 1) && excl == (board.Move{}) {
			adjustedScore := readAdjustMateScore(entry.score, ply)
			switch entry.bound {
			case exactBound:
				return adjustedScore, entry.move
			case lowerBound:
				if adjustedScore > alpha {
					alpha = adjustedScore
				}
			case upperBound:
				if adjustedScore < beta {
					beta = adjustedScore
				}
			}
			if alpha >= beta {
				return adjustedScore, entry.move
			}
		}
	}

	// Generated once, right here, and reused for both the terminal-
	// status check immediately below and the move loop further down
	// (after null-move pruning and the depth==0 quiescence handoff,
	// neither of which need this list themselves). Generating a
	// position's legal moves is by far the most expensive part of a
	// search node — a full pseudo-legal pass plus a king-safety filter
	// per candidate — so computing it twice per node (once inside
	// GameStatus, once for the move loop, as this used to do) was a
	// real, measurable chunk of total search time, not a
	// micro-optimization.
	//
	// Lazy version of the above: the full list is only built when it is
	// needed (null-move attempt, or the move loop), and a node that leaves
	// earlier (reverse futility, razoring, depth 0) only asks the cheap
	// "is there any legal move?" question. terminal() must be called before
	// every return or side effect that the old up-front status check used
	// to precede, so behavior (and node counts) are unchanged.
	// The list lives in a stack buffer (a position with more than 128 legal
	// moves just spills to the heap); negamax recurses, so it can't be shared
	// per ply (IID re-enters this same ply while the outer node's list is live).
	var movesBuf [128]board.Move
	var moves []board.Move
	movesReady := false
	ensureMoves := func() {
		if !movesReady {
			moves = movegen.AppendLegalMoves(b, movesBuf[:0])
			movesReady = true
		}
	}
	terminalChecked := false
	terminal := func() (int, bool) {
		if terminalChecked {
			return 0, false
		}
		var st movegen.Status
		if movesReady {
			st = movegen.GameStatusFromMoves(b, moves)
		} else {
			st = movegen.GameStatusFromAny(b, movegen.HasLegalMove(b))
		}
		switch st {
		case movegen.Checkmate:
			return ply - MateScore, true
		case movegen.Stalemate, movegen.DrawFiftyMove, movegen.DrawInsufficientMaterial:
			return 0, true
		}
		terminalChecked = true
		return 0, false
	}

	// Computed once and reused below by both null-move pruning's guard
	// and Late Move Reductions (see the move loop further down): being
	// in check disqualifies both heuristics for the same reason — every
	// reply is forced/critical here, not "probably safe to skip or
	// search shallowly".
	inCheck := movegen.InCheck(b)

	// Static eval of THIS position, computed lazily and at most once per
	// node: RFP, null-move, razoring and futility below all want it, and
	// every one of them sees the same position (null-move/IID restore
	// the board before returning), so re-running Evaluate per heuristic
	// was pure waste.
	var staticEval int
	staticEvalDone := false
	var corrIdxHere corrIdx
	getStaticEval := func() int {
		if !staticEvalDone {
			staticEval = tt.evaluate(b, ply)
			if swCorr && tt != nil {
				corrIdxHere = corrIndex(b)
				staticEval += tt.corrValue(corrIdxHere)
			}
			staticEvalDone = true
		}
		return staticEval
	}

	// Reverse futility pruning: the mirror of razoring. If the static
	// eval is ALREADY above beta by more than a per-ply margin, this
	// node is almost certainly a fail-high whatever the opponent does
	// with the few plies left, so it's cut without searching. Only in
	// null-window nodes (beta-alpha == 1: nothing to lose on the PV),
	// away from the root, from check and from mate bounds.
	if !isRoot && !inCheck && depth <= rfpMaxDepth && beta-alpha == 1 &&
		beta < mateThreshold && beta > -mateThreshold {
		if e := getStaticEval(); e-rfpMargin*depth >= beta {
			if s, done := terminal(); done {
				return s, board.Move{}
			}
			return e - rfpMargin*depth, board.Move{}
		}
	}

	// Null-move pruning: "if I skipped my turn entirely and the
	// opponent STILL couldn't punish me enough to matter, then any
	// real move I make — which is only ever at least as good as doing
	// nothing — also doesn't need full-width searching here." A
	// reduced-depth search from the null-move position that still
	// fails high (>= beta) is treated as proof of that, and this
	// subtree gets pruned without ever generating or trying its real
	// moves.
	//
	// Guards, each one a known way this heuristic gives wrong answers
	// if left unchecked:
	//   - !isRoot: see negamax's doc comment — the root must return a
	//     real move, which a null-move cutoff never computes.
	//   - !inCheck: "skip a turn while in check" isn't even a legal
	//     concept to reason about — you'd still be in check on the
	//     opponent's next move, which isn't a fair stand-in for what a
	//     real reply looks like.
	//   - hasNonPawnMaterial: in king-and-pawn-only endgames, "doing
	//     nothing" is frequently illegal in spirit (zugzwang) — the
	//     side to move often WANTS to pass but can't, and every legal
	//     move actively worsens their position. Null-move pruning
	//     would badly misjudge exactly these positions, so it's
	//     disabled whenever the side to move has no pieces besides
	//     pawns and king.
	//   - beta < MateScore: keeps this heuristic away from positions
	//     where a forced mate is already being proven/refuted, where
	//     a reduced-depth null-move probe's score isn't a reliable
	//     enough signal to short-circuit on.
	//   - depth >= nullMoveMinDepth: below this there's barely
	//     anything left to reduce into, so the overhead of the probe
	//     isn't worth attempting.
	if !isRoot && excl == (board.Move{}) && depth >= nullMoveMinDepth && beta < MateScore &&
		!inCheck && hasNonPawnMaterial(b, b.SideToMove) &&
		getStaticEval() >= beta {
		ensureMoves()
		if s, done := terminal(); done {
			return s, board.Move{}
		}
		nullUndo := b.MakeNullMove()
		tt.nnuePushNull(ply + 1)
		tt.setMoveKey(ply, -1)
		nmpR := nullMoveReduction
		if swNmp {
			nmpR = nmpBase + depth/nmpDivDepth + min((getStaticEval()-beta)/nmpEvalDiv, 3)
		}
		tt.setCut(ply+1, !cut)
		nullScore, _ := negamax(b, max(depth-1-nmpR, 0), ply+1, -beta, -beta+1, false, tt, checkExtLeft)
		nullScore = -nullScore
		b.UnmakeNullMove(nullUndo)
		if tt.aborted() {
			return 0, board.Move{}
		}
		if nullScore >= beta {
			if swNmp && nullScore < mateThreshold {
				return nullScore, board.Move{}
			}
			return beta, board.Move{}
		}
	}

	// Razoring: futility pruning's mirror image, one level up. Futility
	// pruning (further below) skips individual LATE quiet moves once
	// the static eval plus a margin can't clear alpha; razoring skips
	// the ENTIRE node's search at shallow remaining depth once the
	// static eval plus a (wider — there's a whole subtree of moves to
	// make up the gap with here, not just one) margin already falls
	// short of alpha. Guarded the same way futility pruning is: away
	// from the root (BestMove needs a real move back), away from check
	// (a static snapshot says nothing useful while every reply is
	// forced), and away from mate-score bounds (same reasoning).
	//
	// Not trusted on the raw static eval alone, though — that would
	// risk missing a node where one tactical shot (a hanging piece
	// grab, a good capture) actually closes the gap. A null-window
	// quiescence probe at (alpha-1, alpha) re-checks the claim first:
	// quiescence returning a score still below alpha confirms even the
	// best available captures/check-evasions don't rescue the
	// position, so its (mate-distance-correct, unlike the raw static
	// eval) result is trusted as this node's score outright. A probe
	// that reaches alpha instead means the margin was too pessimistic
	// here, so the real move loop below runs as normal.
	if !isRoot && !inCheck && depth <= razorMaxDepth &&
		alpha > -MateScore && beta < MateScore {
		razorEval := getStaticEval()
		if razorEval+razorMargin[depth] < alpha {
			if s, done := terminal(); done {
				return s, board.Move{}
			}
			score := quiescence(b, ply, alpha-1, alpha, tt)
			if tt.aborted() {
				return 0, board.Move{}
			}
			if score < alpha {
				return score, board.Move{}
			}
		}
	}

	if depth == 0 {
		if s, done := terminal(); done {
			return s, board.Move{}
		}
		return quiescence(b, ply, alpha, beta, tt), board.Move{}
	}

	// From here on the full legal-move list is needed (IID and the move
	// loop). It may already exist from the null-move attempt above.
	ensureMoves()
	if s, done := terminal(); done {
		return s, board.Move{}
	}

	// MultiPV: at the root, the moves of the lines already found are left out
	// (and so is IID, which would search the root without that restriction).
	restricted := isRoot && tt != nil && len(tt.rootSkip) > 0
	if restricted {
		kept := moves[:0]
		for _, m := range moves {
			if !tt.skipsRoot(m) {
				kept = append(kept, m)
			}
		}
		moves = kept
	}

	var ttMove board.Move
	hasTTMove := false
	// singCand: the TT entry is deep enough and bounds from below, so its
	// move is worth a singular-extension check (ttScore is its score).
	singCand := false
	ttScore := 0
	if tt != nil {
		// Quiescence stores entries without a move; those must not count
		// as "has a TT move" (IID would be skipped for them).
		if entry, ok := tt.probe(hash); ok && entry.move != (board.Move{}) {
			ttMove, hasTTMove = entry.move, true
			ttScore = readAdjustMateScore(entry.score, ply)
			singCand = swSing && !isRoot && excl == (board.Move{}) && depth >= singMinDepth &&
				entry.depth >= depth-3 && entry.bound != upperBound &&
				ttScore > -mateThreshold && ttScore < mateThreshold
		}
		if restricted && hasTTMove && tt.skipsRoot(ttMove) {
			ttMove, hasTTMove = board.Move{}, false
		}
	}

	// Internal Iterative Deepening: when there's no TT move to seed
	// ordering with (either this exact position has never been stored
	// before, or its entry was since evicted/overwritten by something
	// else) and enough depth remains that a good first guess is worth
	// paying for, run a cheap reduced-depth search of THIS SAME node
	// first, purely to populate the TT with a best move for it —
	// negamax already unconditionally stores its result at the end of
	// every call (see the store near the bottom of this function), so
	// nothing special is needed here beyond making the call and
	// re-probing afterward. Same window (alpha, beta) as the real
	// search about to happen, isRoot=false regardless of whether THIS
	// node itself is the root (the IID probe is never treated as "the"
	// root move — see negamax's isRoot comment for what that's for).
	// Skipped whenever tt is nil (see negamax's own nil-tt comment):
	// there would be nowhere for a result to land that the real search
	// below could then read back.
	if swIIR && tt != nil && !hasTTMove && !restricted && depth >= iirMinDepth {
		depth--
	} else if tt != nil && !hasTTMove && !restricted && depth >= iidMinDepth {
		negamax(b, depth-iidReduction, ply, alpha, beta, false, tt, checkExtLeft)
		if tt.aborted() {
			return 0, board.Move{}
		}
		if entry, ok := tt.probe(hash); ok {
			ttMove, hasTTMove = entry.move, true
		}
	}

	picker := newMovePicker(b, moves, ttMove, hasTTMove, ply, tt)

	best := -Infinity
	bestHere := moves[0] // placeholder: the first move searched (i == 0) always replaces it

	// Futility pruning setup: applicable at all only away from the
	// root (a root move can never be skipped — BestMove needs a real
	// move back), away from check (every reply is forced/critical
	// there, the same reasoning NMP/LMR's own !inCheck guards use),
	// shallow enough remaining depth, and away from mate-score bounds
	// (a static snapshot has nothing meaningful to say about a
	// position whose value IS a forced-mate distance). staticEval is
	// computed at most once per node, only when actually needed, not
	// once per candidate move.
	futilityApplies := !isRoot && !inCheck && depth <= pruneDepthCap() &&
		alpha > -MateScore && beta < MateScore
	if futilityApplies {
		getStaticEval()
	}

	// Late Move Pruning setup — same guards as futility pruning above
	// (away from root/check/mate-bounds, shallow remaining depth), but
	// no static eval needed: LMP's condition is move count alone. Kept
	// as its own named bool, rather than folded into futilityApplies,
	// since the two techniques have different max-depth constants
	// (lmpMaxDepth/futilityMaxDepth) that happen to currently match but
	// aren't required to.
	lmpApplies := !isRoot && !inCheck && depth <= pruneDepthCap() &&
		alpha > -MateScore && beta < MateScore

	// SEE pruning of captures: a later capture that loses more than
	// seePruneMargin*depth centipawns in the full exchange is skipped
	// unsearched, at shallow remaining depth. Same guards as LMP.
	seePruneApplies := lmpApplies && depth <= seePruneMaxDepth

	// Quiet moves fully searched at this node without causing a cutoff —
	// penalized in history if a later move does cut (see below).
	var quietsTried [64]board.Move
	nQuiets := 0
	var capsTried [32]board.Move
	nCaps := 0
	var b1, b2 int
	if tt != nil {
		b1, b2 = tt.contBases(ply)
	}

	for i := 0; ; i++ {
		m, ok := picker.next()
		if !ok {
			break
		}
		if m == excl {
			continue
		}
		if seePruneApplies && i > 0 && m.Flag == board.Capture && SEE(b, m) < -seePruneMargin*depth {
			continue
		}

		// Singular extension: if every other move fails to reach a bit below
		// the TT move's score in a reduced search, the TT move is the only good
		// one here and is searched a ply deeper; if the others reach beta
		// already, the node is cut (multi-cut); a TT move that already fails
		// high while not being singular is searched a ply shallower.
		singExt := 0
		if singCand && m == ttMove {
			singBeta := ttScore - depth*singMargin/16
			tt.excl[ply] = m
			sScore, _ := negamax(b, (depth-1)/2, ply, singBeta-1, singBeta, false, tt, checkExtLeft)
			tt.excl[ply] = board.Move{}
			if tt.aborted() {
				return 0, board.Move{}
			}
			switch {
			case sScore < singBeta:
				singExt = 1
			case singBeta >= beta:
				if sScore < mateThreshold {
					return sScore, board.Move{}
				}
				return singBeta, board.Move{}
			case swSingNeg && ttScore >= beta:
				singExt = -1
			}
		}

		// lmrD: the depth this move would be searched to after a late-move
		// reduction; the shallow pruning below judges moves by it, not by the
		// node's own depth. quietSee: what the opponent wins on the target
		// square (negative); measured before the move is made.
		lmrD := depth
		quietSee := 0
		if lmpApplies && i > 0 {
			lmrD = max(depth-1-lmrReductionFor(depth, i), 0)
			// A move cannot lose more than its piece is worth, so a piece
			// worth no more than the threshold can never be pruned.
			if swSeeQuiet && depth <= seeQuietMaxDepth &&
				seeValue(b.PieceAt(m.From).Type()) > seeQuietMul*lmrD*lmrD {
				quietSee = seeQuiet(b, m)
			}
		}

		undo := b.MakeMove(m)
		tt.nnuePush(ply+1, m, undo, b)
		mover := undo.MovedPiece
		tt.setMoveKey(ply, moveKey(mover, m.To))
		isQuiet := !m.IsCapture()
		hs := 0
		if tt != nil && isQuiet {
			hs = tt.quietHist(mover, m, b1, b2)
		}

		// givesCheck: does THIS move put the opponent in check? Cheap
		// (movegen.InCheck is an O(1) magic-bitboard lookup, not a
		// scan), and used for three things below: futility pruning's
		// per-move guard, excluding the move from LMR (a check is
		// forcing, not a "probably safe to search shallowly" quiet
		// move — see reduceEligible), and extending the depth passed
		// to whichever recursive call actually searches it, budget
		// permitting.
		givesCheck := movegen.InCheck(b)

		// Futility pruning: a quiet move whose best-case swing (the
		// node's static eval plus a generous per-ply margin) still
		// can't reach alpha has essentially no realistic chance of
		// mattering this close to the leaves — skip it without ever
		// searching it, not even a reduced probe. i > 0 excludes the
		// move ordering has bet everything on being best here (the
		// same conservatism PVS/LMR already give that move elsewhere
		// in this loop) — futility only ever prunes a LATER
		// candidate, never the presumed-best one. Captures and
		// promotions are excluded (their value isn't something a
		// static snapshot judges), and so are check-giving moves
		// (forcing, not "probably safe to skip" — same exclusion
		// reduceEligible makes just below).
		if futilityApplies && i > 0 && !givesCheck && !m.IsCapture() &&
			m.Flag != board.Promotion && m.Flag != board.PromotionCapture &&
			staticEval+futilityMarginAt(futDepth(depth, lmrD)) <= alpha {
			b.UnmakeMove(m, undo)
			continue
		}

		// Late Move Pruning: futility pruning's move-COUNT-based
		// cousin. Futility judges each quiet move individually against
		// a static-eval margin; LMP doesn't look at eval at all — once
		// this many candidates have already been tried in full at this
		// shallow a remaining depth, move ordering has already had its
		// best shots, and everything still left in the quiet, non-check
		// tail of the list essentially never turns out to matter enough
		// to justify searching it. Same exclusions as futility (checks
		// and captures/promotions are never skipped — a check is
		// forcing, and a capture or promotion's value isn't something
		// move-order position alone judges) and the same shallow-depth/
		// away-from-check/away-from-mate-bounds guard.
		if lmpApplies && i >= lmpThresholdAt(depth) && !givesCheck && !m.IsCapture() &&
			m.Flag != board.Promotion && m.Flag != board.PromotionCapture {
			b.UnmakeMove(m, undo)
			continue
		}

		// SEE pruning of quiet moves: one that walks into a losing exchange on
		// its target square is skipped at shallow depth.
		if quietSee < 0 && !givesCheck && quietSee < -seeQuietMul*lmrD*lmrD {
			b.UnmakeMove(m, undo)
			continue
		}

		// History pruning: a quiet move that has done badly in this and the
		// preceding contexts is skipped at shallow depth.
		if swHistPrune && lmpApplies && depth <= histPruneMaxDepth && i > 0 && isQuiet && !givesCheck &&
			m.Flag != board.Promotion && hs < -histPruneMargin*depth {
			b.UnmakeMove(m, undo)
			continue
		}

		extend := 0
		if givesCheck && checkExtLeft > 0 {
			extend = 1
		}
		childCheckExtLeft := checkExtLeft
		if extend > 0 {
			childCheckExtLeft--
		}
		childDepth := depth - 1 + extend + singExt

		// Late Move Reductions guard: move ordering has already put
		// its best guesses (TT move, captures by MVV-LVA, killers,
		// history) at the front of moves — a quiet move still this
		// far down the list, this deep in the tree, is unlikely to be
		// the best one here, so its FIRST probe (see below) is done a
		// few plies shallower than normal. Captures and promotions are
		// excluded (their tactical value isn't something a
		// reduced-depth glance reliably judges), moves that give check
		// are excluded for the same reason (forcing, not "probably
		// safe to skip or search shallowly"), and the whole thing is
		// skipped while already in check, same reasoning as null-move
		// pruning's guard above.
		reduceEligible := !inCheck && !givesCheck &&
			depth >= lmrMinDepth &&
			i >= lmrFullMovesFor(isPVNode) &&
			!m.IsCapture() &&
			m.Flag != board.Promotion && m.Flag != board.PromotionCapture

		var s int
		if i == 0 {
			// Principal Variation Search: the first move is exactly
			// the one move ordering has bet everything on being best
			// here. Rather than making it prove that with a cheap
			// probe first, it goes straight to the full window — the
			// probe/re-search dance below exists to cheaply REJECT
			// moves that don't beat alpha, and this move is expected
			// to set alpha, not merely clear it.
			tt.setCut(ply+1, childCut(0))
			childScore, _ := negamax(b, childDepth, ply+1, -beta, -alpha, false, tt, childCheckExtLeft)
			s = -childScore
		} else {
			// Every later move: a cheap null-window probe first — a
			// yes/no answer to "is this move even better than what we
			// already have (alpha)?" — before anything earns the
			// expensive full-window search. LMR folds in here as
			// simply a shallower depth for that first probe: a
			// reduced-depth null-window answer is cheaper still, and a
			// probe that fails to beat alpha at reduced depth almost
			// always also fails at full depth, so most late quiet
			// moves get rejected without ever running at full depth.
			searchDepth := childDepth
			if reduceEligible {
				r := lmrReductionFor(depth, i)
				if swLmrPV && beta-alpha > 1 && r > 1 {
					r--
				}
				// A cut node's late moves are even less likely to matter, and
				// so are quiet moves when the best move found is a capture.
				if swLmrCut && cut {
					r++
				}
				if swLmrCapt && hasTTMove && ttMove.IsCapture() {
					r++
				}
				if swHistLmr && tt != nil {
					r -= max(-2, min(hs/histLmrDiv, 2))
					r = max(1, min(r, depth-2))
				}
				r = max(1, min(r, depth-2))
				searchDepth = depth - 1 - r
			}
			tt.setCut(ply+1, childCut(i))
			probeScore, _ := negamax(b, searchDepth, ply+1, -alpha-1, -alpha, false, tt, childCheckExtLeft)
			s = -probeScore

			if reduceEligible && s > alpha {
				// The reduced probe beat alpha — that alone might
				// just be an artifact of the shallower depth, not a
				// real signal. Re-confirm with the same cheap null
				// window at full depth before paying for a
				// full-window search.
				tt.setCut(ply+1, childCut(i))
				probeScore, _ = negamax(b, childDepth, ply+1, -alpha-1, -alpha, false, tt, childCheckExtLeft)
				s = -probeScore
			}

			if s > alpha && s < beta {
				// A null window only ever answers "beats alpha or
				// not" — it can't pin down the real score once that's
				// yes. Only a move that earned it gets the full-
				// window search that actually establishes its value.
				tt.setCut(ply+1, false)
				fullScore, _ := negamax(b, childDepth, ply+1, -beta, -alpha, false, tt, childCheckExtLeft)
				s = -fullScore
			}
		}

		b.UnmakeMove(m, undo)

		if tt.aborted() {
			return 0, board.Move{} // s is garbage; don't let it reach the TT
		}

		if isRoot && tt != nil && s > alpha {
			// A completed root move that really raised alpha: if time runs
			// out later in this iteration, it is a usable result.
			tt.rootPartial, tt.rootPartialOK = m, true
		}
		if s > best {
			best = s
			bestHere = m
		}
		if best > alpha {
			alpha = best
		}
		if alpha >= beta {
			// Move-ordering aids only care about QUIET cutoff moves —
			// captures already get their own MVV-LVA ordering, and a
			// killer/history slot "this capture is great" would just
			// be redundant with that.
			if tt != nil {
				if isQuiet {
					tt.recordKiller(ply, m)
					tt.recordHistory(mover, m, ply, depth)
					for _, q := range quietsTried[:nQuiets] {
						tt.penalizeHistory(b.PieceAt(q.From), q, ply, depth)
					}
				} else if swHistCapt {
					tt.updateCaptHist(mover, m, capturedType(b, m), historyBonus(depth))
				}
				if swHistCapt {
					for _, c := range capsTried[:nCaps] {
						tt.updateCaptHist(b.PieceAt(c.From), c, capturedType(b, c), -historyBonus(depth))
					}
				}
			}
			break // alpha-beta cutoff
		}
		if isQuiet && nQuiets < len(quietsTried) {
			quietsTried[nQuiets] = m
			nQuiets++
		} else if !isQuiet && nCaps < len(capsTried) {
			capsTried[nCaps] = m
			nCaps++
		}
	}

	if tt != nil {
		resultBound := exactBound
		if best <= origAlpha {
			resultBound = upperBound
		} else if best >= beta {
			resultBound = lowerBound
		}
		if swCorr && excl == (board.Move{}) && !inCheck && !isRoot && best > -mateThreshold && best < mateThreshold &&
			(resultBound == upperBound || !bestHere.IsCapture()) {
			e := getStaticEval()
			if (best > e) == (resultBound != upperBound) {
				tt.updateCorr(corrIdxHere, best-e, depth)
			}
		}
		storeMove := bestHere
		if swTTKeep && resultBound == upperBound {
			storeMove = board.Move{} // a fail-low "best" move is arbitrary
		}
		if excl == (board.Move{}) {
			tt.store(hash, ttEntry{depth: depth, score: storeAdjustMateScore(best, ply), bound: resultBound, move: storeMove})
		}
	}

	return best, bestHere
}

package search

import (
	"dyanis-chess/internal/board"
)

// --- Move ordering -------------------------------------------------
//
// Alpha-beta pruning's effectiveness depends entirely on searching
// the BEST move at each node first — a cutoff can only happen after
// something has already raised alpha, so a good first guess is what
// lets later siblings get skipped instead of fully searched. This
// section is the "good first guess" logic negamax's move loop above
// relies on, layered in priority order:
//
//  1. the transposition table's remembered best move for this exact
//     position (from a shallower iterative-deepening pass, or an
//     earlier visit via a different move order)
//  2. captures that don't lose material (SEE >= 0), ordered by MVV-LVA
//     (Most Valuable Victim, Least Valuable Attacker) — try pxq before qxp
//  3. killer moves: quiet moves that caused a beta cutoff at this
//     same remaining depth (see TranspositionTable's doc comment)
//  4. quiet moves, by history heuristic score (accumulated across
//     the whole search, not just this depth)
//  5. captures that lose material (SEE < 0), last — still MVV-LVA among
//     themselves

// pieceOrderingValue gives a cheap, ordering-only weight per piece
// type — NOT the same table eval.Evaluate uses for scoring positions,
// just enough to rank captures by "which piece is taken / which piece
// is doing the taking".
var pieceOrderingValue = [7]int{ // indexed by board.PieceType; NoType (0) is 0
	board.Pawn:   100,
	board.Knight: 320,
	board.Bishop: 330,
	board.Rook:   500,
	board.Queen:  900,
	board.King:   20000,
}

// mvvLva scores a capture so that sorting descending tries the most
// valuable victim / least valuable attacker combinations first (e.g.
// pawn takes queen before queen takes pawn). Multiplying the victim's
// value by 10 spreads it out enough that even the cheapest attacker
// capturing the most valuable victim always outranks the most
// expensive attacker capturing the least valuable victim.
func mvvLva(b *board.Board, m board.Move) int {
	victimType := board.Pawn // en passant always takes a pawn, and that
	// pawn isn't on m.To (see move.go), so PieceAt(m.To) would see
	// nothing there — special-cased rather than misreading it as None.
	if m.Flag != board.EnPassantCapture {
		victimType = b.PieceAt(m.To).Type()
	}
	attackerType := b.PieceAt(m.From).Type()
	return pieceOrderingValue[victimType]*10 - pieceOrderingValue[attackerType]
}

// deltaMargin is how many centipawns of headroom, beyond the captured
// piece's raw ordering value, a capture is given the benefit of the
// doubt for in quiescence's delta-pruning filter — covers both
// pieceOrderingValue being a cheap ordering-only approximation (see
// its own comment) and whatever small positional gain a capture might
// carry beyond pure material.
const deltaMargin = 200

// deltaPrune reports whether a capture can be discarded in
// quiescenceSearch WITHOUT ever running SEE on it: even in the most
// optimistic case — the capture nets the full raw value of the piece
// taken, no more — standPat plus that gain plus deltaMargin still
// falls short of alpha, so no SEE outcome (which only ever simulates
// the SAME exchange more precisely, never invents extra material)
// could change that conclusion. A single table lookup here is far
// cheaper than SEE's full exchange simulation, so this runs first and
// only lets what survives it pay for a real SEE call — same "cheap
// filter before the expensive one" shape as futility pruning's
// staticEval check.
//
// Never prunes en passant (the captured pawn isn't on m.To — see
// mvvLva's own comment — so PieceAt(m.To) would misread it as an
// empty square, i.e. a free capture, rather than skip it outright) or
// promotion-captures (the new queen's value swamps whatever this
// margin is tuned for; the plain captured-piece value alone
// understates the real gain and would prune genuinely good moves).
// Both are rare enough in quiescence's capture lists that skipping
// the optimization there costs nothing measurable.
// deltaPruneHits is a TEMPORARY debug counter — how many times
// deltaPrune actually fired during a search. Not meant to stay: added
// only to verify the delta-pruning change is doing anything at all
// (see the plan.md discussion), remove once confirmed.
func deltaPrune(b *board.Board, m board.Move, standPat, alpha int) bool {
	if m.Flag == board.EnPassantCapture || m.Flag == board.PromotionCapture {
		return false
	}
	victimValue := pieceOrderingValue[b.PieceAt(m.To).Type()]
	return standPat+victimValue+deltaMargin < alpha
}

// Score bands keep the ordering tiers from ever overlapping:
// even the worst-scored good capture (mvvLva can go slightly negative,
// e.g. pawn takes nothing valuable while a queen is the attacker)
// still outranks every killer/history-only quiet move, the worst
// killer still outranks a history score of 0, and losing captures
// (badCaptureBand) sit below every quiet move.
const (
	ttMoveScore = 1_000_000
	captureBand = 100_000
	killerBand  = 10_000

	// badCaptureBand puts captures that lose material in the exchange
	// (SEE < 0) below every quiet move: history is bounded by
	// ±maxHistory, and even the worst mvvLva offset (a king taking a
	// pawn, about -19_000) keeps them under that.
	badCaptureBand = -captureBand
)

// losingCapture reports whether m is a plain capture that SEE says loses
// material. A capture whose victim is worth at least as much as the
// capturing piece can't lose anything (at worst it is recaptured for an
// even trade, and SEE lets the capturer stop earlier), so SEE only runs
// for the "lesser piece takes something" case where a defender might
// matter. En passant (pawn takes pawn) and promotion-captures (the new
// piece's value isn't in the capture's own gain) are never treated as
// losing.
func losingCapture(b *board.Board, m board.Move) bool {
	if m.Flag != board.Capture {
		return false
	}
	if pieceOrderingValue[b.PieceAt(m.To).Type()] >= pieceOrderingValue[b.PieceAt(m.From).Type()] {
		return false
	}
	return SEE(b, m) < 0
}

// insertionSortByScoreDesc sorts moves and their parallel scores
// slice in lockstep, descending by score, directly. Move lists in
// this engine are small — rarely more than a few dozen candidates at
// any node — well below the size where a general-purpose sort (with
// its pivot selection, partitioning, and interface-method dispatch)
// earns back its own overhead; Go's sort package already falls back
// to a plain insertion sort internally for small slices, so this just
// skips straight to that.
func insertionSortByScoreDesc(moves []board.Move, scores []int) {
	for i := 1; i < len(moves); i++ {
		m, s := moves[i], scores[i]
		j := i - 1
		for j >= 0 && scores[j] < s {
			moves[j+1] = moves[j]
			scores[j+1] = scores[j]
			j--
		}
		moves[j+1] = m
		scores[j+1] = s
	}
}

// movePicker hands out the moves of one node in exactly the order
// a full descending sort by score would, but scores the quiet moves (history lookups
// plus a sort) only when the search actually gets that far. Most nodes cut
// off on the TT move or a capture and never need them.
//
// Equivalence with the full sort: the score bands (ttMoveScore, good
// captures, killers, history, losing captures) never overlap, so the full
// descending sort is the TT move, then good captures, then killers, then
// quiets, then losing captures, each group sorted stably by its own score.
type movePicker struct {
	moves        []board.Move
	i            int
	base         int // moves[:base] are already handed out (the TT move)
	quietStart   int
	quietEnd     int
	quietsSorted bool
	built        bool
	b            *board.Board
	ttMove       board.Move
	hasTTMove    bool
	ply          int
	tt           *TranspositionTable
}

const pickerBufSize = 256

// newMovePicker only records its inputs; nothing is scored until next() is
// called past the TT move. Many nodes cut off on the TT move alone and never
// pay for scoring, SEE and sorting of the whole list.
func newMovePicker(b *board.Board, moves []board.Move, ttMove board.Move, hasTTMove bool, ply int, tt *TranspositionTable) movePicker {
	return movePicker{moves: moves, b: b, ttMove: ttMove, hasTTMove: hasTTMove, ply: ply, tt: tt}
}

// build reorders moves[base:] in place into the groups (captures, killers
// sorted; quiets in generation order, to be sorted by history on first use).
func (p *movePicker) build() {
	p.built = true
	b, tt := p.b, p.tt
	moves := p.moves[p.base:]

	var killers [killerSlots]board.Move
	if tt != nil {
		killers = *tt.killersAt(p.ply)
	}

	n := len(moves)
	var catBuf [pickerBufSize]int8
	var keyBuf, outKeyBuf [pickerBufSize]int
	var outBuf [pickerBufSize]board.Move
	cat, key, out, outKey := catBuf[:], keyBuf[:], outBuf[:], outKeyBuf[:]
	if n > pickerBufSize {
		cat, key, out, outKey = make([]int8, n), make([]int, n), make([]board.Move, n), make([]int, n)
	}

	for idx, m := range moves {
		switch {
		case m.IsCapture():
			v := mvvLva(b, m)
			if losingCapture(b, m) {
				cat[idx], key[idx] = 4, badCaptureBand+v
			} else {
				cat[idx], key[idx] = 1, captureBand+v
			}
		default:
			cat[idx] = 3
			for s, k := range killers {
				if m == k {
					cat[idx], key[idx] = 2, killerBand-s
					break
				}
			}
		}
	}

	// Stable gather by group; remember where each group starts.
	var start [6]int
	pos := 0
	for c := int8(1); c < 5; c++ {
		start[c] = pos
		for idx := 0; idx < n; idx++ {
			if cat[idx] == c {
				out[pos], outKey[pos] = moves[idx], key[idx]
				pos++
			}
		}
	}
	start[5] = n
	for _, c := range [...]int{1, 2, 4} {
		insertionSortByScoreDesc(out[start[c]:start[c+1]], outKey[start[c]:start[c+1]])
	}
	copy(moves, out[:n])

	p.quietStart = p.base + start[3]
	p.quietEnd = p.base + start[4]
}

// next returns the next move in order, or false when none are left.
func (p *movePicker) next() (board.Move, bool) {
	if p.i >= len(p.moves) {
		return board.Move{}, false
	}
	if !p.built {
		if p.i == 0 && p.hasTTMove {
			for idx, m := range p.moves {
				if m == p.ttMove {
					// Rotate to the front, keeping the others' relative
					// (generation) order: quiet ties depend on it.
					copy(p.moves[1:idx+1], p.moves[:idx])
					p.moves[0] = m
					p.base, p.i = 1, 1
					return m, true
				}
			}
		}
		p.build()
	}
	if p.i == p.quietStart && !p.quietsSorted {
		p.sortQuiets()
	}
	m := p.moves[p.i]
	p.i++
	return m, true
}

// sortQuiets orders the quiet group by history score (a stable sort; with no
// table every score is 0 and generation order is kept).
func (p *movePicker) sortQuiets() {
	p.quietsSorted = true
	q := p.moves[p.quietStart:p.quietEnd]
	if p.tt == nil || len(q) < 2 {
		return
	}
	var buf [pickerBufSize]int
	scores := buf[:]
	if len(q) > pickerBufSize {
		scores = make([]int, len(q))
	}
	scores = scores[:len(q)]
	for i, m := range q {
		scores[i] = p.tt.historyScore(m)
	}
	insertionSortByScoreDesc(q, scores)
}

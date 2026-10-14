package search

import (
	"dyanis-chess/internal/board"
	"dyanis-chess/internal/nnue"
)

// network is the NNUE used for static evaluation instead of eval.Evaluate;
// nil (the default) means the hand-written eval. Set with SetNetwork
// between searches, never during one.
var network *nnue.Network

// SetNetwork switches static evaluation to n, or back to the hand-written
// eval when n is nil. Searches already running keep what they started
// with; call it between searches (UCI only changes options between "go"s).
func SetNetwork(n *nnue.Network) { network = n }

// Network returns the network in use, nil for the hand-written eval.
func Network() *nnue.Network { return network }

// accEntry is one ply of the accumulator stack. The search records every
// move it makes (nnuePush) but only builds the accumulator when that ply
// is actually evaluated: many children are cut, pruned or answered from
// the eval cache before anyone asks.
type accEntry struct {
	acc  *nnue.Accumulator
	ok   bool // acc describes this ply's position on the current line
	null bool // reached by a null move: same pieces as the parent
	move board.Move
	undo board.Undo
	// snap is the piece bitboards after move when it was a king move that
	// changes the king bucket (snapOK): the accumulator is then rebuilt for
	// that side, which needs the position, not just the move.
	snap   [12]board.Bitboard
	snapOK bool
}

// nnueReset prepares tt for a search from root position b: it snapshots the
// current network (so a search never mixes two), drops state built for a
// different one, and computes the root accumulator. Every search entry
// point calls it; tables nobody reset evaluate with the hand-written eval.
func (tt *TranspositionTable) nnueReset(b *board.Board) {
	if tt.net != network {
		tt.net = network
		tt.accStack = nil
		tt.evalCache = nil // cached scores came from the other evaluator
	}
	if tt.net == nil {
		return
	}
	e := tt.accAt(0)
	tt.net.Refresh(e.acc, b)
	e.ok = true
}

// accAt returns the stack entry for ply, growing the stack as needed.
func (tt *TranspositionTable) accAt(ply int) *accEntry {
	for len(tt.accStack) <= ply {
		tt.accStack = append(tt.accStack, accEntry{acc: tt.net.NewAccumulator()})
	}
	return &tt.accStack[ply]
}

// nnuePush records that the position at ply was reached by move m (with u
// from MakeMove) from the position at ply-1. Safe on a nil table.
func (tt *TranspositionTable) nnuePush(ply int, m board.Move, u board.Undo, b *board.Board) {
	if tt == nil || tt.net == nil {
		return
	}
	e := tt.accAt(ply)
	e.ok, e.null, e.move, e.undo = false, false, m, u
	e.snapOK = tt.net.KingRefresh(m, u)
	if e.snapOK {
		e.snap = b.PieceBitboards()
	}
}

// nnuePushNull records that ply was reached by a null move.
func (tt *TranspositionTable) nnuePushNull(ply int) {
	if tt == nil || tt.net == nil {
		return
	}
	e := tt.accAt(ply)
	e.ok, e.null = false, true
}

// nnueEvaluate scores b, the position at ply on the current line, with the
// network: it walks back to the nearest ply whose accumulator is built and
// replays the recorded moves forward from there.
func (tt *TranspositionTable) nnueEvaluate(b *board.Board, ply int) int {
	n := tt.net
	tt.accAt(ply)
	q := ply
	for q > 0 && !tt.accStack[q].ok {
		q--
	}
	if !tt.accStack[q].ok {
		// Nothing built on this line (cannot happen after nnueReset): build
		// this ply from the board itself.
		q = ply
		n.Refresh(tt.accStack[ply].acc, b)
		tt.accStack[ply].ok = true
	}
	for k := q + 1; k <= ply; k++ {
		e, parent := &tt.accStack[k], &tt.accStack[k-1]
		if e.null {
			e.acc.CopyFrom(parent.acc)
		} else {
			var snap *[12]board.Bitboard
			if e.snapOK {
				snap = &e.snap
			}
			n.Update(e.acc, parent.acc, e.move, e.undo, snap)
		}
		e.ok = true
	}
	if nnueVerify {
		check := n.NewAccumulator()
		n.Refresh(check, b)
		if !check.Equal(tt.accStack[ply].acc) {
			panic("search: NNUE accumulator out of sync with the board at " + b.ToFEN())
		}
	}
	return n.Evaluate(tt.accStack[ply].acc, b.SideToMove, b.Occupied().Count())
}

// nnueVerify makes every network evaluation also recompute the accumulator
// from scratch and panic on a mismatch — for tests, which turn it on to
// prove every move the search makes is pushed.
var nnueVerify = false

// Opening-book integration for search's entry point. Kept in its own
// file, separate from search.go's actual negamax/alpha-beta code,
// since this isn't "search" in the tree-walking sense — it's a
// decision made *before* search runs at all.
package search

import (
	"math/rand"
	"time"

	"dyanis-chess/internal/board"
	"dyanis-chess/internal/book"
	"dyanis-chess/internal/movegen"
)

// BestMoveWithBook is BestMove's book-aware entry point. If bk has one
// or more (nonzero-weight) moves recorded for the current position, it
// plays one of those instead of searching — book moves come from real
// games (by strong humans or engines, depending on the book), so for
// the handful of moves a book actually covers, that's a far better
// prior than what a fixed-depth, positionally shallow search can work
// out on its own.
//
// This deliberately does NOT feed book weight into eval.Evaluate.
// Evaluate scores arbitrary positions, including ones many plies deep
// inside the search tree that have nothing to do with the book and
// have no natural "how well-known is this" value a weight could
// represent — corrupting that scoring for the sake of one decision at
// the root would be a bad trade. So the book stays a short-circuit
// here, before search ever runs, exactly as book.go's original TODO
// called for.
//
// bk is a book.Source — either a single *book.Book or a *book.Chain
// of several tried in priority order (see book.go). Taking the
// interface rather than *book.Book means a caller can pass either
// without this function caring which.
//
// bk may be nil (no book loaded), in which case this is identical to
// calling BestMove directly. The bool return reports whether the move
// came from the book, purely so callers (e.g. the CLI) can say so.
//
// Caller beware: the "no book" state must reach here as a literal nil
// book.Source (e.g. a struct field declared as book.Source, left at
// its zero value). If a caller instead holds a `var b *book.Book`
// that is nil and passes it here, Go boxes that nil *book.Book into a
// NON-nil book.Source (the interface value has a type but a nil
// pointer inside it) — `bk != nil` below would then be true, and the
// call would panic on a nil pointer dereference the first time
// LookupBoard runs. Keep book state typed as book.Source (or Chain)
// throughout — game.bk in cmd/wasm and the CLI's book variable both
// do this — and this can't happen.
func BestMoveWithBook(b *board.Board, depth int, bk book.Source) (m board.Move, fromBook bool, err error) {
	return BestMoveWithBookInfo(b, depth, bk, nil)
}

// BestMoveWithBookInfo is BestMoveWithBook with an added onInfo
// callback, forwarded straight to BestMoveInfo when the book doesn't
// have a move for this position — see BestMoveInfo's comment for when
// it fires. A book HIT never calls onInfo at all: there's no search
// info to report when no search ran (see BestMoveWithBook's own
// comment on why the book is a short-circuit, not part of eval/search
// proper).
func BestMoveWithBookInfo(b *board.Board, depth int, bk book.Source, onInfo func(SearchInfo)) (m board.Move, fromBook bool, err error) {
	if bk != nil {
		if bm, ok := pickBookMove(b, bk); ok {
			return bm, true, nil
		}
	}
	m, err = BestMoveInfo(b, depth, onInfo)
	return m, false, err
}

// BestMoveWithBookInfoTT is BestMoveWithBookInfo but searches using
// the given tt instead of letting BestMoveInfo construct a fresh one
// — see TranspositionTable.NewSearch's own comment for why a caller
// (the UCI session, cmd/cli's -play loop) would want to keep a hash
// table warm across several separate calls, e.g. across a whole
// game's worth of moves, rather than starting cold every time. A book
// HIT still never touches tt at all, same as every other *WithBook*
// entry point — there's nothing to search, and so nothing to warm the
// table with, when a book move is played outright.
func BestMoveWithBookInfoTT(b *board.Board, depth int, tt *TranspositionTable, bk book.Source, onInfo func(SearchInfo)) (m board.Move, fromBook bool, err error) {
	if bk != nil {
		if bm, ok := pickBookMove(b, bk); ok {
			return bm, true, nil
		}
	}
	m, err = BestMoveInfoTT(b, depth, tt, onInfo)
	return m, false, err
}

// BestMoveTimedWithBook is BestMoveTimed's book-aware counterpart,
// with the exact same book-first reasoning as BestMoveWithBook — see
// its comment for why the book is checked here rather than folded
// into eval. maxDepth/budget are passed straight through to
// BestMoveTimed when there's no book hit.
func BestMoveTimedWithBook(b *board.Board, maxDepth int, budget time.Duration, bk book.Source) (m board.Move, fromBook bool, err error) {
	return BestMoveTimedWithBookInfo(b, maxDepth, budget, bk, nil)
}

// BestMoveTimedWithBookInfo is BestMoveTimedWithBook with an added
// onInfo callback, forwarded straight to BestMoveTimedInfo (fired once
// per completed depth) when the book doesn't have a move for this
// position — see BestMoveWithBookInfo's comment for why a book HIT
// never calls onInfo.
func BestMoveTimedWithBookInfo(b *board.Board, maxDepth int, budget time.Duration, bk book.Source, onInfo func(SearchInfo)) (m board.Move, fromBook bool, err error) {
	if bk != nil {
		if bm, ok := pickBookMove(b, bk); ok {
			return bm, true, nil
		}
	}
	m, err = BestMoveTimedInfo(b, maxDepth, budget, onInfo)
	return m, false, err
}

// BestMoveTimedWithBookMT is BestMoveTimedWithBook's Lazy SMP
// counterpart (plan.md item 3/D): the same book-first short-circuit
// as every other *WithBook* entry point, but the fallback search runs
// BestMoveTimedMT instead of BestMoveTimedInfo once there's no book
// hit — threads goroutines sharing one hash table (see
// BestMoveTimedMT's own comment). threads<=1 behaves identically to
// BestMoveTimedWithBookInfo, since BestMoveTimedMT itself falls back
// to the plain single-threaded path at threads<=1.
//
// There's deliberately no fixed-depth (non-timed) Lazy SMP
// counterpart to BestMoveWithBook: several threads racing to the
// exact same FIXED depth mostly just duplicates work, for only a
// shared-TT's worth of benefit — none of Lazy SMP's usual point,
// which is using otherwise-idle cores to search DEEPER within a
// wall-clock budget. This is why -threads in cmd/cli and UCI's
// "setoption name Threads" (see internal/uci/uci.go) only affect
// movetime-based searches; a plain "go depth N" or -play without
// -movetime stays single-threaded regardless of -threads.
func BestMoveTimedWithBookMT(b *board.Board, maxDepth int, budget time.Duration, threads int, bk book.Source, onInfo func(SearchInfo)) (m board.Move, fromBook bool, err error) {
	if bk != nil {
		if bm, ok := pickBookMove(b, bk); ok {
			return bm, true, nil
		}
	}
	m, err = BestMoveTimedMT(b, maxDepth, budget, threads, onInfo)
	return m, false, err
}

// BestMoveTimedWithBookMTShared is BestMoveTimedWithBookMT, but the
// fallback search runs BestMoveTimedMTShared instead of
// BestMoveTimedMT — every thread shares persistentTT's underlying
// hash table (see BestMoveTimedMTShared's own comment), so passing
// the SAME *TranspositionTable across repeated calls (one per move
// across a whole game, say) keeps that table warm across MOVES on top
// of what threads already share with each other within one call.
// persistentTT must not be nil. A book HIT still never touches it at
// all, same reasoning as BestMoveWithBookInfoTT's own comment.
func BestMoveTimedWithBookMTShared(b *board.Board, maxDepth int, budget time.Duration, threads int, persistentTT *TranspositionTable, bk book.Source, onInfo func(SearchInfo)) (m board.Move, fromBook bool, err error) {
	if bk != nil {
		if bm, ok := pickBookMove(b, bk); ok {
			return bm, true, nil
		}
	}
	m, err = BestMoveTimedMTShared(b, maxDepth, budget, threads, persistentTT, onInfo)
	return m, false, err
}

// pickBookMove looks up b in the book and, among entries with nonzero
// weight (Polyglot's convention: weight 0 means "deleted", so those
// are skipped), picks one at random weighted by Entry.Weight. A move
// played in 10,000 recorded games is picked far more often than one
// played in 3, but the engine won't always play the single most
// popular try — always doing that would make it perfectly predictable
// and throw away the variety the weights are meant to encode.
func pickBookMove(b *board.Board, bk book.Source) (board.Move, bool) {
	entries := bk.LookupBoard(b)
	if len(entries) == 0 {
		return board.Move{}, false
	}

	total := 0
	for _, e := range entries {
		total += int(e.Weight)
	}
	if total == 0 {
		return board.Move{}, false
	}

	legal := movegen.GenerateLegalMoves(b)

	pick := rand.Intn(total)
	sum := 0
	for _, e := range entries {
		sum += int(e.Weight)
		if pick < sum {
			return book.DecodeMove(e, legal)
		}
	}
	return board.Move{}, false // unreachable given total > 0
}

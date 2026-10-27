package search

import (
	"sync/atomic"
	"time"

	"dyanis-chess/engine/internal/board"
	"dyanis-chess/engine/internal/eval"
	"dyanis-chess/engine/internal/nnue"
)

// storeAdjustMateScore converts a mate score from "relative to the
// search root" (what negamax computes and returns — see MateScore's
// comment) into "relative to this node" before it's cached in the
// transposition table. This is the fix for the documented TT bug:
// without it, a mate score cached while reaching a position at one
// ply from the root would be silently reused, unmodified, if that
// SAME position were later reached via a transposing line at a
// DIFFERENT ply — reporting the wrong mate distance (or in the worst
// case, treating a non-mate as one, or vice versa, since the absolute
// numbers no longer line up with the position's actual distance to
// mate from wherever it's currently being reached). Storing the
// node-relative value instead means readAdjustMateScore can correctly
// re-derive the right root-relative number no matter which ply the
// entry gets read back at.
func storeAdjustMateScore(score, ply int) int {
	switch {
	case score >= mateThreshold:
		return score + ply
	case score <= -mateThreshold:
		return score - ply
	default:
		return score
	}
}

// readAdjustMateScore is storeAdjustMateScore's exact inverse, applied
// when a cached score is read back out of the transposition table at
// the current node's ply (which may differ from the ply it was
// originally stored at — see storeAdjustMateScore's comment).
func readAdjustMateScore(score, ply int) int {
	switch {
	case score >= mateThreshold:
		return score - ply
	case score <= -mateThreshold:
		return score + ply
	default:
		return score
	}
}

// --- Transposition table -------------------------------------------

// bound records which side of the true score a stored value
// represents, since alpha-beta search on its own only ever proves a
// position is "at least this good" (a cutoff on beta) or "at most
// this good" (nothing beat alpha) unless the full window was
// searched — only an exact-bound entry can be trusted as the position's
// actual score outright.
type bound uint8

const (
	exactBound bound = iota
	lowerBound       // stored score is a lower bound (search failed high, score >= beta)
	upperBound       // stored score is an upper bound (search failed low, score <= alpha)
)

type ttEntry struct {
	key   uint64 // full Zobrist hash this slot currently holds — see probe's comment for why this is needed once lookup is by hash&ttMask, not by the hash itself
	depth int
	score int
	bound bound
	move  board.Move
	gen   uint8 // search generation that wrote this entry — see sharedTT.gen
}

// killerSlots is how many killer moves are remembered per depth
// level. Two is the conventional choice: enough that a second good
// quiet move at a given depth still gets tried early even after the
// first killer stops applying (e.g. it's no longer legal in a
// sibling position), without the bookkeeping of a longer list.
const killerSlots = 2

// ttSize is the transposition table's slot count — a fixed power of
// two so hash&ttMask (below) is a plain bitwise AND instead of a
// modulo. 1<<20 (~1M slots, ~32MB at ttEntry's current size) is a
// reasonable default for this project's current search depths; not
// exposed as a constructor parameter yet, simply to avoid touching
// every existing NewTranspositionTable() call site (BestMove,
// BestMoveTimed, every test, uci.go) for a knob nothing currently
// needs tuned.
const ttSize = 1 << 20

// ttSlotBytes is the size of one ttSlot (two uint64 words).
const ttSlotBytes = 16

// DefaultHashMB is the transposition table size used unless a caller asks for
// another one (UCI option Hash): ttSize slots of ttSlotBytes each.
const DefaultHashMB = ttSize * ttSlotBytes >> 20

// ttSlotsForMB turns a size in megabytes into a slot count: the largest power
// of two (hash&mask indexing) that fits, at least 1<<16 slots.
func ttSlotsForMB(mb int) int {
	slots := 1 << 16
	for slots*2*ttSlotBytes <= mb<<20 {
		slots *= 2
	}
	return slots
}

// TranspositionTable caches negamax results by position hash, so a
// position reached by a different move order (a "transposition")
// doesn't get re-searched from scratch, and so the best move found for
// a position at a shallow iterative-deepening depth can be tried FIRST
// at the next, deeper iteration — a good first move is what lets
// alpha-beta actually prune effectively.
//
// Implementation: a fixed-size array (entries), indexed by
// hash&ttMask rather than keyed by the hash directly the way a map
// would be — replacing an earlier plain-map version once profiling
// showed map access (hashing + bucket probing) as a real, measurable
// chunk of total search time (see TECHNICAL.md's profiling notes).
// Slot count is much smaller than the 64-bit hash space, so two
// different positions can legitimately want the same slot — probe/
// store below handle that by storing the full hash alongside each
// entry and checking it, treating a slot that holds a DIFFERENT
// position's data as a miss rather than a wrong answer. Replacement
// is depth-preferred (see store's own comment): a deeper existing
// entry for a different position survives a shallower write rather
// than being evicted by it, since it took more search effort to
// produce and is more likely to still be useful later — a fresh
// result for the SAME position always overwrites regardless of
// depth, and an empty slot always accepts a write. Unlike the map,
// this has a fixed memory footprint that doesn't grow across a long
// search — a real improvement, not just a speed one.
//
// history is the same story: a flat array indexed by historyIndex
// instead of a map keyed by (From, To), for the same reason.
//
// sharedTT is the actual hash-indexed transposition table — the ONE
// piece of state genuinely worth sharing between threads in a future
// Lazy SMP search (see plan.md item 3): it's what lets one thread's
// work help another's. Everything that used to live directly
// alongside entries in TranspositionTable — killers, history,
// searchPath, pawnCache, nodes — stays right where it was, below,
// each already documented as "one per search, not global" specifically
// so this split costs nothing: a future multi-threaded search
// constructs one TranspositionTable per thread, all pointing at the
// same *sharedTT via the shared field, without touching any of that
// per-thread state or a single line of negamax/quiescenceSearch — every
// tt.XXX call already in this file keeps working exactly as before,
// since TranspositionTable still has all the same methods, just
// delegating probe/store to shared underneath.
//
// Not yet synchronized for concurrent access — that's the next step
// (plan.md item B). A single goroutine's use, which is everything
// this codebase does today, needs no locking at all.
type sharedTT struct {
	// entries is lock-free: each slot is two atomic words with an XOR
	// integrity check (see ttSlot and probe/store).
	entries []ttSlot
	mask    uint64 // len(entries)-1; len(entries) is a power of two

	// gen is the current search generation, bumped once per move search
	// (see nextGeneration). store lets any new entry replace one left by
	// an EARLIER generation regardless of depth: with the table kept
	// warm across a whole game, depth-only replacement would let deep
	// entries from long-gone positions squat on slots forever.
	gen atomic.Uint32
}

// nextGeneration starts a new search generation: entries written from
// now on outrank everything written before.
func (s *sharedTT) nextGeneration() {
	s.gen.Add(1)
}

func newSharedTT(size int) *sharedTT {
	return &sharedTT{entries: make([]ttSlot, size), mask: uint64(size - 1)}
}

// ttSlot is one table slot: see the packing comment above probe/store.
type ttSlot struct {
	check atomic.Uint64
	data  atomic.Uint64
}

// probe/store on sharedTT hold the exact same lookup/replacement
// logic that lived directly on TranspositionTable before the sharedTT
// split — see TranspositionTable.probe/store's own comments below for
// the full reasoning (the zero-key sentinel, the depth-preferred
// replacement policy) — with a shard lock now held around the actual
// slot access, so concurrent callers (future Lazy SMP threads) can't
// race on the same slot. Different shards never contend with each
// other, so two threads probing/storing unrelated slots proceed fully
// in parallel.
func (s *sharedTT) probe(hash uint64) (ttEntry, bool) {
	slot := &s.entries[hash&s.mask]
	data := slot.data.Load()
	if slot.check.Load()^data != hash {
		return ttEntry{}, false
	}
	return unpackTT(hash, data), true
}

func (s *sharedTT) store(hash uint64, e ttEntry) {
	slot := &s.entries[hash&s.mask]
	gen := uint8(s.gen.Load())
	oldData := slot.data.Load()
	oldKey := slot.check.Load() ^ oldData
	if oldKey != 0 && oldKey != hash && uint8(oldData>>ttGenShift) == gen &&
		int(uint8(oldData>>ttDepthShift)) > e.depth {
		return
	}
	if swTTKeep && oldKey == hash {
		oldDepth := int(uint8(oldData >> ttDepthShift))
		if e.bound != exactBound && e.depth+4 < oldDepth {
			// Same position, much shallower and not exact: keep the deep
			// entry, only mark it as seen in this generation.
			data := oldData&^(0xFF<<ttGenShift) | uint64(gen)<<ttGenShift
			slot.data.Store(data)
			slot.check.Store(hash ^ data)
			return
		}
		if e.move == (board.Move{}) {
			// No move to offer (fail-low / quiescence): keep the old one.
			const moveMask = 63<<ttFromShift | 63<<ttToShift | 7<<ttFlagShift | 7<<ttPromoShift
			e.gen = gen
			data := packTT(e)&^moveMask | oldData&moveMask
			slot.data.Store(data)
			slot.check.Store(hash ^ data)
			return
		}
	}
	e.gen = gen
	data := packTT(e)
	slot.data.Store(data)
	slot.check.Store(hash ^ data)
}

// A slot is two atomic words: data (the packed entry) and check (hash ^
// data). A reader accepts the slot only if check^data equals the hash it
// looks for, so a torn read (data from one writer, check from another)
// reads as a miss instead of a wrong answer, and no lock is needed.
//
// data layout: score 24 bits (two's complement; |score| <= Infinity is far
// inside +-8M), depth 8, from 6, to 6, flag 3, promotion 3, bound 2, gen 8.
const (
	ttDepthShift = 24
	ttFromShift  = 32
	ttToShift    = 38
	ttFlagShift  = 44
	ttPromoShift = 47
	ttBoundShift = 50
	ttGenShift   = 52
)

func packTT(e ttEntry) uint64 {
	d := e.depth
	if d < 0 {
		d = 0
	} else if d > 255 {
		d = 255
	}
	return uint64(uint32(e.score))&0xFFFFFF |
		uint64(d)<<ttDepthShift |
		uint64(e.move.From)&63<<ttFromShift |
		uint64(e.move.To)&63<<ttToShift |
		uint64(e.move.Flag)&7<<ttFlagShift |
		uint64(e.move.Promotion)&7<<ttPromoShift |
		uint64(e.bound)&3<<ttBoundShift |
		uint64(e.gen)<<ttGenShift
}

func unpackTT(hash, data uint64) ttEntry {
	return ttEntry{
		key:   hash,
		score: int(int32(uint32(data)<<8) >> 8),
		depth: int(uint8(data >> ttDepthShift)),
		move: board.Move{
			From:      board.Square(data >> ttFromShift & 63),
			To:        board.Square(data >> ttToShift & 63),
			Flag:      board.MoveFlag(data >> ttFlagShift & 7),
			Promotion: board.PieceType(data >> ttPromoShift & 7),
		},
		bound: bound(data >> ttBoundShift & 3),
		gen:   uint8(data >> ttGenShift),
	}
}

// killers also lives here, even though it's neither of the above: a
// TranspositionTable is already the one piece of state threaded
// through an entire top-level search call (BestMove/BestMoveTimed
// each construct a fresh one), which is exactly the scope both move-
// ordering aids need — reset between unrelated searches, shared
// across every node and every iterative-deepening pass within one
// search. killers is indexed by ply from the root (not remaining
// depth, which would conflate killers from different lines that
// reach the same remaining depth under LMR/extensions).
type TranspositionTable struct {
	// shared is the hash table proper — see sharedTT's own comment.
	// Every TranspositionTable constructed by NewTranspositionTable
	// today gets its own private *sharedTT (so single-threaded
	// behavior, including this file's node-count tests, is completely
	// unchanged); a future multi-threaded caller will instead point
	// several TranspositionTable instances at ONE shared *sharedTT.
	shared *sharedTT

	killers [][killerSlots]board.Move
	hist *histTables

	// moveKeys[ply] is the (piece, destination) key of the move made at
	// ply on the current line, -1 for a null move: what the continuation
	// history tables index by.
	moveKeys [256]int16

	// evalCache backs evaluate (see there); allocated lazily.
	evalCache []evalEntry

	// net is the NNUE this search evaluates with (nil: hand-written eval),
	// snapshotted by nnueReset; accStack is its accumulator per ply. See
	// nnue.go.
	net      *nnue.Network
	accStack []accEntry

	// searchPath records, at each ply along the search line CURRENTLY
	// being explored (0 at the root), the position hash reached
	// there — see recordPathHash/isRepetition below. Like killers,
	// entries at a ply the recursion has backtracked out of are
	// simply stale leftovers until the next visit to that ply
	// overwrites them (safe — see isRepetition's own comment on why
	// only shallower indices are ever consulted). Unlike killers,
	// this is written and read by BOTH negamax and quiescenceSearch:
	// a repetition — most importantly a perpetual check — can just as
	// easily occur entirely within quiescence's own recursion as in
	// the main search.
	searchPath []uint64

	// gameHistory holds the hashes of the game's positions BEFORE the
	// root, oldest first (see SetGameHistory). isRepetition reads it as
	// plies -1, -2, ... behind searchPath[0], so a line that returns to a
	// position already played in the game scores as a draw. NewSearch
	// carries it over; nil means "no history known" (a bare FEN).
	gameHistory []uint64

	// rootSkip lists root moves the search must not consider (MultiPV: the
	// lines already found are skipped to find the next best one). Empty in
	// every ordinary search, which then behaves exactly as before. See
	// SetRootSkip.
	rootSkip []board.Move

	// excl[ply] is the move a singular-extension verification search at that
	// ply leaves out (zero value: none); see negamax.
	excl [256]board.Move

	// cut[ply] tells the node about to be searched at that ply whether its
	// parent expects it to fail high (LMR reduces cut nodes more).
	cut [256]bool

	// pawnCache is eval.Evaluate's per-search pawn-structure cache —
	// see eval.PawnCache's own doc comment for what it caches and why
	// it lives here (one per top-level search call) rather than as a
	// package-level global in eval: a global would need a mutex the
	// moment this engine's search becomes multi-threaded, or worse,
	// silently leak one search's cached scores into a different,
	// concurrent one.
	pawnCache *eval.PawnCache

	// nodes counts every negamax/quiescenceSearch call made using this
	// table, across however many iterative-deepening passes share it —
	// exposed to callers via SearchInfo.Nodes, for UCI's "info ...
	// nodes N" line. Deliberately NOT reset between BestMoveTimed's
	// depth-by-depth passes (only a brand new TranspositionTable, at
	// the start of a whole BestMove/BestMoveTimed call, starts this
	// back at 0) — a running total across the whole search is what UCI
	// GUIs and "nodes per second" displays actually expect.
	nodes int64

	// deadline/timed/stopped implement the mid-search time cutoff — see
	// shouldStop. timed is off for fixed-depth searches (and for depth 1
	// of iterative deepening, which must always finish to have a move to
	// return), so those never abort. stopped is sticky once set: every
	// frame of the recursion sees it and unwinds without storing
	// anything, and runIterativeDeepening then discards the unfinished
	// iteration.
	deadline time.Time
	timed    bool
	stopped  bool

	// rootPartial is the latest root move of the running iteration that
	// completed and raised alpha (rootPartialOK says one exists); the
	// iterative-deepening loop reads it when the iteration is cut off.
	rootPartial   board.Move
	rootPartialOK bool
}

// shouldStop reports whether the search must unwind now. It reads the
// clock only every 2048 nodes, so the check costs next to nothing. Safe
// on a nil receiver (negamax/quiescence accept a nil table).
func (tt *TranspositionTable) shouldStop() bool {
	if tt == nil || !tt.timed {
		return false
	}
	if !tt.stopped && tt.nodes&2047 == 0 && !time.Now().Before(tt.deadline) {
		tt.stopped = true
	}
	return tt.stopped
}

// aborted reports whether a cutoff has already fired, without touching
// the clock — used after a child call returns, where its score is
// garbage and must not be used or stored.
func (tt *TranspositionTable) aborted() bool {
	return tt != nil && tt.stopped
}

// NewTranspositionTable returns an empty table, with its own private
// hash table (see sharedTT's field comment above).
func NewTranspositionTable() *TranspositionTable {
	return NewTranspositionTableMB(DefaultHashMB)
}

// NewTranspositionTableMB is NewTranspositionTable with a hash table of about
// mb megabytes (rounded down to a power-of-two slot count).
func NewTranspositionTableMB(mb int) *TranspositionTable {
	return &TranspositionTable{
		shared:    newSharedTT(ttSlotsForMB(mb)),
		hist:      newHistTables(),
		pawnCache: eval.NewPawnCache(),
	}
}

// SetRootSkip makes the next searches ignore the given moves at the root, so
// the best move of what is left is found. The caller keeps at least one legal
// move out of the list. Pass nil to search everything again.
func (tt *TranspositionTable) SetRootSkip(ms []board.Move) { tt.rootSkip = ms }

// skipsRoot says whether m is one of the root moves to ignore.
func (tt *TranspositionTable) skipsRoot(m board.Move) bool {
	for _, s := range tt.rootSkip {
		if s == m {
			return true
		}
	}
	return false
}

// pawnCacheOrNil safely fetches tt's pawn cache whether tt itself is a
// real table or nil — negamax/quiescenceSearch are both callable with
// a nil *TranspositionTable (see TestNegamaxNilTableStillWorks), and
// calling a pointer-receiver method on a nil receiver is fine in Go as
// long as the method itself checks for nil before dereferencing,
// which this does.
// NewSearch returns a fresh TranspositionTable that shares tt's
// underlying hash table (the actual position→score/move cache) but
// starts with empty killers/history/searchPath and a zeroed node
// count — exactly the same "share the hash table, reset everything
// else" split Lazy SMP already uses between concurrent threads (see
// BestMoveTimedMT's own comment), applied here across TIME instead of
// across threads. This is what lets a caller like the UCI session
// keep ONE hash table warm across an entire game's worth of separate
// move searches — a real, not hypothetical, gain: the opponent's
// actual reply is very often a position this engine already explored
// on its own immediately preceding move — while still starting each
// individual search with a clean slate for killers/history/nodes,
// which are meant to reflect the CURRENT search's own tree shape, not
// linger from a different position several moves back.
func (tt *TranspositionTable) NewSearch() *TranspositionTable {
	tt.shared.nextGeneration()
	hist := tt.hist
	if !swHistKeep || hist == nil {
		hist = newHistTables()
	}
	return &TranspositionTable{
		shared:      tt.shared,
		hist:        hist,
		pawnCache:   eval.NewPawnCache(),
		gameHistory: tt.gameHistory,
	}
}

// SetGameHistory tells the search which positions the game has already
// been through: hashes is every position from the start of the game up
// to and INCLUDING the one about to be searched (the slice the UCI
// session, cmd/cli -play and cmd/wasm keep for GameStatusWithHistory).
// Without it the search only sees repetitions inside its own tree, and a
// side that is winning walks straight into a threefold repetition of the
// game. Searches started from tt (directly or via NewSearch) use it until
// the next call; nil clears it.
func (tt *TranspositionTable) SetGameHistory(hashes []uint64) {
	if len(hashes) == 0 {
		tt.gameHistory = nil
		return
	}
	tt.gameHistory = append([]uint64(nil), hashes[:len(hashes)-1]...)
}

func (tt *TranspositionTable) pawnCacheOrNil() *eval.PawnCache {
	if tt == nil {
		return nil
	}
	return tt.pawnCache
}

// probe/store delegate straight to shared — see sharedTT.probe/store
// above for the actual lookup/replacement logic. Kept as methods on
// TranspositionTable, unchanged in name and signature from before this
// split, so every existing tt.probe(...)/tt.store(...) call site in
// this file needed no edit at all.
func (tt *TranspositionTable) probe(hash uint64) (ttEntry, bool) {
	return tt.shared.probe(hash)
}

func (tt *TranspositionTable) store(hash uint64, e ttEntry) {
	tt.shared.store(hash, e)
}

// killersAt returns the killer-move slots for a given ply from the root
// (killersAt), growing the backing slice on demand — plies are small,
// bounded ints (rarely more than a couple dozen even with iterative
// deepening), so a plain growable slice is simpler than a map here.
func (tt *TranspositionTable) killersAt(ply int) *[killerSlots]board.Move {
	for len(tt.killers) <= ply {
		tt.killers = append(tt.killers, [killerSlots]board.Move{})
	}
	return &tt.killers[ply]
}

// recordKiller notes that m (a quiet move) caused a beta cutoff at
// this ply, pushing it into slot 0 and bumping the previous slot-0
// killer down to slot 1 — unless m is already slot 0, in which case
// there's nothing to do (repeatedly re-recording the same killer
// shouldn't demote it).
func (tt *TranspositionTable) recordKiller(ply int, m board.Move) {
	slots := tt.killersAt(ply)
	if slots[0] == m {
		return
	}
	slots[1] = slots[0]
	slots[0] = m
}

// maxHistory bounds every history score to [-maxHistory, maxHistory].
const maxHistory = 8000

func historyBonus(depth int) int {
	b := depth * depth * historyBonusMul
	if b > historyBonusCap {
		b = historyBonusCap
	}
	return b
}

// recordHistory rewards a quiet move by mover p that caused a cutoff,
// penalizeHistory is its counterpart for quiet moves tried before the cutoff.
// ply gives the continuation context (the moves made earlier on this line).
func (tt *TranspositionTable) recordHistory(p board.Piece, m board.Move, ply, depth int) {
	b1, b2 := tt.contBases(ply)
	tt.updateQuietHist(p, m, b1, b2, historyBonus(depth))
}

func (tt *TranspositionTable) penalizeHistory(p board.Piece, m board.Move, ply, depth int) {
	b1, b2 := tt.contBases(ply)
	tt.updateQuietHist(p, m, b1, b2, -historyBonus(depth))
}

func (tt *TranspositionTable) historyScore(p board.Piece, m board.Move, ply int) int {
	b1, b2 := tt.contBases(ply)
	return tt.quietHist(p, m, b1, b2)
}

// recordPathHash notes that hash is this node's position, at this
// ply, along the search line currently being explored — see
// isRepetition's own comment for what this is used for. Grows the
// backing slice on demand, same as killersAt: ply is a small, bounded
// int (bounded by depth plus quiescence's own check-extension
// budget), so a plain growable slice is simpler than anything
// fancier here.
func (tt *TranspositionTable) recordPathHash(ply int, hash uint64) {
	for len(tt.searchPath) <= ply {
		tt.searchPath = append(tt.searchPath, 0)
	}
	tt.searchPath[ply] = hash
}

// isRepetition reports whether hash already occurred at a STRICTLY
// SHALLOWER ply along the CURRENT search line — i.e. the move that
// led here would repeat a position this very line has already passed
// through once (Zobrist hashes fold in castling rights/en passant/
// side to move, not just piece placement, so an equal hash really
// does mean an equal position). That alone makes the position — and
// everything reachable from it — a forced draw, independent of
// static eval or of whatever a TT entry for this exact hash might
// say (see negamax's own comment on checking this BEFORE probing the
// TT).
//
// Below ply 0 the line continues into the game itself: gameHistory (see
// SetGameHistory) holds the positions played before the root, so a line
// that returns to one of them is a draw too. Without that, a side with a
// winning score shuffled straight into a threefold repetition of the
// game (the search never knew the position had been seen). Returning to
// a game position once counts as the draw, as with in-tree repetitions:
// the opponent can repeat it again. The root itself (ply 0) is never
// checked — the search must still return a move there.
//
// Inside the tree, only indices strictly below ply are consulted, which
// the depth-first recursion guarantees hold exactly this line's own
// ancestors (see recordPathHash and searchPath's own field comment)
// — never a sibling's, nor an earlier iterative-deepening pass's,
// leftover value at ply itself or deeper.
func (tt *TranspositionTable) isRepetition(ply int, hash uint64, halfmoveClock int) bool {
	limit := ply
	if limit > len(tt.searchPath) {
		limit = len(tt.searchPath)
	}
	// A pawn move or capture can never be undone, so no position from before
	// the last one (halfmoveClock plies back) can occur again.
	start := ply - halfmoveClock
	for i := max(start, 0); i < limit; i++ {
		if tt.searchPath[i] == hash {
			return true
		}
	}
	// The window reaches back past the root: positions -1, -2, ... are the
	// game's, newest last in gameHistory.
	if ply > 0 && start < 0 {
		n := len(tt.gameHistory)
		for i := max(n+start, 0); i < n; i++ {
			if tt.gameHistory[i] == hash {
				return true
			}
		}
	}
	return false
}

// evalCacheBits sizes the static-eval cache: 1<<bits entries per search
// thread (16 bytes each), allocated on first use.
const evalCacheBits = 16

type evalEntry struct {
	key   uint64
	score int32
}

// evaluate is the static eval of b, the position at ply on the current
// line — the NNUE when this search has one (see nnue.go), eval.Evaluate
// otherwise — behind a small direct-mapped cache keyed by the position's
// Zobrist hash. Both depend only on the position, so a hit is exact; the
// same position is reached again through transpositions and across
// iterative-deepening passes. Safe on a nil receiver (hand-written eval,
// no caching).
func (tt *TranspositionTable) evaluate(b *board.Board, ply int) int {
	if tt == nil {
		return eval.Evaluate(b, nil)
	}
	s := tt.evaluateRaw(b, ply)
	if swEvalScale && tt.net != nil && b.HalfmoveClock > 0 {
		s = s * (evalR50Base - min(b.HalfmoveClock, evalR50Base)) / evalR50Base
	}
	return s
}

// scaleMaterial damps the net's score where it is least reliable: with little
// material left (draws get likelier). The eval cache keeps the damped score;
// evaluate adds the fifty-move damping, which the position hash does not
// cover, on every read.
func scaleMaterial(s int, b *board.Board) int {
	m := 0 // non-pawn material of both sides: N, B = 1, R = 2, Q = 4 (24 at the start)
	for c := board.White; c <= board.Black; c++ {
		m += b.Pieces(board.Knight, c).Count() + b.Pieces(board.Bishop, c).Count() +
			2*b.Pieces(board.Rook, c).Count() + 4*b.Pieces(board.Queen, c).Count()
	}
	m = min(m, 24)
	return s * (evalMatBase + m) / (evalMatBase + 24)
}

func (tt *TranspositionTable) evaluateRaw(b *board.Board, ply int) int {
	if tt.evalCache == nil {
		tt.evalCache = make([]evalEntry, 1<<evalCacheBits)
	}
	h := b.Hash()
	e := &tt.evalCache[h&(1<<evalCacheBits-1)]
	if e.key == h {
		return int(e.score)
	}
	var s int
	if tt.net != nil {
		s = tt.nnueEvaluate(b, ply)
		if swEvalScale {
			s = scaleMaterial(s, b)
		}
	} else {
		s = eval.Evaluate(b, tt.pawnCache)
	}
	e.key, e.score = h, int32(s)
	return s
}

// setCut records, before a recursive call, whether the child at ply is a cut
// node. A nil table (plain negamax) ignores it.
func (tt *TranspositionTable) setCut(ply int, v bool) {
	if tt != nil && ply < len(tt.cut) {
		tt.cut[ply] = v
	}
}

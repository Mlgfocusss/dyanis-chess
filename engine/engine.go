// Package engine is the only door between the Dyanis chess engine and the
// rest of the project (backend/, and through it the web site).
//
// Everything the engine is made of (board, move generator, evaluation,
// neural network, search, opening books, UCI) lives under engine/internal
// and the Go compiler refuses imports of it from outside engine/. Code
// elsewhere asks the engine for exactly two things, both defined here:
//
//   - the rules of chess: positions, FEN, legal moves, SAN, game status;
//   - a search: "how good is this position and what is the best move".
//
// To replace the engine with another one (a rewrite, or a UCI program
// started as a subprocess), keep this file's exported API and change what
// is behind it. Nothing in backend/ has to change. The contract is
// documented in engine/README.md.
package engine

import (
	"errors"
	"time"

	"dyanis-chess/engine/internal/board"
	"dyanis-chess/engine/internal/movegen"
	"dyanis-chess/engine/internal/nnue"
	"dyanis-chess/engine/internal/search"
)

// StartFEN is the standard initial position.
const StartFEN = board.StartFEN

// MateScore is the magnitude of a score for "checkmate right now".
// Result.Score values at or beyond MateScore-1000 mean a forced mate:
// MateScore-n is a mate delivered in n plies (n half-moves).
const MateScore = search.MateScore

// Color is a side.
type Color int

const (
	White Color = iota
	Black
)

// Move is one legal move, opaque to callers. Its String form is UCI
// ("e2e4", "e7e8q"); Moves can be compared with ==.
type Move struct{ m board.Move }

// String returns the move in UCI notation.
func (m Move) String() string { return m.m.String() }

// Position is a chess position. The zero value is not usable; start from
// NewPosition or ParseFEN. A Position is not safe for concurrent use;
// Copy it for another goroutine.
type Position struct{ b *board.Board }

// NewPosition returns the standard initial position.
func NewPosition() *Position { return &Position{b: board.NewInitialBoard()} }

// ParseFEN builds a position from a FEN string.
func ParseFEN(fen string) (*Position, error) {
	b, err := board.FromFEN(fen)
	if err != nil {
		return nil, err
	}
	return &Position{b: b}, nil
}

// FEN returns the position as FEN.
func (p *Position) FEN() string { return p.b.ToFEN() }

// Hash returns a 64-bit hash of the position (pieces, side to move,
// castling rights, en-passant square only when a capture is possible).
// Equal positions have equal hashes; it is used to find transpositions
// and repetitions, so it must be the same on every call and every run.
func (p *Position) Hash() uint64 { return p.b.Hash() }

// Copy returns an independent copy.
func (p *Position) Copy() *Position { return &Position{b: p.b.Copy()} }

// SideToMove is the side whose turn it is.
func (p *Position) SideToMove() Color {
	if p.b.SideToMove == board.Black {
		return Black
	}
	return White
}

// LegalMoves lists every legal move.
func (p *Position) LegalMoves() []Move {
	ms := movegen.GenerateLegalMoves(p.b)
	out := make([]Move, len(ms))
	for i, m := range ms {
		out[i] = Move{m}
	}
	return out
}

// ParseSAN resolves a move written in standard algebraic notation ("Nf3",
// "exd5", "O-O", "e8=Q+"; check marks and annotations are tolerated). It
// fails if the text matches no legal move or more than one.
func (p *Position) ParseSAN(s string) (Move, error) {
	m, err := movegen.ParseSAN(p.b, s)
	return Move{m}, err
}

// SAN writes a legal move of this position in normalized SAN, with the
// check or mate suffix ("Nf3", "exd5", "O-O", "e8=Q#").
func (p *Position) SAN(m Move) string { return movegen.SAN(p.b, m.m) }

// Play makes a legal move of this position. Passing a move that is not
// legal here is a programming error.
func (p *Position) Play(m Move) { p.b.MakeMove(m.m) }

// Outcome says whether the game is over in a position.
type Outcome struct {
	Over      bool
	Checkmate bool   // the side to move is checkmated (implies Over)
	Reason    string // "ongoing", "checkmate", "stalemate", "draw by ...", for display
}

// Outcome judges the position. history lists the hashes of the positions
// that occurred earlier in the game plus this one (last), so that
// threefold repetition is seen; it may be nil.
func (p *Position) Outcome(history []uint64) Outcome {
	st := movegen.GameStatusWithHistory(p.b, history)
	return Outcome{Over: st.IsGameOver(), Checkmate: st == movegen.Checkmate, Reason: st.String()}
}

// Limits bounds a search.
type Limits struct {
	// Depth, if > 0, searches exactly to this depth however long it takes:
	// reproducible, the same answer on every machine.
	Depth int
	// MoveTime and MaxDepth bound a timed search (used when Depth is 0):
	// iterative deepening until MoveTime has passed or MaxDepth is reached.
	MoveTime time.Duration
	MaxDepth int
	// HashMB is the memory for the search's tables, in megabytes.
	HashMB int
}

// Result is what a search found.
type Result struct {
	Best Move
	// Score is in centipawns from the point of view of the side to move
	// (positive: good for the side to move). A forced mate is reported as
	// +-(MateScore-n), see MateScore.
	Score int
	Depth int   // depth of the last completed iteration
	Nodes int64 // positions searched
	// PV is the principal variation: the best line of play, starting with
	// Best. May be empty.
	PV []Move
}

// Search looks for the best move in p. history is as in Outcome. It is
// safe to call from several goroutines at once (each call searches its own
// copy of p), and a single call uses one thread. It fails if p has no
// legal move.
func Search(p *Position, history []uint64, lim Limits) (Result, error) {
	work := p.b.Copy()
	tt := search.NewTranspositionTableMB(lim.HashMB)
	if len(history) > 0 {
		tt.SetGameHistory(history)
	}
	var last search.SearchInfo
	onInfo := func(si search.SearchInfo) { last = si }

	maxDepth, budget := lim.MaxDepth, lim.MoveTime
	if lim.Depth > 0 {
		maxDepth, budget = lim.Depth, 24*time.Hour
	}
	best, err := search.BestMoveTimedInfoTT(work, maxDepth, budget, tt, onInfo)
	if err != nil {
		return Result{}, err
	}
	res := Result{Best: Move{best}, Score: last.Score, Depth: last.Depth, Nodes: last.Nodes}
	for _, m := range last.PV {
		res.PV = append(res.PV, Move{m})
	}
	return res, nil
}

// LoadNetwork loads NNUE weights from a file and makes every later Search
// in this process use them. Without it the engine uses its hand-written
// evaluation. Call it once at start-up, before searching.
func LoadNetwork(path string) error {
	if path == "" {
		return errors.New("engine: empty network path")
	}
	n, err := nnue.Load(path)
	if err != nil {
		return err
	}
	search.SetNetwork(n)
	return nil
}

// Package uci implements enough of the Universal Chess Interface
// protocol (https://www.chessprogramming.org/UCI) to let the engine be
// driven from a GUI like Arena or CuteChess, or from any script that
// talks UCI over stdin/stdout.
//
// Supported commands:
//
//	uci          -> "id name ...", "id author ...", "option name
//	                Threads ...", "uciok"
//	isready      -> "readyok"
//	ucinewgame   -> reset to the starting position
//	position ... -> set up a position (startpos or fen) plus a move list
//	setoption    -> "setoption name Threads value N" sets the thread
//	                count movetime-based "go" searches use (Lazy SMP —
//	                see search.BestMoveTimedMT); any other option name
//	                is silently ignored, same policy as an unrecognized
//	                command below
//	go ...       -> search, streaming "info depth/score/nodes/pv" lines
//	                as it goes, then reply "bestmove <move>"
//	quit         -> stop the loop
//
// "go" without an explicit "depth" uses the fixed depth Loop was given
// (mirroring -depth in the CLI's -play mode); "go movetime N" gives a
// flat per-move budget, and "go wtime/btime/winc/binc/movestogo" is
// turned into one from the side-to-move's clock (infinite is accepted
// and ignored). Any
// other unrecognized command (stop, ponderhit, debug, ...) is
// silently ignored per the UCI spec, rather than treated as an error,
// so a GUI sending extra commands this engine doesn't yet support
// can't break the session.
package uci

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"time"

	"dyanis-chess/engine/internal/board"
	"dyanis-chess/engine/internal/book"
	"dyanis-chess/engine/internal/movegen"
	"dyanis-chess/engine/internal/nnue"
	"dyanis-chess/engine/internal/search"
)

const (
	engineName = "Dyanis"
	// TODO: put your own name here before publishing this under UCI —
	// "id author" is meant to identify a real person/team, not a
	// placeholder.
	engineAuthor = "Your Name"
)

// session holds the state a UCI conversation accumulates between
// commands: the current position, the fixed search depth (Loop's
// depth parameter, overridable per-"go" via "go depth N"), and the
// opening book (may be nil).
type session struct {
	pos     *board.Board
	depth   int
	threads int
	book    book.Source
	w       *bufio.Writer

	// tt is the transposition table kept warm ACROSS separate "go"
	// calls, i.e. across a whole game's worth of moves — see
	// search.TranspositionTable.NewSearch's own comment for the
	// reasoning (the opponent's actual reply is very often a position
	// this engine already explored on its own immediately preceding
	// move). Every individual search gets its own fresh view via
	// tt.NewSearch() (single-threaded paths) or
	// search.BestMoveTimedMTShared (multi-threaded), never tt itself
	// directly — only the underlying hash table persists, not
	// killers/history/node counts from an unrelated earlier search.
	// Reset on "ucinewgame", same as pos and history below; NOT reset
	// by "position" alone, since that's just how a GUI resends the
	// current game's state, not a signal of a new, unrelated game.
	tt *search.TranspositionTable

	// hashMB is the size of tt in megabytes ("setoption name Hash"); a new
	// table of this size is created on Hash changes and on "ucinewgame".
	hashMB int

	// history holds the Zobrist hash of every position in the current
	// game, in order, INCLUDING pos itself — needed by
	// GameStatusWithHistory's threefold-repetition check (see
	// handleGo). Unlike cmd/cli and cmd/wasm, this is NOT built up
	// incrementally move by move: a UCI GUI resends the position from
	// scratch on every "position" command (startpos/fen plus the FULL
	// move list since then, not just new moves — see handlePosition),
	// so history is simplest rebuilt from scratch alongside pos every
	// time, rather than trying to diff against whatever the previous
	// "position" command left behind.
	history []uint64
}

// Loop reads UCI commands from r, one per line, and writes responses
// to w, until "quit" is received or r hits EOF/an error. depth is the
// fixed search depth used for "go" (and "go depth N" without a valid
// N); threads is the STARTING thread count for movetime-based
// searches (Lazy SMP — plan.md item 3), overridable at runtime via
// "setoption name Threads value N" the same way depth can be
// overridden per-"go" — values below 1 are clamped up to 1. bk may be
// nil to play without an opening book (a *book.Book or a *book.Chain
// both satisfy book.Source — see internal/book/book.go — so a
// GUI-configured multi-book chain works here exactly as it does in
// the CLI and wasm builds). Loop starts from the standard starting
// position; "position" changes that.
func Loop(r *bufio.Reader, w *bufio.Writer, depth, threads int, bk book.Source) {
	if threads < 1 {
		threads = 1
	}
	start := board.NewInitialBoard()
	s := &session{pos: start, depth: depth, threads: threads, book: bk, w: w, tt: search.NewTranspositionTable(), hashMB: search.DefaultHashMB, history: []uint64{start.Hash()}}

	for {
		line, readErr := r.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" && !s.handle(line) {
			return
		}
		if readErr != nil {
			return // stdin closed or errored: behave like "quit"
		}
	}
}

// handle dispatches one command line. It returns false when the
// session should stop (i.e. "quit" was received).
func (s *session) handle(line string) bool {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return true
	}

	switch fields[0] {
	case "uci":
		fmt.Fprintf(s.w, "id name %s\n", engineName)
		fmt.Fprintf(s.w, "id author %s\n", engineAuthor)
		fmt.Fprintf(s.w, "option name Threads type spin default %d min 1 max %d\n", s.threads, maxUCIThreads)
		fmt.Fprintf(s.w, "option name Hash type spin default %d min 1 max %d\n", search.DefaultHashMB, maxUCIHashMB)
		fmt.Fprint(s.w, "option name EvalFile type string default <empty>\n")
		for _, t := range search.Tunables() {
			fmt.Fprintf(s.w, "option name %s type spin default %d min %d max %d\n", t.Name, t.Default, t.Min, t.Max)
		}
		fmt.Fprint(s.w, "uciok\n")
		s.w.Flush()

	case "isready":
		fmt.Fprint(s.w, "readyok\n")
		s.w.Flush()

	case "ucinewgame":
		start := board.NewInitialBoard()
		s.pos = start
		s.tt = search.NewTranspositionTableMB(s.hashMB)
		s.history = []uint64{start.Hash()}

	case "position":
		s.handlePosition(fields[1:])

	case "setoption":
		s.handleSetOption(fields[1:])

	case "go":
		s.handleGo(fields[1:])

	case "quit":
		return false
	}
	return true
}

// maxUCIThreads is the ceiling advertised in the "Threads" spin
// option and enforced by handleSetOption — a generous but finite
// bound (no real machine has anywhere near this many useful cores) so
// a GUI can't be sent into requesting something absurd by a typo.
const maxUCIThreads = 256

// maxUCIHashMB caps the "Hash" option (megabytes of transposition table).
const maxUCIHashMB = 4096

// handleSetOption implements "setoption name <name> value <value>".
// Only "Threads" is recognized today; anything else (Hash, Ponder,
// ...) is silently ignored — same "never treat an unrecognized
// command as an error" policy Loop's own doc comment already
// describes for whole commands, extended here to unrecognized
// OPTIONS specifically, since a GUI routinely sends setoption for
// options this engine never declared.
func (s *session) handleSetOption(args []string) {
	// Expected shape: ["name", "Threads", "value", "4"]. Scanned
	// rather than indexed positionally past "name", since UCI in
	// general allows a multi-word option name before "value" — none
	// of the options this engine declares need that, but the scan
	// costs nothing and is one less assumption to get wrong.
	if len(args) < 2 || args[0] != "name" {
		return
	}
	name := args[1]

	valueIdx := -1
	for i := 2; i < len(args); i++ {
		if args[i] == "value" {
			valueIdx = i + 1
			break
		}
	}
	if valueIdx == -1 || valueIdx >= len(args) {
		return
	}

	switch name {
	case "Threads":
		n, err := strconv.Atoi(args[valueIdx])
		if err != nil || n < 1 {
			return
		}
		if n > maxUCIThreads {
			n = maxUCIThreads
		}
		s.threads = n
	case "Hash":
		n, err := strconv.Atoi(args[valueIdx])
		if err != nil || n < 1 {
			return
		}
		if n > maxUCIHashMB {
			n = maxUCIHashMB
		}
		if n != s.hashMB {
			s.hashMB = n
			s.tt = search.NewTranspositionTableMB(n)
		}
	case "EvalFile":
		// The path may contain spaces. "<empty>" (the advertised default)
		// switches back to the hand-written eval. A file that fails to
		// load keeps whatever was in use and says why. A change of
		// evaluator starts a fresh hash table: the old one holds scores
		// the new evaluator would not give.
		path := strings.Join(args[valueIdx:], " ")
		if path == "<empty>" {
			if search.Network() != nil {
				search.SetNetwork(nil)
				s.tt = search.NewTranspositionTableMB(s.hashMB)
			}
			return
		}
		n, err := nnue.Load(path)
		if err != nil {
			fmt.Fprintf(s.w, "info string EvalFile: %v\n", err)
			s.w.Flush()
			return
		}
		search.SetNetwork(n)
		s.tt = search.NewTranspositionTableMB(s.hashMB)
		fmt.Fprintf(s.w, "info string EvalFile: %s, hidden %d, %v\n", path, n.Hidden, n.Activation)
		s.w.Flush()
	default:
		// Search tuning parameters (see search.Tunables); unknown names are ignored.
		if n, err := strconv.Atoi(args[valueIdx]); err == nil {
			search.SetTunable(name, n)
		}
	}
}

// handlePosition implements:
//
//	position startpos [moves m1 m2 ...]
//	position fen <6 FEN fields> [moves m1 m2 ...]
//
// Moves are matched against the position's own legal move list by
// exact coordinate-notation string ("e2e4", "e7e8q" for promotion) —
// that's the format UCI GUIs send and the same format board.Move
// already formats itself as, so no separate parser is needed here
// (unlike SAN input in the CLI's -play, which is a human-typing
// convenience UCI doesn't need).
func (s *session) handlePosition(args []string) {
	if len(args) == 0 {
		return
	}

	var pos *board.Board
	i := 0

	switch args[0] {
	case "startpos":
		pos = board.NewInitialBoard()
		i = 1

	case "fen":
		var fenFields []string
		i = 1
		for i < len(args) && args[i] != "moves" {
			fenFields = append(fenFields, args[i])
			i++
		}
		p, err := board.FromFEN(strings.Join(fenFields, " "))
		if err != nil {
			return // malformed "position" from the GUI: ignore, keep old state
		}
		pos = p

	default:
		return
	}

	// history is rebuilt from scratch alongside pos (see session's doc
	// comment on the field) — starts with this position's own hash,
	// then gains one more entry per move successfully applied below.
	history := []uint64{pos.Hash()}

	if i < len(args) && args[i] == "moves" {
		for _, token := range args[i+1:] {
			m, ok := findMoveByCoordinates(pos, token)
			if !ok {
				return // stop applying at the first move that doesn't match
			}
			pos.MakeMove(m)
			history = append(history, pos.Hash())
		}
	}

	s.pos = pos
	s.history = history
}

func findMoveByCoordinates(pos *board.Board, token string) (board.Move, bool) {
	for _, m := range movegen.GenerateLegalMoves(pos) {
		if m.String() == token {
			return m, true
		}
	}
	return board.Move{}, false
}

// handleGo implements "go [depth N] [movetime N] [... other params,
// ignored]": it streams an "info depth D score S nodes N pv ..." line
// per completed depth (via search.BestMoveWithBookInfo /
// search.BestMoveTimedWithBookInfo's onInfo callback — see writeInfo
// below) and finally replies "bestmove <move>". If bk has a book move
// for the current position, that's played instead of running search at
// all — same book-first behavior as the CLI's -play — in which case no
// "info" line is sent at all, since no search ran to report on.
//
// If movetime is given (milliseconds), this uses iterative deepening
// with that as a time budget, with depth (explicit "depth N" if also
// given, else the session default) as the max-depth ceiling — the
// same relationship -movetime/-depth have in the CLI's -play. Without
// movetime, it's a fixed-depth search as before (still just ONE "info"
// line, since there's only ever the one depth to report). With
// wtime/btime (+ winc/binc/movestogo) and no movetime, the side to
// move's clock is turned into a per-move budget (clockBudgetMs);
// infinite isn't implemented and is silently ignored.
func (s *session) handleGo(args []string) {
	depth := s.depth
	explicitDepth := false
	movetimeMs := 0
	var wtime, btime, winc, binc, movesToGo int

	for i := 0; i < len(args); i++ {
		// Every parameter handled here takes exactly one integer value.
		var dst *int
		switch args[i] {
		case "depth":
			dst = &depth
		case "movetime":
			dst = &movetimeMs
		case "wtime":
			dst = &wtime
		case "btime":
			dst = &btime
		case "winc":
			dst = &winc
		case "binc":
			dst = &binc
		case "movestogo":
			dst = &movesToGo
		default:
			continue
		}
		if i+1 < len(args) {
			if v, err := strconv.Atoi(args[i+1]); err == nil {
				*dst = v
				if args[i] == "depth" {
					explicitDepth = true
				}
			}
			i++
		}
	}

	// Game clock: no explicit movetime, but the side to move has time
	// left — derive a per-move budget from it. The depth ceiling is lifted
	// (unless "depth N" was also given) so the budget, not the session's
	// fixed depth, is what ends the search.
	if movetimeMs == 0 {
		timeMs, incMs := wtime, winc
		if s.pos.SideToMove == board.Black {
			timeMs, incMs = btime, binc
		}
		if timeMs > 0 {
			movetimeMs = clockBudgetMs(timeMs, incMs, movesToGo)
			if !explicitDepth {
				depth = maxClockDepth
			}
		}
	}

	// Informational only: UCI leaves the actual draw ruling to the
	// GUI/arbiter, not the engine, so this never withholds bestmove —
	// search still runs and a legal move (if any) still gets sent
	// below. This just surfaces what GameStatusWithHistory already
	// knows, the same awareness cmd/cli and cmd/wasm act on directly,
	// so a GUI operator watching engine output isn't left wondering
	// why the engine keeps happily shuffling in a position that's
	// actually already drawn.
	if status := movegen.GameStatusWithHistory(s.pos, s.history); status.IsDraw() {
		fmt.Fprintf(s.w, "info string %s\n", status)
		s.w.Flush()
	}

	onInfo := func(info search.SearchInfo) {
		s.writeInfo(info)
	}

	s.tt.SetGameHistory(s.history)
	var m board.Move
	var err error
	if movetimeMs > 0 {
		m, _, err = search.BestMoveTimedWithBookMTShared(s.pos, depth, time.Duration(movetimeMs)*time.Millisecond, s.threads, s.tt, s.book, onInfo)
	} else {
		m, _, err = search.BestMoveWithBookInfoTT(s.pos, depth, s.tt.NewSearch(), s.book, onInfo)
	}
	if err != nil {
		return // no legal move (checkmate/stalemate): nothing sensible to send
	}
	fmt.Fprintf(s.w, "bestmove %s\n", m)
	s.w.Flush()
}

// maxClockDepth is the depth ceiling for clock-driven searches: far
// beyond anything reachable in a move's time, so only the budget stops
// the search.
const maxClockDepth = 64

// Clock-budget constants. defaultMovesToGo is the assumed number of
// moves left when the GUI doesn't send movestogo; overheadMs is held
// back from every move for GUI/network latency; the budget never takes
// more than 1/maxClockShare of the remaining time on a single move.
const (
	defaultMovesToGo = 30
	overheadMs       = 30
	maxClockShare    = 3
)

// clockBudgetMs turns the side-to-move's remaining time and increment
// (ms) into a per-move search budget: an even share of the time left
// plus most of the increment, capped to a fraction of the clock and
// reduced by the latency overhead, never below 1 ms.
func clockBudgetMs(timeMs, incMs, movesToGo int) int {
	if movesToGo <= 0 {
		movesToGo = defaultMovesToGo
	}
	budget := timeMs/movesToGo + incMs*3/4
	if limit := timeMs / maxClockShare; budget > limit {
		budget = limit
	}
	budget -= overheadMs
	if budget < 1 {
		budget = 1
	}
	return budget
}

// writeInfo renders one SearchInfo as a UCI "info" line — depth,
// score (as "cp N" or "mate N", see search.ScoreToUCI), running node
// count, and the principal variation as space-separated coordinate
// moves (e.g. "e2e4 e7e5 g1f3") — then flushes immediately, the same
// as every other line this session writes, so a GUI watching stdout
// sees search progress as it happens rather than buffered until
// "bestmove". pv is omitted entirely (rather than printed empty) when
// the transposition table couldn't reconstruct one — see
// principalVariation's comment in search.go for when that happens.
func (s *session) writeInfo(info search.SearchInfo) {
	line := fmt.Sprintf("info depth %d score %s nodes %d", info.Depth, search.ScoreToUCI(info.Score), info.Nodes)
	if len(info.PV) > 0 {
		pvStrs := make([]string, len(info.PV))
		for i, m := range info.PV {
			pvStrs[i] = m.String()
		}
		line += " pv " + strings.Join(pvStrs, " ")
	}
	fmt.Fprintln(s.w, line)
	s.w.Flush()
}

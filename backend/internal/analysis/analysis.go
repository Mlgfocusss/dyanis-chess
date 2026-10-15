// Package analysis reviews a finished game with the engine: it evaluates
// every position of the game, turns the evaluation swings into a
// classification of each move (best / excellent / good / inaccuracy /
// mistake / blunder) and computes a per-player accuracy.
//
// The method is the usual one. A game of N moves has N+1 positions.
// Each position is searched once; the score of position i is the value
// of the best move there (what the player to move could have had), and
// the score of position i+1 is the value of the move actually played
// (seen from the other side). The drop between the two, measured not
// in centipawns but in win probability, is how much the move cost.
// Win probability is used because 100 centipawns matter hugely in an
// equal position and almost nothing in a position that is already won.
package analysis

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"

	"dyanis-chess/backend/internal/eco"
	"dyanis-chess/backend/internal/pgn"
	"dyanis-chess/engine"
)

// Class is the quality label of one move.
type Class string

const (
	Best       Class = "best"       // the engine's own choice (or the only legal move)
	Excellent  Class = "excellent"  // costs under 2 points of win probability
	Good       Class = "good"       // under 5
	Inaccuracy Class = "inaccuracy" // 5 to 10
	Mistake    Class = "mistake"    // 10 to 20
	Blunder    Class = "blunder"    // 20 or more
	// Book marks opening theory: the move leads to a position named in the
	// ECO reference (see internal/eco). It replaces the engine's verdict
	// and is left out of the accuracy.
	Book Class = "book"
)

// Thresholds are the lower bounds, in win-probability percentage points
// lost, of the classes above Best. They are starting values, meant to be
// adjusted by looking at real games, not constants of nature.
type Thresholds struct {
	Good, Inaccuracy, Mistake, Blunder float64
}

// DefaultThresholds: <2 excellent, <5 good, then 5 / 10 / 20.
var DefaultThresholds = Thresholds{Good: 2, Inaccuracy: 5, Mistake: 10, Blunder: 20}

func (t Thresholds) classify(loss float64) Class {
	switch {
	case loss >= t.Blunder:
		return Blunder
	case loss >= t.Mistake:
		return Mistake
	case loss >= t.Inaccuracy:
		return Inaccuracy
	case loss >= t.Good:
		return Good
	default:
		return Excellent
	}
}

// DefaultWinK is the scale of the win-probability curve
// W = 1 / (1 + exp(-cp/K)). 400 is the scale the NNUE nets are trained
// with (train/train.py --k), so it is the natural starting point; it has
// not yet been fitted to game results — see NNUE.md / WEB-PLAN.md.
const DefaultWinK = 400.0

// matePlies is how close to MateScore a search score has to be to count
// as a forced mate (mirrors the search package's own threshold).
const mateThreshold = engine.MateScore - 1000

// Options configures Analyze.
type Options struct {
	// MoveTime is the search time per position. Ignored if Depth > 0.
	// Default 300 ms.
	MoveTime time.Duration
	// Depth, if > 0, searches every position to exactly this depth
	// instead of by time: slower on busy machines but reproducible.
	Depth int
	// MaxDepth caps the depth of a timed search. Default 64.
	MaxDepth int
	// Workers is the number of positions searched concurrently
	// (each search is single-threaded). Default runtime.NumCPU().
	Workers int
	// HashMB is the transposition table size per search. Default 16.
	HashMB int
	// WinK is the win-probability scale; default DefaultWinK.
	WinK float64
	// Thresholds default to DefaultThresholds.
	Thresholds *Thresholds
	// Book names the opening and marks theory moves; nil means the
	// embedded lichess book (eco.Default). Games from a custom start
	// position never use a book.
	Book *eco.Book
	// NoBook turns opening recognition off.
	NoBook bool
	// Progress, if set, is called after each position is evaluated,
	// never concurrently and with increasing done.
	Progress func(done, total int)
}

func (o *Options) fill() {
	if o.MoveTime <= 0 {
		o.MoveTime = 300 * time.Millisecond
	}
	if o.MaxDepth <= 0 {
		o.MaxDepth = 64
	}
	if o.Workers <= 0 {
		o.Workers = runtime.NumCPU()
	}
	if o.HashMB <= 0 {
		o.HashMB = 16
	}
	if o.WinK <= 0 {
		o.WinK = DefaultWinK
	}
	if o.Thresholds == nil {
		t := DefaultThresholds
		o.Thresholds = &t
	}
}

// PositionEval is the engine's verdict on one position of the game.
// Ply 0 is the start position, ply i the position after move i.
type PositionEval struct {
	Ply int    `json:"ply"`
	FEN string `json:"fen"`
	// Score is in centipawns from White's point of view. For a forced
	// mate it is ±MateScore-style and Mate is set; use Mate then.
	Score int `json:"score"`
	// Mate is the number of moves to a forced mate: positive if White
	// mates, negative if Black does, 0 if no mate was found.
	Mate int `json:"mate,omitempty"`
	// WinPct is White's win probability, 0..100.
	WinPct  float64  `json:"winPct"`
	Depth   int      `json:"depth"`
	Nodes   int64    `json:"nodes"`
	BestUCI string   `json:"bestUci,omitempty"`
	BestSAN string   `json:"bestSan,omitempty"`
	PV      []string `json:"pv,omitempty"` // best line in SAN
	// Terminal is set (and nothing was searched) when the game is over
	// in this position: "checkmate", "stalemate", "draw by ...".
	Terminal string `json:"terminal,omitempty"`
}

// MoveReport is the review of one half-move.
type MoveReport struct {
	Ply   int     `json:"ply"`   // 1-based
	Color string  `json:"color"` // "white" or "black"
	SAN   string  `json:"san"`
	UCI   string  `json:"uci"`
	Class Class   `json:"class"`
	Loss  float64 `json:"loss"` // win-probability points lost by the mover (>= 0)
	// Accuracy is this move's 0..100 score (see MoveAccuracy).
	Accuracy float64 `json:"accuracy"`
	BestSAN  string  `json:"bestSan,omitempty"` // engine's choice, if different from the move played
	BestUCI  string  `json:"bestUci,omitempty"`
	Forced   bool    `json:"forced,omitempty"` // only one legal move
	// Clock is the mover's remaining time after the move, in
	// milliseconds, or -1 when the PGN had no clock annotation.
	ClockMs int64 `json:"clockMs"`
	// Opening is set on the plies whose position has a name in the ECO
	// reference; Book is set on every theory move (including a ply or
	// two between named positions).
	Opening *eco.Opening `json:"opening,omitempty"`
	Book    bool         `json:"book,omitempty"`
}

// Report is the full result of Analyze.
type Report struct {
	Evals []PositionEval `json:"evals"` // len(Moves)+1 entries
	Moves []MoveReport   `json:"moves"`
	// Accuracy per player, 0..100: index 0 = White, 1 = Black.
	Accuracy [2]float64 `json:"accuracy"`
	// Counts[color][class] = number of moves.
	Counts [2]map[Class]int `json:"counts"`
	// Key lists the plies where the game turned most (largest losses,
	// worst first), at most 5.
	Key []int `json:"key"`
	// Opening is the last named opening the game went through, or nil
	// if it left the reference at once. BookPlies is how many half-moves
	// were theory (they are excluded from Accuracy).
	Opening   *eco.Opening  `json:"opening,omitempty"`
	BookPlies int           `json:"bookPlies"`
	Options   ReportOptions `json:"options"`
}

// ReportOptions records how the analysis was made.
type ReportOptions struct {
	MoveTimeMs int     `json:"moveTimeMs"`
	Depth      int     `json:"depth"`
	WinK       float64 `json:"winK"`
}

// WinPercent converts a centipawn score (side-agnostic: positive is
// good for the side the caller is rooting for) to a 0..100 win
// probability on the logistic curve with scale k.
func WinPercent(cp int, k float64) float64 {
	if cp >= mateThreshold {
		return 100
	}
	if cp <= -mateThreshold {
		return 0
	}
	return 100 / (1 + math.Exp(-float64(cp)/k))
}

// accuracySteepness is how fast a move's accuracy falls with the
// win-probability points it lost. The shape (an exponential through
// 100 at no loss) is the one lichess publishes; the steepness is ours,
// about twice lichess's 0.04354, so that a decisive error costs a move
// almost all of its score. Accuracy is a model of this project, not a
// copy of any site's figure.
const accuracySteepness = 0.08

// MoveAccuracy maps the win-probability points a move lost to a 0..100
// score: 100 for a move that lost nothing, about 90 for 1 point, 50
// for 8, near 0 from about 40 points (a queen blunder from an equal
// position).
func MoveAccuracy(loss float64) float64 {
	a := 103.1668*math.Exp(-accuracySteepness*loss) - 3.1669
	return math.Max(0, math.Min(100, a))
}

// playerAccuracy combines the per-move accuracies of one player into
// the game figure with a harmonic mean. Unlike the plain average, the
// harmonic mean is dominated by the worst moves: one queen blunder
// among fifty good moves drags the game down to the 30s instead of
// being averaged away to 98, and a handful of small slips barely move
// it. Individual moves are floored at 1 so that a single catastrophic
// move lowers the score a lot but cannot make it exactly 0.
func playerAccuracy(accs []float64) float64 {
	if len(accs) == 0 {
		return 0
	}
	var inv float64
	for _, a := range accs {
		inv += 1 / math.Max(a, 1)
	}
	return float64(len(accs)) / inv
}

// Analyze reviews g. It returns an error only if the game's own moves
// can't be replayed; a game with zero moves yields an empty report with
// one evaluated position.
func Analyze(g *pgn.Game, opt Options) (*Report, error) {
	opt.fill()
	start, err := g.Position()
	if err != nil {
		return nil, err
	}

	// Replay: one board snapshot and one hash history per position.
	n := len(g.Moves)
	boards := make([]*engine.Position, n+1)
	hashes := make([]uint64, n+1)
	cur := start.Copy()
	boards[0], hashes[0] = cur.Copy(), cur.Hash()
	for i, m := range g.Moves {
		cur.Play(m.Move)
		boards[i+1], hashes[i+1] = cur.Copy(), cur.Hash()
	}

	rep := &Report{
		Evals: make([]PositionEval, n+1),
		Moves: make([]MoveReport, n),
		Options: ReportOptions{
			MoveTimeMs: int(opt.MoveTime / time.Millisecond),
			Depth:      opt.Depth,
			WinK:       opt.WinK,
		},
	}
	if opt.Depth > 0 {
		rep.Options.MoveTimeMs = 0
	}

	// Evaluate positions concurrently.
	jobs := make(chan int)
	var wg sync.WaitGroup
	var done int
	var mu sync.Mutex
	for w := 0; w < opt.Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				rep.Evals[i] = evalPosition(boards[i], hashes[:i+1], i, &opt)
				if opt.Progress != nil {
					mu.Lock()
					done++
					opt.Progress(done, n+1) // under the lock: callers see done increase monotonically
					mu.Unlock()
				}
			}
		}()
	}
	for i := 0; i <= n; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	// Opening theory: which plies are book, and what the opening is called.
	var book *eco.Book
	if !opt.NoBook && (g.StartFEN == "" || g.StartFEN == engine.StartFEN) {
		book = opt.Book
		if book == nil {
			book = eco.Default()
		}
	}
	named, bookPlies := markBook(book, hashes)
	rep.BookPlies = bookPlies

	// Judge the moves.
	var accs [2][]float64
	var played [2]int // moves per player, book ones included
	rep.Counts = [2]map[Class]int{{}, {}}
	for i, m := range g.Moves {
		color := i % 2
		if startsBlack(g) {
			color = 1 - color
		}
		before, after := rep.Evals[i], rep.Evals[i+1]
		moverBefore, moverAfter := before.WinPct, after.WinPct
		if color == 1 {
			moverBefore, moverAfter = 100-moverBefore, 100-moverAfter
		}
		loss := math.Max(0, moverBefore-moverAfter)

		uci := m.Move.String()
		legalCount := len(boards[i].LegalMoves())
		mr := MoveReport{
			Ply: i + 1, SAN: m.SAN, UCI: uci, Loss: loss,
			Forced: legalCount == 1, ClockMs: -1,
		}
		if color == 0 {
			mr.Color = "white"
		} else {
			mr.Color = "black"
		}
		if m.Clock >= 0 {
			mr.ClockMs = m.Clock.Milliseconds()
		}
		switch {
		case before.Terminal != "":
			mr.Class = Best
		case mr.Forced || before.BestUCI == uci:
			mr.Class = Best
			mr.Loss = 0
		default:
			mr.Class = opt.Thresholds.classify(loss)
			mr.BestSAN, mr.BestUCI = before.BestSAN, before.BestUCI
		}
		mr.Accuracy = MoveAccuracy(mr.Loss)
		if o, ok := named[i+1]; ok {
			o := o
			mr.Opening = &o
			rep.Opening = &o
		}
		played[color]++
		if i+1 <= bookPlies {
			// Theory: the engine's opinion is kept in Loss/BestSAN for
			// the curious, but the move is not judged and not scored.
			mr.Book, mr.Class, mr.Accuracy = true, Book, 100
			rep.Moves[i] = mr
			rep.Counts[color][Book]++
			continue
		}
		rep.Moves[i] = mr
		rep.Counts[color][mr.Class]++
		accs[color] = append(accs[color], mr.Accuracy)
	}
	for c := 0; c < 2; c++ {
		rep.Accuracy[c] = playerAccuracy(accs[c])
		if len(accs[c]) == 0 && played[c] > 0 {
			rep.Accuracy[c] = 100 // every move was theory
		}
	}
	rep.Key = keyMoments(rep.Moves, 5, opt.Thresholds.Inaccuracy)
	return rep, nil
}

// markBook finds how long the game stayed in opening theory. hashes[i] is
// the position after ply i. A ply is theory while every position so far lies
// on some reference line (the reference names only some positions of a
// line, so being named is not required); the first position off all lines
// ends the book for good. It returns the named positions by ply and the
// number of book plies.
func markBook(book *eco.Book, hashes []uint64) (named map[int]eco.Opening, bookPlies int) {
	named = map[int]eco.Opening{}
	for ply := 1; ply < len(hashes) && book.InBook(hashes[ply]); ply++ {
		bookPlies = ply
		if o, ok := book.Lookup(hashes[ply]); ok {
			named[ply] = o
		}
	}
	return named, bookPlies
}

func startsBlack(g *pgn.Game) bool {
	if g.StartFEN == "" || g.StartFEN == engine.StartFEN {
		return false
	}
	b, err := engine.ParseFEN(g.StartFEN)
	return err == nil && b.SideToMove() == engine.Black
}

// keyMoments returns the plies of the largest losses (worst first),
// ignoring anything below minLoss.
func keyMoments(moves []MoveReport, limit int, minLoss float64) []int {
	idx := make([]int, 0, len(moves))
	for i, m := range moves {
		if m.Loss >= minLoss {
			idx = append(idx, i)
		}
	}
	// Simple selection sort: limit is tiny.
	var out []int
	for len(out) < limit && len(idx) > 0 {
		bi := 0
		for j := range idx {
			if moves[idx[j]].Loss > moves[idx[bi]].Loss {
				bi = j
			}
		}
		out = append(out, moves[idx[bi]].Ply)
		idx = append(idx[:bi], idx[bi+1:]...)
	}
	return out
}

// AnalyzePosition evaluates a single position. history lists the hashes
// of every earlier position of the game plus this one (the last entry),
// so that repetitions are seen; it may be nil.
func AnalyzePosition(b *engine.Position, history []uint64, opt Options) PositionEval {
	opt.fill()
	return evalPosition(b, history, 0, &opt)
}

func evalPosition(b *engine.Position, history []uint64, ply int, opt *Options) PositionEval {
	ev := PositionEval{Ply: ply, FEN: b.FEN()}
	white := b.SideToMove() == engine.White

	if out := b.Outcome(history); out.Over {
		ev.Terminal = out.Reason
		switch {
		case out.Checkmate && white:
			ev.Score, ev.WinPct = -engine.MateScore, 0
		case out.Checkmate:
			ev.Score, ev.WinPct = engine.MateScore, 100
		default:
			ev.Score, ev.WinPct = 0, 50
		}
		return ev
	}

	res, err := engine.Search(b, history, engine.Limits{
		Depth: opt.Depth, MoveTime: opt.MoveTime, MaxDepth: opt.MaxDepth, HashMB: opt.HashMB,
	})
	if err != nil {
		ev.Terminal = err.Error()
		return ev
	}

	stm := res.Score
	if !white {
		stm = -stm
	}
	ev.Score = stm
	ev.WinPct = WinPercent(stm, opt.WinK)
	ev.Depth = res.Depth
	ev.Nodes = res.Nodes
	if abs := absInt(stm); abs >= mateThreshold {
		plies := engine.MateScore - abs
		moves := (plies + 1) / 2
		if stm < 0 {
			moves = -moves
		}
		ev.Mate = moves
	}
	ev.BestUCI = res.Best.String()
	ev.BestSAN = b.SAN(res.Best)
	ev.PV = pvToSAN(b, res.PV, res.Best)
	return ev
}

// pvToSAN renders a principal variation in SAN, replaying it on a copy
// of b. The PV may be empty (a very short search); then it is just the
// best move.
func pvToSAN(b *engine.Position, pv []engine.Move, best engine.Move) []string {
	if len(pv) == 0 {
		return []string{b.SAN(best)}
	}
	cur := b.Copy()
	out := make([]string, 0, len(pv))
	for _, m := range pv {
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
		out = append(out, cur.SAN(m))
		cur.Play(m)
	}
	return out
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// String renders a short human-readable summary line for a class count.
func (r *Report) String() string {
	return fmt.Sprintf("accuracy white %.1f black %.1f", r.Accuracy[0], r.Accuracy[1])
}

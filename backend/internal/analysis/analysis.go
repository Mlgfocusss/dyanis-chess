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
	"sort"
	"sync"
	"time"

	"dyanis-chess/backend/internal/eco"
	"dyanis-chess/backend/internal/pgn"
	"dyanis-chess/engine"
)

// Class is the quality label of one move.
type Class string

const (
	// Brilliant and Great are Best moves that were also something more: a
	// sound sacrifice, or the only move (see special.go).
	Brilliant Class = "brilliant"
	Great     Class = "great"
	// Miss is an inaccuracy or mistake that let slip what the opponent's own
	// mistake had offered (see special.go).
	Miss       Class = "miss"
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
	// NoSpecial turns off the Brilliant and Great classes (they cost
	// extra searches: every alternative of a Best move is looked at).
	NoSpecial bool
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

	// Working data of the judging step (see special.go), not part of the
	// report: material balance (White minus Black, pawns) here and after each
	// ply of the best line, which of those plies captured, and the result of
	// the only-move test of the move played from this position.
	bal       int
	pvBal     []int
	pvCap     []bool
	gap       float64
	playedPct float64
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
	// TimeMs is how long the move took, in milliseconds. Left out when
	// the game carried no clock or move times (older reports have none).
	TimeMs *int64 `json:"timeMs,omitempty"`
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
	// Variant is set for a game of a chess variant ("chess960"): there is no
	// opening book for it and the engine never recommends a castling (variant.go).
	Variant string `json:"variant,omitempty"`
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
// win-probability points it lost. The curve is the one Lichess publishes
// (an exponential through 100 at no loss, steepness 0.04354), made a quarter
// steeper because chess.com's figures run lower: on the reference game of
// testdata (chess.com says 82 / 92) 0.055 gives 87 / 96, Lichess's own steepness
// 89 / 97. One decisive error costs that move most of its score but only its
// share of the game's: see playerAccuracy.
const accuracySteepness = 0.055

// MoveAccuracy maps the win-probability points a move lost to a 0..100
// score: 100 for a move that lost nothing, about 84 for 3 points, 56 for
// 10, 31 for 20 (a clear blunder), near 0 from about 50.
func MoveAccuracy(loss float64) float64 {
	a := 103.1668*math.Exp(-accuracySteepness*loss) - 3.1669
	return math.Max(0, math.Min(100, a))
}

// volatilityWeights gives each move a weight: how much the win
// probability swung around it (the standard deviation over a sliding window
// of positions, clamped to 0.5..12). Errors in sharp, decisive stretches
// count for more than slips in a quiet position, as in the Lichess
// formula. winPcts is White's win probability in every position of the game
// (one more than there are moves).
func volatilityWeights(winPcts []float64) []float64 {
	n := len(winPcts) - 1
	if n <= 0 {
		return nil
	}
	size := min(max(len(winPcts)/10, 2), 8)
	weight := func(w []float64) float64 {
		var mean float64
		for _, x := range w {
			mean += x
		}
		mean /= float64(len(w))
		var v float64
		for _, x := range w {
			v += (x - mean) * (x - mean)
		}
		return math.Min(12, math.Max(0.5, math.Sqrt(v/float64(len(w)))))
	}
	var ws []float64
	first := min(size, len(winPcts))
	for k := 0; k < first-2; k++ { // the opening moves share the first window
		ws = append(ws, weight(winPcts[:first]))
	}
	if len(winPcts) < size {
		ws = append(ws, weight(winPcts))
	}
	for s := 0; s+size <= len(winPcts); s++ {
		ws = append(ws, weight(winPcts[s:s+size]))
	}
	for len(ws) < n {
		ws = append(ws, ws[len(ws)-1])
	}
	return ws[:n]
}

// playerAccuracy combines the per-move accuracies of one player into the
// game figure the way Lichess does: the mean of the volatility-weighted
// average and the harmonic mean. The weighted average lets errors in decisive
// moments count more, the harmonic mean keeps the worst moves from being
// averaged away, and neither lets one blunder wipe the whole game out: a
// queen hung in a 40-move game still leaves a score in the 70s or 80s.
// weights runs parallel to accs; nil weighs every move equally. A move
// is floored at 1 in the harmonic mean so that no single move can make the
// score 0.
func playerAccuracy(accs, weights []float64) float64 {
	if len(accs) == 0 {
		return 0
	}
	var sum, wsum, inv float64
	for i, a := range accs {
		w := 1.0
		if i < len(weights) {
			w = weights[i]
		}
		sum += a * w
		wsum += w
		inv += 1 / math.Max(a, 1)
	}
	return (sum/wsum + float64(len(accs))/inv) / 2
}

// playedMove is one half-move of the game under review, however it came
// (a PGN, a game of a variant).
type playedMove struct {
	SAN, UCI string
	// Move is the engine's move, valid when HasMove: a move the engine cannot
	// make (the Chess960 castling) has none and is judged by the position after it.
	Move    engine.Move
	HasMove bool
	// ClockMs is the mover's time left, -1 when unknown.
	ClockMs int64
	// SpentMs is how long the move took when HasSpent (see times.go).
	SpentMs  int64
	HasSpent bool
	// Legal is how many moves the mover had (1 means a forced move).
	Legal int
}

// timeline is a game ready to be searched: the board before each move and after
// the last, the hash history and the moves.
type timeline struct {
	boards, spare []*engine.Position // spare: touched by nothing but Copy
	hashes        []uint64
	moves         []playedMove
	startBlack    bool // Black moves first
	book          bool // opening theory applies
}

// Analyze reviews g. It returns an error only if the game's own moves
// can't be replayed; a game with zero moves yields an empty report with
// one evaluated position.
func Analyze(g *pgn.Game, opt Options) (*Report, error) {
	start, err := g.Position()
	if err != nil {
		return nil, err
	}

	// Replay: one board snapshot and one hash history per position.
	n := len(g.Moves)
	tl := &timeline{
		boards: make([]*engine.Position, n+1),
		// spare is a second set of the same boards, touched by nothing but
		// Copy: the search of position i uses boards[i] (and writes caches in
		// it), while the only-move test of position i+1 needs to look at it too.
		spare:      make([]*engine.Position, n+1),
		hashes:     make([]uint64, n+1),
		moves:      make([]playedMove, n),
		startBlack: startsBlack(g),
		book:       g.StartFEN == "" || g.StartFEN == engine.StartFEN,
	}
	cur := start.Copy()
	tl.boards[0], tl.spare[0], tl.hashes[0] = cur.Copy(), cur.Copy(), cur.Hash()
	for i, m := range g.Moves {
		tl.moves[i] = playedMove{
			SAN: m.SAN, UCI: m.Move.String(), Move: m.Move, HasMove: true,
			ClockMs: -1, Legal: len(cur.LegalMoves()),
		}
		if m.Clock >= 0 {
			tl.moves[i].ClockMs = m.Clock.Milliseconds()
		}
		cur.Play(m.Move)
		tl.boards[i+1], tl.spare[i+1], tl.hashes[i+1] = cur.Copy(), cur.Copy(), cur.Hash()
	}
	for i, ms := range moveTimes(g, tl.startBlack) {
		if ms >= 0 {
			tl.moves[i].SpentMs, tl.moves[i].HasSpent = ms, true
		}
	}
	return review(tl, opt), nil
}

// review searches every position of the timeline and judges the moves.
func review(tl *timeline, opt Options) *Report {
	opt.fill()
	n := len(tl.moves)
	boards, spare, hashes := tl.boards, tl.spare, tl.hashes

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

	// Opening theory: which plies are book, and what the opening is called.
	// (Known before the search so that theory moves are not examined.)
	var book *eco.Book
	if !opt.NoBook && tl.book {
		book = opt.Book
		if book == nil {
			book = eco.Default()
		}
	}
	named, bookPlies := markBook(book, hashes)
	rep.BookPlies = bookPlies

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
				if i < n && !opt.NoSpecial && i+1 > bookPlies {
					testOnlyMove(&rep.Evals[i], spare, hashes, tl.moves, i, &opt)
				}
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

	// Judge the moves.
	winPcts := make([]float64, len(rep.Evals))
	for i, e := range rep.Evals {
		winPcts[i] = e.WinPct
	}
	weights := volatilityWeights(winPcts)
	var accs, accWeights [2][]float64
	rep.Counts = [2]map[Class]int{{}, {}}
	for i, m := range tl.moves {
		color := i % 2
		if tl.startBlack {
			color = 1 - color
		}
		before, after := rep.Evals[i], rep.Evals[i+1]
		moverBefore, moverAfter := before.WinPct, after.WinPct
		if color == 1 {
			moverBefore, moverAfter = 100-moverBefore, 100-moverAfter
		}
		loss := math.Max(0, moverBefore-moverAfter)

		uci := m.UCI
		mr := MoveReport{
			Ply: i + 1, SAN: m.SAN, UCI: uci, Loss: loss,
			Forced: m.Legal == 1, ClockMs: m.ClockMs,
		}
		if m.HasSpent {
			t := m.SpentMs
			mr.TimeMs = &t
		}
		if color == 0 {
			mr.Color = "white"
		} else {
			mr.Color = "black"
		}
		mates := givesMate(after)
		switch {
		case before.Terminal != "":
			mr.Class = Best
		case mates:
			// Checkmate cannot be improved on, whatever line the engine
			// preferred (another mate, or a mate it saw as a few plies faster).
			mr.Class = Best
			mr.Loss = 0
		case mr.Forced || before.BestUCI == uci || (!m.HasMove && loss == 0):
			// (a move the engine cannot make was no worse than the engine's choice)
			mr.Class = Best
			mr.Loss = 0
		default:
			mr.Class = opt.Thresholds.classify(loss)
			mr.BestSAN, mr.BestUCI = before.BestSAN, before.BestUCI
		}
		if !opt.NoSpecial && !mr.Forced && !mates && before.Terminal == "" && i+1 > bookPlies {
			sign := 1 - 2*color // +1 White, -1 Black
			promotion := len(uci) == 5
			switch {
			case (mr.Class == Best || mr.Class == Excellent) && isBrilliant(before, after, sign, moverBefore, moverAfter, promotion):
				mr.Class = Brilliant
			case mr.Class == Best && isGreat(before.gap, before.playedPct, moverBefore, prevLoss(rep.Moves, i)):
				mr.Class = Great
			case isMiss(mr.Class, prevLoss(rep.Moves, i), moverBefore):
				mr.Class = Miss
			}
		}
		mr.Accuracy = MoveAccuracy(mr.Loss)
		if o, ok := named[i+1]; ok {
			o := o
			mr.Opening = &o
			rep.Opening = &o
		}
		if i+1 <= bookPlies {
			// Theory: the engine's opinion is kept in Loss/BestSAN for
			// the curious, but the move is not judged: it counts as a perfect one.
			mr.Book, mr.Class, mr.Accuracy = true, Book, 100
			rep.Counts[color][Book]++
		} else {
			rep.Counts[color][mr.Class]++
		}
		rep.Moves[i] = mr
		// Theory moves are scored too (100, with the weight of their quiet position): a
		// blunder on move 8 after seven book moves is a bad game, but not one of 3 points.
		accs[color] = append(accs[color], mr.Accuracy)
		accWeights[color] = append(accWeights[color], weights[i])
	}
	for c := 0; c < 2; c++ {
		rep.Accuracy[c] = playerAccuracy(accs[c], accWeights[c])
	}
	rep.Key = keyMoments(rep.Moves, 5, opt.Thresholds.Inaccuracy)
	return rep
}

// givesMate reports whether the position is checkmate: the move that led to
// it was the best a move can be.
func givesMate(after PositionEval) bool {
	return after.Terminal != "" && absInt(after.Score) >= engine.MateScore
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
	ev.bal, _ = material(ev.FEN)
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

	ev.Score, ev.Mate, ev.WinPct = whiteView(res.Score, white, opt.WinK)
	ev.Depth = res.Depth
	ev.Nodes = res.Nodes
	ev.BestUCI = res.Best.String()
	ev.BestSAN = b.SAN(res.Best)
	ev.PV = pvToSAN(b, res.PV, res.Best)
	ev.pvBal, ev.pvCap = pvTrace(b, res.PV)
	return ev
}

// whiteView turns a search score (centipawns for the side to move) into
// White's point of view: the score, the moves to a forced mate (positive when
// White mates, negative when Black does, 0 for none) and White's win
// probability.
func whiteView(score int, whiteToMove bool, winK float64) (cp, mate int, winPct float64) {
	if !whiteToMove {
		score = -score
	}
	cp, winPct = score, WinPercent(score, winK)
	if abs := absInt(score); abs >= mateThreshold {
		plies := engine.MateScore - abs
		mate = (plies + 1) / 2
		if score < 0 {
			mate = -mate
		}
	}
	return cp, mate, winPct
}

// LineEval is one of the best lines of a position (MultiPV): the engine's
// first choice and the next best moves it would play if that one were not
// there. Scores are White's, as in PositionEval.
type LineEval struct {
	Rank    int      `json:"rank"` // 1 = the best line
	Score   int      `json:"score"`
	Mate    int      `json:"mate,omitempty"`
	WinPct  float64  `json:"winPct"`
	Depth   int      `json:"depth"`
	BestUCI string   `json:"bestUci"`
	BestSAN string   `json:"bestSan"`
	PV      []string `json:"pv"` // the line in SAN, starting with BestSAN
}

// MaxLines is the most lines AnalyzeLines gives.
const MaxLines = 5

// AnalyzeLines searches a position for its n best lines, best first. history
// is as in AnalyzePosition. A finished game or a position with a single legal
// move gives fewer lines (none, or one). Every line is searched as deeply as
// opt says, so n lines cost about n searches.
func AnalyzeLines(b *engine.Position, history []uint64, opt Options, n int) ([]LineEval, error) {
	opt.fill()
	n = min(max(n, 1), MaxLines)
	if b.Outcome(history).Over {
		return nil, nil
	}
	results, err := engine.SearchLines(b, history, engine.Limits{
		Depth: opt.Depth, MoveTime: opt.MoveTime, MaxDepth: opt.MaxDepth, HashMB: opt.HashMB,
	}, n)
	if err != nil {
		return nil, err
	}
	white := b.SideToMove() == engine.White
	out := make([]LineEval, 0, len(results))
	for _, r := range results {
		l := LineEval{Depth: r.Depth, BestUCI: r.Best.String(), BestSAN: b.SAN(r.Best), PV: pvToSAN(b, r.PV, r.Best)}
		l.Score, l.Mate, l.WinPct = whiteView(r.Score, white, opt.WinK)
		out = append(out, l)
	}
	// Each line was searched without the first moves of the ones before it, so at a finite depth a
	// later line can come out a little better than an earlier one: the best for the side to move goes first.
	sort.SliceStable(out, func(i, j int) bool {
		if white {
			return out[i].Score > out[j].Score
		}
		return out[i].Score < out[j].Score
	})
	for i := range out {
		out[i].Rank = i + 1
	}
	return out, nil
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

// testOnlyMove runs the only-move test for the move played from position i,
// when it is worth running: the move is the engine's own choice, the position
// is not decided, the mover is not just answering a check or recapturing, and
// there is a real choice of moves. The result is left in ev.
func testOnlyMove(ev *PositionEval, boards []*engine.Position, hashes []uint64, moves []playedMove, i int, opt *Options) {
	if !moves[i].HasMove {
		return
	}
	played := moves[i].Move
	if ev.Terminal != "" || ev.BestUCI != played.String() {
		return
	}
	// The mover's chance of winning before the move: skip decided positions.
	pct := ev.WinPct
	if boards[i].SideToMove() == engine.Black {
		pct = 100 - pct
	}
	if pct > GreatMaxBefore || pct < GreatMinBefore {
		return
	}
	if i > 0 {
		prev := moves[i-1]
		san := prev.SAN
		if prev.HasMove {
			san = boards[i-1].Copy().SAN(prev.Move)
		}
		if len(san) > 0 && (san[len(san)-1] == '+' || san[len(san)-1] == '#') {
			return // answering a check
		}
		_, before := material(boards[i-1].Copy().FEN())
		_, now := material(boards[i].Copy().FEN())
		if now < before && prev.HasMove && prev.UCI[2:4] == played.String()[2:4] {
			return // a recapture
		}
	}
	ev.playedPct, ev.gap = onlyMoveGap(boards[i].Copy(), hashes[:i+1], played, opt)
}

// prevLoss is the win probability the opponent's previous move cost them
// (0 at the first move).
func prevLoss(moves []MoveReport, i int) float64 {
	if i == 0 {
		return 0
	}
	return moves[i-1].Loss
}

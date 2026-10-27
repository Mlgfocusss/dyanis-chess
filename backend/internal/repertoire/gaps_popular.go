package repertoire

import (
	"context"
	"errors"
	"sort"
	"time"

	"dyanis-chess/backend/internal/eco"
	"dyanis-chess/backend/internal/explorer"
	"dyanis-chess/backend/internal/store"
	"dyanis-chess/engine"
)

// The "holes" of a repertoire by popular replies of the opponent (what Chessbook
// calls gaps): a position where the opponent moves, a move many opponents play
// there, and no such move among the children of that node of the tree. Plus the
// holes Gaps finds: the opponent's move that is in the tree and has no answer.
//
// Statistics are the Lichess explorer's (through explorer.Service, whose cache is
// a day long and which waits when Lichess asks to). Without a token, or while
// Lichess asks to wait, the moves of the ECO reference stand in, and the answer
// says so.
//
// The search walks the tree from the start, always taking the heaviest unchecked
// position next (the weight of a position is the chance that a game gets there by
// the repertoire), in small rounds; each round asks for the statistics of the next
// few positions, and a call stops asking Lichess after Budget requests that were
// not in the cache. What was not checked is reported (Partial, Pending); asking
// again goes on from there, since the answers are cached.

// StatsSource is the part of *explorer.Service the search uses.
type StatsSource interface {
	Enabled() bool
	Cached(fen, ratings, speeds string) (explorer.Response, bool)
	Lookup(ctx context.Context, fen, ratings, speeds string) (explorer.Response, bool, error)
}

// Defaults of a search.
const (
	DefaultMinPct    = 5.0  // a reply is popular from this share of the games, in percent
	DefaultMinGames  = 50   // ... and from this many games
	DefaultMinWeight = 0.01 // a hole lighter than this chance is not worth a word
	DefaultBudget    = 40   // requests to Lichess that the cache did not answer, per call
	DefaultGapLimit  = 50   // holes in an answer
	MaxGapLimit      = 200
	roundSize        = 6 // uncached requests between two looks at the tree
	minTheoryLines   = 2 // a position with fewer reference lines is out of the book
)

// GapOptions tunes a search; zero values mean the defaults above.
type GapOptions struct {
	Ratings, Speeds string // as explorer.Params returns them
	MinPct          float64
	MinGames        int
	MinWeight       float64
	Limit           int
	Budget          int
}

func (o GapOptions) norm() GapOptions {
	if o.MinPct <= 0 {
		o.MinPct = DefaultMinPct
	}
	if o.MinGames <= 0 {
		o.MinGames = DefaultMinGames
	}
	if o.MinWeight <= 0 {
		o.MinWeight = DefaultMinWeight
	}
	if o.Limit <= 0 {
		o.Limit = DefaultGapLimit
	}
	o.Limit = min(o.Limit, MaxGapLimit)
	if o.Budget == 0 {
		o.Budget = DefaultBudget
	}
	o.Budget = max(o.Budget, 0) // below zero: no requests at all, only what the cache has
	return o
}

// Gap kinds.
const (
	GapMissing = "missing" // a popular reply of the opponent that the tree does not have
	GapAnswer  = "answer"  // a reply of the opponent that the tree has, and no answer to it
)

// Gap is one hole.
type Gap struct {
	Kind string `json:"kind"`
	// At is the node of the position the hole is shown in: for a missing reply, the
	// position where the opponent moves (0: the start position), for an unanswered
	// reply, the position after it, where the person has to move.
	At   int64    `json:"at"`
	Ply  int      `json:"ply"`  // plies played in that position
	FEN  string   `json:"fen"`  // that position
	Path []string `json:"path"` // the moves (SAN) that lead there from the start
	// UCI and SAN are the opponent's move: the one to add (missing), or the last one
	// of the line (answer; empty for an empty repertoire of White).
	UCI string `json:"uci"`
	SAN string `json:"san"`
	// Pct and Games say how popular the move is (0 when it is not known); Weight is the
	// chance that a game reaches the hole by the repertoire and then goes that way.
	Pct    float64 `json:"pct"`
	Games  int     `json:"games"`
	Lines  int     `json:"lines,omitempty"` // theory: reference lines behind the move (Games is 0 then)
	Weight float64 `json:"weight"`
	// Source is "lichess" (real games) or "theory" (the ECO reference, no statistics).
	Source string `json:"source"`
}

// GapCounts are the figures of a search.
type GapCounts struct {
	Gaps      int `json:"gaps"`    // all holes found (the answer lists up to Limit of them)
	Missing   int `json:"missing"` // popular replies the tree lacks
	Answer    int `json:"answer"`  // lines that end with the opponent's move
	Checked   int `json:"checked"` // positions whose statistics were looked at
	Fetched   int `json:"fetched"` // requests made to Lichess
	FromCache int `json:"fromCache"`
	Pending   int `json:"pending"` // positions that were not checked
}

// PendingNode is a position the search did not get to.
type PendingNode struct {
	At     int64   `json:"at"`
	Weight float64 `json:"weight"`
}

// GapScan is the answer of a search.
type GapScan struct {
	Gaps []Gap `json:"gaps"`
	// Coverage is the share of the likely games the repertoire answers (0..1): one
	// minus the chance of reaching a hole or a position that was not checked. Replies
	// and positions lighter than MinPct and MinWeight count as covered, so it is an
	// estimate; when Partial it is a lower bound.
	Coverage float64       `json:"coverage"`
	Counts   GapCounts     `json:"counts"`
	Partial  bool          `json:"partial"`
	Pending  []PendingNode `json:"pending"`
	// Source is "lichess", "theory" (no statistics at all) or "mixed".
	Source string `json:"source"`
	// Available says that the server has a Lichess token.
	Available  bool    `json:"available"`
	Limited    bool    `json:"limited"`    // Lichess asked to wait during the search
	RetryAfter int     `json:"retryAfter"` // seconds, when Limited
	Ratings    string  `json:"ratings"`
	Speeds     string  `json:"speeds"`
	MinPct     float64 `json:"minPct"`
	At         string  `json:"at"` // when the search was made (RFC 3339)
	// Stale is set on a saved result when the tree has changed since.
	Stale bool `json:"stale,omitempty"`
	// Moves and MaxMove identify the tree the search saw.
	Moves   int   `json:"moves"`
	MaxMove int64 `json:"maxMove"`
}

// statMove is a move of a position with how often it is played.
type statMove struct {
	uci, san string
	games    int // real games (the explorer)
	lines    int // reference lines (theory)
	share    float64
	popular  bool
}

// posInfo is what is known about the moves played in a position.
type posInfo struct {
	source string
	moves  []statMove // most played first
}

func (p *posInfo) find(uci string) (statMove, bool) {
	for _, m := range p.moves {
		if m.uci == uci {
			return m, true
		}
	}
	return statMove{}, false
}

func infoFromStats(resp explorer.Response, o GapOptions) *posInfo {
	info := &posInfo{source: "lichess"}
	if resp.Total <= 0 {
		return info
	}
	for _, m := range resp.Moves {
		sh := float64(m.Games) / float64(resp.Total)
		info.moves = append(info.moves, statMove{uci: m.UCI, san: m.SAN, games: m.Games, share: sh,
			popular: resp.Total >= o.MinGames && m.Games >= o.MinGames && sh*100 >= o.MinPct})
	}
	return info
}

// infoFromTheory takes the shares from the number of reference lines behind each move.
func infoFromTheory(book *eco.Book, fen string, o GapOptions) *posInfo {
	info := &posInfo{source: "theory"}
	pos, err := engine.ParseFEN(fen)
	if err != nil {
		return info
	}
	total := 0
	for _, m := range pos.LegalMoves() {
		c := pos.Copy()
		c.Play(m)
		h := c.Hash()
		if !book.InBook(h) {
			continue
		}
		n := book.Lines(h)
		total += n
		info.moves = append(info.moves, statMove{uci: m.String(), san: pos.SAN(m), lines: n})
	}
	if total < minTheoryLines {
		info.moves = nil
		return info
	}
	sort.SliceStable(info.moves, func(i, j int) bool {
		if info.moves[i].lines != info.moves[j].lines {
			return info.moves[i].lines > info.moves[j].lines
		}
		return info.moves[i].san < info.moves[j].san
	})
	for i := range info.moves {
		m := &info.moves[i]
		m.share = float64(m.lines) / float64(total)
		m.popular = m.share*100 >= o.MinPct
	}
	return info
}

// posClass is a position of the repository with what the search needs of it. The search works
// on positions, not on paths: a position reached by two orders of moves is one class, searched
// once, with the chances of reaching it by either order added.
type posClass struct {
	key  string
	rep  int64 // the node that stands for the position in lists (Graph.Rep)
	fen  string
	ply  int
	kids []store.RepMove // the repository's replies in the position, one per move
	kidC []int           // the class of the position after each reply
	// order is the place in the order parents-first; an edge to a class with a lower or equal
	// order goes back to an earlier position (shuffling the pieces) and is not followed.
	order int

	// per pass
	w      float64 // the chance of getting here by the repository
	h, p   float64 // per unit of w: the chance of ending in a hole / in a position not checked
	answer int     // index in scanner.gaps of the hole "no answer here", -1 if none
	viaW   float64 // the weight of the edge whose statistics the answer hole shows
}

// scanner holds one search.
type scanner struct {
	color   string
	g       *Graph
	o       GapOptions
	classes []posClass // classes[0] is the initial position
	topo    []int      // class indexes, parents first

	known map[string]*posInfo // fen -> statistics
	// per pass
	gaps    []Gap
	need    map[string]float64 // fen -> the greatest weight it was wanted with
	needAt  map[string]int64
	pending int
}

func newScanner(moves []store.RepMove, color string, o GapOptions) *scanner {
	g := NewGraph(moves)
	sc := &scanner{color: color, g: g, o: o, known: map[string]*posInfo{}}
	index := map[string]int{}
	var add func(key string) int
	add = func(key string) int {
		if i, ok := index[key]; ok {
			return i
		}
		rep := g.Rep(key)
		m := g.byID[rep]
		c := posClass{key: key, rep: rep, fen: m.FEN, ply: m.Ply, answer: -1}
		if rep == 0 {
			c.fen = engine.StartFEN
		}
		index[key] = len(sc.classes)
		sc.classes = append(sc.classes, c)
		return len(sc.classes) - 1
	}
	add(startKey)
	for i := 0; i < len(sc.classes); i++ { // grows while it goes: breadth first from the start
		kids := g.Replies(sc.classes[i].rep)
		kidC := make([]int, len(kids))
		for j, k := range kids {
			kidC[j] = add(g.Key(k.ID))
		}
		sc.classes[i].kids, sc.classes[i].kidC = kids, kidC
	}
	// parents before children, the edges back to a position already on the way left out
	state := make([]int8, len(sc.classes))
	var post []int
	var dfs func(i int)
	dfs = func(i int) {
		state[i] = 1
		for _, k := range sc.classes[i].kidC {
			if state[k] == 0 {
				dfs(k)
			}
		}
		state[i] = 2
		post = append(post, i)
	}
	dfs(0)
	for r := len(post) - 1; r >= 0; r-- {
		sc.classes[post[r]].order = len(post) - 1 - r
		sc.topo = append(sc.topo, post[r])
	}
	return sc
}

// opponentMoves says whether the opponent is to move after `ply` plies.
func (sc *scanner) opponentMoves(ply int) bool { return (ply%2 == 0) != (sc.color == "white") }

// pathOf is the moves (SAN) from the start to the node.
func (sc *scanner) pathOf(id int64) []string {
	var rev []string
	for ; id != 0; id = sc.g.byID[id].ParentID {
		m, ok := sc.g.byID[id]
		if !ok {
			break
		}
		rev = append(rev, m.SAN)
	}
	out := make([]string, 0, len(rev))
	for i := len(rev) - 1; i >= 0; i-- {
		out = append(out, rev[i])
	}
	return out
}

// share is how often the move is played in the class's position, as far as it is known: always 1
// for the person's own moves (any answer will do), 0 for a move the statistics do not have.
func (sc *scanner) share(c *posClass, info *posInfo, uci string) (float64, *statMove) {
	if !sc.opponentMoves(c.ply) {
		return 1, nil
	}
	if info != nil {
		if sm, ok := info.find(uci); ok {
			return sm.share, &sm
		}
	}
	return 0, nil
}

// pass walks the repository once with what is known and returns the chance of ending up in a
// hole, and of ending up in a position that was not checked yet. Each class is looked at once:
// first the chance of reaching it is added up over its parents, then, children first, the holes
// below it.
func (sc *scanner) pass() (holes, pend float64) {
	sc.gaps, sc.need, sc.needAt, sc.pending = nil, map[string]float64{}, map[string]int64{}, 0
	for i := range sc.classes {
		c := &sc.classes[i]
		c.w, c.h, c.p, c.answer, c.viaW = 0, 0, 0, -1, -1
	}
	sc.classes[0].w = 1
	for _, ci := range sc.topo {
		c := &sc.classes[ci]
		info := sc.known[c.fen]
		for k, kc := range c.kidC {
			if child := &sc.classes[kc]; child.order > c.order {
				s, _ := sc.share(c, info, c.kids[k].UCI)
				child.w += c.w * s
			}
		}
	}
	for r := len(sc.topo) - 1; r >= 0; r-- {
		c := &sc.classes[sc.topo[r]]
		if !sc.opponentMoves(c.ply) { // the person's move: any answer of theirs will do
			if len(c.kids) == 0 {
				g := Gap{Kind: GapAnswer, At: c.rep, Ply: c.ply, FEN: c.fen, Path: sc.pathOf(c.rep), Weight: c.w}
				if c.rep != 0 {
					m := sc.g.byID[c.rep]
					g.UCI, g.SAN = m.UCI, m.SAN
				}
				c.answer = len(sc.gaps)
				sc.gaps = append(sc.gaps, g)
				c.h = 1
				continue
			}
			best, bestP := -1.0, 0.0
			for _, kc := range c.kidC {
				ch := &sc.classes[kc]
				if ch.order <= c.order {
					continue
				}
				if best < 0 || ch.h+ch.p < best+bestP {
					best, bestP = ch.h, ch.p
				}
			}
			c.h, c.p = max(best, 0), bestP
			continue
		}

		info := sc.known[c.fen]
		if info == nil && c.w >= sc.o.MinWeight {
			if c.w > sc.need[c.fen] {
				sc.need[c.fen], sc.needAt[c.fen] = c.w, c.rep
			}
			c.p = 1
			sc.pending++
		}
		// the statistics may leave out moves of the tree: their weight is the share when it is
		// known, 0 when it is not (the line is still searched for holes)
		byUCI := make(map[string]bool, len(c.kids))
		for k, kc := range c.kidC {
			uci := c.kids[k].UCI
			byUCI[uci] = true
			ch := &sc.classes[kc]
			if ch.order <= c.order {
				continue
			}
			s, sm := sc.share(c, info, uci)
			c.h += s * ch.h
			c.p += s * ch.p
			// the unanswered reply itself is a hole that can say how popular the move is
			if sm != nil && info != nil && ch.answer >= 0 && c.w*s > ch.viaW {
				ch.viaW = c.w * s
				g := &sc.gaps[ch.answer]
				g.Pct, g.Games, g.Lines, g.Source = 100*sm.share, sm.games, sm.lines, info.source
			}
		}
		if info != nil {
			for _, m := range info.moves {
				if !m.popular || byUCI[m.uci] || c.w*m.share < sc.o.MinWeight {
					continue
				}
				sc.gaps = append(sc.gaps, Gap{Kind: GapMissing, At: c.rep, Ply: c.ply, FEN: c.fen, Path: sc.pathOf(c.rep),
					UCI: m.uci, SAN: m.san, Pct: 100 * m.share, Games: m.games, Lines: m.lines, Weight: c.w * m.share, Source: info.source})
				c.h += m.share
			}
		}
	}
	return sc.classes[0].h, sc.classes[0].p
}

// ScanGaps searches the repertoire (the moves of a repertoire of `color`) for holes.
// src may be nil: the reference stands in for the statistics.
func ScanGaps(ctx context.Context, moves []store.RepMove, color string, src StatsSource, book *eco.Book, o GapOptions, now time.Time) GapScan {
	o = o.norm()
	if book == nil {
		book = eco.Default()
	}
	sc := newScanner(moves, color, o)
	var maxMove int64
	for _, m := range moves {
		maxMove = max(maxMove, m.ID)
	}
	useStats := src != nil && src.Enabled()
	out := GapScan{Available: useStats, Ratings: o.Ratings, Speeds: o.Speeds, MinPct: o.MinPct, At: now.UTC().Format(time.RFC3339), Moves: len(moves), MaxMove: maxMove}
	stopExternal := !useStats
	errs := 0
	var holes, pend float64
	var lastNeed map[string]float64
	var lastNeedAt map[string]int64

	for round := 0; ; round++ {
		holes, pend = sc.pass()
		lastNeed, lastNeedAt = sc.need, sc.needAt
		if len(sc.need) == 0 || ctx.Err() != nil || round > 2000 {
			break
		}
		fens := make([]string, 0, len(sc.need))
		for f := range sc.need {
			fens = append(fens, f)
		}
		sort.Slice(fens, func(i, j int) bool {
			if sc.need[fens[i]] != sc.need[fens[j]] {
				return sc.need[fens[i]] > sc.need[fens[j]]
			}
			return fens[i] < fens[j]
		})
		progressed, fetchedNow := false, 0
		for _, fen := range fens {
			if ctx.Err() != nil || fetchedNow >= roundSize {
				break
			}
			if !stopExternal {
				if resp, ok := src.Cached(fen, o.Ratings, o.Speeds); ok {
					sc.known[fen] = infoFromStats(resp, o)
					out.Counts.FromCache++
					progressed = true
					continue
				}
				if out.Counts.Fetched >= o.Budget {
					continue // out of requests: this one waits for the next call
				}
				resp, cached, err := src.Lookup(ctx, fen, o.Ratings, o.Speeds)
				var wait *explorer.WaitError
				switch {
				case err == nil:
					sc.known[fen] = infoFromStats(resp, o)
					if cached {
						out.Counts.FromCache++
					} else {
						out.Counts.Fetched++
						fetchedNow++
					}
					progressed = true
					continue
				case errors.As(err, &wait):
					out.Limited, out.RetryAfter = true, int(wait.Wait.Seconds())+1
					stopExternal = true
				case ctx.Err() != nil:
					continue
				default:
					out.Counts.Fetched++
					fetchedNow++
					if errs++; errs >= 3 {
						stopExternal = true
					}
				}
			}
			// no statistics for this position (no token, a pause, an error): the reference
			sc.known[fen] = infoFromTheory(book, fen, o)
			progressed = true
		}
		if !progressed {
			break
		}
	}

	// the figures
	sort.SliceStable(sc.gaps, func(i, j int) bool {
		a, b := sc.gaps[i], sc.gaps[j]
		if a.Weight != b.Weight {
			return a.Weight > b.Weight
		}
		if a.Ply != b.Ply {
			return a.Ply < b.Ply
		}
		return a.UCI < b.UCI
	})
	for _, g := range sc.gaps {
		out.Counts.Gaps++
		if g.Kind == GapMissing {
			out.Counts.Missing++
		} else {
			out.Counts.Answer++
		}
	}
	out.Gaps = sc.gaps
	if len(out.Gaps) > o.Limit {
		out.Gaps = out.Gaps[:o.Limit]
	}
	if out.Gaps == nil {
		out.Gaps = []Gap{}
	}
	out.Counts.Checked = len(sc.known)
	out.Counts.Pending = len(lastNeed)
	out.Partial = len(lastNeed) > 0
	out.Pending = []PendingNode{}
	for f, w := range lastNeed {
		out.Pending = append(out.Pending, PendingNode{At: lastNeedAt[f], Weight: w})
	}
	sort.Slice(out.Pending, func(i, j int) bool {
		if out.Pending[i].Weight != out.Pending[j].Weight {
			return out.Pending[i].Weight > out.Pending[j].Weight
		}
		return out.Pending[i].At < out.Pending[j].At
	})
	if len(out.Pending) > 20 {
		out.Pending = out.Pending[:20]
	}
	out.Coverage = max(0, min(1, 1-holes-pend))

	lichess, theory := 0, 0
	for _, info := range sc.known {
		if info.source == "lichess" {
			lichess++
		} else {
			theory++
		}
	}
	switch {
	case lichess > 0 && theory > 0:
		out.Source = "mixed"
	case lichess > 0:
		out.Source = "lichess"
	case useStats && !out.Limited && out.Counts.Checked == 0:
		out.Source = "lichess" // nothing needed any statistics
	default:
		out.Source = "theory"
	}
	return out
}

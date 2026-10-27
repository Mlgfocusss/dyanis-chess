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

// scanner holds one search.
type scanner struct {
	color    string
	byID     map[int64]store.RepMove
	children map[int64][]store.RepMove
	o        GapOptions

	known map[string]*posInfo // fen -> statistics
	// per pass
	gaps    []Gap
	need    map[string]float64 // fen -> the greatest weight it was wanted with
	needAt  map[string]int64
	pending int
}

func (sc *scanner) fenOf(id int64) string {
	if id == 0 {
		return engine.StartFEN
	}
	return sc.byID[id].FEN
}

// opponentMoves says whether the opponent is to move after `ply` plies.
func (sc *scanner) opponentMoves(ply int) bool { return (ply%2 == 0) != (sc.color == "white") }

// visit walks the node `id` (plies played, weight w, the moves that lead here) and
// returns the chance of ending up in a hole below it, and of ending up in a
// position that was not checked yet.
func (sc *scanner) visit(id int64, ply int, w float64, trail []string) (holes, pend float64) {
	kids := sc.children[id]
	fen := sc.fenOf(id)
	if !sc.opponentMoves(ply) { // the person's move: any answer of theirs will do
		if len(kids) == 0 {
			g := Gap{Kind: GapAnswer, At: id, Ply: ply, FEN: fen, Path: append([]string{}, trail...), Weight: w}
			if id != 0 {
				m := sc.byID[id]
				g.UCI, g.SAN = m.UCI, m.SAN
			}
			sc.gaps = append(sc.gaps, g)
			return w, 0
		}
		best, bestP := -1.0, 0.0
		for _, c := range kids {
			h, p := sc.visit(c.ID, ply+1, w, append(trail, c.SAN))
			if best < 0 || h+p < best+bestP {
				best, bestP = h, p
			}
		}
		return best, bestP
	}

	info := sc.known[fen]
	if info == nil && w >= sc.o.MinWeight {
		if w > sc.need[fen] {
			sc.need[fen], sc.needAt[fen] = w, id
		}
		pend = w
		sc.pending++
	}
	// the statistics may also leave out moves of the tree: their weight is the share
	// when it is known, 0 when it is not (the line is still searched for holes)
	seen := map[string]bool{}
	byUCI := map[string]store.RepMove{}
	for _, c := range kids {
		byUCI[c.UCI] = c
	}
	if info != nil {
		for _, m := range info.moves {
			if !m.popular {
				continue
			}
			seen[m.uci] = true
			if c, ok := byUCI[m.uci]; ok {
				h, p := sc.visitReply(c, ply, w*m.share, trail, &m, info)
				holes, pend = holes+h, pend+p
			} else if w*m.share >= sc.o.MinWeight {
				sc.gaps = append(sc.gaps, Gap{Kind: GapMissing, At: id, Ply: ply, FEN: fen, Path: append([]string{}, trail...),
					UCI: m.uci, SAN: m.san, Pct: 100 * m.share, Games: m.games, Lines: m.lines, Weight: w * m.share, Source: info.source})
				holes += w * m.share
			}
		}
	}
	for _, c := range kids {
		if seen[c.UCI] {
			continue
		}
		var m *statMove
		share := 0.0
		if info != nil {
			if sm, ok := info.find(c.UCI); ok {
				m, share = &sm, sm.share
			}
		}
		h, p := sc.visitReply(c, ply, w*share, trail, m, info)
		holes, pend = holes+h, pend+p
	}
	return holes, pend
}

func (sc *scanner) visitReply(c store.RepMove, ply int, w float64, trail []string, m *statMove, info *posInfo) (float64, float64) {
	before := len(sc.gaps)
	h, p := sc.visit(c.ID, ply+1, w, append(trail, c.SAN))
	// the unanswered reply itself is a hole that can say how popular the move is
	if m != nil && info != nil {
		for i := before; i < len(sc.gaps); i++ {
			if g := &sc.gaps[i]; g.Kind == GapAnswer && g.At == c.ID {
				g.Pct, g.Games, g.Lines, g.Source = 100*m.share, m.games, m.lines, info.source
			}
		}
	}
	return h, p
}

// ScanGaps searches the repertoire (the moves of a repertoire of `color`) for holes.
// src may be nil: the reference stands in for the statistics.
func ScanGaps(ctx context.Context, moves []store.RepMove, color string, src StatsSource, book *eco.Book, o GapOptions, now time.Time) GapScan {
	o = o.norm()
	if book == nil {
		book = eco.Default()
	}
	sc := &scanner{color: color, byID: map[int64]store.RepMove{}, children: map[int64][]store.RepMove{}, o: o, known: map[string]*posInfo{}}
	var maxMove int64
	for _, m := range moves {
		sc.byID[m.ID] = m
		sc.children[m.ParentID] = append(sc.children[m.ParentID], m)
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
		sc.gaps, sc.need, sc.needAt, sc.pending = nil, map[string]float64{}, map[string]int64{}, 0
		holes, pend = sc.visit(0, 0, 1, nil)
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

package repertoire

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"dyanis-chess/backend/internal/explorer"
	"dyanis-chess/backend/internal/store"
)

// scanTimeout is the longest a search for holes may take.
const scanTimeout = 60 * time.Second

// One person may cost Lichess this many requests an hour through searches (the cache does not count):
// a guest account is free to make, and the token of the site is shared.
const hourlyQuota = 300

type quotaState struct {
	since time.Time
	used  int
}

// takeQuota says how many requests to Lichess a search of this person may still make now (at most want).
func (s *Service) takeQuota(uid string, want int, now time.Time) int {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	q := s.quota[uid]
	if now.Sub(q.since) >= time.Hour {
		q = quotaState{since: now}
	}
	left := max(hourlyQuota-q.used, 0)
	s.quota[uid] = q
	return min(want, left)
}

// spendQuota records the requests a search made.
func (s *Service) spendQuota(uid string, n int, now time.Time) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	q := s.quota[uid]
	if q.since.IsZero() {
		q.since = now
	}
	q.used += n
	s.quota[uid] = q
}

// GapSummary is what the list of repertoires says about holes without asking Lichess: the lines
// that end with the opponent's move (counted afresh each time) and the figures of the last search.
type GapSummary struct {
	Open     int     `json:"open"`
	Scanned  bool    `json:"scanned"`
	At       string  `json:"at,omitempty"`
	Missing  int     `json:"missing"`
	Answer   int     `json:"answer"`
	Coverage float64 `json:"coverage"`
	Partial  bool    `json:"partial"`
	Source   string  `json:"source,omitempty"`
	Stale    bool    `json:"stale"`
}

func summaryOf(o store.RepOverview, moves int) GapSummary {
	g := GapSummary{Open: o.Open}
	if o.Scan != nil {
		g.Scanned = true
		g.At = o.Scan.ScannedAt.UTC().Format(time.RFC3339)
		g.Missing, g.Answer, g.Coverage, g.Partial, g.Source = o.Scan.Missing, o.Scan.Answer, o.Scan.Coverage, o.Scan.Partial, o.Scan.Source
		g.Stale = o.Scan.Moves != moves || o.Scan.MaxMoveID != o.MaxMove
	}
	return g
}

func floatParam(r *http.Request, name string, def, lo, hi float64) float64 {
	v, err := strconv.ParseFloat(r.URL.Query().Get(name), 64)
	if err != nil || v != v {
		return def
	}
	return max(lo, min(hi, v))
}

// gaps answers GET /api/repertoires/{id}/gaps: a search of the tree for popular replies of the
// opponent that it lacks (see ScanGaps). ratings, speeds as for /api/explorer; minPct (1..50,
// default 5), limit (holes in the answer). The result is saved for the start page and for
// ?saved=1, which answers with the last saved search (204 when there was none) and asks nobody.
func (s *Service) gaps(w http.ResponseWriter, r *http.Request) {
	rep, ok := s.owned(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	moves, err := s.cfg.Store.RepMoves(r.Context(), rep.ID)
	if err != nil {
		serverError(w, "moves", err)
		return
	}
	var maxMove int64
	for _, m := range moves {
		maxMove = max(maxMove, m.ID)
	}

	if q.Get("saved") != "" {
		saved, err := s.cfg.Store.RepScan(r.Context(), rep.UserID, rep.ID)
		if errors.Is(err, store.ErrNotFound) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err != nil {
			serverError(w, "saved scan", err)
			return
		}
		var out GapScan
		if err := json.Unmarshal([]byte(saved.Result), &out); err != nil {
			serverError(w, "saved scan", err)
			return
		}
		out.Stale = saved.Moves != len(moves) || saved.MaxMoveID != maxMove
		writeJSON(w, http.StatusOK, out)
		return
	}

	ratings, speeds, ok := explorer.Params(q.Get("ratings"), q.Get("speeds"))
	if !ok {
		writeError(w, http.StatusBadRequest, "неверный рейтинг или контроль времени")
		return
	}
	opts := GapOptions{
		Ratings: ratings, Speeds: speeds,
		MinPct: floatParam(r, "minPct", DefaultMinPct, 1, 50),
		Limit:  intParam(r, "limit", DefaultGapLimit, MaxGapLimit),
		Budget: s.cfg.GapBudget,
	}

	s.scanMu.Lock()
	if s.scanning[rep.ID] {
		s.scanMu.Unlock()
		writeError(w, http.StatusConflict, "проверка этого репозитория уже идёт")
		return
	}
	s.scanning[rep.ID] = true
	s.scanMu.Unlock()
	defer func() {
		s.scanMu.Lock()
		delete(s.scanning, rep.ID)
		s.scanMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(r.Context(), scanTimeout)
	defer cancel()
	var src StatsSource
	if s.cfg.Stats != nil {
		src = s.cfg.Stats
	}
	want := opts.Budget
	if want == 0 {
		want = DefaultBudget
	}
	if left := s.takeQuota(rep.UserID, want, s.cfg.Now()); left < want {
		opts.Budget = left
		if left == 0 {
			opts.Budget = -1 // nothing more from Lichess this hour; the cache still answers
		}
	}
	out := ScanGaps(ctx, moves, rep.Color, src, s.cfg.Book, opts, s.cfg.Now())
	s.spendQuota(rep.UserID, out.Counts.Fetched, s.cfg.Now())

	if blob, err := json.Marshal(out); err == nil {
		err = s.cfg.Store.SaveRepScan(r.Context(), rep.UserID, store.RepScan{
			RepertoireID: rep.ID, ScannedAt: s.cfg.Now(), Moves: out.Moves, MaxMoveID: out.MaxMove,
			Missing: out.Counts.Missing, Answer: out.Counts.Answer, Coverage: out.Coverage, Partial: out.Partial, Source: out.Source, Result: string(blob),
		})
		if err != nil {
			serverError(w, "save scan", err) // the answer is lost with it: the next search repeats it from the cache
			return
		}
	}
	writeJSON(w, http.StatusOK, out)
}

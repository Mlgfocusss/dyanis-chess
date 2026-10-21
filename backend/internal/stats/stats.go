// Package stats builds the numbers of the profile's statistics tab: games, reviews, puzzles and
// the streak of days with puzzles.
package stats

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"dyanis-chess/backend/internal/ratings"
	"dyanis-chess/backend/internal/store"
)

// Store is the part of *store.Store the service uses.
type Store interface {
	UserByID(ctx context.Context, id string) (*store.User, error)
	GameTally(ctx context.Context, userID string) (humans, bots store.Tally, err error)
	DuelTally(ctx context.Context, userID string) (store.Tally, error)
	ReviewCount(ctx context.Context, userID string) (int, error)
	RecentReviews(ctx context.Context, userID string, limit int) ([]store.ReviewBrief, error)
	PuzzleRatingOf(ctx context.Context, userID string) (store.PuzzleRating, error)
	PuzzleThemeFailures(ctx context.Context, userID string) ([]store.ThemeFailures, error)
	RecentPuzzleAttempts(ctx context.Context, userID string, limit int) ([]store.AttemptRow, error)
	PuzzleDays(ctx context.Context, userID string) (map[string]int, error)
	PuzzlePointsOf(ctx context.Context, userID, day string) (store.PuzzlePoints, error)
	RushBests(ctx context.Context, userID string) (map[string]store.RushBest, error)
}

// Ratings is the part of *ratings.Service the service uses.
type Ratings interface {
	All(ctx context.Context, userID string) ([]ratings.Entry, error)
}

// Config tunes a Service.
type Config struct {
	Store Store
	// Ratings gives the Glicko-2 ratings; nil: the ratings section stays empty.
	Ratings Ratings
	// UserID names the signed-in user of a request, "" for nobody.
	UserID func(*http.Request) string
	// Now is the clock; days are counted in UTC.
	Now func() time.Time
}

// Service is the statistics API.
type Service struct{ cfg Config }

// New returns a Service.
func New(cfg Config) *Service {
	if cfg.UserID == nil {
		cfg.UserID = func(*http.Request) string { return "" }
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{cfg: cfg}
}

// Handler returns the routes of the statistics API.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/stats", s.get)
	return mux
}

// Limits of the lists in the answer.
const (
	SeriesLen    = 20 // reviews in the accuracy series
	reviewSample = 100
	HistoryLen   = 30 // tries in the history of puzzles
	WeakThemes   = 8
	ActivityDays = 30
)

// ---------------------------------------------------------------- answer

// Record is wins, draws and losses.
type Record struct {
	Total  int `json:"total"`
	Wins   int `json:"wins"`
	Draws  int `json:"draws"`
	Losses int `json:"losses"`
}

func record(t store.Tally) Record {
	return Record{Total: t.Total(), Wins: t.Wins, Draws: t.Draws, Losses: t.Losses}
}

// Category is a person's rating in one kind of game.
type Category struct {
	Category    string `json:"category"`
	Name        string `json:"name"`
	Rating      int    `json:"rating"`
	Provisional bool   `json:"provisional"`
	Games       int    `json:"games"`
	Wins        int    `json:"wins"`
	Draws       int    `json:"draws"`
	Losses      int    `json:"losses"`
}

// Games is how the person has done in online games.
type Games struct {
	Record
	Humans Record `json:"humans"`
	Bots   Record `json:"bots"`
	// Categories holds bullet, blitz and rapid: the rated games between people (the ratings table).
	Categories []Category `json:"categories"`
	// Duel is the rating of the puzzle duel; nil without ratings.
	Duel *Category `json:"duel"`
}

// AccuracyPoint is the accuracy of one review.
type AccuracyPoint struct {
	ID       string    `json:"id"`
	Accuracy float64   `json:"accuracy"` // 0..100
	Side     string    `json:"side"`     // "white" or "black": the person's colour; "both": the mean of both sides
	At       time.Time `json:"at"`
}

// Reviews is how the person's reviewed games went.
type Reviews struct {
	Count int `json:"count"` // finished reviews in all
	// Sample is how many of the latest reviews the average is taken over.
	Sample int `json:"sample"`
	// Known is how many of those had the person's colour recognised by name; in the rest the
	// accuracy is the mean of both sides.
	Known   int             `json:"known"`
	Average *float64        `json:"average"` // nil without reviews
	Series  []AccuracyPoint `json:"series"`  // the latest SeriesLen, oldest first
}

// Theme is a puzzle theme the person fails at.
type Theme struct {
	Theme    string `json:"theme"`
	Attempts int    `json:"attempts"`
	Failed   int    `json:"failed"`
}

// Attempt is one try at a puzzle.
type Attempt struct {
	Puzzle string    `json:"puzzle"`
	Solved bool      `json:"solved"`
	Before int       `json:"before"`
	After  int       `json:"after"`
	Delta  int       `json:"delta"`
	At     time.Time `json:"at"`
}

// Puzzles is how the person has done in puzzles.
type Puzzles struct {
	Attempts int                       `json:"attempts"`
	Solved   int                       `json:"solved"`
	Percent  int                       `json:"percent"` // solved of attempts, 0 without attempts
	Rating   int                       `json:"rating"`
	Weak     []Theme                   `json:"weak"`
	Rush     map[string]store.RushBest `json:"rush"`
	Duels    Record                    `json:"duels"`
	History  []Attempt                 `json:"history"`
	// Points are the puzzle points of the points mode (settings → Игровой процесс).
	Points PuzzlePoints `json:"points"`
}

// PuzzlePoints is the person's puzzle points: all of them, today's (UTC), the series of solved
// puzzles in a row now and the best series.
type PuzzlePoints struct {
	Total  int `json:"total"`
	Today  int `json:"today"`
	Streak int `json:"streak"`
	Best   int `json:"best"`
}

// Streak is the series of days with puzzles. A day counts when the person did at least one thing in
// puzzles on it (a rated try, the puzzle of the day, a run of the storm or a duel); days are UTC
// calendar days. The current streak is the run of days that ends today, or ends yesterday when
// today has not had a puzzle yet (it is not broken until the day is over).
type Streak struct {
	Current int `json:"current"`
	Best    int `json:"best"`
	// Today says whether today already has a puzzle.
	Today bool `json:"today"`
	// From is the first day of Days (YYYY-MM-DD, UTC); Days has one number per day up to today.
	From string `json:"from"`
	Days []int  `json:"days"`
}

// Answer is the body of GET /api/stats.
type Answer struct {
	Games   Games   `json:"games"`
	Reviews Reviews `json:"reviews"`
	Puzzles Puzzles `json:"puzzles"`
	Streak  Streak  `json:"streak"`
}

// ---------------------------------------------------------------- handler

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	uid := s.cfg.UserID(r)
	if uid == "" {
		writeError(w, http.StatusUnauthorized, "войдите, чтобы увидеть статистику")
		return
	}
	ans, err := s.Build(r.Context(), uid)
	if err != nil {
		log.Printf("stats: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось собрать статистику")
		return
	}
	writeJSON(w, http.StatusOK, ans)
}

// Build collects the statistics of one person.
func (s *Service) Build(ctx context.Context, uid string) (*Answer, error) {
	st := s.cfg.Store
	ans := &Answer{}

	humans, bots, err := st.GameTally(ctx, uid)
	if err != nil {
		return nil, err
	}
	ans.Games = Games{
		Record: record(store.Tally{Wins: humans.Wins + bots.Wins, Draws: humans.Draws + bots.Draws, Losses: humans.Losses + bots.Losses}),
		Humans: record(humans), Bots: record(bots), Categories: []Category{},
	}
	if s.cfg.Ratings != nil {
		all, err := s.cfg.Ratings.All(ctx, uid)
		if err != nil {
			return nil, err
		}
		for _, e := range all {
			c := Category{Category: e.Category, Name: e.Name, Rating: e.Rating, Provisional: e.Provisional,
				Games: e.Games, Wins: e.Wins, Draws: e.Draws, Losses: e.Losses}
			if e.Category == ratings.Duel {
				ans.Games.Duel = &c
				continue
			}
			ans.Games.Categories = append(ans.Games.Categories, c)
		}
	}

	name := ""
	if u, err := st.UserByID(ctx, uid); err == nil {
		name = u.Name
	} else if err != store.ErrNotFound {
		return nil, err
	}
	if ans.Reviews, err = s.reviews(ctx, uid, name); err != nil {
		return nil, err
	}

	if ans.Puzzles, err = s.puzzles(ctx, uid); err != nil {
		return nil, err
	}

	days, err := st.PuzzleDays(ctx, uid)
	if err != nil {
		return nil, err
	}
	ans.Streak = BuildStreak(days, s.cfg.Now())
	return ans, nil
}

func (s *Service) reviews(ctx context.Context, uid, name string) (Reviews, error) {
	out := Reviews{Series: []AccuracyPoint{}}
	var err error
	if out.Count, err = s.cfg.Store.ReviewCount(ctx, uid); err != nil {
		return out, err
	}
	if out.Count == 0 {
		return out, nil
	}
	rows, err := s.cfg.Store.RecentReviews(ctx, uid, reviewSample)
	if err != nil {
		return out, err
	}
	points := make([]AccuracyPoint, 0, len(rows)) // newest first, as read
	for _, r := range rows {
		var sm struct {
			Accuracy [2]float64 `json:"accuracy"`
		}
		var g struct {
			White string `json:"white"`
			Black string `json:"black"`
		}
		if json.Unmarshal(r.Summary, &sm) != nil || json.Unmarshal(r.Game, &g) != nil {
			continue // an unreadable row is not worth failing the page
		}
		side := SideOf(name, g.White, g.Black)
		var acc float64
		switch side {
		case "white":
			acc = sm.Accuracy[0]
		case "black":
			acc = sm.Accuracy[1]
		default:
			acc = (sm.Accuracy[0] + sm.Accuracy[1]) / 2
		}
		if side != "both" {
			out.Known++
		}
		points = append(points, AccuracyPoint{ID: r.ID, Accuracy: round1(acc), Side: side, At: r.CreatedAt})
	}
	out.Sample = len(points)
	if len(points) == 0 {
		return out, nil
	}
	sum := 0.0
	for _, p := range points {
		sum += p.Accuracy
	}
	avg := round1(sum / float64(len(points)))
	out.Average = &avg
	n := min(SeriesLen, len(points))
	for i := n - 1; i >= 0; i-- { // oldest first
		out.Series = append(out.Series, points[i])
	}
	return out, nil
}

// SideOf finds the colour of the person in a game by the names in the PGN: "white" or "black"
// when exactly one of them is the person's name (ignoring case and spaces at the ends), and
// "both" when that cannot be told (neither name, or both, matches).
func SideOf(name, white, black string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return "both"
	}
	w := strings.ToLower(strings.TrimSpace(white)) == n
	b := strings.ToLower(strings.TrimSpace(black)) == n
	switch {
	case w && !b:
		return "white"
	case b && !w:
		return "black"
	}
	return "both"
}

func round1(x float64) float64 { return math.Round(x*10) / 10 }

// technical themes say how long a puzzle is or how it was found, not what it is about: they never
// show up among the weak themes (the same set as ideaThemes in web/src/puzzles/themes.js, plus the
// phases of the game, which fit every second puzzle).
var technical = map[string]bool{
	"short": true, "long": true, "veryLong": true, "oneMove": true, "master": true, "masterVsMaster": true,
	"superGM": true, "crushing": true, "advantage": true, "equality": true,
	"opening": true, "middlegame": true, "endgame": true,
}

func (s *Service) puzzles(ctx context.Context, uid string) (Puzzles, error) {
	st := s.cfg.Store
	out := Puzzles{Weak: []Theme{}, History: []Attempt{}, Rush: map[string]store.RushBest{}}
	rec, err := st.PuzzleRatingOf(ctx, uid)
	if err != nil {
		return out, err
	}
	out.Attempts, out.Solved, out.Rating = rec.Attempts, rec.Solved, rec.Rating
	if rec.Attempts > 0 {
		out.Percent = int(math.Round(100 * float64(rec.Solved) / float64(rec.Attempts)))
	}

	fails, err := st.PuzzleThemeFailures(ctx, uid)
	if err != nil {
		return out, err
	}
	out.Weak = WeakestThemes(fails, WeakThemes)

	if out.Rush, err = st.RushBests(ctx, uid); err != nil {
		return out, err
	}
	if out.Rush == nil {
		out.Rush = map[string]store.RushBest{}
	}
	duels, err := st.DuelTally(ctx, uid)
	if err != nil {
		return out, err
	}
	out.Duels = record(duels)

	pts, err := st.PuzzlePointsOf(ctx, uid, s.cfg.Now().UTC().Format(dayFormat))
	if err != nil {
		return out, err
	}
	out.Points = PuzzlePoints{Total: pts.Total, Today: pts.Today, Streak: pts.Streak, Best: pts.BestStreak}

	attempts, err := st.RecentPuzzleAttempts(ctx, uid, HistoryLen)
	if err != nil {
		return out, err
	}
	for _, a := range attempts {
		out.History = append(out.History, Attempt{Puzzle: a.PuzzleID, Solved: a.Solved, Before: a.Before, After: a.After, Delta: a.After - a.Before, At: a.At})
	}
	return out, nil
}

// WeakestThemes picks the themes with the most failures, leaving out the technical ones: more
// failures first, then the worse share of failures, then by name so that the order is stable.
func WeakestThemes(fails []store.ThemeFailures, limit int) []Theme {
	out := []Theme{}
	for _, f := range fails {
		if f.Failed > 0 && !technical[f.Theme] {
			out = append(out, Theme{Theme: f.Theme, Attempts: f.Attempts, Failed: f.Failed})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Failed != b.Failed {
			return a.Failed > b.Failed
		}
		// a.Failed/a.Attempts > b.Failed/b.Attempts, without dividing
		if l, r := a.Failed*b.Attempts, b.Failed*a.Attempts; l != r {
			return l > r
		}
		return a.Theme < b.Theme
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ---------------------------------------------------------------- streak

const dayFormat = "2006-01-02"

// BuildStreak counts the streaks from the number of puzzles done on each day (keys are
// "YYYY-MM-DD", UTC) as of now, and lists the last ActivityDays days.
func BuildStreak(days map[string]int, now time.Time) Streak {
	today := now.UTC().Truncate(24 * time.Hour)
	active := func(d time.Time) bool { return days[d.Format(dayFormat)] > 0 }

	s := Streak{Today: active(today), Days: make([]int, ActivityDays)}
	first := today.AddDate(0, 0, -(ActivityDays - 1))
	s.From = first.Format(dayFormat)
	for i := range s.Days {
		s.Days[i] = days[first.AddDate(0, 0, i).Format(dayFormat)]
	}

	// current: back from today, or from yesterday while today is still empty
	d := today
	if !s.Today {
		d = d.AddDate(0, 0, -1)
	}
	for active(d) {
		s.Current++
		d = d.AddDate(0, 0, -1)
	}

	// best: the longest run of consecutive days anywhere
	keys := make([]string, 0, len(days))
	for k, n := range days {
		if n > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	run := 0
	var prev time.Time
	for _, k := range keys {
		t, err := time.Parse(dayFormat, k)
		if err != nil {
			continue
		}
		if run > 0 && t.Equal(prev.AddDate(0, 0, 1)) {
			run++
		} else {
			run = 1
		}
		prev = t
		s.Best = max(s.Best, run)
	}
	s.Best = max(s.Best, s.Current)
	return s
}

// ---------------------------------------------------------------- JSON

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

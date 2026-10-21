// Package trainer is the personal trainer of the site: it looks at how a person plays, from the
// games, reviews, puzzles and repositories of moves they already have here, and makes a plan for
// the day, a picture of their play and a few recommendations. It is rules over the person's own
// data: no model and no outside service. All thresholds are in rules.go.
package trainer

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"dyanis-chess/backend/internal/stats"
	"dyanis-chess/backend/internal/store"
)

// Store is the part of *store.Store the service uses.
type Store interface {
	UserByID(ctx context.Context, id string) (*store.User, error)
	PuzzleRatingOf(ctx context.Context, userID string) (store.PuzzleRating, error)
	PuzzleThemeFailures(ctx context.Context, userID string) ([]store.ThemeFailures, error)
	PuzzleDays(ctx context.Context, userID string) (map[string]int, error)
	PuzzlesSince(ctx context.Context, userID string, since time.Time, theme string) (all, onTheme int, err error)
	RepertoireReviewedSince(ctx context.Context, userID string, since time.Time) (int, error)
	ListRepertoires(ctx context.Context, userID string, now time.Time) ([]store.RepertoireWithCounts, error)
	RepMoves(ctx context.Context, repertoireID int64) ([]store.RepMove, error)
	ListGames(ctx context.Context, userID string, limit int) ([]*store.Game, error)
	ListAnalyses(ctx context.Context, userID string, limit int) ([]*store.Analysis, error)
	RecentReports(ctx context.Context, userID string, limit int) ([]store.ReportRow, error)
	RecordPractice(ctx context.Context, userID, puzzleID string, solved bool, at time.Time) error
}

// Config tunes a Service.
type Config struct {
	Store Store
	// UserID names the signed-in user of a request, "" for nobody.
	UserID func(*http.Request) string
	// Now is the clock; the days of the plan are UTC days.
	Now func() time.Time
}

// Service is the trainer API.
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

// Handler returns the routes of the trainer API.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/trainer", s.get)
	mux.HandleFunc("POST /api/trainer/practice", s.practice)
	return mux
}

// StreakInfo is the streak of days with puzzles, as the greeting needs it.
type StreakInfo struct {
	Current int  `json:"current"`
	Best    int  `json:"best"`
	Today   bool `json:"today"`
}

// Answer is the body of GET /api/trainer.
type Answer struct {
	Name     string     `json:"name"`
	Newcomer bool       `json:"newcomer"`
	Greeting Greeting   `json:"greeting"`
	Streak   StreakInfo `json:"streak"`
	// PuzzlesToday is how many puzzles were tried today (UTC).
	PuzzlesToday int        `json:"puzzlesToday"`
	Day          Day        `json:"day"`
	Plan         []PlanItem `json:"plan"`
	Insights     Insights   `json:"insights"`
	Advice       []Advice   `json:"advice"`
}

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	uid := s.cfg.UserID(r)
	if uid == "" {
		writeError(w, http.StatusUnauthorized, "войдите, чтобы увидеть тренера")
		return
	}
	hour := s.cfg.Now().UTC().Hour()
	if h, err := strconv.Atoi(r.URL.Query().Get("hour")); err == nil && h >= 0 && h <= 23 {
		hour = h // the person's own clock: the server does not know the time zone
	}
	ans, err := s.Build(r.Context(), uid, hour)
	if err != nil {
		log.Printf("trainer: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось составить план")
		return
	}
	writeJSON(w, http.StatusOK, ans)
}

var puzzleIDRe = regexp.MustCompile(`^[A-Za-z0-9]{1,12}$`)

// practice records a puzzle done as practice (by theme), so that the plan can count it.
func (s *Service) practice(w http.ResponseWriter, r *http.Request) {
	uid := s.cfg.UserID(r)
	if uid == "" {
		writeError(w, http.StatusUnauthorized, "войдите, чтобы вести план")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	var req struct {
		Puzzle string `json:"puzzle"`
		Solved bool   `json:"solved"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || !puzzleIDRe.MatchString(req.Puzzle) {
		writeError(w, http.StatusBadRequest, "ожидался JSON {\"puzzle\": \"...\", \"solved\": true}")
		return
	}
	if err := s.cfg.Store.RecordPractice(r.Context(), uid, req.Puzzle, req.Solved, s.cfg.Now()); err != nil {
		log.Printf("trainer: practice: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось записать задачу")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Build collects everything the page shows for one person; hour is their local hour.
func (s *Service) Build(ctx context.Context, uid string, hour int) (*Answer, error) {
	st := s.cfg.Store
	now := s.cfg.Now().UTC()
	dayStart := now.Truncate(24 * time.Hour)
	ans := &Answer{}

	if u, err := st.UserByID(ctx, uid); err == nil {
		ans.Name = u.Name
	} else if err != store.ErrNotFound {
		return nil, err
	}

	// puzzles and the streak
	rec, err := st.PuzzleRatingOf(ctx, uid)
	if err != nil {
		return nil, err
	}
	fails, err := st.PuzzleThemeFailures(ctx, uid)
	if err != nil {
		return nil, err
	}
	weak := stats.WeakestThemes(fails, stats.WeakThemes)
	days, err := st.PuzzleDays(ctx, uid)
	if err != nil {
		return nil, err
	}
	streak := stats.BuildStreak(days, now)
	ans.Streak = StreakInfo{Current: streak.Current, Best: streak.Best, Today: streak.Today}
	yesterday := days[dayStart.AddDate(0, 0, -1).Format("2006-01-02")]

	var weakest *stats.Theme
	theme := ""
	if len(weak) > 0 {
		weakest, theme = &weak[0], weak[0].Theme
	}
	today, onTheme, err := st.PuzzlesSince(ctx, uid, dayStart, theme)
	if err != nil {
		return nil, err
	}
	ans.PuzzlesToday = today

	// repositories
	reps, err := st.ListRepertoires(ctx, uid, now)
	if err != nil {
		return nil, err
	}
	repos := make([]Repo, 0, len(reps))
	for _, r := range reps {
		repos = append(repos, Repo{ID: r.ID, Name: r.Name, Due: r.Counts.Due, New: r.Counts.New, Mine: r.Counts.Mine})
	}
	repReviewed, err := st.RepertoireReviewedSince(ctx, uid, dayStart)
	if err != nil {
		return nil, err
	}

	// games and the reviews of them
	gameRows, err := st.ListGames(ctx, uid, 60)
	if err != nil {
		return nil, err
	}
	listed, err := st.ListAnalyses(ctx, uid, 80)
	if err != nil {
		return nil, err
	}
	infos, reviewsToday := reviewInfos(listed, dayStart)
	recent := ReadGames(gameRows, uid, now.AddDate(0, 0, -RecentGameDays), MinReviewPlies)
	games := make([]RecentGame, 0, len(recent))
	for _, g := range recent {
		games = append(games, RecentGame{ID: g.ID, Opponent: g.Opponent, Result: g.Result, At: g.At, Reviewed: Reviewed(g, infos)})
	}

	// the reviews themselves
	reports, err := st.RecentReports(ctx, uid, ReviewSample)
	if err != nil {
		return nil, err
	}
	var used []*Review
	skipped := 0
	for _, row := range reports {
		if rv, why := ReadReview(row, ans.Name); why == "" {
			used = append(used, rv)
		} else {
			skipped++
		}
	}
	ans.Insights = BuildInsights(used, len(reports), skipped)

	ans.Newcomer = len(gameRows) == 0 && len(listed) == 0 && rec.Attempts == 0 && streak.Best == 0 && len(reps) == 0
	ans.Plan = BuildPlan(PlanInput{
		Newcomer: ans.Newcomer, WeakTheme: weakest, PuzzlesToday: today, ThemeToday: onTheme,
		Repos: repos, RepReviewedToday: repReviewed, Games: games, ReviewsToday: reviewsToday,
	})
	ans.Day = Tally(ans.Plan)

	dev, err := s.deviations(ctx, uid, reps, gameRows, now)
	if err != nil {
		return nil, err
	}
	ans.Advice = BuildAdvice(AdviceInput{Newcomer: ans.Newcomer, Insights: ans.Insights, WeakThemes: weak, Deviation: dev, Streak: streak})
	ans.Greeting = Greet(GreetingInput{
		Name: ans.Name, Hour: hour, Newcomer: ans.Newcomer, Streak: streak.Current, Best: streak.Best, Today: streak.Today,
		Yesterday: yesterday, Trend: ans.Insights.Trend, Day: ans.Day,
	})
	return ans, nil
}

// deviations puts the person's games of the last DeviationDays days against their repositories (the
// biggest MaxRepositories of them).
func (s *Service) deviations(ctx context.Context, uid string, reps []store.RepertoireWithCounts, rows []*store.Game, now time.Time) (Deviation, error) {
	var withMoves []store.RepertoireWithCounts
	for _, r := range reps {
		if r.Counts.Mine > 0 {
			withMoves = append(withMoves, r)
		}
	}
	if len(withMoves) == 0 {
		return Deviation{}, nil
	}
	sort.SliceStable(withMoves, func(i, j int) bool { return withMoves[i].Counts.Moves > withMoves[j].Counts.Moves })
	if len(withMoves) > MaxRepositories {
		withMoves = withMoves[:MaxRepositories]
	}
	seen := ReadGames(rows, uid, now.AddDate(0, 0, -DeviationDays), 2)
	if len(seen) > DeviationGames {
		seen = seen[:DeviationGames]
	}
	if len(seen) == 0 {
		return Deviation{}, nil
	}
	repos := make([]RepoMoves, 0, len(withMoves))
	for _, r := range withMoves {
		moves, err := s.cfg.Store.RepMoves(ctx, r.ID)
		if err != nil {
			return Deviation{}, err
		}
		repos = append(repos, RepoMoves{Color: r.Color, Moves: moves})
	}
	games := make([]GameMoves, 0, len(seen))
	for _, g := range seen {
		games = append(games, GameMoves{Color: g.Color, UCIs: g.UCIs})
	}
	return DeviationOf(repos, games), nil
}

// ---------------------------------------------------------------- reviews of games

// ReviewInfo is a review of the person as far as it tells which game it is about.
type ReviewInfo struct {
	White, Black string
	At           time.Time
}

// reviewInfos reads the names of the players of the person's reviews, and counts those started since
// dayStart.
func reviewInfos(list []*store.Analysis, dayStart time.Time) (infos []ReviewInfo, today int) {
	for _, a := range list {
		if a.Status != "done" {
			continue
		}
		var g struct {
			White string `json:"white"`
			Black string `json:"black"`
		}
		if json.Unmarshal(a.Game, &g) != nil {
			continue
		}
		at := a.CreatedAt.UTC()
		infos = append(infos, ReviewInfo{White: g.White, Black: g.Black, At: at})
		if !at.Before(dayStart) {
			today++
		}
	}
	return infos, today
}

// pgnName is a player's name as the PGN of an online game writes it (see play.Game.PGN).
func pgnName(n string) string {
	if n == "" {
		return "?"
	}
	return strings.NewReplacer(`"`, "'", `\`, "'", "\n", " ", "\r", " ").Replace(n)
}

// Reviewed says whether the person has a review of this game: one made after the game began, whose
// players have the names of the game's players. (A review keeps no link to the game it came from, so
// this is the best the data allows; two games against the same opponent are told apart by time.)
func Reviewed(g Seen, infos []ReviewInfo) bool {
	white, black := pgnName(g.WhiteName), pgnName(g.BlackName)
	for _, in := range infos {
		if in.White == white && in.Black == black && !in.At.Before(g.CreatedAt.Add(-time.Minute)) {
			return true
		}
	}
	return false
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

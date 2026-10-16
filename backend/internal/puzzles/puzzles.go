package puzzles

import (
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"log"
	"math"
	mrand "math/rand"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"dyanis-chess/backend/internal/store"
)

// Store is the part of *store.Store the service uses.
type Store interface {
	PuzzleByID(ctx context.Context, id string) (*store.Puzzle, error)
	PickPuzzle(ctx context.Context, userID, theme string, lo, hi int, skipTried bool) (*store.Puzzle, error)
	DailyPuzzle(ctx context.Context, lo, hi, minPopularity int, seed uint64) (*store.Puzzle, error)
	DailyPuzzleResult(ctx context.Context, userID, day string) (tried, solved bool, err error)
	RecordDailyPuzzle(ctx context.Context, userID, day, puzzleID string, solved bool) error
	RecordRush(ctx context.Context, userID, mode string, score int) error
	SaveDuel(ctx context.Context, d *store.DuelRow) error
	RushBests(ctx context.Context, userID string) (map[string]store.RushBest, error)
	PuzzleThemes(ctx context.Context) ([]store.ThemeCount, error)
	PuzzleRatingOf(ctx context.Context, userID string) (store.PuzzleRating, error)
	RecordPuzzleAttempt(ctx context.Context, userID, puzzleID string, solved bool, update func(rating, attempts int) int) (store.PuzzleAttempt, error)
}

// Config tunes a Service.
type Config struct {
	Store Store
	// UserID names the signed-in user of a request, "" for nobody.
	UserID func(*http.Request) string
	// Now is the clock; the puzzle of the day changes at midnight UTC.
	Now func() time.Time
	// Ratings rates the puzzle duels; nil: they are not rated.
	Ratings DuelRatings
}

// Service is the puzzle API.
type Service struct {
	cfg Config

	mu         sync.Mutex
	themes     []store.ThemeCount
	themesTime time.Time
	dailyDate  string // the day dailyID was picked for
	dailyID    string

	duelMu sync.Mutex
	duels  map[string]*duel

	rand func(n int) int // a random number in [0, n); a test replaces it
}

// New returns a Service.
func New(cfg Config) *Service {
	if cfg.UserID == nil {
		cfg.UserID = func(*http.Request) string { return "" }
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{cfg: cfg, duels: map[string]*duel{}, rand: mrand.Intn}
}

// Handler returns the routes of the puzzle API.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/puzzles/next", s.next)
	mux.HandleFunc("GET /api/puzzles/daily", s.daily)
	mux.HandleFunc("POST /api/puzzles/daily/result", s.dailyResult)
	mux.HandleFunc("GET /api/puzzles/themes", s.listThemes)
	mux.HandleFunc("GET /api/puzzles/rush/next", s.rushNext)
	mux.HandleFunc("GET /api/puzzles/rush/best", s.rushBest)
	mux.HandleFunc("POST /api/puzzles/rush/result", s.rushResult)
	mux.HandleFunc("GET /api/duels/{id}", s.duelState)
	mux.HandleFunc("POST /api/duels/{id}/progress", s.duelProgressed)
	mux.HandleFunc("GET /api/duels/{id}/puzzle/{k}", s.duelPuzzle)
	mux.HandleFunc("GET /api/puzzles/{id}", s.byID)
	mux.HandleFunc("POST /api/puzzles/{id}/result", s.result)
	return mux
}

// ---------------------------------------------------------------- rating

// MinRating is the lowest rating a person can fall to.
const MinRating = 100

// kFactor is how far one result moves the rating: a lot for a newcomer,
// less as the rating settles.
func kFactor(attempts int) float64 {
	switch {
	case attempts < 10:
		return 48
	case attempts < 50:
		return 32
	default:
		return 20
	}
}

// NewRating is the person's rating after one puzzle (Elo against the
// puzzle's own, fixed rating; the Lichess ratings of the puzzles do not move).
func NewRating(rating, attempts, puzzleRating int, solved bool) int {
	expected := 1 / (1 + math.Pow(10, float64(puzzleRating-rating)/400))
	score := 0.0
	if solved {
		score = 1
	}
	next := int(math.Round(float64(rating) + kFactor(attempts)*(score-expected)))
	return max(next, MinRating)
}

// ------------------------------------------------------------------ API

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// Record is a person's puzzle rating and record, as the browser sees it.
type Record struct {
	Rating   int `json:"rating"`
	Attempts int `json:"attempts"`
	Solved   int `json:"solved"`
}

// PuzzleJSON is one puzzle with its line.
type PuzzleJSON struct {
	ID         string   `json:"id"`
	Rating     int      `json:"rating"`
	Popularity int      `json:"popularity"`
	Plays      int      `json:"plays"`
	Themes     []string `json:"themes"`
	*Line
}

type nextResponse struct {
	Puzzle PuzzleJSON `json:"puzzle"`
	You    *Record    `json:"you,omitempty"`   // nil for nobody signed in
	Daily  *DailyInfo `json:"daily,omitempty"` // set for the puzzle of the day
}

// DailyInfo says which day's puzzle this is and how the person did on it.
type DailyInfo struct {
	Date   string `json:"date"`   // YYYY-MM-DD, UTC
	Status string `json:"status"` // "new", "solved" or "failed" (the first try); "new" for nobody signed in
}

func render(p *store.Puzzle) (PuzzleJSON, error) {
	line, err := BuildLine(p.FEN, p.Moves)
	if err != nil {
		return PuzzleJSON{}, err
	}
	return PuzzleJSON{
		ID: p.ID, Rating: p.Rating, Popularity: p.Popularity, Plays: p.Plays,
		Themes: strings.Fields(p.Themes), Line: line,
	}, nil
}

var themeRe = regexp.MustCompile(`^[A-Za-z0-9]{1,40}$`)

// windows are how far, in rating, the search for a puzzle widens when the
// narrow one is empty (a theme may have no puzzle near the person's rating).
var windows = []int{100, 200, 400, 800, 3000}

func (s *Service) next(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	theme := r.URL.Query().Get("theme")
	if theme != "" && !themeRe.MatchString(theme) {
		writeError(w, http.StatusBadRequest, "неизвестная тема")
		return
	}
	userID := s.cfg.UserID(r)
	rec := store.PuzzleRating{Rating: store.StartPuzzleRating}
	if userID != "" {
		var err error
		if rec, err = s.cfg.Store.PuzzleRatingOf(ctx, userID); err != nil {
			log.Printf("puzzles: rating: %v", err)
			writeError(w, http.StatusInternalServerError, "не удалось загрузить рейтинг")
			return
		}
	}
	p, err := s.pick(ctx, userID, theme, levelRating(rec.Rating, r.URL.Query().Get("level"), s.rand))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "задач с такой темой нет")
		return
	}
	if err != nil {
		log.Printf("puzzles: pick: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось выбрать задачу")
		return
	}
	respond(w, p, userID, rec, nil)
}

// levelRating is the rating the next puzzle is looked for near: the person's own, or a little lower
// ("easier") or higher ("harder"), or at random within 300 points ("random") as they chose in the
// settings. Anything else is their own rating.
func levelRating(rating int, level string, rnd func(n int) int) int {
	shift := 0
	switch level {
	case "easier":
		shift = -150
	case "harder":
		shift = 150
	case "random":
		shift = rnd(601) - 300
	}
	return max(rushMinRating, min(rushMaxRating, rating+shift))
}

// pick finds a puzzle near the rating: one the person has not tried if there
// is any, in a window that widens until something fits.
func (s *Service) pick(ctx context.Context, userID, theme string, rating int) (*store.Puzzle, error) {
	for _, skip := range []bool{true, false} {
		for _, win := range windows {
			p, err := s.cfg.Store.PickPuzzle(ctx, userID, theme, rating-win, rating+win, skip)
			if !errors.Is(err, store.ErrNotFound) {
				return p, err
			}
		}
	}
	return nil, store.ErrNotFound
}

func (s *Service) byID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, err := s.cfg.Store.PuzzleByID(ctx, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "такой задачи нет")
		return
	}
	if err != nil {
		log.Printf("puzzles: get: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось загрузить задачу")
		return
	}
	userID := s.cfg.UserID(r)
	rec := store.PuzzleRating{Rating: store.StartPuzzleRating}
	if userID != "" {
		if rec, err = s.cfg.Store.PuzzleRatingOf(ctx, userID); err != nil {
			log.Printf("puzzles: rating: %v", err)
			writeError(w, http.StatusInternalServerError, "не удалось загрузить рейтинг")
			return
		}
	}
	respond(w, p, userID, rec, nil)
}

func respond(w http.ResponseWriter, p *store.Puzzle, userID string, rec store.PuzzleRating, daily *DailyInfo) {
	pj, err := render(p)
	if err != nil {
		log.Printf("puzzles: %s: %v", p.ID, err)
		writeError(w, http.StatusInternalServerError, "задача повреждена")
		return
	}
	resp := nextResponse{Puzzle: pj, Daily: daily}
	if userID != "" {
		resp.You = &Record{Rating: rec.Rating, Attempts: rec.Attempts, Solved: rec.Solved}
	}
	writeJSON(w, http.StatusOK, resp)
}

// The puzzle of the day is picked from puzzles that are neither easy nor
// brutal and that people liked, by the date alone: everybody gets the same one.
const (
	dailyLo, dailyHi = 1400, 1800
	dailyMinPopular  = 90
)

// dailySeed turns a date into the number that picks the day's puzzle.
func dailySeed(date string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(date))
	return h.Sum64()
}

// dailyPuzzle returns the puzzle of the day, remembered until the date changes.
func (s *Service) dailyPuzzle(ctx context.Context, date string) (*store.Puzzle, error) {
	s.mu.Lock()
	id := ""
	if s.dailyDate == date {
		id = s.dailyID
	}
	s.mu.Unlock()
	if id != "" {
		if p, err := s.cfg.Store.PuzzleByID(ctx, id); !errors.Is(err, store.ErrNotFound) {
			return p, err
		}
	}
	p, err := s.cfg.Store.DailyPuzzle(ctx, dailyLo, dailyHi, dailyMinPopular, dailySeed(date))
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.dailyDate, s.dailyID = date, p.ID
	s.mu.Unlock()
	return p, nil
}

func (s *Service) daily(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	date := s.cfg.Now().UTC().Format("2006-01-02")
	p, err := s.dailyPuzzle(ctx, date)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "задачи дня пока нет")
		return
	}
	if err != nil {
		log.Printf("puzzles: daily: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось выбрать задачу дня")
		return
	}
	userID := s.cfg.UserID(r)
	rec := store.PuzzleRating{Rating: store.StartPuzzleRating}
	info := &DailyInfo{Date: date, Status: "new"}
	if userID != "" {
		if rec, err = s.cfg.Store.PuzzleRatingOf(ctx, userID); err != nil {
			log.Printf("puzzles: rating: %v", err)
			writeError(w, http.StatusInternalServerError, "не удалось загрузить рейтинг")
			return
		}
		tried, solved, err := s.cfg.Store.DailyPuzzleResult(ctx, userID, date)
		if err != nil {
			log.Printf("puzzles: daily result: %v", err)
			writeError(w, http.StatusInternalServerError, "не удалось загрузить попытку")
			return
		}
		switch {
		case tried && solved:
			info.Status = "solved"
		case tried:
			info.Status = "failed"
		}
	}
	respond(w, p, userID, rec, info)
}

// dailyResult takes the first try at the puzzle of the day. It is practice:
// the rating is not touched, only the person's record of the day is kept.
func (s *Service) dailyResult(w http.ResponseWriter, r *http.Request) {
	userID := s.cfg.UserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "войдите, чтобы результат сохранился")
		return
	}
	var req resultRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}
	ctx := r.Context()
	date := s.cfg.Now().UTC().Format("2006-01-02")
	p, err := s.dailyPuzzle(ctx, date)
	if err != nil {
		log.Printf("puzzles: daily: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось найти задачу дня")
		return
	}
	if err := s.cfg.Store.RecordDailyPuzzle(ctx, userID, date, p.ID, req.Solved); err != nil {
		log.Printf("puzzles: record daily: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось записать результат")
		return
	}
	tried, solved, err := s.cfg.Store.DailyPuzzleResult(ctx, userID, date)
	if err != nil || !tried {
		log.Printf("puzzles: daily result: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось прочитать результат")
		return
	}
	info := DailyInfo{Date: date, Status: "failed"}
	if solved {
		info.Status = "solved"
	}
	writeJSON(w, http.StatusOK, info)
}

type resultRequest struct {
	Solved bool `json:"solved"`
}

type resultResponse struct {
	Counted bool   `json:"counted"` // false: tried before, rating unchanged
	Before  int    `json:"before"`
	After   int    `json:"after"`
	You     Record `json:"you"`
}

// result takes the outcome of a person's first try. The browser reports it:
// the rating is personal, so there is nothing to gain by lying.
func (s *Service) result(w http.ResponseWriter, r *http.Request) {
	userID := s.cfg.UserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "войдите, чтобы рейтинг сохранялся")
		return
	}
	var req resultRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}
	ctx := r.Context()
	p, err := s.cfg.Store.PuzzleByID(ctx, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "такой задачи нет")
		return
	}
	if err != nil {
		log.Printf("puzzles: get: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось загрузить задачу")
		return
	}
	at, err := s.cfg.Store.RecordPuzzleAttempt(ctx, userID, p.ID, req.Solved, func(rating, attempts int) int {
		return NewRating(rating, attempts, p.Rating, req.Solved)
	})
	if err != nil {
		log.Printf("puzzles: record: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось записать результат")
		return
	}
	writeJSON(w, http.StatusOK, resultResponse{
		Counted: at.Counted, Before: at.Before, After: at.After,
		You: Record{Rating: at.Record.Rating, Attempts: at.Record.Attempts, Solved: at.Record.Solved},
	})
}

type themeJSON struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

// themesTTL is how long the list of themes is remembered; it only changes by an import.
const themesTTL = 10 * time.Minute

func (s *Service) listThemes(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	themes, fresh := s.themes, time.Since(s.themesTime) < themesTTL
	s.mu.Unlock()
	if !fresh {
		var err error
		if themes, err = s.cfg.Store.PuzzleThemes(r.Context()); err != nil {
			log.Printf("puzzles: themes: %v", err)
			writeError(w, http.StatusInternalServerError, "не удалось загрузить темы")
			return
		}
		s.mu.Lock()
		s.themes, s.themesTime = themes, time.Now()
		s.mu.Unlock()
	}
	out := make([]themeJSON, len(themes))
	for i, t := range themes {
		out[i] = themeJSON{ID: t.Theme, Count: t.Count}
	}
	writeJSON(w, http.StatusOK, map[string]any{"themes": out})
}

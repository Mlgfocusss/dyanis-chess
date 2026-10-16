// Package ratings keeps the Glicko-2 ratings of people (internal/glicko) in one category per kind of
// game, as chess.com does: bullet, blitz and rapid by the time control of the game, and the puzzle
// duel. Only games between two people count; a bot's rating is its own fixed label.
package ratings

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"dyanis-chess/backend/internal/glicko"
	"dyanis-chess/backend/internal/store"
)

// Categories.
const (
	Bullet = "bullet"
	Blitz  = "blitz"
	Rapid  = "rapid"
	Duel   = "duel"
)

// Categories in the order they are shown.
var Categories = []string{Bullet, Blitz, Rapid, Duel}

// Names of the categories for people.
var Names = map[string]string{Bullet: "Пуля", Blitz: "Блиц", Rapid: "Рапид", Duel: "Дуэль"}

// CategoryFor names the kind of a game by its clock, by the estimated length of a game as Lichess
// counts it: the base time plus 40 moves of increment. Under 3 minutes is bullet, under 8 blitz,
// the rest rapid (a classical game is rapid here). A game with no clock is not rated.
func CategoryFor(baseMs, incMs int64) string {
	if baseMs <= 0 {
		return ""
	}
	total := baseMs/1000 + 40*incMs/1000
	switch {
	case total < 180:
		return Bullet
	case total < 480:
		return Blitz
	default:
		return Rapid
	}
}

// Store is the part of *store.Store the service uses.
type Store interface {
	Rating(ctx context.Context, userID, category string) (*store.RatingRow, error)
	Ratings(ctx context.Context, userID string) ([]*store.RatingRow, error)
	SaveRating(ctx context.Context, x *store.RatingRow) error
}

// Service holds the ratings.
type Service struct {
	store Store
	now   func() time.Time
	mu    sync.Mutex // one update at a time: two games of a person may end together
}

// New returns a Service; now is time.Now when nil.
func New(st Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: st, now: now}
}

// Entry is a rating as people see it.
type Entry struct {
	Category    string  `json:"category"`
	Name        string  `json:"name"`
	Rating      int     `json:"rating"`
	RD          int     `json:"rd"`
	Provisional bool    `json:"provisional"` // still a rough guess: shown with "?"
	Games       int     `json:"games"`
	Wins        int     `json:"wins"`
	Draws       int     `json:"draws"`
	Losses      int     `json:"losses"`
	Raw         float64 `json:"-"`
}

// Change is what a game did to a person's rating.
type Change struct {
	Category string `json:"category"`
	Before   int    `json:"before"`
	After    int    `json:"after"`
	// Provisional says the new rating is still a rough guess.
	Provisional bool `json:"provisional,omitempty"`
}

// current is the stored rating of a person with the uncertainty grown by the days since their last game.
func (s *Service) current(ctx context.Context, userID, category string) (glicko.Rating, *store.RatingRow, error) {
	row, err := s.store.Rating(ctx, userID, category)
	if errors.Is(err, store.ErrNotFound) {
		return glicko.New(), nil, nil
	}
	if err != nil {
		return glicko.Rating{}, nil, err
	}
	r := glicko.Rating{R: row.Rating, RD: row.RD, Sigma: row.Volatility}
	days := s.now().Sub(row.UpdatedAt).Hours() / 24
	return glicko.Inflate(r, days), row, nil
}

// Get returns the person's rating in the category (a new player's if they have none yet).
func (s *Service) Get(ctx context.Context, userID, category string) (Entry, error) {
	r, row, err := s.current(ctx, userID, category)
	if err != nil {
		return Entry{}, err
	}
	e := entry(category, r)
	if row != nil {
		e.Games, e.Wins, e.Draws, e.Losses = row.Games, row.Wins, row.Draws, row.Losses
	}
	return e, nil
}

func entry(category string, r glicko.Rating) Entry {
	return Entry{
		Category: category, Name: Names[category], Rating: int(math.Round(r.R)), RD: int(math.Round(r.RD)),
		Provisional: r.IsProvisional(), Raw: r.R,
	}
}

// All returns the person's rating in every category, a new player's where they have none.
func (s *Service) All(ctx context.Context, userID string) ([]Entry, error) {
	out := make([]Entry, 0, len(Categories))
	for _, c := range Categories {
		e, err := s.Get(ctx, userID, c)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// Apply updates the ratings of two people after a game between them in the category. scoreA is
// a's result: 1 a win, 0.5 a draw, 0 a loss. Both are rated from the ratings they had before it.
func (s *Service) Apply(ctx context.Context, category, aID, bID string, scoreA float64) (a, b Change, err error) {
	if aID == "" || bID == "" || aID == bID {
		return a, b, errors.New("ratings: two different people are needed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ra, rowA, err := s.current(ctx, aID, category)
	if err != nil {
		return a, b, err
	}
	rb, rowB, err := s.current(ctx, bID, category)
	if err != nil {
		return a, b, err
	}
	na := glicko.Update(ra, []glicko.Result{{Opp: rb, Score: scoreA}})
	nb := glicko.Update(rb, []glicko.Result{{Opp: ra, Score: 1 - scoreA}})
	if err := s.save(ctx, aID, category, rowA, na, scoreA); err != nil {
		return a, b, err
	}
	if err := s.save(ctx, bID, category, rowB, nb, 1-scoreA); err != nil {
		return a, b, err
	}
	change := func(before, after glicko.Rating) Change {
		return Change{Category: category, Before: int(math.Round(before.R)), After: int(math.Round(after.R)), Provisional: after.IsProvisional()}
	}
	return change(ra, na), change(rb, nb), nil
}

func (s *Service) save(ctx context.Context, userID, category string, old *store.RatingRow, r glicko.Rating, score float64) error {
	row := &store.RatingRow{UserID: userID, Category: category, Rating: r.R, RD: r.RD, Volatility: r.Sigma, UpdatedAt: s.now().UTC()}
	if old != nil {
		row.Games, row.Wins, row.Draws, row.Losses = old.Games, old.Wins, old.Draws, old.Losses
	}
	row.Games++
	switch {
	case score > 0.5:
		row.Wins++
	case score < 0.5:
		row.Losses++
	default:
		row.Draws++
	}
	return s.store.SaveRating(ctx, row)
}

// Handler serves GET /api/ratings: the signed-in person's ratings. user names the person of a request, "" for nobody.
func (s *Service) Handler(user func(*http.Request) string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ratings", func(w http.ResponseWriter, r *http.Request) {
		uid := user(r)
		if uid == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "войдите, чтобы увидеть рейтинги"})
			return
		}
		all, err := s.All(r.Context(), uid)
		if err != nil {
			log.Printf("ratings: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "не удалось прочитать рейтинги"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ratings": all})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

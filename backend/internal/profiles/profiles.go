// Package profiles serves what anybody may see of other people: the leaderboard of a rating
// category and the public profile of a player. Nothing private leaves here (no e-mail, settings
// or sessions); the ratings are the Glicko-2 ones of internal/ratings.
package profiles

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"dyanis-chess/backend/internal/bots"
	"dyanis-chess/backend/internal/countries"
	"dyanis-chess/backend/internal/glicko"
	"dyanis-chess/backend/internal/ratings"
	"dyanis-chess/backend/internal/store"
)

// Who gets on a leaderboard: enough games and a firm rating (not the "?" kind).
const (
	MinGames     = 5
	DefaultLimit = 50
	MaxLimit     = 100
	// candidateCap bounds the rows read for one category; far above any list people look at.
	candidateCap = 20000
	// cacheTTL is how long a category's standings are reused: a profile asks for four of them.
	cacheTTL = 15 * time.Second
	maxIDLen = 32
)

// Store is the part of *store.Store the service uses.
type Store interface {
	LeaderCandidates(ctx context.Context, category string, minGames int, maxRD float64, limit int, country string) ([]store.LeaderRow, error)
	PublicUser(ctx context.Context, id string) (*store.PublicUser, error)
	PublicUsers(ctx context.Context, ids []string) (map[string]*store.PublicUser, error)
	BotProgress(ctx context.Context, userID string) ([]store.BotRecord, error)
	CountEndedGames(ctx context.Context, userID string) (int, error)
}

// Service answers the leaderboard and profile requests.
type Service struct {
	store   Store
	ratings *ratings.Service
	now     func() time.Time
	ttl     time.Duration

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	at   time.Time
	rows []store.LeaderRow
}

// New returns a Service; now is time.Now when nil.
func New(st Store, r *ratings.Service, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: st, ratings: r, now: now, ttl: cacheTTL, cache: map[string]cached{}}
}

// Row is a line of a leaderboard.
type Row struct {
	Place     int    `json:"place"`
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	Country   string `json:"country,omitempty"` // ISO 3166-1 alpha-2, when the person named one
	Rating    int    `json:"rating"`
	Games     int    `json:"games"`
}

// standings are the people on the leaderboard of a category, the best first: those with enough
// games whose deviation, grown by the days since their last game, is still firm. A non-empty country
// keeps only the people of that country (the code is checked by the caller).
func (s *Service) standings(ctx context.Context, category, country string) ([]store.LeaderRow, error) {
	now := s.now()
	key := category + "/" + country
	s.mu.Lock()
	c, ok := s.cache[key]
	s.mu.Unlock()
	if ok && s.ttl > 0 && now.Sub(c.at) < s.ttl {
		return c.rows, nil
	}
	rows, err := s.store.LeaderCandidates(ctx, category, MinGames, glicko.Provisional, candidateCap, country)
	if err != nil {
		return nil, err
	}
	firm := rows[:0]
	for _, x := range rows {
		r := glicko.Inflate(glicko.Rating{R: x.Rating, RD: x.RD, Sigma: x.Volatility}, now.Sub(x.UpdatedAt).Hours()/24)
		if !r.IsProvisional() {
			firm = append(firm, x)
		}
	}
	s.mu.Lock()
	s.cache[key] = cached{at: now, rows: firm}
	s.mu.Unlock()
	return firm, nil
}

// Leaderboard returns the best limit players of the category; country "" means everybody.
func (s *Service) Leaderboard(ctx context.Context, category, country string, limit int) ([]Row, error) {
	all, err := s.standings(ctx, category, country)
	if err != nil {
		return nil, err
	}
	if len(all) > limit {
		all = all[:limit]
	}
	ids := make([]string, len(all))
	for i, x := range all {
		ids[i] = x.UserID
	}
	users, err := s.store.PublicUsers(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Row, 0, len(all))
	for _, x := range all {
		u := users[x.UserID]
		if u == nil {
			continue // the account is gone
		}
		out = append(out, Row{ID: u.ID, Kind: u.Kind, Name: u.Name, AvatarURL: u.AvatarURL, Country: u.Country,
			Rating: int(math.Round(x.Rating)), Games: x.Games})
	}
	// Places count the rows shown, so a gap left by a deleted account does not show.
	for i := range out {
		out[i].Place = i + 1
	}
	return out, nil
}

// place is the person's place in the category, 0 when they are not on its leaderboard.
func (s *Service) place(ctx context.Context, category, userID string) (int, error) {
	all, err := s.standings(ctx, category, "")
	if err != nil {
		return 0, err
	}
	for i, x := range all {
		if x.UserID == userID {
			return i + 1, nil
		}
	}
	return 0, nil
}

// RatingJSON is a rating on a profile, with the place in the category's leaderboard if there is one.
type RatingJSON struct {
	ratings.Entry
	Place int `json:"place,omitempty"`
}

// BotsJSON is how a person stands against the bots.
type BotsJSON struct {
	Points int `json:"points"`
	Beaten int `json:"beaten"`
	Total  int `json:"total"`
}

// Profile is the public profile of a player. The open-profile fields are what the person filled in
// themselves (Settings -> Profile) and left out when empty; FullName is "First Last" of the two names
// that are set. About is plain text: the page shows it as text, never as markup.
type Profile struct {
	ID          string       `json:"id"`
	Kind        string       `json:"kind"`
	Name        string       `json:"name"`
	AvatarURL   string       `json:"avatarUrl,omitempty"`
	FirstName   string       `json:"firstName,omitempty"`
	LastName    string       `json:"lastName,omitempty"`
	FullName    string       `json:"fullName,omitempty"`
	Location    string       `json:"location,omitempty"`
	Country     string       `json:"country,omitempty"`
	CountryName string       `json:"countryName,omitempty"`
	About       string       `json:"about,omitempty"`
	CreatedAt   time.Time    `json:"createdAt"`
	Ratings     []RatingJSON `json:"ratings"`
	Games       int          `json:"games"`
	Bots        BotsJSON     `json:"bots"`
}

// Profile returns the public profile of a person, store.ErrNotFound for an unknown id.
func (s *Service) Profile(ctx context.Context, id string) (*Profile, error) {
	u, err := s.store.PublicUser(ctx, id)
	if err != nil {
		return nil, err
	}
	entries, err := s.ratings.All(ctx, id)
	if err != nil {
		return nil, err
	}
	p := &Profile{ID: u.ID, Kind: u.Kind, Name: u.Name, AvatarURL: u.AvatarURL, CreatedAt: u.CreatedAt,
		FirstName: u.FirstName, LastName: u.LastName, FullName: strings.TrimSpace(u.FirstName + " " + u.LastName),
		Location: u.Location, About: u.About}
	if c, ok := countries.Lookup(u.Country); ok { // a code the list no longer knows is not shown
		p.Country, p.CountryName = c.Code, c.Ru
	}
	for _, e := range entries {
		rj := RatingJSON{Entry: e}
		if e.Games >= MinGames {
			if rj.Place, err = s.place(ctx, e.Category, id); err != nil {
				return nil, err
			}
		}
		p.Ratings = append(p.Ratings, rj)
	}
	if p.Games, err = s.store.CountEndedGames(ctx, id); err != nil {
		return nil, err
	}
	progress, err := s.store.BotProgress(ctx, id)
	if err != nil {
		return nil, err
	}
	p.Bots.Total = len(bots.All())
	for _, b := range progress {
		p.Bots.Points += b.Points
		if b.Wins > 0 {
			p.Bots.Beaten++
		}
	}
	return p, nil
}

// Handler serves GET /api/leaderboard and GET /api/users/{id}. Both are public.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/leaderboard", s.leaderboard)
	mux.HandleFunc("GET /api/users/{id}", s.user)
	return mux
}

func (s *Service) leaderboard(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	if category == "" {
		category = ratings.Blitz
	}
	if _, ok := ratings.Names[category]; !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неизвестная категория"})
		return
	}
	limit := DefaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный limit"})
			return
		}
		limit = min(n, MaxLimit)
	}
	country := r.URL.Query().Get("country")
	if country != "" && !countries.Valid(country) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неизвестная страна"})
		return
	}
	rows, err := s.Leaderboard(r.Context(), category, country, limit)
	if err != nil {
		log.Printf("profiles: leaderboard: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "не удалось прочитать таблицу лидеров"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"category": category, "name": ratings.Names[category], "country": country, "minGames": MinGames, "rows": rows})
}

func (s *Service) user(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || len(id) > maxIDLen {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "игрок не найден"})
		return
	}
	p, err := s.Profile(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "игрок не найден"})
		return
	}
	if err != nil {
		log.Printf("profiles: user: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "не удалось прочитать профиль"})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

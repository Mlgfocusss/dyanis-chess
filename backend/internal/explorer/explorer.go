// Package explorer brings the statistics of the Lichess opening explorer to the
// repository builder: for a position, which moves were played in how many games,
// and how those games ended.
//
// It is a proxy. The Lichess explorer wants an access token (a personal token
// without any scope will do) and rate-limits its users, so the token stays on this
// server, the answers are remembered for a day, and a "slow down" from Lichess is
// respected for as long as it asks. Without a token the endpoint says the
// statistics are not available and the site goes on with its own reference.
package explorer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"dyanis-chess/backend/internal/eco"
	"dyanis-chess/engine"
)

// BaseURL is where the explorer lives.
const BaseURL = "https://explorer.lichess.ovh"

// Ratings and speeds Lichess knows, as the query of this endpoint may name them.
var (
	allowedRatings = map[string]bool{"0": true, "1000": true, "1200": true, "1400": true, "1600": true, "1800": true, "2000": true, "2200": true, "2500": true}
	allowedSpeeds  = map[string]bool{"ultraBullet": true, "bullet": true, "blitz": true, "rapid": true, "classical": true, "correspondence": true}
)

const (
	defaultRatings = "1600,1800"
	defaultSpeeds  = "blitz,rapid,classical"
	maxMoves       = 12
)

// Config tunes a Service.
type Config struct {
	// Token is the Lichess access token; empty means the statistics are off.
	Token string
	// BaseURL replaces BaseURL (tests).
	BaseURL string
	HTTP    *http.Client
	// UserID names the signed-in user of a request, "" for nobody: only people with an
	// account may use the token.
	UserID func(*http.Request) string
	Now    func() time.Time
	// TTL is how long an answer is remembered; a day by default.
	TTL time.Duration
	// MaxCache is how many answers are remembered; 5000 by default.
	MaxCache int
}

// Service is the explorer proxy.
type Service struct {
	cfg Config
	sem chan struct{} // requests to Lichess in flight

	mu           sync.Mutex
	cache        map[string]cached
	blockedUntil time.Time
}

type cached struct {
	at   time.Time
	resp Response
}

// New returns a Service.
func New(cfg Config) *Service {
	if cfg.BaseURL == "" {
		cfg.BaseURL = BaseURL
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 10 * time.Second}
	}
	if cfg.UserID == nil {
		cfg.UserID = func(*http.Request) string { return "" }
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 24 * time.Hour
	}
	if cfg.MaxCache <= 0 {
		cfg.MaxCache = 5000
	}
	return &Service{cfg: cfg, sem: make(chan struct{}, 2), cache: map[string]cached{}}
}

// Handler returns the route of the endpoint.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/explorer", s.handle)
	return mux
}

// Opening names a position, as the explorer does.
type Opening struct {
	ECO  string `json:"eco"`
	Name string `json:"name"`
	// NameRU is Name in Russian (eco.TranslateName), added by normalize.
	NameRU string `json:"nameRu,omitempty"`
}

// Move is one move played from the position in the games of the explorer.
type Move struct {
	UCI           string   `json:"uci"`
	SAN           string   `json:"san"`
	Games         int      `json:"games"`
	White         int      `json:"white"`
	Draws         int      `json:"draws"`
	Black         int      `json:"black"`
	AverageRating int      `json:"averageRating"`
	Opening       *Opening `json:"opening,omitempty"`
}

// Response is what the endpoint gives.
type Response struct {
	// Available is false when the server has no token, and nothing else is set then.
	Available bool   `json:"available"`
	Total     int    `json:"total"`
	White     int    `json:"white"`
	Draws     int    `json:"draws"`
	Black     int    `json:"black"`
	Moves     []Move `json:"moves"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// list keeps the allowed words of a comma-separated query value, in order and once.
func list(raw, def string, allowed map[string]bool) string {
	if raw == "" {
		raw = def
	}
	var out []string
	seen := map[string]bool{}
	for _, w := range strings.Split(raw, ",") {
		w = strings.TrimSpace(w)
		if allowed[w] && !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return strings.Join(out, ",")
}

func (s *Service) handle(w http.ResponseWriter, r *http.Request) {
	if s.cfg.UserID(r) == "" {
		writeError(w, http.StatusUnauthorized, "войдите в аккаунт или продолжите как гость")
		return
	}
	if s.cfg.Token == "" {
		writeJSON(w, http.StatusOK, Response{Available: false, Moves: []Move{}})
		return
	}
	q := r.URL.Query()
	pos, err := engine.ParseFEN(q.Get("fen"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "неверная позиция")
		return
	}
	ratings := list(q.Get("ratings"), defaultRatings, allowedRatings)
	speeds := list(q.Get("speeds"), defaultSpeeds, allowedSpeeds)
	if ratings == "" || speeds == "" {
		writeError(w, http.StatusBadRequest, "неверный рейтинг или контроль времени")
		return
	}
	fen := pos.FEN()
	key := fen + "|" + ratings + "|" + speeds

	if resp, ok := s.fromCache(key); ok {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if wait := s.blocked(); wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "Lichess просит подождать, статистика скоро вернётся")
		return
	}
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-r.Context().Done():
		return
	}
	resp, err := s.fetch(r.Context(), fen, ratings, speeds)
	var limited *rateLimited
	switch {
	case errors.As(err, &limited):
		s.mu.Lock()
		s.blockedUntil = s.cfg.Now().Add(limited.wait)
		s.mu.Unlock()
		w.Header().Set("Retry-After", strconv.Itoa(int(limited.wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "Lichess просит подождать, статистика скоро вернётся")
		return
	case err != nil:
		log.Printf("explorer: %v", err)
		writeError(w, http.StatusBadGateway, "не удалось получить статистику Lichess")
		return
	}
	s.remember(key, resp)
	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------- for other packages

// WaitError is Lichess asking to be left alone for Wait.
type WaitError struct{ Wait time.Duration }

func (e *WaitError) Error() string { return fmt.Sprintf("lichess asks to wait %s", e.Wait) }

// Enabled says whether the server has a token, so that statistics can be had at all.
func (s *Service) Enabled() bool { return s.cfg.Token != "" }

// Params cleans the rating groups and speeds of a query the way the endpoint does
// (white lists, defaults for empty); ok is false when nothing allowed is left.
func Params(ratings, speeds string) (string, string, bool) {
	r := list(ratings, defaultRatings, allowedRatings)
	sp := list(speeds, defaultSpeeds, allowedSpeeds)
	return r, sp, r != "" && sp != ""
}

func (s *Service) cacheKey(fen, ratings, speeds string) (string, string, error) {
	pos, err := engine.ParseFEN(fen)
	if err != nil {
		return "", "", err
	}
	fen = pos.FEN()
	return fen, fen + "|" + ratings + "|" + speeds, nil
}

// Cached gives the remembered answer for a position, if there is a fresh one; it asks
// nobody. ratings and speeds are as Params returns them.
func (s *Service) Cached(fen, ratings, speeds string) (Response, bool) {
	_, key, err := s.cacheKey(fen, ratings, speeds)
	if err != nil || !s.Enabled() {
		return Response{}, false
	}
	return s.fromCache(key)
}

// Lookup is the endpoint for code on the server: the statistics of a position.
// cached says that the answer came from memory and cost Lichess nothing. Without a
// token the answer is {Available: false}; when Lichess has asked to wait (now or just
// now) the error is a *WaitError. ratings and speeds are as Params returns them.
func (s *Service) Lookup(ctx context.Context, fen, ratings, speeds string) (resp Response, cached bool, err error) {
	if !s.Enabled() {
		return Response{Moves: []Move{}}, true, nil
	}
	fen, key, err := s.cacheKey(fen, ratings, speeds)
	if err != nil {
		return Response{}, false, err
	}
	if resp, ok := s.fromCache(key); ok {
		return resp, true, nil
	}
	if wait := s.blocked(); wait > 0 {
		return Response{}, false, &WaitError{Wait: wait}
	}
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return Response{}, false, ctx.Err()
	}
	resp, err = s.fetch(ctx, fen, ratings, speeds)
	var limited *rateLimited
	switch {
	case errors.As(err, &limited):
		s.mu.Lock()
		s.blockedUntil = s.cfg.Now().Add(limited.wait)
		s.mu.Unlock()
		return Response{}, false, &WaitError{Wait: limited.wait}
	case err != nil:
		log.Printf("explorer: %v", err)
		return Response{}, false, err
	}
	s.remember(key, resp)
	return resp, false, nil
}

func (s *Service) fromCache(key string) (Response, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cache[key]
	if !ok || s.cfg.Now().Sub(c.at) > s.cfg.TTL {
		return Response{}, false
	}
	return c.resp, true
}

func (s *Service) remember(key string, resp Response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cache) >= s.cfg.MaxCache { // full: forget what is stale, and everything if that is not enough
		now := s.cfg.Now()
		for k, c := range s.cache {
			if now.Sub(c.at) > s.cfg.TTL {
				delete(s.cache, k)
			}
		}
		if len(s.cache) >= s.cfg.MaxCache {
			s.cache = map[string]cached{}
		}
	}
	s.cache[key] = cached{at: s.cfg.Now(), resp: resp}
}

// blocked is how long Lichess has asked to be left alone, 0 if it has not.
func (s *Service) blocked() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d := s.blockedUntil.Sub(s.cfg.Now()); d > 0 {
		return d
	}
	return 0
}

// rateLimited is Lichess saying "too many requests", with how long to wait.
type rateLimited struct{ wait time.Duration }

func (e *rateLimited) Error() string { return fmt.Sprintf("lichess asks to wait %s", e.wait) }

// upstream is the explorer's own answer.
type upstream struct {
	White int `json:"white"`
	Draws int `json:"draws"`
	Black int `json:"black"`
	Moves []struct {
		UCI           string   `json:"uci"`
		SAN           string   `json:"san"`
		White         int      `json:"white"`
		Draws         int      `json:"draws"`
		Black         int      `json:"black"`
		AverageRating int      `json:"averageRating"`
		Opening       *Opening `json:"opening"`
	} `json:"moves"`
}

func (s *Service) fetch(ctx context.Context, fen, ratings, speeds string) (Response, error) {
	v := url.Values{}
	v.Set("variant", "standard")
	v.Set("fen", fen)
	v.Set("ratings", ratings)
	v.Set("speeds", speeds)
	v.Set("moves", strconv.Itoa(maxMoves))
	v.Set("topGames", "0")
	v.Set("recentGames", "0")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.BaseURL+"/lichess?"+v.Encode(), nil)
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.Token)
	req.Header.Set("Accept", "application/json")
	res, err := s.cfg.HTTP.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusTooManyRequests:
		wait := time.Minute // Lichess asks for a minute
		if n, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && n > 0 {
			wait = time.Duration(n) * time.Second
		}
		return Response{}, &rateLimited{wait: wait}
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return Response{}, errors.New("lichess did not accept the token (check LICHESS_TOKEN)")
	case res.StatusCode != http.StatusOK:
		return Response{}, fmt.Errorf("lichess answered %d", res.StatusCode)
	}
	var up upstream
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&up); err != nil {
		return Response{}, fmt.Errorf("lichess answer: %w", err)
	}
	return normalize(up), nil
}

func normalize(up upstream) Response {
	out := Response{Available: true, White: up.White, Draws: up.Draws, Black: up.Black, Total: up.White + up.Draws + up.Black, Moves: []Move{}}
	for _, m := range up.Moves {
		if m.Opening != nil {
			m.Opening.NameRU = eco.TranslateName(m.Opening.Name)
		}
		out.Moves = append(out.Moves, Move{
			UCI: fixCastling(m.UCI, m.SAN), SAN: m.SAN, Games: m.White + m.Draws + m.Black,
			White: m.White, Draws: m.Draws, Black: m.Black, AverageRating: m.AverageRating, Opening: m.Opening,
		})
	}
	return out
}

// fixCastling turns the explorer's castling, written as the king taking its own
// rook ("e1h1"), into the engine's king move ("e1g1").
func fixCastling(uci, san string) string {
	if !strings.HasPrefix(san, "O-O") || len(uci) != 4 {
		return uci
	}
	long := strings.HasPrefix(san, "O-O-O")
	rank := uci[1:2]
	switch {
	case uci[0:2] == "e1" || uci[0:2] == "e8":
		if long {
			return uci[0:2] + "c" + rank
		}
		return uci[0:2] + "g" + rank
	}
	return uci
}

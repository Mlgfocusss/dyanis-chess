package repertoire

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"dyanis-chess/backend/internal/eco"
	"dyanis-chess/backend/internal/store"
	"dyanis-chess/engine"
)

// Store is the part of *store.Store the service uses.
type Store interface {
	CreateRepertoire(ctx context.Context, userID, name, color string) (*store.Repertoire, error)
	Repertoire(ctx context.Context, userID string, id int64) (*store.Repertoire, error)
	ListRepertoires(ctx context.Context, userID string, now time.Time) ([]store.RepertoireWithCounts, error)
	RenameRepertoire(ctx context.Context, userID string, id int64, name string) error
	DeleteRepertoire(ctx context.Context, userID string, id int64) error
	RepMoves(ctx context.Context, repertoireID int64) ([]store.RepMove, error)
	RepMove(ctx context.Context, repertoireID, id int64) (store.RepMove, error)
	AddRepMove(ctx context.Context, m store.RepMove) (store.RepMove, bool, error)
	AddRepMovesBulk(ctx context.Context, repertoireID int64, moves []store.NewRepMove) error
	DeleteRepMove(ctx context.Context, repertoireID, id int64) (int, error)
	ReviewRepMove(ctx context.Context, repertoireID, id int64, next func(box, lapses int) store.Card) (store.Card, error)
	ListGames(ctx context.Context, userID string, limit int) ([]*store.Game, error)
	SaveRepScan(ctx context.Context, userID string, sc store.RepScan) error
	RepScan(ctx context.Context, userID string, repertoireID int64) (store.RepScan, error)
	RepOverviews(ctx context.Context, userID string) (map[int64]store.RepOverview, error)
}

// Config tunes a Service.
type Config struct {
	Store Store
	// UserID names the signed-in user of a request, "" for nobody.
	UserID func(*http.Request) string
	// Now is the clock; time.Now unless a test says otherwise.
	Now func() time.Time
	// Stats gives the statistics of real games for the search for holes (the explorer);
	// nil: the ECO reference stands in.
	Stats StatsSource
	// Book is the ECO reference; eco.Default() if nil.
	Book *eco.Book
	// GapBudget is how many requests to Lichess a search may make, DefaultBudget if 0.
	GapBudget int
}

// Service is the repertoire API.
type Service struct {
	cfg Config

	scanMu   sync.Mutex
	scanning map[int64]bool // repertoires being searched for holes right now
	quota    map[string]quotaState
}

// New returns a Service.
func New(cfg Config) *Service {
	if cfg.UserID == nil {
		cfg.UserID = func(*http.Request) string { return "" }
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{cfg: cfg, scanning: map[int64]bool{}, quota: map[string]quotaState{}}
}

// Handler returns the routes of the API.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/repertoires", s.list)
	mux.HandleFunc("POST /api/repertoires", s.create)
	mux.HandleFunc("POST /api/repertoires/import", s.importNew)
	mux.HandleFunc("POST /api/repertoires/{id}/import", s.importInto)
	mux.HandleFunc("GET /api/repertoires/{id}/export", s.export)
	mux.HandleFunc("GET /api/repertoires/{id}", s.get)
	mux.HandleFunc("PATCH /api/repertoires/{id}", s.rename)
	mux.HandleFunc("DELETE /api/repertoires/{id}", s.remove)
	mux.HandleFunc("POST /api/repertoires/{id}/moves", s.addMove)
	mux.HandleFunc("DELETE /api/repertoires/{id}/moves/{move}", s.deleteMove)
	mux.HandleFunc("GET /api/repertoires/{id}/train", s.train)
	mux.HandleFunc("POST /api/repertoires/{id}/review", s.review)
	mux.HandleFunc("GET /api/repertoires/{id}/games", s.games)
	mux.HandleFunc("POST /api/repertoires/{id}/compare", s.comparePGN)
	mux.HandleFunc("GET /api/repertoires/{id}/gaps", s.gaps)
	return mux
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

func serverError(w http.ResponseWriter, what string, err error) {
	log.Printf("repertoire: %s: %v", what, err)
	writeError(w, http.StatusInternalServerError, "не удалось выполнить запрос")
}

// ------------------------------------------------------------------ shapes

// Counts is what a repertoire shows about its moves.
type Counts struct {
	Moves   int `json:"moves"`
	Mine    int `json:"mine"`
	New     int `json:"new"`
	Due     int `json:"due"`
	Learned int `json:"learned"`
}

// RepertoireJSON is a repertoire in a list or on its own page.
type RepertoireJSON struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Color  string `json:"color"`
	Counts Counts `json:"counts"`
	// Gaps is in the list only: the holes the person can see without asking Lichess.
	Gaps *GapSummary `json:"gaps,omitempty"`
}

// MoveJSON is one move of the tree.
type MoveJSON struct {
	ID      int64   `json:"id"`
	Parent  int64   `json:"parent"`
	UCI     string  `json:"uci"`
	SAN     string  `json:"san"`
	FEN     string  `json:"fen"`
	Ply     int     `json:"ply"`
	Mine    bool    `json:"mine"`
	Box     int     `json:"box"`
	Due     *string `json:"due"` // RFC 3339, null: never asked
	Lapses  int     `json:"lapses"`
	Reviews int     `json:"reviews"`
}

func moveJSON(m store.RepMove) MoveJSON {
	j := MoveJSON{ID: m.ID, Parent: m.ParentID, UCI: m.UCI, SAN: m.SAN, FEN: m.FEN, Ply: m.Ply, Mine: m.Mine, Box: m.Box, Lapses: m.Lapses, Reviews: m.Reviews}
	if m.DueAt.Valid {
		s := m.DueAt.Time.UTC().Format(time.RFC3339)
		j.Due = &s
	}
	return j
}

func countsOf(moves []store.RepMove, now time.Time) Counts {
	c := Counts{Moves: len(moves)}
	for _, m := range moves {
		if !m.Mine {
			continue
		}
		c.Mine++
		switch {
		case !m.DueAt.Valid:
			c.New++
		case !m.DueAt.Time.After(now):
			c.Due++
		}
		if Learned(m.Box) && m.DueAt.Valid {
			c.Learned++
		}
	}
	return c
}

// ---------------------------------------------------------------- helpers

func (s *Service) user(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := s.cfg.UserID(r)
	if id == "" {
		writeError(w, http.StatusUnauthorized, "войдите в аккаунт или продолжите как гость")
		return "", false
	}
	return id, true
}

// owned finds the repertoire named in the path among the person's own.
func (s *Service) owned(w http.ResponseWriter, r *http.Request) (*store.Repertoire, bool) {
	uid, ok := s.user(w, r)
	if !ok {
		return nil, false
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "такого репозитория нет")
		return nil, false
	}
	rep, err := s.cfg.Store.Repertoire(r.Context(), uid, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "такого репозитория нет")
		return nil, false
	}
	if err != nil {
		serverError(w, "repertoire", err)
		return nil, false
	}
	return rep, true
}

func cleanName(name string) (string, bool) {
	name = strings.Join(strings.Fields(name), " ")
	n := utf8.RuneCountInString(name)
	return name, n >= 1 && n <= 80
}

// ---------------------------------------------------------------- handlers

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.user(w, r)
	if !ok {
		return
	}
	reps, err := s.cfg.Store.ListRepertoires(r.Context(), uid, s.cfg.Now())
	if err != nil {
		serverError(w, "list", err)
		return
	}
	overviews, err := s.cfg.Store.RepOverviews(r.Context(), uid)
	if err != nil {
		serverError(w, "overviews", err)
		return
	}
	out := make([]RepertoireJSON, 0, len(reps))
	for _, x := range reps {
		g := summaryOf(overviews[x.ID], x.Counts.Moves)
		out = append(out, RepertoireJSON{ID: x.ID, Name: x.Name, Color: x.Color,
			Counts: Counts{Moves: x.Counts.Moves, Mine: x.Counts.Mine, New: x.Counts.New, Due: x.Counts.Due, Learned: x.Counts.Learned},
			Gaps:   &g})
	}
	writeJSON(w, http.StatusOK, map[string]any{"repertoires": out})
}

type createRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (s *Service) create(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.user(w, r)
	if !ok {
		return
	}
	var req createRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}
	name, ok := cleanName(req.Name)
	if !ok {
		writeError(w, http.StatusBadRequest, "название от 1 до 80 символов")
		return
	}
	if req.Color != "white" && req.Color != "black" {
		writeError(w, http.StatusBadRequest, "цвет: white или black")
		return
	}
	rep, err := s.cfg.Store.CreateRepertoire(r.Context(), uid, name, req.Color)
	if errors.Is(err, store.ErrLimit) {
		writeError(w, http.StatusConflict, "слишком много репозиториев")
		return
	}
	if err != nil {
		serverError(w, "create", err)
		return
	}
	writeJSON(w, http.StatusCreated, RepertoireJSON{ID: rep.ID, Name: rep.Name, Color: rep.Color})
}

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	rep, ok := s.owned(w, r)
	if !ok {
		return
	}
	moves, err := s.cfg.Store.RepMoves(r.Context(), rep.ID)
	if err != nil {
		serverError(w, "moves", err)
		return
	}
	out := make([]MoveJSON, len(moves))
	for i, m := range moves {
		out[i] = moveJSON(m)
	}
	gaps := Gaps(moves)
	if gaps == nil {
		gaps = []int64{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"repertoire": RepertoireJSON{ID: rep.ID, Name: rep.Name, Color: rep.Color, Counts: countsOf(moves, s.cfg.Now())},
		"moves":      out,
		"gaps":       gaps,
	})
}

func (s *Service) rename(w http.ResponseWriter, r *http.Request) {
	rep, ok := s.owned(w, r)
	if !ok {
		return
	}
	var req createRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}
	name, ok := cleanName(req.Name)
	if !ok {
		writeError(w, http.StatusBadRequest, "название от 1 до 80 символов")
		return
	}
	if err := s.cfg.Store.RenameRepertoire(r.Context(), rep.UserID, rep.ID, name); err != nil {
		serverError(w, "rename", err)
		return
	}
	writeJSON(w, http.StatusOK, RepertoireJSON{ID: rep.ID, Name: name, Color: rep.Color})
}

func (s *Service) remove(w http.ResponseWriter, r *http.Request) {
	rep, ok := s.owned(w, r)
	if !ok {
		return
	}
	if err := s.cfg.Store.DeleteRepertoire(r.Context(), rep.UserID, rep.ID); err != nil {
		serverError(w, "delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type addMoveRequest struct {
	Parent int64  `json:"parent"` // 0: a first move from the start position
	UCI    string `json:"uci"`
}

// addMove puts a move into the tree: the person's own move or an answer of the
// opponent they expect. The server checks it is legal and works out its colour.
func (s *Service) addMove(w http.ResponseWriter, r *http.Request) {
	rep, ok := s.owned(w, r)
	if !ok {
		return
	}
	var req addMoveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}
	fen, ply := engine.StartFEN, 0
	if req.Parent != 0 {
		parent, err := s.cfg.Store.RepMove(r.Context(), rep.ID, req.Parent)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "такого хода в репозитории нет")
			return
		}
		if err != nil {
			serverError(w, "parent", err)
			return
		}
		fen, ply = parent.FEN, parent.Ply
	}
	pos, err := engine.ParseFEN(fen)
	if err != nil {
		serverError(w, "fen", err)
		return
	}
	var mv engine.Move
	found := false
	for _, m := range pos.LegalMoves() {
		if m.String() == req.UCI {
			mv, found = m, true
			break
		}
	}
	if !found {
		writeError(w, http.StatusBadRequest, "такой ход здесь невозможен")
		return
	}
	san := pos.SAN(mv)
	pos.Play(mv)
	ply++
	mine := (ply%2 == 1) == (rep.Color == "white")
	m, created, err := s.cfg.Store.AddRepMove(r.Context(), store.RepMove{
		RepertoireID: rep.ID, ParentID: req.Parent, UCI: req.UCI, SAN: san, FEN: pos.FEN(), Ply: ply, Mine: mine,
	})
	if errors.Is(err, store.ErrLimit) {
		writeError(w, http.StatusConflict, "в репозитории слишком много ходов")
		return
	}
	if err != nil {
		serverError(w, "add", err)
		return
	}
	code := http.StatusOK
	if created {
		code = http.StatusCreated
	}
	writeJSON(w, code, map[string]any{"move": moveJSON(m), "created": created})
}

func (s *Service) deleteMove(w http.ResponseWriter, r *http.Request) {
	rep, ok := s.owned(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("move"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "такого хода нет")
		return
	}
	n, err := s.cfg.Store.DeleteRepMove(r.Context(), rep.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "такого хода нет")
		return
	}
	if err != nil {
		serverError(w, "delete move", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": n})
}

const (
	defaultNew, maxNewPerSession = 10, 50
	maxDuePerSession             = 40
)

func intParam(r *http.Request, name string, def, max int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil || v < 0 {
		return def
	}
	return min(v, max)
}

// train builds a session: the due moves, then some new ones.
func (s *Service) train(w http.ResponseWriter, r *http.Request) {
	rep, ok := s.owned(w, r)
	if !ok {
		return
	}
	moves, err := s.cfg.Store.RepMoves(r.Context(), rep.ID)
	if err != nil {
		serverError(w, "moves", err)
		return
	}
	sel := Select(moves, s.cfg.Now(), intParam(r, "new", defaultNew, maxNewPerSession), maxDuePerSession)
	drills := []Drill{}
	for _, group := range []struct {
		ids   []int64
		isNew bool
	}{{sel.Due, false}, {sel.New, true}} {
		for _, id := range group.ids {
			if d, ok := BuildDrill(moves, id, group.isNew); ok {
				drills = append(drills, d)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"color":    rep.Color,
		"name":     rep.Name,
		"drills":   drills,
		"dueTotal": sel.DueTotal,
		"newTotal": sel.NewTotal,
		"nextDue":  NextDue(moves, s.cfg.Now()),
	})
}

type reviewRequest struct {
	Move    int64 `json:"move"`
	Correct bool  `json:"correct"`
}

// review takes the first answer to a question of a session.
func (s *Service) review(w http.ResponseWriter, r *http.Request) {
	rep, ok := s.owned(w, r)
	if !ok {
		return
	}
	var req reviewRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}
	now := s.cfg.Now()
	c, err := s.cfg.Store.ReviewRepMove(r.Context(), rep.ID, req.Move, func(box, lapses int) store.Card {
		return Review(box, lapses, req.Correct, now)
	})
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "такого хода нет")
		return
	}
	if err != nil {
		serverError(w, "review", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"box": c.Box, "due": c.DueAt.UTC().Format(time.RFC3339), "lapses": c.Lapses, "learned": Learned(c.Box)})
}

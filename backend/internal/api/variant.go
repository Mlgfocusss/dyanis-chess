package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"dyanis-chess/backend/internal/analysis"
	"dyanis-chess/backend/internal/variants"
)

// ErrNoVariantGame is the answer of Config.VariantGame to an unknown game.
var ErrNoVariantGame = errors.New("api: no such variant game")

// VariantSource is a game played on the site in a chess variant, as the review
// needs it.
type VariantSource struct {
	Variant string
	Options variants.Options
	Actions []string
	// Status is the game's: only "ended" can be reviewed.
	Status string
	Result string
	// White and Black are the players' names.
	White, Black string
}

type variantRequest struct {
	Game  string `json:"game"`  // the id of the game of the variant
	Depth int    `json:"depth"` // 0 = server default
}

// variantReviewID is the id of the review of a game: it follows from the game and
// the depth, so that a second click on "Разбор партии" (by either player, or after
// a reload) finds the review instead of making another one.
func variantReviewID(game string, depth int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("variant-review:%s:%d", game, depth)))
	return hex.EncodeToString(sum[:8])
}

// createVariant answers POST /api/analyses/variant {"game": "<id>", "depth": 14}:
// the review of a finished game of Chess960, in the same queue, store and format
// as any other (GET /api/analyses/{id}); the report has "variant" set.
func (s *Server) createVariant(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var req variantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Game == "" {
		writeError(w, http.StatusBadRequest, "ожидался JSON {\"game\": \"<номер партии>\"}")
		return
	}
	if s.cfg.VariantGame == nil {
		writeError(w, http.StatusNotFound, "партии видов недоступны")
		return
	}
	src, err := s.cfg.VariantGame(r.Context(), req.Game)
	switch {
	case errors.Is(err, ErrNoVariantGame):
		writeError(w, http.StatusNotFound, "партия не найдена")
		return
	case err != nil:
		log.Printf("api: variant game %s: %v", req.Game, err)
		writeError(w, http.StatusInternalServerError, "не удалось открыть партию")
		return
	}
	if !analysis.SupportsVariant(src.Variant) {
		writeError(w, http.StatusBadRequest, analysis.ErrVariantUnsupported.Error())
		return
	}
	if src.Status != "ended" {
		writeError(w, http.StatusConflict, "партия ещё не закончена")
		return
	}
	if len(src.Actions) == 0 {
		writeError(w, http.StatusBadRequest, "в партии нет ходов")
		return
	}
	if len(src.Actions) > s.cfg.MaxPlies {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("партия длиннее %d полуходов", s.cfg.MaxPlies))
		return
	}
	depth := req.Depth
	if depth <= 0 {
		depth = s.cfg.DefaultDepth
	}
	depth = min(depth, s.cfg.MaxDepth)
	id := variantReviewID(req.Game, depth)

	// Already made (or being made): hand out the same link.
	s.mu.Lock()
	if j := s.jobs[id]; j != nil && j.Status != StatusError {
		s.mu.Unlock()
		writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
		return
	}
	s.mu.Unlock()
	if stored, err := s.loadStored(r.Context(), id); err != nil {
		log.Printf("api: load analysis %s: %v", id, err)
	} else if stored != nil && stored.Status == StatusDone {
		writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
		return
	}

	vg, err := analysis.NewVariantGame(src.Variant, src.Options, src.Actions)
	if err != nil {
		writeError(w, http.StatusBadRequest, "не удалось воспроизвести партию: "+err.Error())
		return
	}
	event := "Chess960"
	if n, ok := src.Options.Params["number"]; ok {
		event = fmt.Sprintf("Chess960, расстановка %v", n)
	}
	job := &Job{
		ID: id, Status: StatusQueued, Total: len(vg.Moves) + 1,
		Game: GameInfo{
			White: src.White, Black: src.Black, Result: src.Result, Event: event,
			Date: time.Now().UTC().Format("2006.01.02"), StartFEN: vg.FENs[0], Variant: src.Variant,
		},
		Created: time.Now().UTC(), vgame: vg, depth: depth, userID: s.cfg.UserID(r),
	}
	s.mu.Lock()
	if j := s.jobs[id]; j != nil && j.Status != StatusError { // another click got here first
		s.mu.Unlock()
		writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
		return
	}
	select {
	case s.queue <- job:
	default:
		s.mu.Unlock()
		writeError(w, http.StatusServiceUnavailable, "очередь разбора переполнена, попробуйте позже")
		return
	}
	if s.jobs[id] == nil {
		s.order = append(s.order, id)
	}
	s.jobs[id] = job
	s.evictLocked()
	s.mu.Unlock()
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
}

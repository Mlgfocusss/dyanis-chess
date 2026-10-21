package api

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"

	"dyanis-chess/backend/internal/coach"
)

// The coach of the review page: GET /api/coach says whether it is switched on,
// POST /api/coach {"id": review, "ply": n} explains the n-th half-move of a
// reviewed game. The facts come from the review's own report (the engine's
// evaluations and lines), so nothing is searched here; the model only words them.

type coachRequest struct {
	ID  string `json:"id"`
	Ply int    `json:"ply"` // 1-based half-move of the game
}

type coachResponse struct {
	Text   string `json:"text"`
	Cached bool   `json:"cached,omitempty"`
	// The lines the explanation is about, in SAN: BestLine from the position
	// before the move, ReplyLine (what the opponent does) from the position
	// after it. The page plays them on the board.
	BestSAN   string   `json:"bestSan,omitempty"`
	BestLine  []string `json:"bestLine,omitempty"`
	ReplyLine []string `json:"replyLine,omitempty"`
}

func (s *Server) coachStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": s.cfg.Coach.Enabled()})
}

func (s *Server) coachExplain(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Coach.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "тренер не подключён")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	var req coachRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}

	rep, err := s.reportOf(r, req.ID)
	if err != nil {
		log.Printf("api: coach: load analysis %s: %v", req.ID, err)
		writeError(w, http.StatusInternalServerError, "не удалось прочитать разбор")
		return
	}
	if rep == nil {
		writeError(w, http.StatusNotFound, "разбор не найден или ещё не готов")
		return
	}
	facts, err := coach.BuildFacts(rep, req.Ply)
	if err != nil {
		writeError(w, http.StatusBadRequest, "в партии нет такого хода")
		return
	}

	who := s.cfg.UserID(r)
	if who == "" {
		who = "ip:" + clientIP(r)
	}
	note, err := s.cfg.Coach.Explain(r.Context(), who, facts)
	switch {
	case errors.Is(err, coach.ErrLimit):
		writeError(w, http.StatusTooManyRequests, "лимит объяснений на этот час исчерпан, попробуйте позже")
		return
	case errors.Is(err, coach.ErrUpstream):
		writeError(w, http.StatusBadGateway, "тренер сейчас недоступен, попробуйте позже")
		return
	case r.Context().Err() != nil:
		return // the person left
	case err != nil:
		log.Printf("api: coach: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось получить объяснение")
		return
	}
	writeJSON(w, http.StatusOK, coachResponse{
		Text: note.Text, Cached: note.Cached,
		BestSAN: facts.BestSAN, BestLine: facts.BestLine, ReplyLine: facts.ReplyLine,
	})
}

// clientIP is the caller's address; nginx puts the real one in X-Real-IP.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

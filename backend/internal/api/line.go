package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"dyanis-chess/backend/internal/analysis"
	"dyanis-chess/engine"
)

// The "try a line" endpoint of the review page: the browser has no rules of
// its own, so for a branch the person plays (or a line the engine suggests)
// it asks here for the positions, the legal moves and, when wanted, the
// engine's verdict on the last position.

const (
	maxLineMoves  = 40
	lineEvalSlots = 2 // evaluations running at once, so a crowd cannot starve the review queue
)

type lineRequest struct {
	FEN   string   `json:"fen"`
	Moves []string `json:"moves"` // UCI ("e2e4") or SAN ("Nf3"), played from FEN in turn
	Eval  bool     `json:"eval"`  // also search the last position
	Depth int      `json:"depth"` // search depth for Eval; 0 = the server default
}

// LineStep is one move of a line with the position after it.
type LineStep struct {
	UCI string `json:"uci"`
	SAN string `json:"san"`
	FEN string `json:"fen"`
}

type lineResponse struct {
	Steps []LineStep `json:"steps"`
	FEN   string     `json:"fen"`   // the last position
	Legal []string   `json:"legal"` // legal moves there, UCI
	// Status is "" while the game goes on, else "checkmate" or "draw".
	Status string                 `json:"status,omitempty"`
	Eval   *analysis.PositionEval `json:"eval,omitempty"`
}

// uciOrSAN finds the legal move a request item means.
func uciOrSAN(pos *engine.Position, item string) (engine.Move, error) {
	item = strings.TrimSpace(item)
	for _, m := range pos.LegalMoves() {
		if m.String() == item {
			return m, nil
		}
	}
	return pos.ParseSAN(item)
}

func (s *Server) line(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var req lineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}
	if len(req.Moves) > maxLineMoves {
		writeError(w, http.StatusBadRequest, "слишком длинная линия")
		return
	}
	pos, err := engine.ParseFEN(req.FEN)
	if err != nil {
		writeError(w, http.StatusBadRequest, "неверная позиция")
		return
	}
	resp := lineResponse{Steps: []LineStep{}}
	for _, item := range req.Moves {
		m, err := uciOrSAN(pos, item)
		if err != nil {
			writeError(w, http.StatusBadRequest, "неверный ход: "+item)
			return
		}
		san := pos.SAN(m)
		pos.Play(m)
		resp.Steps = append(resp.Steps, LineStep{UCI: m.String(), SAN: san, FEN: pos.FEN()})
	}
	resp.FEN = pos.FEN()
	resp.Legal = []string{}
	for _, m := range pos.LegalMoves() {
		resp.Legal = append(resp.Legal, m.String())
	}
	if out := pos.Outcome(nil); out.Over {
		resp.Status = "draw"
		if out.Checkmate {
			resp.Status = "checkmate"
		}
	}

	if req.Eval {
		depth := req.Depth
		if depth <= 0 {
			depth = s.cfg.DefaultDepth
		}
		depth = min(depth, s.cfg.MaxDepth)
		select {
		case s.evalSlots <- struct{}{}:
			defer func() { <-s.evalSlots }()
		case <-time.After(5 * time.Second):
			writeError(w, http.StatusServiceUnavailable, "движок занят, попробуйте ещё раз")
			return
		case <-r.Context().Done():
			return
		}
		ev, err := s.safeEval(pos, analysis.Options{Depth: depth})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось оценить позицию")
			return
		}
		resp.Eval = &ev
	}
	writeJSON(w, http.StatusOK, resp)
}

// safeEval evaluates a position, turning a panic in the engine into an error
// (a bad position must not take the server down).
func (s *Server) safeEval(pos *engine.Position, opt analysis.Options) (ev analysis.PositionEval, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("evaluation panicked")
		}
	}()
	return s.cfg.EvalPosition(pos, nil, opt), nil
}

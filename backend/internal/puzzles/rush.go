package puzzles

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"dyanis-chess/backend/internal/store"
)

// The tactics storm: puzzles one after another, easy ones first and harder with every one
// solved, until the clock runs out or the first mistake. The browser keeps the run; the server
// picks puzzles near a rating it is told and remembers the finished runs. It is practice: the
// puzzle rating does not change.

// RushModes are the lengths of a run.
var RushModes = map[string]bool{"3min": true, "5min": true}

const (
	rushMinRating = 400
	rushMaxRating = 3000
	rushMaxScore  = 500 // more than anybody can solve in five minutes
	rushTries     = 4   // picks made to avoid a puzzle already seen in the run
)

// rushNext gives a puzzle near ?rating, not one of ?exclude (ids of this run, comma separated).
func (s *Service) rushNext(w http.ResponseWriter, r *http.Request) {
	rating, err := strconv.Atoi(r.URL.Query().Get("rating"))
	if err != nil {
		rating = rushMinRating
	}
	rating = max(rushMinRating, min(rushMaxRating, rating))
	seen := map[string]bool{}
	for _, id := range strings.Split(r.URL.Query().Get("exclude"), ",") {
		if id != "" {
			seen[id] = true
		}
	}
	var p *store.Puzzle
	for i := 0; i < rushTries; i++ {
		p, err = s.pick(r.Context(), "", "", rating)
		if err != nil || !seen[p.ID] {
			break
		}
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "задач не нашлось")
		return
	}
	if err != nil {
		log.Printf("puzzles: rush pick: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось выбрать задачу")
		return
	}
	respond(w, p, "", store.PuzzleRating{}, nil)
}

type rushRequest struct {
	Mode  string `json:"mode"`
	Score int    `json:"score"`
}

// rushResult takes a finished run. The score is the browser's word: it is only the person's own record.
func (s *Service) rushResult(w http.ResponseWriter, r *http.Request) {
	userID := s.cfg.UserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "войдите, чтобы результат сохранился")
		return
	}
	var req rushRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || !RushModes[req.Mode] || req.Score < 0 || req.Score > rushMaxScore {
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}
	if err := s.cfg.Store.RecordRush(r.Context(), userID, req.Mode, req.Score); err != nil {
		log.Printf("puzzles: record rush: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось записать результат")
		return
	}
	s.rushBest(w, r)
}

// rushBest gives the person's records by mode: {"best": {"3min": {"best": 21, "runs": 4}}}.
func (s *Service) rushBest(w http.ResponseWriter, r *http.Request) {
	best := map[string]store.RushBest{}
	if userID := s.cfg.UserID(r); userID != "" {
		var err error
		if best, err = s.cfg.Store.RushBests(r.Context(), userID); err != nil {
			log.Printf("puzzles: rush best: %v", err)
			writeError(w, http.StatusInternalServerError, "не удалось прочитать рекорды")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"best": best})
}

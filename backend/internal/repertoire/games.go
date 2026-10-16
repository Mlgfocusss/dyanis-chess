package repertoire

import (
	"encoding/json"
	"net/http"

	"dyanis-chess/backend/internal/store"
)

// maxGames is how many of the person's recent games are compared with a repository.
const (
	gamesLookedAt = 80
	gamesShown    = 30
)

// savedGame is the part of an online game's stored state (see internal/play) that
// the comparison needs.
type savedGame struct {
	Moves []struct {
		UCI string `json:"uci"`
	} `json:"moves"`
	White *seat `json:"white"`
	Black *seat `json:"black"`
	// EndedMs is when the game ended, in unix milliseconds (0 while it is going on).
	EndedMs int64 `json:"endedMs"`
}

type seat struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Bot  string `json:"bot"`
	Elo  int    `json:"elo"`
}

// GameJSON is one of the person's games compared with a repository.
type GameJSON struct {
	ID       string    `json:"id"`
	Opponent string    `json:"opponent"`
	Bot      bool      `json:"bot"`
	Result   string    `json:"result"`
	Status   string    `json:"gameStatus"` // "playing" or "ended"
	EndedMs  int64     `json:"endedMs"`
	Date     string    `json:"date,omitempty"` // the Date tag of a pasted game, as it reads
	Dev      Deviation `json:"deviation"`
}

// colourOf is the side the person played in a stored game: "" if they did not play it.
func colourOf(g *store.Game, userID string) string {
	switch {
	case g.WhiteID == userID:
		return "white"
	case g.BlackID == userID:
		return "black"
	}
	return ""
}

// compareGames puts the person's games of the repository's colour against its tree.
func compareGames(moves []store.RepMove, color, userID string, games []*store.Game) []GameJSON {
	out := []GameJSON{}
	for _, g := range games {
		if colourOf(g, userID) != color {
			continue
		}
		var s savedGame
		if json.Unmarshal(g.State, &s) != nil || len(s.Moves) < 2 {
			continue // too short to say anything
		}
		ucis := make([]string, len(s.Moves))
		for i, m := range s.Moves {
			ucis[i] = m.UCI
		}
		them := s.Black
		if color == "black" {
			them = s.White
		}
		item := GameJSON{ID: g.ID, Result: g.Result, Status: g.Status, EndedMs: s.EndedMs, Dev: Compare(moves, color, ucis)}
		if them != nil {
			item.Opponent, item.Bot = them.Name, them.Bot != ""
		}
		out = append(out, item)
		if len(out) >= gamesShown {
			break
		}
	}
	return out
}

// games compares the person's recent games with the repository: where each one left
// it, and whose move it was.
func (s *Service) games(w http.ResponseWriter, r *http.Request) {
	rep, ok := s.owned(w, r)
	if !ok {
		return
	}
	moves, err := s.cfg.Store.RepMoves(r.Context(), rep.ID)
	if err != nil {
		serverError(w, "moves", err)
		return
	}
	games, err := s.cfg.Store.ListGames(r.Context(), rep.UserID, gamesLookedAt)
	if err != nil {
		serverError(w, "games", err)
		return
	}
	items := compareGames(moves, rep.Color, rep.UserID, games)
	counts := map[string]int{}
	for _, it := range items {
		counts[it.Dev.Status]++
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": items, "counts": counts})
}

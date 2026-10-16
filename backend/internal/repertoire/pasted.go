package repertoire

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"dyanis-chess/backend/internal/pgn"
	"dyanis-chess/backend/internal/store"
	"dyanis-chess/engine"
)

// maxPasted is how many games of a pasted file are compared.
const maxPasted = 200

type compareRequest struct {
	PGN string `json:"pgn"`
	// Name is the person's name in the games' White/Black tags. When it is set, games in which
	// they did not play the repository's colour are left out; empty: every game is taken as
	// played by the person in the repository's colour.
	Name string `json:"name"`
}

// comparePasted compares the games of a pasted file with the tree; skipped counts the games left
// out (another start position, another player, too short).
func comparePasted(moves []store.RepMove, color, name string, games []*pgn.Game) (items []GameJSON, skipped int) {
	items = []GameJSON{}
	name = strings.ToLower(strings.Join(strings.Fields(name), " "))
	for _, g := range games {
		if g.StartFEN != "" && g.StartFEN != engine.StartFEN {
			skipped++
			continue
		}
		mine, theirs := g.Tag("White"), g.Tag("Black")
		if color == "black" {
			mine, theirs = theirs, mine
		}
		if name != "" && strings.ToLower(strings.Join(strings.Fields(mine), " ")) != name {
			skipped++
			continue
		}
		if len(g.Moves) < 2 {
			skipped++
			continue
		}
		ucis := make([]string, len(g.Moves))
		for i, m := range g.Moves {
			ucis[i] = m.Move.String()
		}
		item := GameJSON{
			ID:       strconv.Itoa(len(items) + 1),
			Opponent: cleanTag(theirs),
			Result:   g.Result,
			Status:   "ended",
			Date:     cleanDate(g.Tag("Date")),
			Dev:      Compare(moves, color, ucis),
		}
		items = append(items, item)
		if len(items) >= maxPasted {
			break
		}
	}
	return items, skipped
}

func cleanTag(s string) string {
	if s = strings.TrimSpace(s); s == "?" {
		return ""
	}
	return s
}

// cleanDate is a PGN date ("2024.03.17", "2024.??.??") as it reads in the list; "" when unknown.
func cleanDate(s string) string {
	parts := strings.Split(strings.TrimSpace(s), ".")
	if len(parts) != 3 || strings.Contains(parts[0], "?") {
		return ""
	}
	if strings.Contains(parts[1], "?") || strings.Contains(parts[2], "?") {
		return parts[0]
	}
	return parts[2] + "." + parts[1] + "." + parts[0]
}

// comparePGN puts games the person pasted (a PGN file or text) against a repository: the same
// comparison "my games" does for the games played on the site.
func (s *Service) comparePGN(w http.ResponseWriter, r *http.Request) {
	rep, ok := s.owned(w, r)
	if !ok {
		return
	}
	var req compareRequest
	if err := decodeLarge(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос или файл слишком большой (до 4 МБ)")
		return
	}
	if strings.TrimSpace(req.PGN) == "" {
		writeError(w, http.StatusBadRequest, "файл пуст")
		return
	}
	games, err := pgn.ParseString(req.PGN)
	warning := ""
	var pe *pgn.ParseError
	switch {
	case errors.As(err, &pe) && len(games) == 0 && pe.Ply > 0:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("в PGN ошибка: партия %d, полуход %d: %s", pe.Game, pe.Ply, pe.Msg))
		return
	case errors.As(err, &pe) && len(games) == 0:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("в PGN ошибка: партия %d: %s", pe.Game, pe.Msg))
		return
	case errors.As(err, &pe):
		// the games before the broken one are still compared
		warning = fmt.Sprintf("партия %d не прочитана (%s), сравнены партии до неё", pe.Game, pe.Msg)
	case err != nil:
		writeError(w, http.StatusBadRequest, "в PGN ошибка: "+err.Error())
		return
	case len(games) == 0:
		writeError(w, http.StatusBadRequest, "в тексте не найдено ни одной партии")
		return
	}
	moves, err := s.cfg.Store.RepMoves(r.Context(), rep.ID)
	if err != nil {
		serverError(w, "moves", err)
		return
	}
	items, skipped := comparePasted(moves, rep.Color, req.Name, games)
	counts := map[string]int{}
	for _, it := range items {
		counts[it.Dev.Status]++
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": items, "counts": counts, "read": len(games), "skipped": skipped, "warning": warning})
}

package play

import (
	"context"
	"encoding/json"
	"log"

	"dyanis-chess/backend/internal/ratings"
	"dyanis-chess/backend/internal/store"
)

// Correspondence games (days per move) and the games two friends start from a challenge.

// corrStore is the part of the store that correspondence games need. It is asked for apart from Store so
// that a store without it (a test's) still works for everything else.
type corrStore interface {
	CorrespondenceGames(ctx context.Context, userID string) ([]*store.Game, error)
	ActiveCorrespondenceIDs(ctx context.Context) ([]string, error)
}

// NewChallenged starts a game between two people who agreed to it (an accepted challenge): white
// plays white, black plays black. The game begins at once.
func (h *Hub) NewChallenged(ctx context.Context, white, black Player, tc TimeControl, rated bool) (string, error) {
	if err := tc.Validate(); err != nil {
		return "", err
	}
	if rated && tc.BaseMs == 0 {
		return "", errNotRatable
	}
	if rated {
		cat := ratings.CategoryFor(tc.BaseMs, tc.IncMs)
		h.stamp(ctx, &white, cat)
		h.stamp(ctx, &black, cat)
	}
	g := newGame(newID(), h.cfg.Now, tc, white, White)
	g.rated = rated
	g.black = &black
	h.register(g)
	g.mu.Lock()
	g.start()
	g.changed()
	g.mu.Unlock()
	return g.ID, nil
}

// CorrGame is a line of "my correspondence games".
type CorrGame struct {
	ID       string  `json:"id"`
	Opponent *Player `json:"opponent"`
	You      Side    `json:"you"`
	YourTurn bool    `json:"yourTurn"`
	Due      int64   `json:"due"` // unix ms
	Plies    int     `json:"plies"`
	Days     int     `json:"days"`
}

// Correspondence lists the unfinished correspondence games of a person, the nearest deadline first.
func (h *Hub) Correspondence(ctx context.Context, userID string) ([]CorrGame, error) {
	cs, ok := h.cfg.Store.(corrStore)
	if !ok {
		return []CorrGame{}, nil
	}
	rows, err := cs.CorrespondenceGames(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]CorrGame, 0, len(rows))
	for _, row := range rows {
		var s saved
		if json.Unmarshal(row.State, &s) != nil {
			continue
		}
		c := CorrGame{ID: row.ID, Plies: len(s.Moves), Days: s.TC.Days, Due: row.DueAt.UnixMilli()}
		if row.WhiteID == userID {
			c.You, c.Opponent = White, s.Black
		} else {
			c.You, c.Opponent = Black, s.White
		}
		c.YourTurn = (len(s.Moves)%2 == 0) == (c.You == White)
		out = append(out, c)
	}
	return out, nil
}

// Resume brings the unfinished correspondence games back into memory after a restart, which also sets
// their deadlines running again: nobody has to open a game for its time to count.
func (h *Hub) Resume(ctx context.Context) {
	cs, ok := h.cfg.Store.(corrStore)
	if !ok {
		return
	}
	ids, err := cs.ActiveCorrespondenceIDs(ctx)
	if err != nil {
		log.Printf("play: resume correspondence games: %v", err)
		return
	}
	n := 0
	for _, id := range ids {
		if _, err := h.find(ctx, id); err != nil {
			log.Printf("play: resume game %s: %v", id, err)
			continue
		}
		n++
	}
	if n > 0 {
		log.Printf("play: %d correspondence games resumed", n)
	}
}

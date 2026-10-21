package vplay

import (
	"context"
	"errors"

	"dyanis-chess/backend/internal/variants"
)

// ErrNoGame is the answer of ReviewGame to an unknown game (or a game of four).
var ErrNoGame = errors.New("партия не найдена")

// ReviewGame is what the review of a finished game needs: the variant, its
// options (the seed included, the server's own secret) and the actions.
type ReviewGame struct {
	Variant string
	Options variants.Options
	Actions []string
	Status  string
	Result  string
	// White and Black are the players' names.
	White, Black string
}

// ReviewGame reads a game of two players, from memory or the database. The seed
// never reaches a client: the server alone replays the game with it.
func (h *Hub) ReviewGame(ctx context.Context, id string) (*ReviewGame, error) {
	rm, err := h.findRoom(ctx, id)
	if errors.Is(err, errNoGame) {
		return nil, ErrNoGame
	}
	if err != nil {
		return nil, err
	}
	g, ok := rm.(*Game)
	if !ok {
		return nil, ErrNoGame
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	out := &ReviewGame{Variant: g.vid, Options: g.opts, Status: g.status, Result: g.result}
	for _, m := range g.moves {
		out.Actions = append(out.Actions, m.Action)
	}
	if g.white != nil {
		out.White = g.white.Name
	}
	if g.black != nil {
		out.Black = g.black.Name
	}
	return out, nil
}

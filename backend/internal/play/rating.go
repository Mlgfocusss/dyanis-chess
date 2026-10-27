package play

import (
	"context"
	"crypto/rand"
	"errors"
	"log"
	"time"

	"dyanis-chess/backend/internal/ratings"
)

// Ratings is the part of internal/ratings that games use. nil means no game is rated.
type Ratings interface {
	Get(ctx context.Context, userID, category string) (ratings.Entry, error)
	Apply(ctx context.Context, category, aID, bID string, scoreA float64) (a, b ratings.Change, err error)
}

// RatingChanges is what a rated game did to each side's rating.
type RatingChanges struct {
	White *ratings.Change `json:"white,omitempty"`
	Black *ratings.Change `json:"black,omitempty"`
	// Reason says why nobody's rating moved: "shared" when the two accounts look like one person
	// (the same browser or network, see internal/sameperson).
	Reason string `json:"reason,omitempty"`
}

// category is the rating category of the game, "" when it has no clock.
func (g *Game) category() string { return ratings.CategoryFor(g.tc.BaseMs, g.tc.IncMs) }

// stamp puts the person's current rating in the category on the seat, for the header of the game.
func (h *Hub) stamp(ctx context.Context, p *Player, category string) {
	if h.cfg.Ratings == nil || category == "" || p == nil || p.IsBot() {
		return
	}
	e, err := h.cfg.Ratings.Get(ctx, p.ID, category)
	if err != nil {
		log.Printf("play: rating of %s: %v", p.ID, err)
		return
	}
	p.Elo, p.Prov = e.Rating, e.Provisional
}

// afterRating starts the rating update of a finished rated game between two people. The caller
// holds the game's lock; the update runs apart, because it waits for the database.
func (h *Hub) afterRating(g *Game) {
	if g.status != StatusEnded || !g.rated || g.rateDone || h.cfg.Ratings == nil {
		return
	}
	g.rateDone = true
	if g.white == nil || g.black == nil || g.white.IsBot() || g.black.IsBot() || g.result == "" {
		return
	}
	short := len(g.moves) < minRatedPlies // a game given up at once is not a game: nobody's rating moves
	var score float64
	switch g.result {
	case "1-0":
		score = 1
	case "0-1":
		score = 0
	default:
		score = 0.5
	}
	cat, w, b := g.category(), g.white.ID, g.black.ID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var cw, cb ratings.Change
		reason := ""
		if !short {
			var err error
			cw, cb, err = h.cfg.Ratings.Apply(ctx, cat, w, b, score)
			switch {
			case errors.Is(err, ratings.ErrSamePerson):
				short, reason = true, "shared"
			case err != nil:
				log.Printf("play: rating of game %s: %v", g.ID, err)
				return
			}
		}
		g.mu.Lock()
		g.change = &RatingChanges{Reason: reason}
		if !short {
			g.change = &RatingChanges{White: &cw, Black: &cb}
		}
		g.changed() // saves it and shows the change to both players
		g.mu.Unlock()
	}()
}

// minRatedPlies is how many half-moves a rated game needs to count for the rating (two moves each).
const minRatedPlies = 4

var errNotRatable = errors.New("рейтинговая партия идёт только с часами")

// NewMatched makes a running game between two people that matchmaking has brought together, the
// colours drawn at random. A rated game counts for both people's rating in its category.
func (h *Hub) NewMatched(ctx context.Context, a, b Player, tc TimeControl, rated bool) (string, error) {
	if err := tc.Validate(); err != nil {
		return "", err
	}
	if rated && tc.BaseMs == 0 {
		return "", errNotRatable
	}
	if rated {
		h.stamp(ctx, &a, ratings.CategoryFor(tc.BaseMs, tc.IncMs))
		h.stamp(ctx, &b, ratings.CategoryFor(tc.BaseMs, tc.IncMs))
	}
	side := White
	if randomBit() {
		side = Black
	}
	g := newGame(newID(), h.cfg.Now, tc, a, side)
	g.rated = rated
	if side == White {
		g.black = &b
	} else {
		g.white = &b
	}
	h.register(g)
	g.mu.Lock()
	g.start()
	g.changed()
	g.mu.Unlock()
	return g.ID, nil
}

func randomBit() bool {
	var b [1]byte
	_, _ = rand.Read(b[:])
	return b[0]&1 == 1
}

func (g *Game) isRated() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.rated
}

// refreshed is a copy of the seat with the person's rating as it is now (after the last game).
func (h *Hub) refreshed(p *Player, category string) *Player {
	if p == nil {
		return nil
	}
	c := *p
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	h.stamp(ctx, &c, category)
	return &c
}

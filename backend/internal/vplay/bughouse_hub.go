package vplay

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"dyanis-chess/backend/internal/store"
	"dyanis-chess/backend/internal/variants"
)

// The hub's side of bughouse games (see bughouse.go): finding a game of either kind, making one, saving it.

// room answers for a request: a game of two or of four, from memory or the database.
func (h *Hub) room(w http.ResponseWriter, r *http.Request) room {
	rm, err := h.findRoom(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, errNoGame):
		writeError(w, http.StatusNotFound, "партия не найдена")
	case err != nil:
		log.Printf("vplay: load game %s: %v", r.PathValue("id"), err)
		writeError(w, http.StatusInternalServerError, "не удалось открыть партию")
	default:
		return rm
	}
	return nil
}

func (h *Hub) findRoom(ctx context.Context, id string) (room, error) {
	if !idPattern.MatchString(id) {
		return nil, errNoGame
	}
	h.mu.Lock()
	b := h.bugs[id]
	g := h.games[id]
	h.mu.Unlock()
	switch {
	case b != nil:
		return b, nil
	case g != nil:
		return g, nil
	case h.cfg.Store == nil:
		return nil, errNoGame
	}
	row, err := h.cfg.Store.LoadVariantGame(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNoGame
	}
	if err != nil {
		return nil, err
	}
	if !isBugState(row.State) {
		return h.find(ctx, id)
	}
	loaded, err := restoreBug(id, h.cfg.Now, row.State)
	if err != nil {
		return nil, err
	}
	// The columns follow a merge of accounts; the document does not.
	for i, uid := range [variants.BugSeats]string{row.WhiteID, row.BlackID, row.ThirdID, row.FourthID} {
		if loaded.seats[i] != nil && uid != "" {
			loaded.seats[i].ID = uid
		}
	}
	loaded.persist = h.persistBug
	h.mu.Lock()
	if existing := h.bugs[id]; existing != nil {
		h.mu.Unlock()
		return existing, nil
	}
	h.bugs[id] = loaded
	h.mu.Unlock()
	loaded.tick() // a clock may have run out while the server was down
	return loaded, nil
}

func (h *Hub) registerBug(g *BugGame) {
	g.persist = h.persistBug
	h.mu.Lock()
	h.bugs[g.ID] = g
	h.mu.Unlock()
}

// persistBug queues the state of a bughouse game for the saver. Called with the game's lock held.
func (h *Hub) persistBug(g *BugGame, state []byte) {
	if h.cfg.Store == nil {
		return
	}
	row := &store.VariantGame{
		ID: g.ID, Variant: g.vid, Status: g.status, Result: g.result, State: state,
		CreatedAt: g.created, UpdatedAt: h.cfg.Now(),
	}
	ids := [variants.BugSeats]*string{&row.WhiteID, &row.BlackID, &row.ThirdID, &row.FourthID}
	for i, p := range g.seats {
		if p != nil {
			*ids[i] = p.ID
		}
	}
	h.saveMu.Lock()
	h.pending[g.ID] = row
	h.saveMu.Unlock()
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

// createBug makes a waiting game of bughouse with its creator seated at board 1 (White or Black as asked).
func (h *Hub) createBug(w http.ResponseWriter, u *store.User, v variants.Variant, opts variants.Options, tc TimeControl, side Side) {
	h.mu.Lock()
	waiting := 0
	for _, g := range h.games {
		g.mu.Lock()
		if g.status == StatusWaiting && g.creator == u.ID {
			waiting++
		}
		g.mu.Unlock()
	}
	for _, g := range h.bugs {
		g.mu.Lock()
		if g.status == StatusWaiting && g.creator == u.ID {
			waiting++
		}
		g.mu.Unlock()
	}
	full := len(h.games)+len(h.bugs) >= maxGames
	h.mu.Unlock()
	if full {
		writeError(w, http.StatusServiceUnavailable, "сервер перегружен, попробуйте позже")
		return
	}
	if waiting >= maxWaitingPerUsr {
		writeError(w, http.StatusTooManyRequests, "у вас уже много приглашений без ответа: отмените ненужные")
		return
	}
	g, err := newBugGame(newID(), h.cfg.Now, v, opts, tc, playerOf(u), variants.BugSeat(0, side))
	if err != nil {
		log.Printf("vplay: new %s game: %v", v.Info().ID, err)
		writeError(w, http.StatusInternalServerError, "не удалось создать партию")
		return
	}
	h.registerBug(g)
	g.mu.Lock()
	g.changed() // saves it
	g.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]string{"id": g.ID})
}

// rematchBug starts the new game when all four want one: the same teams, the colours swapped.
func (h *Hub) rematchBug(g *BugGame, uid string) error {
	n, err := g.Rematch(uid, func() (*BugGame, error) {
		v, opts, err := variants.Prepare(g.vid, g.opts.Params, newSeed())
		if err != nil {
			return nil, err
		}
		n, err := blankBugGame(newID(), h.cfg.Now, v, opts, g.tc)
		if err == nil {
			h.registerBug(n) // before the old game announces its id
		}
		return n, err
	})
	if err != nil || n == nil {
		return err
	}
	n.mu.Lock()
	n.changed()
	n.mu.Unlock()
	return nil
}

// sweepBugs gives up on invitations nobody completed and forgets ended games nobody watches.
func (h *Hub) sweepBugs(now time.Time) {
	h.mu.Lock()
	var drop []*BugGame
	for id, g := range h.bugs {
		g.mu.Lock()
		switch {
		case g.status == StatusWaiting && now.Sub(g.created) > waitingTTL:
			g.finish("", TermWaitingGaveUp)
			g.changed()
		case (g.status == StatusEnded || g.status == StatusAborted) && len(g.conns) == 0 && now.Sub(g.ended) > keepEnded:
			drop = append(drop, g)
			delete(h.bugs, id)
		}
		g.mu.Unlock()
	}
	h.mu.Unlock()
	for _, g := range drop {
		g.Close()
	}
}

// joinBody reads the optional {"seat": n} of a join request: -1 for "any free place".
func joinBody(r *http.Request) int {
	var b struct {
		Seat *int `json:"seat"`
	}
	if r.Body == nil || json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBody)).Decode(&b) != nil || b.Seat == nil {
		return -1
	}
	return *b.Seat
}

// listedBug fills a row of "my games" from the saved document of a bughouse game.
func listedBug(row *store.VariantGame, userID string) (Listed, bool) {
	var s bugSaved
	if json.Unmarshal(row.State, &s) != nil {
		return Listed{}, false
	}
	l := Listed{
		ID: row.ID, Variant: row.Variant, Status: row.Status, Result: row.Result, Termination: s.Termination,
		TC: s.TC, Plies: len(s.Moves), Updated: row.UpdatedAt.UnixMilli(), Players: s.Seats[:],
	}
	if v, ok := variants.Get(row.Variant); ok {
		l.VariantName = v.Info().Name
	}
	for i, id := range [variants.BugSeats]string{row.WhiteID, row.BlackID, row.ThirdID, row.FourthID} {
		if id == userID {
			_, l.You = variants.BugPlace(i)
			seat := i
			l.Seat = &seat
		}
	}
	return l, true
}

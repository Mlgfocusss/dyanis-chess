package vplay

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"dyanis-chess/backend/internal/variants"
)

// The quick game: a person asks for a game of a variant with a clock and waits;
// the first other person who asked for the same (variant, clock and lobby
// choices) is paired with them at once. There are no ratings, so there is no
// window of acceptable difference and no waiting loop: the pairing happens in
// the request of the second person.

// Timing of the queue.
const (
	seekIdle   = 20 * time.Second // a seek nobody has asked about for this long is dropped (the person left)
	seekFound  = 2 * time.Minute  // a pairing that was not collected is forgotten
	seekReread = 10 * time.Second // a pairing that was collected is repeated to a retry for this long
)

type seekRequest struct {
	Variant string         `json:"variant"`
	BaseMs  int64          `json:"baseMs"`
	IncMs   int64          `json:"incMs"`
	Options map[string]any `json:"options,omitempty"`
}

type seekEntry struct {
	who      Player
	req      seekRequest
	key      string
	joined   time.Time
	lastSeen time.Time
	found    string // the id of the game made for the pair, once paired
	foundAt  time.Time
	readAt   time.Time // when the person first learned of it
}

// SeekStatus is the state of a person's seek.
type SeekStatus struct {
	State  string `json:"status"` // "none", "waiting" or "found"
	ID     string `json:"id,omitempty"`
	Waited int    `json:"waited,omitempty"` // seconds
	Count  int    `json:"count,omitempty"`  // people waiting for the same game
}

// seekKey says which seeks may be paired: the same variant, clock and choices.
// json.Marshal sorts map keys, so equal choices give equal keys.
func seekKey(r seekRequest) string {
	opts, _ := json.Marshal(r.Options)
	b, _ := json.Marshal([]any{r.Variant, r.BaseMs, r.IncMs, string(opts)})
	return string(b)
}

// sweepSeeks forgets the abandoned. Called with the queue unlocked.
func (h *Hub) sweepSeeks() {
	h.seekMu.Lock()
	defer h.seekMu.Unlock()
	h.sweepSeeksLocked(h.cfg.Now())
}

func (h *Hub) sweepSeeksLocked(now time.Time) {
	for id, k := range h.seeks {
		switch {
		case k.found == "" && now.Sub(k.lastSeen) > seekIdle:
			delete(h.seeks, id)
		case k.found != "" && now.Sub(k.foundAt) > seekFound:
			delete(h.seeks, id)
		case !k.readAt.IsZero() && now.Sub(k.readAt) > seekReread:
			delete(h.seeks, id) // told, and the person has gone to the game
		}
	}
}

func (h *Hub) statusLocked(userID string) SeekStatus {
	now := h.cfg.Now()
	k := h.seeks[userID]
	if k == nil {
		return SeekStatus{State: "none"}
	}
	k.lastSeen = now
	if k.found != "" {
		if k.readAt.IsZero() {
			k.readAt = now
		}
		return SeekStatus{State: "found", ID: k.found}
	}
	n := 0
	for _, o := range h.seeks {
		if o.found == "" && o.key == k.key {
			n++
		}
	}
	return SeekStatus{State: "waiting", Waited: int(now.Sub(k.joined).Seconds()), Count: n}
}

func (h *Hub) seekPost(w http.ResponseWriter, r *http.Request) {
	u := h.user(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "чтобы играть, войдите или продолжите как гость")
		return
	}
	var req seekRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "ожидался JSON {variant, baseMs, incMs, options}")
		return
	}
	if err := (TimeControl{BaseMs: req.BaseMs, IncMs: req.IncMs}).Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if v, _, err := variants.Prepare(req.Variant, req.Options, 0); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	} else if _, four := v.(variants.BugVariant); four {
		writeError(w, http.StatusBadRequest, "в шведские шахматы играют по ссылке: быстрого подбора на четверых нет")
		return
	}
	now := h.cfg.Now()
	me := &seekEntry{who: playerOf(u), req: req, key: seekKey(req), joined: now, lastSeen: now}

	h.seekMu.Lock()
	defer h.seekMu.Unlock()
	h.sweepSeeksLocked(now)
	delete(h.seeks, u.ID) // a new search replaces the old one
	// The longest waiting person of the same game, if any.
	var mate *seekEntry
	for _, o := range h.seeks {
		if o.found == "" && o.key == me.key && o.who.ID != u.ID && (mate == nil || o.joined.Before(mate.joined)) {
			mate = o
		}
	}
	if mate != nil {
		id, err := h.newMatched(mate.who, me.who, req)
		if err != nil {
			log.Printf("vplay: making a game for %s and %s: %v", mate.who.ID, me.who.ID, err)
			delete(h.seeks, mate.who.ID)
			writeError(w, http.StatusInternalServerError, "не удалось создать партию, попробуйте ещё раз")
			return
		}
		mate.found, mate.foundAt = id, now
		me.found, me.foundAt = id, now
	}
	h.seeks[u.ID] = me
	writeJSON(w, http.StatusOK, h.statusLocked(u.ID))
}

func (h *Hub) seekGet(w http.ResponseWriter, r *http.Request) {
	u := h.user(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "чтобы играть, войдите или продолжите как гость")
		return
	}
	h.seekMu.Lock()
	defer h.seekMu.Unlock()
	h.sweepSeeksLocked(h.cfg.Now())
	writeJSON(w, http.StatusOK, h.statusLocked(u.ID))
}

func (h *Hub) seekDelete(w http.ResponseWriter, r *http.Request) {
	u := h.user(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "чтобы играть, войдите или продолжите как гость")
		return
	}
	h.seekMu.Lock()
	defer h.seekMu.Unlock()
	if k := h.seeks[u.ID]; k != nil {
		delete(h.seeks, u.ID)
		if k.found != "" { // too late to cancel: the game exists
			writeJSON(w, http.StatusOK, SeekStatus{State: "found", ID: k.found})
			return
		}
	}
	writeJSON(w, http.StatusOK, SeekStatus{State: "none"})
}

// newMatched makes a started game of two people; the colours are drawn.
func (h *Hub) newMatched(a, b Player, req seekRequest) (string, error) {
	v, opts, err := variants.Prepare(req.Variant, req.Options, newSeed())
	if err != nil {
		return "", err
	}
	side, _ := pickColor("random")
	first, second := a, b
	if side == Black {
		first, second = b, a
	}
	g, err := newGame(newID(), h.cfg.Now, v, opts, TimeControl{BaseMs: req.BaseMs, IncMs: req.IncMs}, first, White)
	if err != nil {
		return "", err
	}
	g.black = &second
	g.mu.Lock()
	g.start()
	g.mu.Unlock()
	h.register(g)
	g.mu.Lock()
	g.changed()
	g.mu.Unlock()
	return g.ID, nil
}

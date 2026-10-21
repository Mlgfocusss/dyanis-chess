package puzzles

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"dyanis-chess/backend/internal/ratings"
	"dyanis-chess/backend/internal/store"
)

// The puzzle duel ("Тактическая дуэль"): two people, brought together by matchmaking, solve the same
// puzzles at the same time for three minutes. Each goes through the same list at their own pace;
// a puzzle solved is a point, a mistake costs the puzzle (and counts against the person on a tie).
// Whoever has more points when the time is up wins; the result moves the "duel" Glicko-2 rating.
// The browser plays the puzzles (it has the solution, as in the other modes) and tells the server
// how each ended; the server keeps the clock, the scores and the list, and shows each person the
// other's progress. A duel lives in memory; the finished ones are written down.

// Timing and size of a duel.
const (
	duelCountdown = 5 * time.Second
	duelLength    = 3 * time.Minute
	duelGrace     = 2 * time.Second // a report this late after the end still counts (network)
	duelStep      = 35              // rating added to each next puzzle
	duelMaxIndex  = 200
	duelKeep      = 15 * time.Minute // an over duel stays readable this long
)

// DuelPlayer is a person in a duel.
type DuelPlayer struct {
	ID, Name, Avatar string
	Rating           int  // the duel rating when it began
	Provisional      bool // that rating is still a rough guess
}

// DuelRatings is the part of internal/ratings a duel uses; nil: the duel is not rated.
type DuelRatings interface {
	Apply(ctx context.Context, category, aID, bID string, scoreA float64) (a, b ratings.Change, err error)
}

type duelSide struct {
	DuelPlayer
	score, mistakes, index int
	change                 *ratings.Change
}

type duel struct {
	id       string
	a, b     *duelSide
	startsAt time.Time
	endsAt   time.Time
	start    int // the rating of the first puzzle
	puzzles  []*store.Puzzle
	done     bool   // finalised: written down and rated
	result   string // "a", "b" or "draw" once done
	finishAt *time.Timer
}

func (d *duel) side(userID string) (me, opp *duelSide) {
	switch userID {
	case d.a.ID:
		return d.a, d.b
	case d.b.ID:
		return d.b, d.a
	}
	return nil, nil
}

// startRating is where a duel's puzzles begin: well below the players' own level, so that the first
// ones are quick. The duel rating starts at 1200, the puzzles at 500 to 800.
func startRating(a, b DuelPlayer) int {
	avg := (a.Rating + b.Rating) / 2
	return max(rushMinRating+100, min(1800, avg-500))
}

// NewDuel makes a duel for two people and returns its id. It starts after a short countdown.
func (s *Service) NewDuel(_ context.Context, a, b DuelPlayer) (string, error) {
	if a.ID == "" || b.ID == "" || a.ID == b.ID {
		return "", errors.New("дуэль: нужны два разных человека")
	}
	var raw [8]byte
	_, _ = rand.Read(raw[:])
	now := s.cfg.Now()
	d := &duel{
		id: hex.EncodeToString(raw[:]), a: &duelSide{DuelPlayer: a}, b: &duelSide{DuelPlayer: b},
		startsAt: now.Add(duelCountdown), endsAt: now.Add(duelCountdown + duelLength), start: startRating(a, b),
	}
	s.duelMu.Lock()
	for id, o := range s.duels { // forget the old ones
		if o.done && now.Sub(o.endsAt) > duelKeep {
			delete(s.duels, id)
		}
	}
	s.duels[d.id] = d
	s.duelMu.Unlock()
	// the duel is finalised even when nobody asks about it any more
	d.finishAt = time.AfterFunc(duelCountdown+duelLength+duelGrace+time.Second, func() { s.finishDuel(d.id) })
	return d.id, nil
}

// finishDuel ends a duel whose time is up: the winner, the record, the ratings. Once only.
func (s *Service) finishDuel(id string) {
	s.duelMu.Lock()
	d := s.duels[id]
	if d == nil || d.done || s.cfg.Now().Before(d.endsAt.Add(duelGrace)) {
		s.duelMu.Unlock()
		return
	}
	d.done = true
	switch {
	case d.a.score > d.b.score, d.a.score == d.b.score && d.a.mistakes < d.b.mistakes:
		d.result = "a"
	case d.b.score > d.a.score, d.a.score == d.b.score && d.b.mistakes < d.a.mistakes:
		d.result = "b"
	default:
		d.result = "draw"
	}
	row := &store.DuelRow{
		ID: d.id, AID: d.a.ID, BID: d.b.ID, AScore: d.a.score, BScore: d.b.score,
		AMistakes: d.a.mistakes, BMistakes: d.b.mistakes, Result: d.result, CreatedAt: d.startsAt,
	}
	result, a, b := d.result, d.a, d.b
	s.duelMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.cfg.Store.SaveDuel(ctx, row); err != nil {
		log.Printf("puzzles: save duel %s: %v", id, err)
	}
	if s.cfg.Ratings == nil {
		return
	}
	score := 0.5
	switch result {
	case "a":
		score = 1
	case "b":
		score = 0
	}
	ca, cb, err := s.cfg.Ratings.Apply(ctx, ratings.Duel, a.ID, b.ID, score)
	if err != nil {
		log.Printf("puzzles: rate duel %s: %v", id, err)
		return
	}
	s.duelMu.Lock()
	a.change, b.change = &ca, &cb
	s.duelMu.Unlock()
}

// duelSideJSON is one person's progress as it is shown.
type duelSideJSON struct {
	Name        string          `json:"name"`
	Avatar      string          `json:"avatar,omitempty"`
	Rating      int             `json:"rating"`
	Provisional bool            `json:"provisional,omitempty"`
	Score       int             `json:"score"`
	Mistakes    int             `json:"mistakes"`
	Index       int             `json:"index"` // the puzzle this person is on
	Change      *ratings.Change `json:"change,omitempty"`
}

type duelJSON struct {
	ID        string       `json:"id"`
	Status    string       `json:"status"` // "countdown", "running" or "over"
	ServerNow int64        `json:"serverNow"`
	StartsAt  int64        `json:"startsAt"`
	EndsAt    int64        `json:"endsAt"`
	Me        duelSideJSON `json:"me"`
	Opp       duelSideJSON `json:"opponent"`
	Outcome   string       `json:"outcome,omitempty"` // "win", "loss" or "draw" once over
}

func sideJSON(x *duelSide) duelSideJSON {
	return duelSideJSON{Name: x.Name, Avatar: x.Avatar, Rating: x.Rating, Provisional: x.Provisional, Score: x.score, Mistakes: x.mistakes, Index: x.index, Change: x.change}
}

func (s *Service) viewDuel(d *duel, userID string, now time.Time) (duelJSON, bool) {
	me, opp := d.side(userID)
	if me == nil {
		return duelJSON{}, false
	}
	v := duelJSON{ID: d.id, ServerNow: now.UnixMilli(), StartsAt: d.startsAt.UnixMilli(), EndsAt: d.endsAt.UnixMilli(), Me: sideJSON(me), Opp: sideJSON(opp)}
	switch {
	case now.Before(d.startsAt):
		v.Status = "countdown"
	case now.Before(d.endsAt):
		v.Status = "running"
	default:
		v.Status = "over"
		if d.done {
			switch {
			case d.result == "draw":
				v.Outcome = "draw"
			case (d.result == "a") == (me == d.a):
				v.Outcome = "win"
			default:
				v.Outcome = "loss"
			}
		}
	}
	return v, true
}

// duelFor finds the duel of the request's person; it writes the error otherwise.
func (s *Service) duelFor(w http.ResponseWriter, r *http.Request) (*duel, string, bool) {
	userID := s.cfg.UserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "войдите, чтобы играть")
		return nil, "", false
	}
	s.duelMu.Lock()
	d := s.duels[r.PathValue("id")]
	s.duelMu.Unlock()
	if d == nil {
		writeError(w, http.StatusNotFound, "такой дуэли нет")
		return nil, "", false
	}
	if me, _ := d.side(userID); me == nil {
		writeError(w, http.StatusNotFound, "такой дуэли нет") // not yours: as if it did not exist
		return nil, "", false
	}
	return d, userID, true
}

func (s *Service) duelState(w http.ResponseWriter, r *http.Request) {
	d, uid, ok := s.duelFor(w, r)
	if !ok {
		return
	}
	now := s.cfg.Now()
	if !now.Before(d.endsAt.Add(duelGrace)) {
		s.finishDuel(d.id)
	}
	s.duelMu.Lock()
	v, _ := s.viewDuel(d, uid, now)
	s.duelMu.Unlock()
	writeJSON(w, http.StatusOK, v)
}

type duelProgress struct {
	Index  int  `json:"index"`
	Solved bool `json:"solved"`
}

// duelProgressed takes the end of the person's puzzle number Index: solved or not.
func (s *Service) duelProgressed(w http.ResponseWriter, r *http.Request) {
	d, uid, ok := s.duelFor(w, r)
	if !ok {
		return
	}
	var req duelProgress
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}
	now := s.cfg.Now()
	s.duelMu.Lock()
	me, _ := d.side(uid)
	switch {
	case now.Before(d.startsAt) || !now.Before(d.endsAt.Add(duelGrace)) || d.done:
		s.duelMu.Unlock()
		writeError(w, http.StatusConflict, "дуэль сейчас не идёт")
		return
	case req.Index != me.index || me.index >= duelMaxIndex:
		v, _ := s.viewDuel(d, uid, now) // out of step: the client is told where it really is
		s.duelMu.Unlock()
		writeJSON(w, http.StatusConflict, v)
		return
	}
	if req.Solved {
		me.score++
	} else {
		me.mistakes++
	}
	me.index++
	v, _ := s.viewDuel(d, uid, now)
	s.duelMu.Unlock()
	writeJSON(w, http.StatusOK, v)
}

// duelPuzzle gives puzzle number k of the duel, the same for both people. The list is made as it is asked for.
func (s *Service) duelPuzzle(w http.ResponseWriter, r *http.Request) {
	d, _, ok := s.duelFor(w, r)
	if !ok {
		return
	}
	k, err := strconv.Atoi(r.PathValue("k"))
	if err != nil || k < 0 || k >= duelMaxIndex {
		writeError(w, http.StatusBadRequest, "неверный номер задачи")
		return
	}
	if s.cfg.Now().Before(d.startsAt.Add(-2 * time.Second)) {
		writeError(w, http.StatusConflict, "дуэль ещё не началась") // no peeking during the countdown
		return
	}
	s.duelMu.Lock()
	defer s.duelMu.Unlock()
	if k > len(d.puzzles)+1 { // only the next one or two ahead
		writeError(w, http.StatusBadRequest, "задачи идут по порядку")
		return
	}
	for len(d.puzzles) <= k {
		seen := make([]string, 0, len(d.puzzles))
		for _, p := range d.puzzles {
			seen = append(seen, p.ID)
		}
		rating := min(2800, d.start+duelStep*len(d.puzzles))
		var p *store.Puzzle
		for i := 0; i < rushTries; i++ {
			if p, err = s.pick(r.Context(), "", "", rating); err != nil {
				break
			}
			dup := false
			for _, id := range seen {
				dup = dup || id == p.ID
			}
			if !dup {
				break
			}
		}
		if err != nil {
			log.Printf("puzzles: duel pick: %v", err)
			writeError(w, http.StatusInternalServerError, "не удалось выбрать задачу")
			return
		}
		d.puzzles = append(d.puzzles, p)
	}
	respond(w, d.puzzles[k], "", store.PuzzleRating{}, nil, nil)
}

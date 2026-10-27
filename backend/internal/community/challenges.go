package community

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"time"

	"dyanis-chess/backend/internal/play"
	"dyanis-chess/backend/internal/ratings"
	"dyanis-chess/backend/internal/store"
)

// How long a challenge waits for an answer: a game with a clock is wanted now, a correspondence
// game can wait.
const (
	clockChallengeTTL = 2 * time.Hour
	corrChallengeTTL  = 7 * 24 * time.Hour
	answeredShownFor  = 10 * time.Minute // the sender sees the answer this long
	minVacationGap    = 14 * 24 * time.Hour
	maxVacationDays   = 14
)

func newChallengeID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// ChallengeJSON is a challenge as one of its two people sees it; Person is the other side.
type ChallengeJSON struct {
	ID      string `json:"id"`
	Person  Person `json:"person"`
	Color   string `json:"color"` // the sender's colour: white, black or random
	BaseMs  int64  `json:"baseMs"`
	IncMs   int64  `json:"incMs"`
	Days    int    `json:"days,omitempty"`
	Rated   bool   `json:"rated,omitempty"`
	Status  string `json:"status"`
	GameID  string `json:"gameId,omitempty"`
	Created int64  `json:"created"`
	Expires int64  `json:"expires"`
	Friend  bool   `json:"friend,omitempty"`
	Rating  int    `json:"rating,omitempty"` // incoming: the sender's rating in the kind of game
	Prov    bool   `json:"prov,omitempty"`
}

// categoryOf is the rating category a challenge counts in (a correspondence game, which is never
// rated, is compared with the slowest one).
func categoryOf(c *store.ChallengeRow) string {
	if c.Days > 0 {
		return ratings.Rapid
	}
	if cat := ratings.CategoryFor(c.BaseMs, c.IncMs); cat != "" {
		return cat
	}
	return ratings.Rapid
}

func (s *Service) challenges(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	ctx, now := r.Context(), s.cfg.Now()
	in, err := s.cfg.Store.ChallengesFor(ctx, u.ID, true, now, answeredShownFor)
	if err != nil {
		serverError(w, "incoming challenges", err)
		return
	}
	out, err := s.cfg.Store.ChallengesFor(ctx, u.ID, false, now, answeredShownFor)
	if err != nil {
		serverError(w, "outgoing challenges", err)
		return
	}
	var ids []string
	for _, c := range in {
		ids = append(ids, c.FromID)
	}
	for _, c := range out {
		ids = append(ids, c.ToID)
	}
	people, err := s.people(ctx, u.ID, ids)
	if err != nil {
		serverError(w, "people", err)
		return
	}
	friends, err := s.cfg.Store.Friends(ctx, u.ID)
	if err != nil {
		serverError(w, "friends", err)
		return
	}
	isFriend := map[string]bool{}
	for _, f := range friends {
		isFriend[f.FriendID] = true
	}
	view := func(c *store.ChallengeRow, other string, withRating bool) (ChallengeJSON, bool) {
		p, ok := people[other]
		if !ok {
			return ChallengeJSON{}, false
		}
		j := ChallengeJSON{ID: c.ID, Person: p, Color: c.Color, BaseMs: c.BaseMs, IncMs: c.IncMs, Days: c.Days, Rated: c.Rated,
			Status: c.Status, GameID: c.GameID, Created: c.Created.UnixMilli(), Expires: c.Expires.UnixMilli(), Friend: isFriend[other]}
		if withRating && s.cfg.Rating != nil {
			if e, err := s.cfg.Rating(ctx, other, categoryOf(c)); err == nil {
				j.Rating, j.Prov = e.Rating, e.Provisional
			}
		}
		return j, true
	}
	incoming, outgoing := []ChallengeJSON{}, []ChallengeJSON{}
	for _, c := range in {
		if j, ok := view(c, c.FromID, true); ok {
			incoming = append(incoming, j)
		}
	}
	for _, c := range out {
		if j, ok := view(c, c.ToID, false); ok {
			outgoing = append(outgoing, j)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"incoming": incoming, "outgoing": outgoing})
}

// challenge sends a challenge to a game. The addressee decides who may: everybody, only friends or
// nobody; a friend can be muted; a range of the challenger's rating can be set; and a person who is
// away may refuse challenges until they are back. The answers do not say which of the reasons it was
// when that would give a person's private choice away.
func (s *Service) challenge(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	var req struct {
		To     string `json:"to"`
		Color  string `json:"color"`
		BaseMs int64  `json:"baseMs"`
		IncMs  int64  `json:"incMs"`
		Days   int    `json:"days"`
		Rated  bool   `json:"rated"`
	}
	if !decode(w, r, &req) {
		return
	}
	other := s.existing(w, r, u, req.To)
	if other == nil {
		return
	}
	switch req.Color {
	case "white", "black", "random":
	case "":
		req.Color = "random"
	default:
		writeError(w, http.StatusBadRequest, "цвет: white, black или random")
		return
	}
	tc := play.TimeControl{BaseMs: req.BaseMs, IncMs: req.IncMs, Days: req.Days}
	if err := tc.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Rated && (tc.Days > 0 || tc.BaseMs == 0) {
		writeError(w, http.StatusBadRequest, "рейтинговой может быть только партия с часами")
		return
	}
	c := &store.ChallengeRow{ID: newChallengeID(), FromID: u.ID, ToID: other.ID, Color: req.Color, BaseMs: tc.BaseMs, IncMs: tc.IncMs, Days: tc.Days, Rated: req.Rated}
	ctx, now := r.Context(), s.cfg.Now()
	if !s.gate(w, r, u.ID, other.ID, "не удалось отправить вызов") {
		return
	}

	// What the addressee allows.
	target, err := s.cfg.Store.UserByID(ctx, other.ID)
	if err != nil {
		serverError(w, "addressee", err)
		return
	}
	prefs := ParsePrefs(target.Settings)
	friend, err := s.cfg.Store.Friend(ctx, other.ID, u.ID) // the addressee's side of the friendship
	isFriend := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		serverError(w, "friend check", err)
		return
	}
	const refused = "игрок не принимает ваши вызовы"
	switch {
	case prefs.ChallengesFrom == "none":
		writeError(w, http.StatusForbidden, refused)
		return
	case prefs.ChallengesFrom == "friends" && !isFriend:
		writeError(w, http.StatusForbidden, "игрок принимает вызовы только от друзей")
		return
	case isFriend && friend.MuteChallenges:
		writeError(w, http.StatusForbidden, refused)
		return
	case !prefs.Offline && !s.online(other.ID):
		writeError(w, http.StatusConflict, "игрок сейчас не в сети и не принимает вызовы, пока его нет")
		return
	}
	if (prefs.Min > 0 || prefs.Max > 0) && !(isFriend && prefs.FriendsIgnore) && s.cfg.Rating != nil {
		e, err := s.cfg.Rating(ctx, u.ID, categoryOf(c))
		if err != nil {
			serverError(w, "rating", err)
			return
		}
		if (prefs.Min > 0 && e.Rating < prefs.Min) || (prefs.Max > 0 && e.Rating > prefs.Max) {
			writeError(w, http.StatusForbidden, "ваш рейтинг не подходит под условия игрока для входящих вызовов")
			return
		}
	}

	// What the sender may do.
	if !s.allow("challenge:"+u.ID, 10, 10*time.Minute) {
		writeError(w, http.StatusTooManyRequests, "слишком много вызовов, подождите немного")
		return
	}
	if n, err := s.cfg.Store.CountPendingChallenges(ctx, u.ID, other.ID, now); err != nil {
		serverError(w, "count challenges", err)
		return
	} else if n > 0 {
		writeError(w, http.StatusConflict, "вы уже отправили вызов этому игроку: дождитесь ответа или отмените его")
		return
	}
	if n, err := s.cfg.Store.CountPendingChallenges(ctx, u.ID, "", now); err != nil {
		serverError(w, "count challenges", err)
		return
	} else if n >= MaxPendingChallenge {
		writeError(w, http.StatusConflict, "слишком много неотвеченных вызовов: отмените ненужные")
		return
	}
	ttl := clockChallengeTTL
	if tc.Days > 0 {
		ttl = corrChallengeTTL
	}
	c.Created, c.Expires = now, now.Add(ttl)
	if err := s.cfg.Store.CreateChallenge(ctx, c); err != nil {
		serverError(w, "create challenge", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": c.ID, "expires": c.Expires.UnixMilli()})
}

func (s *Service) acceptChallenge(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	ctx, now := r.Context(), s.cfg.Now()
	c, err := s.cfg.Store.Challenge(ctx, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) || (err == nil && c.ToID != u.ID) {
		writeError(w, http.StatusNotFound, "вызов не найден")
		return
	}
	if err != nil {
		serverError(w, "challenge", err)
		return
	}
	if c.Status != store.ChallengePending || !now.Before(c.Expires) {
		writeError(w, http.StatusConflict, "вызов уже недействителен")
		return
	}
	// A block cancels the challenges between the two; this catches one made in the same instant.
	if st, err := s.cfg.Store.BlockBetween(ctx, u.ID, c.FromID); err != nil {
		serverError(w, "accept block", err)
		return
	} else if st.Any() {
		_, _ = s.cfg.Store.SetChallengeStatus(ctx, c.ID, store.ChallengeCancelled, "")
		writeError(w, http.StatusConflict, "вызов уже недействителен")
		return
	}
	from, err := s.cfg.Store.UserByID(ctx, c.FromID)
	if err != nil {
		writeError(w, http.StatusConflict, "отправитель вызова больше не на сайте")
		return
	}
	// The first to answer wins; a second press on another tab finds the challenge taken.
	if ok, err := s.cfg.Store.SetChallengeStatus(ctx, c.ID, store.ChallengeAccepted, ""); err != nil {
		serverError(w, "accept challenge", err)
		return
	} else if !ok {
		writeError(w, http.StatusConflict, "вызов уже недействителен")
		return
	}
	sender, receiver := play.Player{ID: from.ID, Name: from.Name, Avatar: from.AvatarURL}, play.Player{ID: u.ID, Name: u.Name, Avatar: u.AvatarURL}
	senderWhite := c.Color == "white"
	if c.Color == "random" {
		var b [1]byte
		_, _ = rand.Read(b[:])
		senderWhite = b[0]&1 == 0
	}
	white, black := sender, receiver
	if !senderWhite {
		white, black = receiver, sender
	}
	tc := play.TimeControl{BaseMs: c.BaseMs, IncMs: c.IncMs, Days: c.Days}
	gameID, err := s.cfg.Start(ctx, white, black, tc, c.Rated)
	if err != nil {
		log.Printf("community: start game of challenge %s: %v", c.ID, err)
		_, _ = s.cfg.Store.SetChallengeStatus(ctx, c.ID, store.ChallengeCancelled, "")
		writeError(w, http.StatusInternalServerError, "не удалось начать партию")
		return
	}
	if err := s.cfg.Store.SetChallengeGame(ctx, c.ID, gameID); err != nil {
		log.Printf("community: record game of challenge %s: %v", c.ID, err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"gameId": gameID})
}

func (s *Service) declineChallenge(w http.ResponseWriter, r *http.Request) {
	s.closeChallenge(w, r, true)
}

func (s *Service) cancelChallenge(w http.ResponseWriter, r *http.Request) {
	s.closeChallenge(w, r, false)
}

// closeChallenge: the addressee declines, or the sender takes the challenge back.
func (s *Service) closeChallenge(w http.ResponseWriter, r *http.Request, decline bool) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	ctx := r.Context()
	c, err := s.cfg.Store.Challenge(ctx, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) || (err == nil && ((decline && c.ToID != u.ID) || (!decline && c.FromID != u.ID))) {
		writeError(w, http.StatusNotFound, "вызов не найден")
		return
	}
	if err != nil {
		serverError(w, "challenge", err)
		return
	}
	status := store.ChallengeCancelled
	if decline {
		status = store.ChallengeDeclined
	}
	if _, err := s.cfg.Store.SetChallengeStatus(ctx, c.ID, status, ""); err != nil {
		serverError(w, "close challenge", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

// Sweep removes old challenges; the server calls it now and then.
func (s *Service) Sweep() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.cfg.Store.CleanChallenges(ctx, s.cfg.Now()); err != nil {
		log.Printf("community: clean challenges: %v", err)
	}
	s.forget()
}

// forget drops what was remembered about people who have not been here for an hour: when they were seen,
// their settings and the counts of the rate limits.
func (s *Service) forget() {
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, t := range s.seen {
		if now.Sub(t) > time.Hour {
			delete(s.seen, id)
		}
	}
	for id, c := range s.prefs {
		if now.Sub(c.at) > time.Hour {
			delete(s.prefs, id)
		}
	}
	for k, hits := range s.hits {
		if len(hits) == 0 || now.Sub(hits[len(hits)-1]) > time.Hour {
			delete(s.hits, k)
		}
	}
}

// ---------------------------------------------------- correspondence, vacation

func (s *Service) correspondence(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	items := []play.CorrGame{}
	if s.cfg.Correspondence != nil {
		list, err := s.cfg.Correspondence(r.Context(), u.ID)
		if err != nil {
			serverError(w, "correspondence", err)
			return
		}
		items = list
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// setVacation starts a vacation of up to two weeks, or ends the one that is on (days 0). While it
// lasts the person's clocks in correspondence games do not fall. Not more often than once in two weeks
// after the last one, so that it is a rest and not a way of never losing on time.
func (s *Service) setVacation(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	var req struct {
		Days int `json:"days"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Days < 0 || req.Days > maxVacationDays {
		writeError(w, http.StatusBadRequest, "отпуск: от 1 до 14 дней")
		return
	}
	ctx, now := r.Context(), s.cfg.Now()
	v, err := s.cfg.Store.Vacation(ctx, u.ID)
	if err != nil {
		serverError(w, "vacation", err)
		return
	}
	if req.Days == 0 {
		if v.Ends.After(now) {
			v = store.VacationRow{LastEnded: now}
			if err := s.cfg.Store.SetVacation(ctx, u.ID, v); err != nil {
				serverError(w, "end vacation", err)
				return
			}
			s.forgetPrefs(u.ID)
		}
		writeJSON(w, http.StatusOK, map[string]int64{"vacation": 0})
		return
	}
	if v.Ends.After(now) {
		writeError(w, http.StatusConflict, "отпуск уже идёт")
		return
	}
	if last := latest(v.LastEnded, v.Ends); !last.IsZero() && now.Sub(last) < minVacationGap { // also when the last one simply ran out
		writeError(w, http.StatusConflict, "новый отпуск можно взять через 14 дней после прошлого")
		return
	}
	v = store.VacationRow{Ends: now.Add(time.Duration(req.Days) * 24 * time.Hour), LastEnded: latest(v.LastEnded, v.Ends)}
	if err := s.cfg.Store.SetVacation(ctx, u.ID, v); err != nil {
		serverError(w, "start vacation", err)
		return
	}
	s.forgetPrefs(u.ID)
	writeJSON(w, http.StatusOK, map[string]int64{"vacation": v.Ends.UnixMilli()})
}

func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

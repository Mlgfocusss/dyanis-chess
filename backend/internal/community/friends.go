package community

import (
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"dyanis-chess/backend/internal/store"
)

// FriendJSON is a friend in the list.
type FriendJSON struct {
	Person
	Muted  bool  `json:"muted,omitempty"` // challenges of this friend are refused
	Unread int   `json:"unread,omitempty"`
	Since  int64 `json:"since"`
}

// RequestJSON is a request to be friends, waiting.
type RequestJSON struct {
	Person
	At int64 `json:"at"`
}

func (s *Service) friends(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	ctx := r.Context()
	rows, err := s.cfg.Store.Friends(ctx, u.ID)
	if err != nil {
		serverError(w, "friends", err)
		return
	}
	in, err := s.cfg.Store.IncomingRequests(ctx, u.ID)
	if err != nil {
		serverError(w, "incoming requests", err)
		return
	}
	out, err := s.cfg.Store.OutgoingRequests(ctx, u.ID)
	if err != nil {
		serverError(w, "outgoing requests", err)
		return
	}
	unread, err := s.cfg.Store.UnreadByFriend(ctx, u.ID)
	if err != nil {
		serverError(w, "unread", err)
		return
	}
	var ids []string
	for _, f := range rows {
		ids = append(ids, f.FriendID)
	}
	for _, q := range in {
		ids = append(ids, q.UserID)
	}
	for _, q := range out {
		ids = append(ids, q.UserID)
	}
	people, err := s.people(ctx, u.ID, ids)
	if err != nil {
		serverError(w, "people", err)
		return
	}
	friends := make([]FriendJSON, 0, len(rows))
	for _, f := range rows {
		if p, ok := people[f.FriendID]; ok {
			friends = append(friends, FriendJSON{Person: p, Muted: f.MuteChallenges, Unread: unread[f.FriendID], Since: f.Since.UnixMilli()})
		}
	}
	// Those online first, then by name.
	sort.SliceStable(friends, func(i, j int) bool {
		if friends[i].Online != friends[j].Online {
			return friends[i].Online
		}
		return strings.ToLower(friends[i].Name) < strings.ToLower(friends[j].Name)
	})
	list := func(rs []store.RequestRow) []RequestJSON {
		res := make([]RequestJSON, 0, len(rs))
		for _, q := range rs {
			if p, ok := people[q.UserID]; ok {
				res = append(res, RequestJSON{Person: p, At: q.At.UnixMilli()})
			}
		}
		return res
	}
	writeJSON(w, http.StatusOK, map[string]any{"friends": friends, "incoming": list(in), "outgoing": list(out)})
}

// relation tells how the person stands to another, for the buttons on a profile.
func (s *Service) relation(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	id := r.PathValue("id")
	if id == u.ID {
		writeJSON(w, http.StatusOK, map[string]any{"relation": "self"})
		return
	}
	ctx := r.Context()
	// Only the one who blocked is told; to the blocked person the other is just not a friend.
	if st, err := s.cfg.Store.BlockBetween(ctx, u.ID, id); err != nil {
		serverError(w, "relation block", err)
		return
	} else if st.ByMe {
		writeJSON(w, http.StatusOK, map[string]any{"relation": "blocked"})
		return
	}
	if f, err := s.cfg.Store.Friend(ctx, u.ID, id); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"relation": "friend", "muted": f.MuteChallenges, "online": s.online(id)})
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		serverError(w, "relation", err)
		return
	}
	in, err := s.cfg.Store.IncomingRequests(ctx, u.ID)
	if err != nil {
		serverError(w, "relation requests", err)
		return
	}
	for _, q := range in {
		if q.UserID == id {
			writeJSON(w, http.StatusOK, map[string]any{"relation": "incoming"})
			return
		}
	}
	out, err := s.cfg.Store.OutgoingRequests(ctx, u.ID)
	if err != nil {
		serverError(w, "relation requests", err)
		return
	}
	for _, q := range out {
		if q.UserID == id {
			writeJSON(w, http.StatusOK, map[string]any{"relation": "outgoing"})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"relation": "none"})
}

func (s *Service) requestFriend(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	var req struct {
		To string `json:"to"`
	}
	if !decode(w, r, &req) {
		return
	}
	other := s.existing(w, r, u, req.To)
	if other == nil {
		return
	}
	ctx := r.Context()
	if !s.gate(w, r, u.ID, other.ID, "не удалось отправить запрос") {
		return
	}
	if !s.allow("friend-req:"+u.ID, 20, time.Hour) {
		writeError(w, http.StatusTooManyRequests, "слишком много запросов в друзья, попробуйте позже")
		return
	}
	if n, err := s.cfg.Store.CountFriends(ctx, u.ID); err != nil {
		serverError(w, "count friends", err)
		return
	} else if n >= MaxFriends {
		writeError(w, http.StatusConflict, "друзей уже слишком много")
		return
	}
	if out, err := s.cfg.Store.OutgoingRequests(ctx, u.ID); err != nil {
		serverError(w, "count requests", err)
		return
	} else if len(out) >= MaxOutgoingRequests {
		writeError(w, http.StatusConflict, "слишком много неотвеченных запросов: отмените ненужные")
		return
	}
	accepted, err := s.cfg.Store.RequestFriend(ctx, u.ID, other.ID, s.cfg.Now())
	switch {
	case errors.Is(err, store.ErrAlreadyFriends):
		writeError(w, http.StatusConflict, "вы уже друзья")
	case errors.Is(err, store.ErrRequestExists):
		writeError(w, http.StatusConflict, "запрос уже отправлен")
	case err != nil:
		serverError(w, "request friend", err)
	case accepted:
		writeJSON(w, http.StatusOK, map[string]string{"status": "friends"})
	default:
		writeJSON(w, http.StatusCreated, map[string]string{"status": "requested"})
	}
}

func (s *Service) acceptFriend(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	from := r.PathValue("id")
	if st, err := s.cfg.Store.BlockBetween(r.Context(), u.ID, from); err != nil {
		serverError(w, "accept block", err)
		return
	} else if st.Any() { // a request cannot outlive a block; this is for a race
		writeError(w, http.StatusNotFound, "запроса нет: возможно, его уже отозвали")
		return
	}
	err := s.cfg.Store.AcceptFriend(r.Context(), u.ID, from, s.cfg.Now())
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "запроса нет: возможно, его уже отозвали")
	case err != nil:
		serverError(w, "accept friend", err)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "friends"})
	}
}

func (s *Service) declineFriend(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	if _, err := s.cfg.Store.DeleteRequest(r.Context(), r.PathValue("id"), u.ID); err != nil {
		serverError(w, "decline friend", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "none"})
}

// removeFriend ends a friendship, or takes back the person's own request if that is what is there.
func (s *Service) removeFriend(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	id := r.PathValue("id")
	removed, err := s.cfg.Store.RemoveFriend(r.Context(), u.ID, id)
	if err == nil && !removed {
		_, err = s.cfg.Store.DeleteRequest(r.Context(), u.ID, id)
	}
	if err != nil {
		serverError(w, "remove friend", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "none"})
}

func (s *Service) muteFriend(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	var req struct {
		Muted bool `json:"muted"`
	}
	if !decode(w, r, &req) {
		return
	}
	err := s.cfg.Store.MuteFriend(r.Context(), u.ID, r.PathValue("id"), req.Muted)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "этого игрока нет среди ваших друзей")
	case err != nil:
		serverError(w, "mute friend", err)
	default:
		writeJSON(w, http.StatusOK, map[string]bool{"muted": req.Muted})
	}
}

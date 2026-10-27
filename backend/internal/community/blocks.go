package community

import (
	"net/http"
	"time"
)

// Blocking a player. A block works both ways for what it forbids: the blocked person cannot write to
// the one who blocked them, send them a friend request or challenge them; and the one who blocked
// cannot do these to the blocked person until they unblock. Making a block also ends the friendship
// and cancels the requests and open challenges between the two (store.Block).
//
// What the blocked person is told stays neutral ("не удалось отправить"): they must not learn
// more than the failure. The one who blocked sees the list and may take a block back.

// Limits of blocking (maxBlocks is a variable so that a test can lower it).
const (
	MaxBlocks      = 500
	blockPerMinute = 30
)

const (
	youBlockedMsg = "Вы заблокировали этого игрока: чтобы писать ему или вызывать его, сначала разблокируйте"
	sendFailedMsg = "не удалось отправить"
)

// gate refuses an action of me towards other when either of them has blocked the other: it writes the
// answer (403) and says false. failed is what the blocked side reads, a neutral phrase.
func (s *Service) gate(w http.ResponseWriter, r *http.Request, me, other, failed string) bool {
	st, err := s.cfg.Store.BlockBetween(r.Context(), me, other)
	if err != nil {
		serverError(w, "block check", err)
		return false
	}
	switch {
	case st.ByMe:
		writeError(w, http.StatusForbidden, youBlockedMsg)
		return false
	case st.ByThem:
		writeError(w, http.StatusForbidden, failed)
		return false
	}
	return true
}

// BlockedJSON is a line of the list of blocked people.
type BlockedJSON struct {
	Person
	At int64 `json:"at"`
}

func (s *Service) blocks(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	ctx := r.Context()
	rows, err := s.cfg.Store.Blocks(ctx, u.ID)
	if err != nil {
		serverError(w, "blocks", err)
		return
	}
	ids := make([]string, 0, len(rows))
	for _, b := range rows {
		ids = append(ids, b.BlockedID)
	}
	users, err := s.cfg.Store.PublicUsers(ctx, ids)
	if err != nil {
		serverError(w, "blocked people", err)
		return
	}
	items := make([]BlockedJSON, 0, len(rows))
	for _, b := range rows {
		if p, ok := users[b.BlockedID]; ok {
			c := s.person(p)
			c.Online = false // nobody sees whether a person they blocked is here, and the reverse
			items = append(items, BlockedJSON{Person: c, At: b.At.UnixMilli()})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "max": maxBlocks})
}

func (s *Service) block(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	other := s.existing(w, r, u, r.PathValue("id"))
	if other == nil {
		return
	}
	if !s.allow("block:"+u.ID, blockPerMinute, time.Minute) {
		writeError(w, http.StatusTooManyRequests, "слишком много действий, подождите минуту")
		return
	}
	ctx := r.Context()
	st, err := s.cfg.Store.BlockBetween(ctx, u.ID, other.ID)
	if err != nil {
		serverError(w, "block state", err)
		return
	}
	if st.ByMe { // already blocked: the same answer
		writeJSON(w, http.StatusOK, map[string]string{"status": "blocked"})
		return
	}
	if n, err := s.cfg.Store.CountBlocks(ctx, u.ID); err != nil {
		serverError(w, "count blocks", err)
		return
	} else if n >= maxBlocks {
		writeError(w, http.StatusConflict, "заблокировано слишком много игроков: освободите место в списке")
		return
	}
	if _, err := s.cfg.Store.Block(ctx, u.ID, other.ID, s.cfg.Now()); err != nil {
		serverError(w, "block", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "blocked"})
}

func (s *Service) unblock(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	id := r.PathValue("id")
	if id == "" || len(id) > 32 {
		writeError(w, http.StatusNotFound, errGone.Error())
		return
	}
	if !s.allow("block:"+u.ID, blockPerMinute, time.Minute) {
		writeError(w, http.StatusTooManyRequests, "слишком много действий, подождите минуту")
		return
	}
	if _, err := s.cfg.Store.Unblock(r.Context(), u.ID, id); err != nil {
		serverError(w, "unblock", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "none"})
}

var maxBlocks = MaxBlocks

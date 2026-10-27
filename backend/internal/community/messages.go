package community

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"dyanis-chess/backend/internal/store"
)

// MessageJSON is a message in a talk.
type MessageJSON struct {
	ID   uint64 `json:"id"`
	Mine bool   `json:"mine"`
	Text string `json:"text"`
	At   int64  `json:"at"`
	Read bool   `json:"read,omitempty"` // for mine: the friend has opened it
}

// cleanText trims a message, drops control characters (a line break stays) and cuts it to the limit.
func cleanText(s string) string {
	var b strings.Builder
	n, nl := 0, 0
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r == '\n' || r == '\r':
			if nl++; nl > 2 { // no more than one empty line in a row
				continue
			}
			r = '\n'
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || r == 0x2028 || r == 0x2029:
			r, nl = ' ', 0
		default:
			nl = 0
		}
		b.WriteRune(r)
		if n++; n >= MaxMessageRunes {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

// friendOnly returns the friend, or writes the answer: only friends talk.
func (s *Service) friendOnly(w http.ResponseWriter, r *http.Request, me *store.User) *store.PublicUser {
	id := r.PathValue("id")
	other := s.existing(w, r, me, id)
	if other == nil {
		return nil
	}
	if _, err := s.cfg.Store.Friend(r.Context(), me.ID, id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusForbidden, "писать можно только друзьям")
		return nil
	} else if err != nil {
		serverError(w, "friend check", err)
		return nil
	}
	return other
}

// messages returns the talk with a friend (the last 100 messages, or those after ?after=<id>) and
// marks what the friend wrote as read.
func (s *Service) messages(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	other := s.friendOnly(w, r, u)
	if other == nil {
		return
	}
	var after uint64
	if v := r.URL.Query().Get("after"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "неверный after")
			return
		}
		after = n
	}
	ctx := r.Context()
	rows, err := s.cfg.Store.Conversation(ctx, u.ID, other.ID, after, 100)
	if err != nil {
		serverError(w, "conversation", err)
		return
	}
	if err := s.cfg.Store.MarkRead(ctx, u.ID, other.ID, s.cfg.Now()); err != nil {
		serverError(w, "mark read", err)
		return
	}
	out := make([]MessageJSON, 0, len(rows))
	for _, m := range rows {
		out = append(out, MessageJSON{ID: m.ID, Mine: m.FromID == u.ID, Text: m.Body, At: m.At.UnixMilli(), Read: m.Read})
	}
	writeJSON(w, http.StatusOK, map[string]any{"friend": s.person(other), "messages": out})
}

func (s *Service) sendMessage(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !s.gate(w, r, u.ID, r.PathValue("id"), sendFailedMsg) {
		return
	}
	other := s.friendOnly(w, r, u)
	if other == nil {
		return
	}
	text := cleanText(req.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, "пустое сообщение")
		return
	}
	if !s.allow("msg:"+u.ID, 20, time.Minute) {
		writeError(w, http.StatusTooManyRequests, "не так быстро")
		return
	}
	at := s.cfg.Now()
	id, err := s.cfg.Store.AddMessage(r.Context(), u.ID, other.ID, text, at)
	if err != nil {
		serverError(w, "add message", err)
		return
	}
	writeJSON(w, http.StatusCreated, MessageJSON{ID: id, Mine: true, Text: text, At: at.UnixMilli()})
}

// ConversationJSON is a line of the list of talks.
type ConversationJSON struct {
	Person
	Last   string `json:"last"`
	LastAt int64  `json:"lastAt"`
	Mine   bool   `json:"mine"` // the last message is the person's own
	Unread int    `json:"unread,omitempty"`
}

// conversations lists the talks the person has had, the latest first. A friend removed since is still
// there with the talk, but cannot be written to.
func (s *Service) conversations(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	ctx := r.Context()
	last, err := s.cfg.Store.LastMessages(ctx, u.ID)
	if err != nil {
		serverError(w, "last messages", err)
		return
	}
	unread, err := s.cfg.Store.UnreadByFriend(ctx, u.ID)
	if err != nil {
		serverError(w, "unread", err)
		return
	}
	ids := make([]string, 0, len(last))
	for id := range last {
		ids = append(ids, id)
	}
	people, err := s.people(ctx, u.ID, ids)
	if err != nil {
		serverError(w, "people", err)
		return
	}
	items := make([]ConversationJSON, 0, len(last))
	for id, m := range last {
		p, ok := people[id]
		if !ok {
			continue
		}
		items = append(items, ConversationJSON{Person: p, Last: previewOf(m.Body), LastAt: m.At.UnixMilli(), Mine: m.FromID == u.ID, Unread: unread[id]})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].LastAt > items[j].LastAt })
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func previewOf(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return s
}

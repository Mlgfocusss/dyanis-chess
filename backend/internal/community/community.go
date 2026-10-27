// Package community is the social side of the site: friends, private messages between friends,
// challenges to a game (with the rules for who may send them) and the correspondence games and
// vacations that go with them. Everything needs a signed-in person; a guest counts.
//
// Nothing here is pushed: the pages ask. The menu asks GET /api/community/summary every few seconds
// (that call is also what tells the others the person is online), the open talk asks for new
// messages, and so on, like the matchmaking queue does.
package community

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"dyanis-chess/backend/internal/play"
	"dyanis-chess/backend/internal/ratings"
	"dyanis-chess/backend/internal/store"
)

// Store is the part of *store.Store the service uses.
type Store interface {
	UserByID(ctx context.Context, id string) (*store.User, error)
	PublicUser(ctx context.Context, id string) (*store.PublicUser, error)
	PublicUsers(ctx context.Context, ids []string) (map[string]*store.PublicUser, error)
	SearchUsers(ctx context.Context, q, exclude string, limit int) ([]*store.PublicUser, error)

	Friends(ctx context.Context, userID string) ([]store.FriendRow, error)
	Friend(ctx context.Context, userID, friendID string) (*store.FriendRow, error)
	CountFriends(ctx context.Context, userID string) (int, error)
	IncomingRequests(ctx context.Context, userID string) ([]store.RequestRow, error)
	OutgoingRequests(ctx context.Context, userID string) ([]store.RequestRow, error)
	RequestFriend(ctx context.Context, from, to string, at time.Time) (bool, error)
	AcceptFriend(ctx context.Context, userID, from string, at time.Time) error
	DeleteRequest(ctx context.Context, from, to string) (bool, error)
	RemoveFriend(ctx context.Context, a, b string) (bool, error)
	MuteFriend(ctx context.Context, userID, friendID string, muted bool) error

	AddMessage(ctx context.Context, from, to, body string, at time.Time) (uint64, error)
	Conversation(ctx context.Context, a, b string, after uint64, limit int) ([]store.MessageRow, error)
	MarkRead(ctx context.Context, me, other string, at time.Time) error
	UnreadByFriend(ctx context.Context, me string) (map[string]int, error)
	LastMessages(ctx context.Context, me string) (map[string]store.MessageRow, error)
	CountUnread(ctx context.Context, me string) (int, error)

	CreateChallenge(ctx context.Context, c *store.ChallengeRow) error
	Challenge(ctx context.Context, id string) (*store.ChallengeRow, error)
	SetChallengeStatus(ctx context.Context, id, status, gameID string) (bool, error)
	SetChallengeGame(ctx context.Context, id, gameID string) error
	ChallengesFor(ctx context.Context, userID string, incoming bool, now time.Time, answeredFor time.Duration) ([]*store.ChallengeRow, error)
	CountIncomingChallenges(ctx context.Context, userID string, now time.Time) (int, error)
	CountPendingChallenges(ctx context.Context, from, to string, now time.Time) (int, error)
	CleanChallenges(ctx context.Context, now time.Time) error

	Vacation(ctx context.Context, userID string) (store.VacationRow, error)
	SetVacation(ctx context.Context, userID string, v store.VacationRow) error
	CountMyTurn(ctx context.Context, userID string) (int, error)

	Block(ctx context.Context, blocker, blocked string, at time.Time) (bool, error)
	Unblock(ctx context.Context, blocker, blocked string) (bool, error)
	Blocks(ctx context.Context, blocker string) ([]store.BlockRow, error)
	CountBlocks(ctx context.Context, blocker string) (int, error)
	BlockBetween(ctx context.Context, me, other string) (store.BlockState, error)
	BlockerIDs(ctx context.Context, blockedID string) (map[string]bool, error)
}

// Config of a Service.
type Config struct {
	Store Store
	// User returns the signed-in user of a request, or nil.
	User func(r *http.Request) *store.User
	// Start makes the game of an accepted challenge and returns its id (play.Hub.NewChallenged).
	Start func(ctx context.Context, white, black play.Player, tc play.TimeControl, rated bool) (string, error)
	// Rating is the person's rating in a category (ratings.Service.Get); nil: everybody is 1200.
	Rating func(ctx context.Context, userID, category string) (ratings.Entry, error)
	// Correspondence lists a person's unfinished correspondence games (play.Hub.Correspondence).
	Correspondence func(ctx context.Context, userID string) ([]play.CorrGame, error)
	// Now is the clock; time.Now unless a test says otherwise.
	Now func() time.Time
}

// Limits.
const (
	MaxFriends          = 300
	MaxOutgoingRequests = 50
	MaxPendingChallenge = 10
	MaxMessageRunes     = 1000
	onlineFor           = 90 * time.Second // seen this recently means online
	prefsTTL            = 30 * time.Second
	listLimit           = 60
)

// Service serves the community API.
type Service struct {
	cfg Config

	mu    sync.Mutex
	seen  map[string]time.Time // last request of each person: who is online
	prefs map[string]cachedPrefs
	hits  map[string][]time.Time // for the rate limits
}

type cachedPrefs struct {
	at time.Time
	p  play.CorrPrefs
}

// New returns a Service.
func New(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{cfg: cfg, seen: map[string]time.Time{}, prefs: map[string]cachedPrefs{}, hits: map[string][]time.Time{}}
}

// Handler returns the routes of the package.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/community/summary", s.summary)
	mux.HandleFunc("GET /api/community/search", s.search)
	mux.HandleFunc("GET /api/community/friends", s.friends)
	mux.HandleFunc("POST /api/community/friends", s.requestFriend)
	mux.HandleFunc("GET /api/community/friends/{id}", s.relation)
	mux.HandleFunc("POST /api/community/friends/{id}/accept", s.acceptFriend)
	mux.HandleFunc("POST /api/community/friends/{id}/decline", s.declineFriend)
	mux.HandleFunc("PUT /api/community/friends/{id}/mute", s.muteFriend)
	mux.HandleFunc("DELETE /api/community/friends/{id}", s.removeFriend)
	mux.HandleFunc("GET /api/community/conversations", s.conversations)
	mux.HandleFunc("GET /api/community/messages/{id}", s.messages)
	mux.HandleFunc("POST /api/community/messages/{id}", s.sendMessage)
	mux.HandleFunc("GET /api/community/challenges", s.challenges)
	mux.HandleFunc("POST /api/community/challenges", s.challenge)
	mux.HandleFunc("POST /api/community/challenges/{id}/accept", s.acceptChallenge)
	mux.HandleFunc("POST /api/community/challenges/{id}/decline", s.declineChallenge)
	mux.HandleFunc("DELETE /api/community/challenges/{id}", s.cancelChallenge)
	mux.HandleFunc("GET /api/community/correspondence", s.correspondence)
	mux.HandleFunc("PUT /api/community/vacation", s.setVacation)
	mux.HandleFunc("GET /api/community/blocks", s.blocks)
	mux.HandleFunc("PUT /api/community/blocks/{id}", s.block)
	mux.HandleFunc("DELETE /api/community/blocks/{id}", s.unblock)
	return mux
}

// ------------------------------------------------------------------ helpers

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func serverError(w http.ResponseWriter, what string, err error) {
	log.Printf("community: %s: %v", what, err)
	writeError(w, http.StatusInternalServerError, "не удалось выполнить запрос")
}

// me returns the person making the request and notes that they are here; nil (with the answer
// already written) when nobody is signed in.
func (s *Service) me(w http.ResponseWriter, r *http.Request) *store.User {
	var u *store.User
	if s.cfg.User != nil {
		u = s.cfg.User(r)
	}
	if u == nil {
		writeError(w, http.StatusUnauthorized, "чтобы пользоваться сообществом, войдите или продолжите как гость")
		return nil
	}
	s.mu.Lock()
	s.seen[u.ID] = s.cfg.Now()
	s.mu.Unlock()
	return u
}

// decode reads a small JSON body; only application/json is taken (a form from another site is not).
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "ожидался JSON")
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "не удалось прочитать запрос")
		return false
	}
	return true
}

func (s *Service) online(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.seen[id]
	return ok && s.cfg.Now().Sub(t) < onlineFor
}

// allow counts one action of key and says whether it is within n per window.
func (s *Service) allow(key string, n int, window time.Duration) bool {
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.hits[key][:0]
	for _, t := range s.hits[key] {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= n {
		s.hits[key] = kept
		return false
	}
	s.hits[key] = append(kept, now)
	return true
}

// Person is how a person is shown in the lists.
type Person struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	Country   string `json:"country,omitempty"`
	Kind      string `json:"kind"`
	Online    bool   `json:"online,omitempty"`
}

func (s *Service) person(u *store.PublicUser) Person {
	return Person{ID: u.ID, Name: u.Name, AvatarURL: u.AvatarURL, Country: u.Country, Kind: u.Kind, Online: s.online(u.ID)}
}

// people looks up several people at once as the viewer sees them; the ones whose account is gone are
// left out. A person who has blocked the viewer is never shown as being on the site.
func (s *Service) people(ctx context.Context, viewer string, ids []string) (map[string]Person, error) {
	users, err := s.cfg.Store.PublicUsers(ctx, ids)
	if err != nil {
		return nil, err
	}
	blockers, err := s.cfg.Store.BlockerIDs(ctx, viewer)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Person, len(users))
	for id, u := range users {
		p := s.person(u)
		if blockers[id] {
			p.Online = false
		}
		out[id] = p
	}
	return out, nil
}

// ----------------------------------------------------------------- settings

// Prefs are the settings of a person that other people's actions run into. They are read from the
// settings document the settings page saves (internal/auth keeps it as it is).
type Prefs struct {
	ChallengesFrom string // "all", "friends" or "none"
	Min, Max       int    // the rating of a challenger must lie in [Min, Max]; 0 is no limit
	FriendsIgnore  bool   // ... unless the challenger is a friend
	Offline        bool   // challenges are taken while the person is away
	AutoClaim      bool   // a win on time in a correspondence game is given at once
}

// ParsePrefs reads the keys it knows from a settings document; whatever is missing or wrong is the default.
func ParsePrefs(settings []byte) Prefs {
	p := Prefs{ChallengesFrom: "friends", FriendsIgnore: true, Offline: true, AutoClaim: true}
	if len(settings) == 0 {
		return p
	}
	var raw struct {
		From          *string `json:"challengesFrom"`
		Min           *int    `json:"challengeMin"`
		Max           *int    `json:"challengeMax"`
		FriendsIgnore *bool   `json:"challengeFriendsIgnoreRating"`
		Offline       *bool   `json:"challengeOffline"`
		AutoClaim     *bool   `json:"corrAutoClaim"`
	}
	if json.Unmarshal(settings, &raw) != nil {
		return p
	}
	if raw.From != nil && (*raw.From == "all" || *raw.From == "friends" || *raw.From == "none") {
		p.ChallengesFrom = *raw.From
	}
	if raw.Min != nil && *raw.Min > 0 && *raw.Min <= 3500 {
		p.Min = *raw.Min
	}
	if raw.Max != nil && *raw.Max > 0 && *raw.Max <= 3500 {
		p.Max = *raw.Max
	}
	if raw.FriendsIgnore != nil {
		p.FriendsIgnore = *raw.FriendsIgnore
	}
	if raw.Offline != nil {
		p.Offline = *raw.Offline
	}
	if raw.AutoClaim != nil {
		p.AutoClaim = *raw.AutoClaim
	}
	return p
}

// PlayPrefs is what play.Config.Prefs asks for: the settings that decide the deadlines of a
// correspondence game. Answers are kept for a few seconds, because a game asks on every move.
func (s *Service) PlayPrefs(userID string) play.CorrPrefs {
	now := s.cfg.Now()
	s.mu.Lock()
	c, ok := s.prefs[userID]
	s.mu.Unlock()
	if ok && now.Sub(c.at) < prefsTTL {
		return c.p
	}
	out := play.CorrPrefs{AutoClaim: true}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if u, err := s.cfg.Store.UserByID(ctx, userID); err == nil {
		out.AutoClaim = ParsePrefs(u.Settings).AutoClaim
	}
	if v, err := s.cfg.Store.Vacation(ctx, userID); err == nil && v.Ends.After(now) {
		out.Vacation = v.Ends
	}
	s.mu.Lock()
	s.prefs[userID] = cachedPrefs{at: now, p: out}
	s.mu.Unlock()
	return out
}

func (s *Service) forgetPrefs(userID string) {
	s.mu.Lock()
	delete(s.prefs, userID)
	s.mu.Unlock()
}

// ------------------------------------------------------------------ summary

// Summary is what the menu shows: the numbers behind the badges.
type Summary struct {
	FriendRequests int `json:"friendRequests"`
	Messages       int `json:"messages"`
	Challenges     int `json:"challenges"`
	YourTurn       int `json:"yourTurn"`
	// Accepted are the challenges of this person that were taken a moment ago: the games are ready.
	Accepted []Accepted `json:"accepted,omitempty"`
	// Vacation is when the vacation ends (unix ms), 0 for none.
	Vacation int64 `json:"vacation,omitempty"`
}

// Accepted is a challenge someone has accepted.
type Accepted struct {
	ID     string `json:"id"`
	GameID string `json:"gameId"`
}

func (s *Service) summary(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	ctx, now := r.Context(), s.cfg.Now()
	var out Summary
	reqs, err := s.cfg.Store.IncomingRequests(ctx, u.ID)
	if err != nil {
		serverError(w, "summary requests", err)
		return
	}
	out.FriendRequests = len(reqs)
	if out.Messages, err = s.cfg.Store.CountUnread(ctx, u.ID); err != nil {
		serverError(w, "summary messages", err)
		return
	}
	if out.Challenges, err = s.cfg.Store.CountIncomingChallenges(ctx, u.ID, now); err != nil {
		serverError(w, "summary challenges", err)
		return
	}
	if out.YourTurn, err = s.cfg.Store.CountMyTurn(ctx, u.ID); err != nil {
		serverError(w, "summary turns", err)
		return
	}
	mine, err := s.cfg.Store.ChallengesFor(ctx, u.ID, false, now, 2*time.Minute)
	if err != nil {
		serverError(w, "summary accepted", err)
		return
	}
	for _, c := range mine {
		if c.Status == store.ChallengeAccepted && c.GameID != "" {
			out.Accepted = append(out.Accepted, Accepted{ID: c.ID, GameID: c.GameID})
		}
	}
	if v, err := s.cfg.Store.Vacation(ctx, u.ID); err == nil && v.Ends.After(now) {
		out.Vacation = v.Ends.UnixMilli()
	}
	writeJSON(w, http.StatusOK, out)
}

// ------------------------------------------------------------------- search

func (s *Service) search(w http.ResponseWriter, r *http.Request) {
	u := s.me(w, r)
	if u == nil {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 {
		writeJSON(w, http.StatusOK, map[string]any{"items": []Person{}})
		return
	}
	if len([]rune(q)) > 40 {
		q = string([]rune(q)[:40])
	}
	if !s.allow("search:"+u.ID, 30, time.Minute) {
		writeError(w, http.StatusTooManyRequests, "слишком много поисков, подождите минуту")
		return
	}
	found, err := s.cfg.Store.SearchUsers(r.Context(), q, u.ID, 20)
	if err != nil {
		serverError(w, "search", err)
		return
	}
	items := make([]Person, 0, len(found))
	for _, f := range found {
		p := s.person(f)
		p.Online = false // who is on the site is shown to friends only
		items = append(items, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

var errGone = errors.New("игрок не найден")

// existing returns the public record of a person who is somebody else, or writes the answer.
func (s *Service) existing(w http.ResponseWriter, r *http.Request, me *store.User, id string) *store.PublicUser {
	if id == "" || len(id) > 32 {
		writeError(w, http.StatusNotFound, errGone.Error())
		return nil
	}
	if id == me.ID {
		writeError(w, http.StatusBadRequest, "это вы сами")
		return nil
	}
	p, err := s.cfg.Store.PublicUser(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, errGone.Error())
		return nil
	}
	if err != nil {
		serverError(w, "user", err)
		return nil
	}
	return p
}

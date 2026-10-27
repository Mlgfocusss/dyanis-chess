// Package match brings people together: a person asks for a game with a clock (rated or casual)
// or for a puzzle duel, waits in a queue, and is paired with somebody who asked for the same and
// has a close rating. The window of acceptable difference widens with the time spent waiting, so
// nobody waits forever and a newcomer is not thrown at a master at once.
package match

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"

	"dyanis-chess/backend/internal/play"
	"dyanis-chess/backend/internal/ratings"
	"dyanis-chess/backend/internal/store"
)

// Kinds of games to ask for.
const (
	KindChess = "chess"
	KindDuel  = "duel"
)

// Seeker is a person in the queue.
type Seeker struct {
	ID, Name, Avatar string
	Rating           int
	Provisional      bool
}

// Request is what a person asks for.
type Request struct {
	Kind   string `json:"kind"`
	BaseMs int64  `json:"baseMs,omitempty"`
	IncMs  int64  `json:"incMs,omitempty"`
	Rated  bool   `json:"rated,omitempty"`
	// RangeDown and RangeUp limit how much weaker and how much stronger (rating points) the opponent
	// may be; 0 means no limit. The opponent's own limits apply too.
	RangeDown int `json:"rangeDown,omitempty"`
	RangeUp   int `json:"rangeUp,omitempty"`
}

// Maker creates the game of two matched people and returns its id.
type Maker func(ctx context.Context, a, b Seeker, req Request) (string, error)

// Config of a Service.
type Config struct {
	// User is the signed-in user of a request, or nil.
	User func(r *http.Request) *store.User
	// Rating is the person's rating in a category.
	Rating func(ctx context.Context, userID, category string) (ratings.Entry, error)
	// Chess and Duel make the games.
	Chess, Duel Maker
	// Avoid returns the people the person has a block with (either way); they are never paired with
	// them. Read when the person joins the queue. nil: nobody is avoided.
	Avoid func(ctx context.Context, userID string) (map[string]bool, error)
	Now   func() time.Time
}

// Timing.
const (
	baseWindow = 100.0            // rating points of difference allowed at once
	windowPerS = 25.0             // how much the window grows each second of waiting
	maxWindow  = 1000.0           // beyond this anybody may play anybody
	idleDrop   = 20 * time.Second // a seek nobody has asked about for this long is dropped (the person left)
	foundKeep  = 2 * time.Minute  // a pairing that was not collected is forgotten
	readKeep   = 10 * time.Second // a pairing that was collected is repeated to a retry for this long
	tickEvery  = time.Second
)

type seek struct {
	who      Seeker
	req      Request
	key      string
	category string
	joined   time.Time
	lastSeen time.Time
	avoid    map[string]bool // people this one has a block with: never paired
	found    string          // the id of the game made for the pair, once paired
	foundAt  time.Time
	readAt   time.Time // when the person first learned of it
}

// Service is the queue and its HTTP interface.
type Service struct {
	cfg  Config
	mu   sync.Mutex
	list map[string]*seek // by user id
	quit chan struct{}
	once sync.Once
	wg   sync.WaitGroup
}

// New starts the matching loop; Close stops it.
func New(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	s := &Service{cfg: cfg, list: map[string]*seek{}, quit: make(chan struct{})}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		t := time.NewTicker(tickEvery)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				s.Pair()
			case <-s.quit:
				return
			}
		}
	}()
	return s
}

// Close stops the matching loop.
func (s *Service) Close() {
	s.once.Do(func() { close(s.quit) })
	s.wg.Wait()
}

// keyOf says which seeks may be paired: the same kind, clock and rated flag.
func keyOf(r Request) string {
	if r.Kind == KindDuel {
		return "duel"
	}
	rated := "c"
	if r.Rated {
		rated = "r"
	}
	return fmt.Sprintf("chess:%d+%d:%s", r.BaseMs, r.IncMs, rated)
}

// category is the rating category of a request.
func categoryOf(r Request) string {
	if r.Kind == KindDuel {
		return ratings.Duel
	}
	return ratings.CategoryFor(r.BaseMs, r.IncMs)
}

// maxRange is the widest limit a person may set.
const maxRange = 3000

// validate reports what is wrong with a request, for the person.
func validate(r Request) error {
	if r.RangeDown < 0 || r.RangeUp < 0 || r.RangeDown > maxRange || r.RangeUp > maxRange {
		return errors.New("границы рейтинга: от 0 до 3000")
	}
	switch r.Kind {
	case KindDuel:
		return nil
	case KindChess:
		if err := (play.TimeControl{BaseMs: r.BaseMs, IncMs: r.IncMs}).Validate(); err != nil {
			return err
		}
		if r.BaseMs == 0 {
			return errors.New("для подбора нужны часы")
		}
		return nil
	}
	return errors.New("неизвестный вид игры")
}

// within says whether b is in the limits a has set for the opponent's rating.
func within(a, b *seek) bool {
	d := b.who.Rating - a.who.Rating
	if d < 0 && a.req.RangeDown > 0 && -d > a.req.RangeDown {
		return false
	}
	return !(d > 0 && a.req.RangeUp > 0 && d > a.req.RangeUp)
}

// window is how far apart in rating two people may be after waiting this long.
func window(waited time.Duration) float64 {
	return math.Min(maxWindow, baseWindow+windowPerS*waited.Seconds())
}

// Join puts a person in the queue, replacing a seek of theirs that is still there.
func (s *Service) Join(ctx context.Context, u Seeker, req Request) error {
	if err := validate(req); err != nil {
		return err
	}
	var avoid map[string]bool
	if s.cfg.Avoid != nil {
		var err error
		if avoid, err = s.cfg.Avoid(ctx, u.ID); err != nil {
			log.Printf("match: blocks of %s: %v", u.ID, err) // better to play than to wait for ever
			avoid = nil
		}
	}
	now := s.cfg.Now()
	s.mu.Lock()
	s.list[u.ID] = &seek{who: u, req: req, key: keyOf(req), category: categoryOf(req), joined: now, lastSeen: now, avoid: avoid}
	s.mu.Unlock()
	s.Pair()
	return nil
}

// Leave takes the person out of the queue. If they had already been paired the game is returned
// (it exists: the opponent is waiting in it).
func (s *Service) Leave(userID string) (foundID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if k := s.list[userID]; k != nil {
		foundID = k.found
		delete(s.list, userID)
	}
	return foundID
}

// Status is the state of a person's seek.
type Status struct {
	State  string  `json:"status"` // "none", "waiting" or "found"
	Kind   string  `json:"kind,omitempty"`
	ID     string  `json:"id,omitempty"`     // the game or the duel, once found
	Waited int     `json:"waited,omitempty"` // seconds
	Window float64 `json:"window,omitempty"` // rating difference allowed now
	Count  int     `json:"count,omitempty"`  // people in the queue for the same game
}

// Check says how a person's seek stands, and shows the server the person is still there.
func (s *Service) Check(userID string) Status {
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	k := s.list[userID]
	if k == nil {
		return Status{State: "none"}
	}
	k.lastSeen = now
	if k.found != "" {
		if k.readAt.IsZero() {
			k.readAt = now
		}
		return Status{State: "found", Kind: k.req.Kind, ID: k.found}
	}
	n := 0
	for _, o := range s.list {
		if o.found == "" && o.key == k.key {
			n++
		}
	}
	w := now.Sub(k.joined)
	return Status{State: "waiting", Kind: k.req.Kind, Waited: int(w.Seconds()), Window: window(w), Count: n}
}

// Pair pairs the people who can be and forgets the abandoned. It runs every second and whenever somebody joins.
func (s *Service) Pair() {
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, k := range s.list {
		switch {
		case k.found == "" && now.Sub(k.lastSeen) > idleDrop:
			delete(s.list, id)
		case k.found != "" && now.Sub(k.foundAt) > foundKeep:
			delete(s.list, id)
		case !k.readAt.IsZero() && now.Sub(k.readAt) > readKeep:
			delete(s.list, id) // told, and the person has gone to the game
		}
	}
	// the longest waiting first: they have the widest window
	var waiting []*seek
	for _, k := range s.list {
		if k.found == "" {
			waiting = append(waiting, k)
		}
	}
	sort.Slice(waiting, func(i, j int) bool { return waiting[i].joined.Before(waiting[j].joined) })
	for _, a := range waiting {
		if a.found != "" {
			continue
		}
		var best *seek
		bestDiff := math.Inf(1)
		for _, b := range waiting {
			if b == a || b.found != "" || b.key != a.key || a.avoid[b.who.ID] || b.avoid[a.who.ID] {
				continue
			}
			diff := math.Abs(float64(a.who.Rating - b.who.Rating))
			if diff <= window(now.Sub(a.joined)) && diff <= window(now.Sub(b.joined)) && within(a, b) && within(b, a) && diff < bestDiff {
				best, bestDiff = b, diff
			}
		}
		if best == nil {
			continue
		}
		maker := s.cfg.Chess
		if a.req.Kind == KindDuel {
			maker = s.cfg.Duel
		}
		if maker == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		id, err := maker(ctx, a.who, best.who, a.req)
		cancel()
		if err != nil {
			log.Printf("match: making a game for %s and %s: %v", a.who.ID, best.who.ID, err)
			delete(s.list, a.who.ID)
			delete(s.list, best.who.ID)
			continue
		}
		a.found, a.foundAt = id, now
		best.found, best.foundAt = id, now
	}
}

// Handler serves /api/match/seek.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/match/seek", s.post)
	mux.HandleFunc("GET /api/match/seek", s.get)
	mux.HandleFunc("DELETE /api/match/seek", s.del)
	return mux
}

func (s *Service) user(w http.ResponseWriter, r *http.Request) *store.User {
	u := s.cfg.User(r)
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "чтобы играть, войдите или продолжите как гость"})
	}
	return u
}

func (s *Service) post(w http.ResponseWriter, r *http.Request) {
	u := s.user(w, r)
	if u == nil {
		return
	}
	var req Request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный запрос"})
		return
	}
	if err := validate(req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	e, err := s.cfg.Rating(r.Context(), u.ID, categoryOf(req))
	if err != nil {
		log.Printf("match: rating of %s: %v", u.ID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "не удалось прочитать рейтинг"})
		return
	}
	who := Seeker{ID: u.ID, Name: u.Name, Avatar: u.AvatarURL, Rating: e.Rating, Provisional: e.Provisional}
	if err := s.Join(r.Context(), who, req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, s.Check(u.ID))
}

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	if u := s.user(w, r); u != nil {
		writeJSON(w, http.StatusOK, s.Check(u.ID))
	}
}

func (s *Service) del(w http.ResponseWriter, r *http.Request) {
	u := s.user(w, r)
	if u == nil {
		return
	}
	if id := s.Leave(u.ID); id != "" { // too late to cancel: the game exists
		writeJSON(w, http.StatusOK, Status{State: "found", ID: id})
		return
	}
	writeJSON(w, http.StatusOK, Status{State: "none"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

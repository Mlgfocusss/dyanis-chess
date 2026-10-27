package achievements

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"dyanis-chess/backend/internal/stats"
	"dyanis-chess/backend/internal/store"
)

// Store is the part of *store.Store the service uses.
type Store interface {
	UserByID(ctx context.Context, id string) (*store.User, error)
	AchievementFacts(ctx context.Context, userID string, q store.AchievementQuery) (store.AchievementFacts, error)
	AccuracyReviews(ctx context.Context, userID string, min float64, minPlies, limit int) ([]store.ReviewBrief, error)
	BrilliantReviews(ctx context.Context, userID string, limit int) ([]store.BrilliantReview, error)
	PuzzleDays(ctx context.Context, userID string) (map[string]int, error)
	Achievements(ctx context.Context, userID string) ([]store.AchievementRow, error)
	AddAchievements(ctx context.Context, userID string, keys []string, at time.Time) ([]string, error)
	MarkAchievementsSeen(ctx context.Context, userID string, keys []string) error
}

// Config tunes a Service.
type Config struct {
	Store Store
	// UserID names the signed-in user of a request, "" for nobody.
	UserID func(*http.Request) string
	// Sections are the lesson ids of each section of the course (from the lessons library).
	Sections [][]string
	// Now is the clock.
	Now func() time.Time
}

// Service computes and remembers the badges.
type Service struct {
	cfg Config

	mu    sync.Mutex
	cache map[string]*entry
}

// entry is what is remembered about one person: the last evaluation and, so that the costly searches of
// the reviews are not repeated for nothing, how many reviews there were when each search found nothing.
type entry struct {
	mu          sync.Mutex // one evaluation of the person at a time
	at          time.Time
	progress    []Progress
	accScanned  int // reviews at the last search for a high accuracy that found none; -1: never
	brilScanned int
}

// New returns a Service.
func New(cfg Config) *Service {
	if cfg.UserID == nil {
		cfg.UserID = func(*http.Request) string { return "" }
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{cfg: cfg, cache: map[string]*entry{}}
}

// Handler returns the routes of the achievements API.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/achievements", s.mine)
	mux.HandleFunc("POST /api/achievements/seen", s.seen)
	mux.HandleFunc("GET /api/users/{id}/achievements", s.public)
	return mux
}

// ---------------------------------------------------------------- answers

// Item is one badge in the answer for the person themselves.
type Item struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Tier     string `json:"tier"`
	// Target and Progress are the numbers of the progress bar; Target is 1 for a yes-or-no badge.
	Target   int  `json:"target"`
	Progress int  `json:"progress"`
	Unlocked bool `json:"unlocked"`
	// UnlockedAt is when the badge was first noticed as earned.
	UnlockedAt *time.Time `json:"unlockedAt,omitempty"`
	// Seen is false until the browser has shown the notice of a new badge.
	Seen bool `json:"seen"`
}

// Mine is the body of GET /api/achievements.
type Mine struct {
	Items    []Item   `json:"items"`
	Unlocked int      `json:"unlocked"`
	Total    int      `json:"total"`
	Unseen   []string `json:"unseen"` // ids of the earned badges whose notice was not shown yet, the oldest first
}

// PublicItem is a badge in the answer about another person: only earned ones.
type PublicItem struct {
	ID         string    `json:"id"`
	Category   string    `json:"category"`
	Tier       string    `json:"tier"`
	UnlockedAt time.Time `json:"unlockedAt"`
}

// Public is the body of GET /api/users/{id}/achievements; Items go from the newest.
type Public struct {
	Items    []PublicItem `json:"items"`
	Unlocked int          `json:"unlocked"`
	Total    int          `json:"total"`
}

// ---------------------------------------------------------------- handlers

func (s *Service) mine(w http.ResponseWriter, r *http.Request) {
	uid := s.cfg.UserID(r)
	if uid == "" {
		writeError(w, http.StatusUnauthorized, "войдите, чтобы увидеть достижения")
		return
	}
	age := ProfileMaxAge
	if r.URL.Query().Get("poll") != "" {
		age = PollMaxAge
	}
	ans, err := s.Mine(r.Context(), uid, age)
	if err != nil {
		log.Printf("achievements: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось собрать достижения")
		return
	}
	writeJSON(w, http.StatusOK, ans)
}

func (s *Service) seen(w http.ResponseWriter, r *http.Request) {
	uid := s.cfg.UserID(r)
	if uid == "" {
		writeError(w, http.StatusUnauthorized, "войдите, чтобы отметить достижения")
		return
	}
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	if len(body.IDs) > len(Catalog) {
		writeError(w, http.StatusBadRequest, "слишком много достижений")
		return
	}
	for _, id := range body.IDs {
		if _, ok := Lookup(id); !ok {
			writeError(w, http.StatusBadRequest, "неизвестное достижение")
			return
		}
	}
	if err := s.cfg.Store.MarkAchievementsSeen(r.Context(), uid, body.IDs); err != nil {
		log.Printf("achievements: seen: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось отметить")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

const maxIDLen = 32

func (s *Service) public(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || len(id) > maxIDLen {
		writeError(w, http.StatusNotFound, "игрок не найден")
		return
	}
	ans, err := s.Public(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "игрок не найден")
		return
	}
	if err != nil {
		log.Printf("achievements: public: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось собрать достижения")
		return
	}
	writeJSON(w, http.StatusOK, ans)
}

// ---------------------------------------------------------------- service

// Mine returns every badge of the person with the progress, noticing and recording the new ones. An
// evaluation younger than maxAge is reused.
func (s *Service) Mine(ctx context.Context, uid string, maxAge time.Duration) (*Mine, error) {
	prog, rows, err := s.refresh(ctx, uid, maxAge)
	if err != nil {
		return nil, err
	}
	got := map[string]store.AchievementRow{}
	for _, r := range rows {
		got[r.Key] = r
	}
	ans := &Mine{Items: make([]Item, 0, len(prog)), Total: len(Catalog), Unseen: []string{}}
	for _, p := range prog {
		it := Item{ID: p.ID, Category: string(p.Category), Tier: string(p.Tier), Target: p.Target, Progress: p.Current}
		if r, ok := got[p.ID]; ok { // earned once stays earned, whatever the numbers do now
			t := r.UnlockedAt
			it.Unlocked, it.UnlockedAt, it.Seen, it.Progress = true, &t, r.Seen, p.Target
			ans.Unlocked++
		}
		ans.Items = append(ans.Items, it)
	}
	// the unseen, the oldest first, so that the notices come in the order they were earned
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].UnlockedAt.Before(rows[j].UnlockedAt) })
	for _, r := range rows {
		if _, ok := Lookup(r.Key); ok && !r.Seen {
			ans.Unseen = append(ans.Unseen, r.Key)
		}
	}
	return ans, nil
}

// Public returns the badges another person has earned. The person must exist.
func (s *Service) Public(ctx context.Context, uid string) (*Public, error) {
	s.mu.Lock()
	_, known := s.cache[uid]
	s.mu.Unlock()
	if !known { // somebody who does not exist must not get a place in the cache
		if _, err := s.cfg.Store.UserByID(ctx, uid); err != nil {
			return nil, err
		}
	}
	_, rows, err := s.refresh(ctx, uid, PublicMaxAge)
	if err != nil {
		return nil, err
	}
	ans := &Public{Items: []PublicItem{}, Total: len(Catalog)}
	for _, r := range rows { // newest first, as read
		d, ok := Lookup(r.Key)
		if !ok {
			continue
		}
		ans.Items = append(ans.Items, PublicItem{ID: d.ID, Category: string(d.Category), Tier: string(d.Tier), UnlockedAt: r.UnlockedAt})
	}
	ans.Unlocked = len(ans.Items)
	return ans, nil
}

func (s *Service) entryOf(uid string) *entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.cache[uid]
	if e == nil {
		if len(s.cache) >= cacheLimit { // a simple bound: forget the oldest half
			type kv struct {
				k string
				t time.Time
			}
			all := make([]kv, 0, len(s.cache))
			for k, v := range s.cache {
				all = append(all, kv{k, v.at})
			}
			sort.Slice(all, func(i, j int) bool { return all[i].t.Before(all[j].t) })
			for _, x := range all[:len(all)/2] {
				delete(s.cache, x.k)
			}
		}
		e = &entry{accScanned: -1, brilScanned: -1}
		s.cache[uid] = e
	}
	return e
}

// refresh brings the evaluation of the person up to date when it is older than maxAge, records the
// badges it found, and returns it with the recorded badges (newest first). The evaluation of one person
// never runs twice at once; the second caller waits and then finds it fresh.
func (s *Service) refresh(ctx context.Context, uid string, maxAge time.Duration) ([]Progress, []store.AchievementRow, error) {
	e := s.entryOf(uid)
	e.mu.Lock()
	defer e.mu.Unlock()
	now := s.cfg.Now()
	if e.progress == nil || now.Sub(e.at) >= maxAge {
		got, err := s.cfg.Store.Achievements(ctx, uid)
		if err != nil {
			return nil, nil, err
		}
		have := map[string]bool{}
		for _, r := range got {
			have[r.Key] = true
		}
		facts, err := s.facts(ctx, uid, e, have)
		if err != nil {
			return nil, nil, err
		}
		prog := Evaluate(facts)
		var fresh []string
		for _, p := range prog {
			if p.Done && !have[p.ID] {
				fresh = append(fresh, p.ID)
			}
		}
		if _, err := s.cfg.Store.AddAchievements(ctx, uid, fresh, now); err != nil {
			return nil, nil, err
		}
		e.progress, e.at = prog, now
	}
	rows, err := s.cfg.Store.Achievements(ctx, uid)
	if err != nil {
		return nil, nil, err
	}
	return e.progress, rows, nil
}

// facts collects the numbers the badges are decided by. The two searches of the reviews are costly,
// so they run only for a badge still not earned and only when there are reviews that no earlier search saw.
func (s *Service) facts(ctx context.Context, uid string, e *entry, have map[string]bool) (Facts, error) {
	st := s.cfg.Store
	base, err := st.AchievementFacts(ctx, uid, store.AchievementQuery{
		MinRatedGames: MinRatedGames, MinPuzzleAttempts: MinPuzzleAttempts, LongGamePlies: LongGamePlies,
	})
	if err != nil {
		return Facts{}, err
	}
	f := Facts{AchievementFacts: base, Sections: s.cfg.Sections}

	days, err := st.PuzzleDays(ctx, uid)
	if err != nil {
		return f, err
	}
	f.StreakDays = stats.BuildStreak(days, s.cfg.Now()).Best

	if base.Reviews == 0 {
		return f, nil
	}
	var name string
	nameKnown := false
	userName := func() string {
		if !nameKnown {
			nameKnown = true
			if u, err := st.UserByID(ctx, uid); err == nil {
				name = u.Name
			}
		}
		return name
	}

	if !have["accuracy_90"] && e.accScanned != base.Reviews {
		rows, err := st.AccuracyReviews(ctx, uid, AccuracyHigh, AccuracyMinPlies, ReviewScanLimit)
		if err != nil {
			return f, err
		}
		f.AccuracyHigh = ownAccuracyHigh(userName(), rows)
		if !f.AccuracyHigh {
			e.accScanned = base.Reviews
		}
	}
	if !have["brilliant_move"] && e.brilScanned != base.Reviews {
		rows, err := st.BrilliantReviews(ctx, uid, ReviewScanLimit)
		if err != nil {
			return f, err
		}
		f.Brilliant = ownBrilliant(userName(), rows)
		if !f.Brilliant {
			e.brilScanned = base.Reviews
		}
	}
	return f, nil
}

// ownAccuracyHigh says whether in one of the reviews the person's own side (found by name in the game
// record) reached the accuracy. A review in which the side cannot be told does not count.
func ownAccuracyHigh(name string, rows []store.ReviewBrief) bool {
	for _, r := range rows {
		var sm struct {
			Accuracy [2]float64 `json:"accuracy"`
		}
		var g struct {
			White string `json:"white"`
			Black string `json:"black"`
		}
		if json.Unmarshal(r.Summary, &sm) != nil || json.Unmarshal(r.Game, &g) != nil {
			continue
		}
		switch stats.SideOf(name, g.White, g.Black) {
		case "white":
			if sm.Accuracy[0] >= AccuracyHigh {
				return true
			}
		case "black":
			if sm.Accuracy[1] >= AccuracyHigh {
				return true
			}
		}
	}
	return false
}

// ownBrilliant says whether a brilliant move of a review was played by the person: the moves of the
// report go from the first half-move, so White's have even indexes.
func ownBrilliant(name string, rows []store.BrilliantReview) bool {
	for _, r := range rows {
		var g struct {
			White string `json:"white"`
			Black string `json:"black"`
		}
		if json.Unmarshal(r.Game, &g) != nil {
			continue
		}
		side := stats.SideOf(name, g.White, g.Black)
		for _, i := range r.Moves {
			if (side == "white" && i%2 == 0) || (side == "black" && i%2 == 1) {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------- JSON

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// Package api is the HTTP interface of the game-review service:
//
//	POST /api/analyses        {"pgn": "...", "game": 1, "depth": 14}  -> 202 {"id": "..."}
//	GET  /api/analyses/{id}   status, progress and, when done, the report
//	GET  /api/analyses        the signed-in user's reviews, newest first
//	DELETE /api/analyses/{id} remove one of the user's reviews
//	GET  /api/opening?san=e4,e5,Nf3   the opening a game has reached so far
//	GET  /api/health          liveness probe for Docker
//
// Analyses are queued and run one at a time (every analysis already uses
// all CPUs), so a burst of requests cannot overload the machine. Finished
// results are kept in memory and, when a Store is configured, in MySQL.
// Whoever is signed in (Config.UserID) owns the reviews they start; the
// link of a review works for everyone.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"dyanis-chess/backend/internal/analysis"
	"dyanis-chess/backend/internal/eco"
	"dyanis-chess/backend/internal/pgn"
	"dyanis-chess/backend/internal/store"
	"dyanis-chess/engine"
)

// Analyzer reviews one game; analysis.Analyze in production, a stub in tests.
type Analyzer func(g *pgn.Game, opt analysis.Options) (*analysis.Report, error)

// Store persists finished analyses; *store.Store in production. nil keeps
// results in memory only.
type Store interface {
	SaveAnalysis(ctx context.Context, a *store.Analysis) error
	LoadAnalysis(ctx context.Context, id string) (*store.Analysis, error)
	ListAnalyses(ctx context.Context, userID string, limit int) ([]*store.Analysis, error)
	DeleteAnalysis(ctx context.Context, userID, id string) error
}

// Config tunes a Server. Zero values get the defaults noted.
type Config struct {
	Analyze Analyzer // default analysis.Analyze
	// EvalPosition judges one position for the "try a line" endpoint;
	// default analysis.AnalyzePosition, a stub in tests.
	EvalPosition func(*engine.Position, []uint64, analysis.Options) analysis.PositionEval
	DefaultDepth int   // search depth when the request names none; default 14
	MaxDepth     int   // largest depth a request may ask for; default 20
	MaxPlies     int   // longest game accepted, in half-moves; default 400
	MaxBodyBytes int64 // request size limit; default 512 KiB
	QueueLen     int   // waiting analyses before 503; default 20
	KeepJobs     int   // analyses remembered; default 200
	Store        Store // optional database for finished analyses

	// UserID names the signed-in user of a request, "" for nobody; the
	// default treats everybody as anonymous.
	UserID func(*http.Request) string
}

func (c *Config) fill() {
	if c.Analyze == nil {
		c.Analyze = analysis.Analyze
	}
	if c.EvalPosition == nil {
		c.EvalPosition = analysis.AnalyzePosition
	}
	if c.DefaultDepth <= 0 {
		c.DefaultDepth = 14
	}
	if c.MaxDepth <= 0 {
		c.MaxDepth = 20
	}
	if c.MaxPlies <= 0 {
		c.MaxPlies = 400
	}
	if c.MaxBodyBytes <= 0 {
		c.MaxBodyBytes = 512 << 10
	}
	if c.QueueLen <= 0 {
		c.QueueLen = 20
	}
	if c.KeepJobs <= 0 {
		c.KeepJobs = 200
	}
	if c.UserID == nil {
		c.UserID = func(*http.Request) string { return "" }
	}
}

// maxListed is how many reviews "my games" returns.
const maxListed = 200

// Status values of a job.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusError   = "error"
)

// GameInfo is the part of the PGN the review page shows.
type GameInfo struct {
	White    string `json:"white"`
	Black    string `json:"black"`
	Result   string `json:"result"`
	Date     string `json:"date,omitempty"`
	Event    string `json:"event,omitempty"`
	StartFEN string `json:"startFen"`
}

// Job is the public state of one analysis.
type Job struct {
	ID      string           `json:"id"`
	Status  string           `json:"status"`
	Done    int              `json:"done"`
	Total   int              `json:"total"`
	Game    GameInfo         `json:"game"`
	Report  *analysis.Report `json:"report,omitempty"`
	Error   string           `json:"error,omitempty"`
	Created time.Time        `json:"created"`

	game   *pgn.Game
	depth  int
	userID string
}

// Summary is the handful of figures the "my games" list shows.
type Summary struct {
	Accuracy [2]float64 `json:"accuracy"`
	Opening  string     `json:"opening,omitempty"`
	ECO      string     `json:"eco,omitempty"`
	Plies    int        `json:"plies"`
}

// Listed is one row of "my games".
type Listed struct {
	ID      string    `json:"id"`
	Status  string    `json:"status"`
	Depth   int       `json:"depth"`
	Created time.Time `json:"created"`
	Game    GameInfo  `json:"game"`
	Summary *Summary  `json:"summary,omitempty"`
}

// Server holds the queue and the results.
type Server struct {
	cfg   Config
	queue chan *Job
	// evalSlots limits the position evaluations of the line endpoint.
	evalSlots chan struct{}

	mu    sync.Mutex
	jobs  map[string]*Job
	order []string // ids, oldest first
	quit  chan struct{}
	wg    sync.WaitGroup
}

// New starts the worker goroutine; call Close to stop it.
func New(cfg Config) *Server {
	cfg.fill()
	s := &Server{
		cfg:       cfg,
		queue:     make(chan *Job, cfg.QueueLen),
		evalSlots: make(chan struct{}, lineEvalSlots),
		jobs:      map[string]*Job{},
		quit:      make(chan struct{}),
	}
	s.wg.Add(1)
	go s.worker()
	return s
}

// Close stops the worker after the analysis it is running, if any.
func (s *Server) Close() {
	close(s.quit)
	s.wg.Wait()
}

// Handler returns the routes of the API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/analyses", s.create)
	mux.HandleFunc("GET /api/analyses/{id}", s.get)
	mux.HandleFunc("GET /api/analyses", s.list)
	mux.HandleFunc("DELETE /api/analyses/{id}", s.remove)
	mux.HandleFunc("GET /api/opening", s.opening)
	mux.HandleFunc("POST /api/line", s.line)
	mux.HandleFunc("POST /api/continuations", s.continuations)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return mux
}

type createRequest struct {
	PGN   string `json:"pgn"`
	Game  int    `json:"game"`  // 1-based index in a multi-game PGN; default 1
	Depth int    `json:"depth"` // 0 = server default
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes)
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "PGN слишком большой")
			return
		}
		writeError(w, http.StatusBadRequest, "ожидался JSON {\"pgn\": \"...\"}")
		return
	}
	if strings.TrimSpace(req.PGN) == "" {
		writeError(w, http.StatusBadRequest, "пустой PGN")
		return
	}
	games, err := pgn.ParseString(req.PGN)
	if err != nil && len(games) == 0 {
		writeError(w, http.StatusBadRequest, "не удалось разобрать PGN: "+err.Error())
		return
	}
	if len(games) == 0 {
		writeError(w, http.StatusBadRequest, "в PGN нет партий")
		return
	}
	idx := req.Game
	if idx == 0 {
		idx = 1
	}
	if idx < 1 || idx > len(games) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("в PGN партий: %d, а запрошена %d-я", len(games), idx))
		return
	}
	g := games[idx-1]
	if len(g.Moves) == 0 {
		writeError(w, http.StatusBadRequest, "в партии нет ходов")
		return
	}
	if len(g.Moves) > s.cfg.MaxPlies {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("партия длиннее %d полуходов", s.cfg.MaxPlies))
		return
	}
	depth := req.Depth
	if depth <= 0 {
		depth = s.cfg.DefaultDepth
	}
	if depth > s.cfg.MaxDepth {
		depth = s.cfg.MaxDepth
	}

	job := &Job{
		ID: newID(), Status: StatusQueued, Total: len(g.Moves) + 1,
		Game: gameInfo(g), Created: time.Now().UTC(), game: g, depth: depth,
		userID: s.cfg.UserID(r),
	}
	// Register before queueing: the worker may pick the job up at once.
	s.mu.Lock()
	select {
	case s.queue <- job:
	default:
		s.mu.Unlock()
		writeError(w, http.StatusServiceUnavailable, "очередь разбора переполнена, попробуйте позже")
		return
	}
	s.jobs[job.ID] = job
	s.order = append(s.order, job.ID)
	s.evictLocked()
	s.mu.Unlock()
	writeJSON(w, http.StatusAccepted, map[string]string{"id": job.ID})
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	job, ok := s.jobs[id]
	var snap Job
	if ok {
		snap = *job // copy under the lock; Report is immutable once set
	}
	s.mu.Unlock()
	if !ok {
		loaded, err := s.loadStored(r.Context(), id)
		if err != nil {
			log.Printf("api: load analysis %s: %v", id, err)
			writeError(w, http.StatusInternalServerError, "не удалось прочитать разбор из базы")
			return
		}
		if loaded == nil {
			writeError(w, http.StatusNotFound, "разбор не найден")
			return
		}
		snap = *loaded
	}
	writeJSON(w, http.StatusOK, &snap)
}

// list returns the signed-in user's reviews, newest first. Reviews still
// waiting or running (memory only) come first, so a page that was reloaded
// during an analysis still shows it.
func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	uid := s.cfg.UserID(r)
	if uid == "" {
		writeError(w, http.StatusUnauthorized, "нужно войти, чтобы увидеть свои партии")
		return
	}
	items := []Listed{}
	inMemory := map[string]bool{}
	s.mu.Lock()
	for i := len(s.order) - 1; i >= 0; i-- {
		j := s.jobs[s.order[i]]
		if j == nil || j.userID != uid {
			continue
		}
		inMemory[j.ID] = true
		l := Listed{ID: j.ID, Status: j.Status, Depth: j.depth, Created: j.Created, Game: j.Game}
		if j.Report != nil {
			l.Summary = summarize(j.Report)
		}
		items = append(items, l)
	}
	s.mu.Unlock()

	if s.cfg.Store != nil {
		rows, err := s.cfg.Store.ListAnalyses(r.Context(), uid, maxListed)
		if err != nil {
			log.Printf("api: list analyses: %v", err)
			writeError(w, http.StatusInternalServerError, "не удалось прочитать список партий")
			return
		}
		for _, a := range rows {
			if inMemory[a.ID] {
				continue
			}
			l := Listed{ID: a.ID, Status: a.Status, Depth: a.Depth, Created: a.CreatedAt}
			if json.Unmarshal(a.Game, &l.Game) != nil {
				continue // an unreadable row is not worth failing the list
			}
			if a.Summary != nil {
				var sm Summary
				if json.Unmarshal(a.Summary, &sm) == nil {
					l.Summary = &sm
				}
			}
			items = append(items, l)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Created.After(items[j].Created) })
	if len(items) > maxListed {
		items = items[:maxListed]
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// remove deletes one of the user's reviews.
func (s *Server) remove(w http.ResponseWriter, r *http.Request) {
	uid := s.cfg.UserID(r)
	if uid == "" {
		writeError(w, http.StatusUnauthorized, "нужно войти")
		return
	}
	id := r.PathValue("id")
	found := false
	s.mu.Lock()
	if j := s.jobs[id]; j != nil && j.userID == uid {
		if j.Status == StatusQueued || j.Status == StatusRunning {
			s.mu.Unlock()
			writeError(w, http.StatusConflict, "разбор ещё идёт, подождите его конца")
			return
		}
		delete(s.jobs, id)
		found = true
	}
	s.mu.Unlock()
	if s.cfg.Store != nil {
		err := s.cfg.Store.DeleteAnalysis(r.Context(), uid, id)
		switch {
		case err == nil:
			found = true
		case !errors.Is(err, store.ErrNotFound):
			log.Printf("api: delete analysis %s: %v", id, err)
			writeError(w, http.StatusInternalServerError, "не удалось удалить разбор")
			return
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "разбор не найден")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func summarize(rep *analysis.Report) *Summary {
	sm := &Summary{Accuracy: rep.Accuracy, Plies: len(rep.Moves)}
	if rep.Opening != nil {
		sm.Opening, sm.ECO = rep.Opening.Name, rep.Opening.ECO
	}
	return sm
}

// loadStored reads a finished analysis from the database; nil, nil when
// there is no store or no such analysis.
func (s *Server) loadStored(ctx context.Context, id string) (*Job, error) {
	if s.cfg.Store == nil {
		return nil, nil
	}
	a, err := s.cfg.Store.LoadAnalysis(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	j := &Job{ID: a.ID, Status: a.Status, Error: a.Error, Created: a.CreatedAt}
	if err := json.Unmarshal(a.Game, &j.Game); err != nil {
		return nil, fmt.Errorf("stored game: %w", err)
	}
	if a.Report != nil {
		j.Report = new(analysis.Report)
		if err := json.Unmarshal(a.Report, j.Report); err != nil {
			return nil, fmt.Errorf("stored report: %w", err)
		}
		j.Total = len(j.Report.Evals)
		j.Done = j.Total
	}
	return j, nil
}

// persist writes a finished job to the database. A failure is logged, not
// fatal: the result is still served from memory.
func (s *Server) persist(job Job, depth int) {
	if s.cfg.Store == nil {
		return
	}
	game, err := json.Marshal(job.Game)
	if err == nil {
		a := &store.Analysis{
			ID: job.ID, UserID: job.userID, Status: job.Status, Depth: depth, Game: game,
			Error: job.Error, CreatedAt: job.Created, FinishedAt: time.Now(),
		}
		if job.Report != nil {
			a.Report, err = json.Marshal(job.Report)
			if err == nil {
				a.Summary, err = json.Marshal(summarize(job.Report))
			}
		}
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			err = s.cfg.Store.SaveAnalysis(ctx, a)
		}
	}
	if err != nil {
		log.Printf("api: save analysis %s: %v", job.ID, err)
	}
}

func (s *Server) worker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.quit:
			return
		case job := <-s.queue:
			s.run(job)
		}
	}
}

func (s *Server) run(job *Job) {
	s.mu.Lock()
	job.Status = StatusRunning
	g, depth := job.game, job.depth
	s.mu.Unlock()

	opt := analysis.Options{
		Depth: depth,
		Progress: func(done, total int) {
			s.mu.Lock()
			job.Done, job.Total = done, total
			s.mu.Unlock()
		},
	}
	rep, err := s.safeAnalyze(g, opt)

	s.mu.Lock()
	job.game = nil // the parsed game is no longer needed
	if err != nil {
		job.Status, job.Error = StatusError, err.Error()
	} else {
		job.Status, job.Report, job.Done = StatusDone, rep, job.Total
	}
	snap := *job
	s.mu.Unlock()
	s.persist(snap, depth)
}

// safeAnalyze turns a panic inside the analysis into an error, so one bad
// game cannot kill the worker (and with it every later analysis).
func (s *Server) safeAnalyze(g *pgn.Game, opt analysis.Options) (rep *analysis.Report, err error) {
	defer func() {
		if p := recover(); p != nil {
			rep, err = nil, fmt.Errorf("внутренняя ошибка разбора: %v", p)
		}
	}()
	return s.cfg.Analyze(g, opt)
}

// evictLocked forgets the oldest jobs beyond KeepJobs, never one that is
// still waiting or running.
func (s *Server) evictLocked() {
	for len(s.order) > s.cfg.KeepJobs {
		id := s.order[0]
		if j := s.jobs[id]; j != nil && (j.Status == StatusQueued || j.Status == StatusRunning) {
			return
		}
		delete(s.jobs, id)
		s.order = s.order[1:]
	}
}

func gameInfo(g *pgn.Game) GameInfo {
	name := func(tag string) string {
		if v := g.Tag(tag); v != "" && v != "?" {
			return v
		}
		return ""
	}
	date := name("Date")
	if strings.HasPrefix(date, "????") {
		date = ""
	}
	start := g.StartFEN
	if start == "" {
		start = engine.StartFEN
	}
	return GameInfo{
		White: name("White"), Black: name("Black"), Result: g.Result,
		Date: date, Event: name("Event"), StartFEN: start,
	}
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // the system RNG failing is not recoverable
	}
	return hex.EncodeToString(b[:])
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// opening answers GET /api/opening?san=e4,e5,Nf3: the name of the opening a game
// from the starting position has reached (the last named position on its way,
// as the review does), and whether the position is still theory. The browser
// game against the engine asks it after every move.
func (s *Server) opening(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("san")
	var sans []string
	if raw != "" {
		sans = strings.Split(raw, ",")
	}
	if len(sans) > 120 {
		writeError(w, http.StatusBadRequest, "слишком длинная партия")
		return
	}
	book := eco.Default()
	pos := engine.NewPosition()
	var found *eco.Opening
	inBook := true
	for _, san := range sans {
		m, err := pos.ParseSAN(strings.TrimSpace(san))
		if err != nil {
			writeError(w, http.StatusBadRequest, "неверный ход: "+san)
			return
		}
		pos.Play(m)
		h := pos.Hash()
		if o, ok := book.Lookup(h); ok {
			o := o
			found = &o
		}
		inBook = book.InBook(h)
	}
	writeJSON(w, http.StatusOK, map[string]any{"opening": found, "book": inBook && len(sans) > 0})
}

package play

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	mrand "math/rand"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/coder/websocket"

	"dyanis-chess/backend/internal/bots"
	"dyanis-chess/backend/internal/store"
)

// Store keeps games between restarts; *store.Store in production. nil keeps
// them in memory only.
type Store interface {
	SaveGame(ctx context.Context, g *store.Game) error
	LoadGame(ctx context.Context, id string) (*store.Game, error)
	ListGames(ctx context.Context, userID string, limit int) ([]*store.Game, error)
	// RecordBotResult adds a finished game against a bot to the person's
	// progress (outcome "win", "draw" or "loss"; points are earned by the
	// first win only) and says whether it was that first win.
	RecordBotResult(ctx context.Context, userID, botID, outcome string, points int) (firstWin bool, err error)
	BotProgress(ctx context.Context, userID string) ([]store.BotRecord, error)
}

// Config of a Hub.
type Config struct {
	Store Store
	// User returns the signed-in user of a request, or nil.
	User func(r *http.Request) *store.User
	// Now is the clock; time.Now unless a test says otherwise.
	Now func() time.Time
	// BotThink is how long a bot lets a move take; nil is the human-like
	// default (tests make it instant).
	BotThink func(b *bots.Bot, rng *mrand.Rand) time.Duration
	// Bot finds a bot by id; bots.Get unless a test supplies its own.
	Bot func(id string) (*bots.Bot, bool)
	// AllowedOrigins are extra origin patterns the WebSocket accepts (the
	// request's own host is always accepted).
	AllowedOrigins []string
}

// Limits.
const (
	maxGames         = 20000 // games kept in memory
	maxWaitingPerUsr = 5     // open invitations one person may have
	waitingTTL       = 24 * time.Hour
	keepEnded        = 10 * time.Minute // an ended game stays in memory this long without viewers
	maxListed        = 100
)

var idPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// Hub holds the games that are in memory and serves their HTTP and
// WebSocket interface.
type Hub struct {
	cfg Config

	mu    sync.Mutex
	games map[string]*Game

	saveMu  sync.Mutex
	pending map[string]*store.Game // latest unsaved state of each game
	wake    chan struct{}
	quit    chan struct{}
	wg      sync.WaitGroup
	once    sync.Once
}

// New starts the background workers; Close stops them.
func (c *Config) bot(id string) (*bots.Bot, bool) {
	if c.Bot != nil {
		return c.Bot(id)
	}
	return bots.Get(id)
}

func (c *Config) botThink(b *bots.Bot, rng *mrand.Rand) time.Duration {
	if c.BotThink != nil {
		return c.BotThink(b, rng)
	}
	return botThink(b, rng)
}

func New(cfg Config) *Hub {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	h := &Hub{
		cfg: cfg, games: map[string]*Game{}, pending: map[string]*store.Game{},
		wake: make(chan struct{}, 1), quit: make(chan struct{}),
	}
	h.wg.Add(2)
	go h.saver()
	go h.janitor()
	return h
}

// Close saves what is pending and stops the workers.
func (h *Hub) Close() {
	h.once.Do(func() { close(h.quit) })
	h.wg.Wait()
	h.mu.Lock()
	for _, g := range h.games {
		g.Close()
	}
	h.mu.Unlock()
}

// Handler returns the routes of the package.
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/games", h.create)
	mux.HandleFunc("GET /api/games", h.list)
	mux.HandleFunc("GET /api/bots", h.listBots)
	mux.HandleFunc("GET /api/games/{id}", h.get)
	mux.HandleFunc("POST /api/games/{id}/join", h.join)
	mux.HandleFunc("GET /api/games/{id}/pgn", h.pgn)
	mux.HandleFunc("GET /api/games/{id}/ws", h.ws)
	return mux
}

// How many messages a connection may send: a burst of msgBurst, then msgRate
// per second. Variables so that a test, whose "person" is a program, can lift them.
var (
	msgBurst = 20.0
	msgRate  = 5.0
)

// ------------------------------------------------------------ persistence

// persist is installed in every game: it queues the state for the saver.
// Called with the game's lock held.
func (h *Hub) persist(g *Game, state []byte) {
	if h.cfg.Store == nil {
		return
	}
	row := &store.Game{
		ID: g.ID, Status: g.status, Result: g.result, State: state,
		CreatedAt: g.created, UpdatedAt: h.cfg.Now(),
	}
	if g.white != nil && !g.white.IsBot() { // a bot is not a user: no row to point at
		row.WhiteID = g.white.ID
	}
	if g.black != nil && !g.black.IsBot() {
		row.BlackID = g.black.ID
	}
	h.saveMu.Lock()
	h.pending[g.ID] = row
	h.saveMu.Unlock()
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

// saver writes the latest state of each changed game. Several changes of a
// game between two writes cost one write, and the order is never mixed up.
func (h *Hub) saver() {
	defer h.wg.Done()
	flush := func() {
		h.saveMu.Lock()
		batch := h.pending
		h.pending = map[string]*store.Game{}
		h.saveMu.Unlock()
		for _, row := range batch {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := h.cfg.Store.SaveGame(ctx, row); err != nil {
				log.Printf("play: save game %s: %v", row.ID, err)
			}
			cancel()
		}
	}
	for {
		select {
		case <-h.wake:
			flush()
		case <-h.quit:
			flush()
			return
		}
	}
}

// janitor forgets games nobody needs in memory any more, and gives up on
// invitations nobody accepted.
func (h *Hub) janitor() {
	defer h.wg.Done()
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			h.sweep()
		case <-h.quit:
			return
		}
	}
}

func (h *Hub) sweep() {
	now := h.cfg.Now()
	h.mu.Lock()
	var drop []*Game
	for id, g := range h.games {
		g.mu.Lock()
		switch {
		case g.status == StatusWaiting && now.Sub(g.created) > waitingTTL:
			g.finish("", TermWaitingGaveUp)
			g.changed()
		case (g.status == StatusEnded || g.status == StatusAborted) && len(g.conns) == 0 && now.Sub(g.ended) > keepEnded:
			drop = append(drop, g)
			delete(h.games, id)
		}
		g.mu.Unlock()
	}
	h.mu.Unlock()
	for _, g := range drop {
		g.Close()
	}
}

// -------------------------------------------------------------- the games

var errNoGame = errors.New("партия не найдена")

// register puts a game in memory.
func (h *Hub) register(g *Game) {
	g.persist = h.persist
	g.after = h.after
	h.mu.Lock()
	h.games[g.ID] = g
	h.mu.Unlock()
}

// find returns the game, from memory or else from the database.
func (h *Hub) find(ctx context.Context, id string) (*Game, error) {
	if !idPattern.MatchString(id) {
		return nil, errNoGame
	}
	h.mu.Lock()
	g := h.games[id]
	h.mu.Unlock()
	if g != nil {
		return g, nil
	}
	if h.cfg.Store == nil {
		return nil, errNoGame
	}
	row, err := h.cfg.Store.LoadGame(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNoGame
	}
	if err != nil {
		return nil, err
	}
	loaded, err := restore(id, h.cfg.Now, row.State)
	if err != nil {
		return nil, err
	}
	// The columns follow a merge of accounts; the document does not.
	if loaded.white != nil && row.WhiteID != "" {
		loaded.white.ID = row.WhiteID
	}
	if loaded.black != nil && row.BlackID != "" {
		loaded.black.ID = row.BlackID
	}
	loaded.persist = h.persist
	loaded.after = h.after
	h.mu.Lock()
	if existing := h.games[id]; existing != nil { // someone else loaded it meanwhile
		h.mu.Unlock()
		return existing, nil
	}
	h.games[id] = loaded
	h.mu.Unlock()
	loaded.tick() // a clock may have run out while the server was down
	loaded.mu.Lock()
	h.after(loaded) // a bot that was to move when the server stopped moves now
	loaded.mu.Unlock()
	return loaded, nil
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func playerOf(u *store.User) Player {
	return Player{ID: u.ID, Name: u.Name, Avatar: u.AvatarURL}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (h *Hub) user(r *http.Request) *store.User {
	if h.cfg.User == nil {
		return nil
	}
	return h.cfg.User(r)
}

type createRequest struct {
	Color  string `json:"color"` // "white", "black" or "random"
	TimeMs int64  `json:"timeMs"`
	IncMs  int64  `json:"incMs"`
	Bot    string `json:"bot,omitempty"` // play against this bot instead of inviting a person
}

func (h *Hub) create(w http.ResponseWriter, r *http.Request) {
	u := h.user(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "чтобы играть, войдите или продолжите как гость")
		return
	}
	var req createRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "ожидался JSON {color, timeMs, incMs}")
		return
	}
	tc := TimeControl{BaseMs: req.TimeMs, IncMs: req.IncMs}
	if err := tc.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	side := Side(req.Color)
	switch side {
	case White, Black:
	case "random", "":
		var b [1]byte
		_, _ = rand.Read(b[:])
		side = White
		if b[0]&1 == 1 {
			side = Black
		}
	default:
		writeError(w, http.StatusBadRequest, "цвет: white, black или random")
		return
	}

	if req.Bot != "" {
		b, ok := h.cfg.bot(req.Bot)
		if !ok {
			writeError(w, http.StatusBadRequest, "такого бота нет")
			return
		}
		h.startBotGame(w, u, b, side, tc)
		return
	}

	h.mu.Lock()
	waiting := 0
	for _, g := range h.games {
		g.mu.Lock()
		if g.status == StatusWaiting && g.creator == u.ID {
			waiting++
		}
		g.mu.Unlock()
	}
	full := len(h.games) >= maxGames
	h.mu.Unlock()
	if full {
		writeError(w, http.StatusServiceUnavailable, "сервер перегружен, попробуйте позже")
		return
	}
	if waiting >= maxWaitingPerUsr {
		writeError(w, http.StatusTooManyRequests, "у вас уже много приглашений без ответа: отмените ненужные")
		return
	}

	g := newGame(newID(), h.cfg.Now, tc, playerOf(u), side)
	h.register(g)
	g.mu.Lock()
	g.changed() // saves it
	g.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]string{"id": g.ID})
}

func (h *Hub) game(w http.ResponseWriter, r *http.Request) *Game {
	g, err := h.find(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, errNoGame):
		writeError(w, http.StatusNotFound, "партия не найдена")
	case err != nil:
		log.Printf("play: load game %s: %v", r.PathValue("id"), err)
		writeError(w, http.StatusInternalServerError, "не удалось открыть партию")
	default:
		return g
	}
	return nil
}

func (h *Hub) get(w http.ResponseWriter, r *http.Request) {
	g := h.game(w, r)
	if g == nil {
		return
	}
	uid := ""
	if u := h.user(r); u != nil {
		uid = u.ID
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(g.Snapshot(uid))
}

func (h *Hub) join(w http.ResponseWriter, r *http.Request) {
	u := h.user(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "чтобы играть, войдите или продолжите как гость")
		return
	}
	g := h.game(w, r)
	if g == nil {
		return
	}
	if err := g.Join(playerOf(u)); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(g.Snapshot(u.ID))
}

func (h *Hub) pgn(w http.ResponseWriter, r *http.Request) {
	g := h.game(w, r)
	if g == nil {
		return
	}
	g.mu.Lock()
	ended := g.status == StatusEnded
	g.mu.Unlock()
	if !ended {
		writeError(w, http.StatusConflict, "партия ещё не закончилась")
		return
	}
	w.Header().Set("Content-Type", "application/x-chess-pgn; charset=utf-8")
	_, _ = w.Write([]byte(g.PGN()))
}

// Listed is one row of "my online games".
type Listed struct {
	ID          string      `json:"id"`
	Status      string      `json:"status"`
	Result      string      `json:"result,omitempty"`
	Termination string      `json:"termination,omitempty"`
	White       *Player     `json:"white"`
	Black       *Player     `json:"black"`
	TC          TimeControl `json:"tc"`
	Plies       int         `json:"plies"`
	Updated     int64       `json:"updated"`
	You         Side        `json:"you"`
}

func (h *Hub) list(w http.ResponseWriter, r *http.Request) {
	u := h.user(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "нужно войти")
		return
	}
	items := []Listed{}
	if h.cfg.Store != nil {
		rows, err := h.cfg.Store.ListGames(r.Context(), u.ID, maxListed)
		if err != nil {
			log.Printf("play: list games: %v", err)
			writeError(w, http.StatusInternalServerError, "не удалось прочитать список партий")
			return
		}
		for _, row := range rows {
			var s saved
			if json.Unmarshal(row.State, &s) != nil {
				continue
			}
			l := Listed{
				ID: row.ID, Status: row.Status, Result: row.Result, Termination: s.Termination,
				White: s.White, Black: s.Black, TC: s.TC, Plies: len(s.Moves), Updated: row.UpdatedAt.UnixMilli(),
			}
			switch u.ID {
			case row.WhiteID:
				l.You = White
			case row.BlackID:
				l.You = Black
			}
			items = append(items, l)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// ------------------------------------------------------------- the socket

type incoming struct {
	Type string `json:"type"`
	UCI  string `json:"uci,omitempty"`
	Text string `json:"text,omitempty"`
}

func (h *Hub) ws(w http.ResponseWriter, r *http.Request) {
	g := h.game(w, r)
	if g == nil {
		return
	}
	uid := ""
	if u := h.user(r); u != nil {
		uid = u.ID
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.cfg.AllowedOrigins})
	if err != nil {
		return // Accept has answered the client
	}
	c.SetReadLimit(4096)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cn := newConn(uid)
	g.addConn(cn)
	defer func() {
		g.removeConn(cn)
		cn.shut()
	}()

	// Writer: snapshots out, and a ping now and then so that proxies and
	// NATs do not close an idle connection.
	done := make(chan struct{})
	go func() {
		defer close(done)
		ping := time.NewTicker(25 * time.Second)
		defer ping.Stop()
		for {
			select {
			case msg, ok := <-cn.send:
				if !ok {
					c.Close(websocket.StatusPolicyViolation, "слишком медленное соединение")
					return
				}
				wctx, wcancel := context.WithTimeout(ctx, 10*time.Second)
				err := c.Write(wctx, websocket.MessageText, msg)
				wcancel()
				if err != nil {
					return
				}
			case <-ping.C:
				pctx, pcancel := context.WithTimeout(ctx, 10*time.Second)
				err := c.Ping(pctx)
				pcancel()
				if err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Reader: what the person wants to do. At most a handful of messages
	// per second; a burst beyond that is a script, not a player.
	budget, last := msgBurst, time.Now()
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			break
		}
		now := time.Now()
		budget = min(msgBurst, budget+now.Sub(last).Seconds()*msgRate) - 1
		last = now
		if budget < 0 {
			c.Close(websocket.StatusPolicyViolation, "слишком много сообщений")
			break
		}
		var in incoming
		if json.Unmarshal(data, &in) != nil {
			continue
		}
		if err := h.handle(g, uid, in); err != nil {
			e, _ := json.Marshal(Message{Type: "error", Err: err.Error()})
			cn.offer(e)
			cn.offer(g.Snapshot(uid)) // and the true state, so the screen is right whatever it believed
		}
	}
	cancel()
	<-done
	c.CloseNow()
}

func (h *Hub) handle(g *Game, uid string, in incoming) error {
	switch in.Type {
	case "move":
		return g.Move(uid, in.UCI)
	case "resign":
		return g.Resign(uid)
	case "abort":
		return g.Abort(uid)
	case "draw":
		return g.OfferDraw(uid)
	case "drawDecline":
		return g.DeclineDraw(uid)
	case "chat":
		return g.Chat(uid, in.Text)
	case "rematch":
		return h.rematch(g, uid)
	}
	return nil
}

// rematch records a wish to play again and, when both sides have it, starts
// the new game.
func (h *Hub) rematch(g *Game, uid string) error {
	n, err := g.Rematch(uid, newID())
	if err != nil || n == nil {
		return err
	}
	h.register(n)
	n.mu.Lock()
	n.changed() // saves it, starts its timer and, if a bot plays White, its first move
	n.mu.Unlock()
	return nil
}

// Package vplay runs games of the chess variants (internal/variants) between
// two people: rooms, clocks and the WebSocket that keeps both browsers in step.
// It is the sibling of internal/play and works the same way: the server is the
// only source of truth, a client asks to act, resign or offer a draw, the server
// checks it against the variant's State and the clock and sends everybody the
// new state as one whole snapshot. A game lives in memory while it is played and
// is written to MySQL (table variant_games) after every change; it is restored
// by replaying its actions (variants.Replay), so a restart loses nothing.
//
// What is different from play: the rules come from a variants.State, the unit
// of play is an action (a turn may hold several of them: the clock changes hands
// only when State.Turn changes), there are no ratings, no bots and no engine
// analysis, and each viewer gets their own view of the position (a variant may
// hide things).
package vplay

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"dyanis-chess/backend/internal/play"
	"dyanis-chess/backend/internal/variants"
)

// Side and its two values, shared with the rules package.
type Side = variants.Side

const (
	White = variants.White
	Black = variants.Black
)

// Game statuses.
const (
	StatusWaiting = "waiting" // created, the second player has not come yet
	StatusPlaying = "playing"
	StatusEnded   = "ended"   // has a result
	StatusAborted = "aborted" // cancelled before it began; no result
)

// How a game ended that the rules did not end (Game.Termination); a game the
// rules ended has the reason State.Over gave ("checkmate", "king-captured", ...).
const (
	TermResign        = "resign"
	TermTimeout       = "timeout"
	TermAgreement     = "agreement"
	TermAborted       = "aborted"
	TermNoFirstMove   = "no-first-move"
	TermWaitingGaveUp = "expired"
)

const firstMoveTimeout = 60 * time.Second // each side's first turn; past it the game is aborted

// TimeControl is the clock rule, as in internal/play.
type TimeControl = play.TimeControl

// Player is a seat's occupant as clients see them.
type Player struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`
}

// ChatMsg is one line of the chat between the two players.
type ChatMsg struct {
	Side Side   `json:"side"`
	Text string `json:"text"`
	At   int64  `json:"at"` // unix ms
}

// Chat limits.
const (
	maxChatRunes = 200
	maxChatKept  = 100
	chatGap      = 700 * time.Millisecond // least time between two lines of one person
)

// conn is one browser connected to a game. Only the hub's pumps touch the
// socket; the game just drops finished snapshots into send.
type conn struct {
	userID string // "" for someone not signed in (can watch only)
	send   chan []byte

	mu     sync.Mutex
	closed bool
}

func newConn(userID string) *conn { return &conn{userID: userID, send: make(chan []byte, 16)} }

// offer queues a message without ever blocking; false means the client is
// gone or cannot keep up (the caller drops it, and it reconnects).
func (c *conn) offer(b []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- b:
		return true
	default:
		return false
	}
}

// shut closes the queue once; the socket's writer then closes the socket.
func (c *conn) shut() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.send)
	}
}

// Game is one game. All fields are guarded by mu.
type Game struct {
	mu  sync.Mutex
	ID  string
	now func() time.Time

	vid     string // the id of the variant
	vname   string
	opts    variants.Options
	st      variants.State
	tc      TimeControl
	status  string
	result  string
	term    string
	creator string
	white   *Player
	black   *Player

	moves     []variants.MoveRec
	turnsDone int            // how many times the turn has passed from one side to the other
	ms        map[Side]int64 // time left at turnAt
	turnAt    time.Time      // when the side to act was given the turn
	created   time.Time
	started   time.Time
	ended     time.Time

	drawOffer    Side // who offered a draw, NoSide for nobody
	rematchOffer Side
	rematchID    string

	conns map[*conn]struct{}
	timer *time.Timer

	// persist receives the saved state after every change (nil: nowhere).
	persist func(g *Game, state []byte)

	chat    []ChatMsg
	lastSay map[Side]time.Time
}

// newGame makes a waiting game with its creator seated as side.
func newGame(id string, now func() time.Time, v variants.Variant, o variants.Options, tc TimeControl, creator Player, side Side) (*Game, error) {
	g, err := blankGame(id, now, v, o, tc)
	if err != nil {
		return nil, err
	}
	g.creator = creator.ID
	p := creator
	if side == White {
		g.white = &p
	} else {
		g.black = &p
	}
	return g, nil
}

func blankGame(id string, now func() time.Time, v variants.Variant, o variants.Options, tc TimeControl) (*Game, error) {
	st, err := v.New(o)
	if err != nil {
		return nil, err
	}
	info := v.Info()
	return &Game{
		ID: id, now: now, vid: info.ID, vname: info.Name, opts: o, st: st, tc: tc, status: StatusWaiting,
		ms:      map[Side]int64{White: tc.BaseMs, Black: tc.BaseMs},
		conns:   map[*conn]struct{}{},
		lastSay: map[Side]time.Time{},
		created: now(),
	}, nil
}

func (g *Game) timed() bool { return g.tc.BaseMs > 0 }

// clockRunning: before both sides have had a turn the clocks stand still
// (each side only has firstMoveTimeout to start).
func (g *Game) clockRunning() bool {
	return g.status == StatusPlaying && g.timed() && g.turnsDone >= 2
}

// remaining is the time the side has left right now.
func (g *Game) remaining(s Side, now time.Time) int64 {
	if g.clockRunning() && s == g.st.Turn() {
		return g.ms[s] - now.Sub(g.turnAt).Milliseconds()
	}
	return g.ms[s]
}

func (g *Game) sideOf(userID string) Side {
	if userID == "" {
		return variants.NoSide
	}
	if g.white != nil && g.white.ID == userID {
		return White
	}
	if g.black != nil && g.black.ID == userID {
		return Black
	}
	return variants.NoSide
}

// ----------------------------------------------------------------- actions

var (
	errNotPlaying = errors.New("партия не идёт")
	errNotYours   = errors.New("сейчас не ваш ход")
	errNotPlayer  = errors.New("вы не участник партии")
)

// Join seats p in the empty place and starts the game.
func (g *Game) Join(p Player) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.status != StatusWaiting {
		return errors.New("в партию уже нельзя вступить")
	}
	if g.sideOf(p.ID) != variants.NoSide {
		return errors.New("вы уже в этой партии: отправьте ссылку сопернику")
	}
	if g.white == nil {
		g.white = &p
	} else {
		g.black = &p
	}
	g.start()
	g.changed()
	return nil
}

// start begins play; both seats are filled.
func (g *Game) start() {
	now := g.now()
	g.status = StatusPlaying
	g.started, g.turnAt = now, now
	// a variant could be over before the first action (a position with no moves)
	if res, reason, over := g.st.Over(); over {
		g.finish(res, reason)
	}
}

// Act performs one action for the user.
func (g *Game) Act(userID, action string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	side := g.sideOf(userID)
	if side == variants.NoSide {
		return errNotPlayer
	}
	if g.status != StatusPlaying {
		return errNotPlaying
	}
	now := g.now()
	if g.flagLocked(now) {
		return errNotPlaying
	}
	prev := g.st.Turn()
	if side != prev {
		return errNotYours
	}
	running := g.clockRunning()
	before := turnsPassed(g.st)
	note, err := g.st.Apply(action)
	if err != nil {
		return err
	}
	g.moves = append(g.moves, variants.MoveRec{Side: side, Action: action, SAN: note})
	g.drawOffer = variants.NoSide
	// The turn passed: this side's clock stops, with the increment. A State that can skip a whole
	// turn of the other side (dice chess) says so in TurnsPassed: the skipped turn costs no time and
	// no increment, but counts as a turn done.
	passed := turnsPassed(g.st) - before
	if _, counts := g.st.(variants.TurnCounter); !counts && g.st.Turn() != prev {
		passed = 1
	}
	if passed > 0 {
		if running {
			g.ms[prev] = g.ms[prev] - now.Sub(g.turnAt).Milliseconds() + g.tc.IncMs
		}
		g.turnsDone += passed
		g.turnAt = now
	}
	if res, reason, over := g.st.Over(); over {
		g.finish(res, reason)
	}
	g.changed()
	return nil
}

// turnsPassed is the state's own count of passes of the turn (0 for a State without TurnCounter).
func turnsPassed(st variants.State) int {
	if tc, ok := st.(variants.TurnCounter); ok {
		return tc.TurnsPassed()
	}
	return 0
}

// finish ends the game with a result. Callers hold the lock and call changed.
func (g *Game) finish(result, termination string) {
	if g.clockRunning() { // stop the running clock where it stands, so the final times are true
		now := g.now()
		t := g.st.Turn()
		g.ms[t] = max(0, g.ms[t]-now.Sub(g.turnAt).Milliseconds())
		g.turnAt = now
	}
	g.status = StatusEnded
	if termination == TermAborted || termination == TermNoFirstMove || termination == TermWaitingGaveUp {
		g.status = StatusAborted
		result = ""
	}
	g.result, g.term = result, termination
	g.ended = g.now()
	g.drawOffer = variants.NoSide
	if f, ok := g.st.(variants.Finisher); ok && (g.status == StatusEnded || g.status == StatusAborted) {
		// ended by a resignation, a flag, a draw, or aborted (fog of war shows everything then): the state learns of it
		if _, _, over := g.st.Over(); !over {
			f.Finish(result, termination)
		}
	}
}

// Resign gives the game to the opponent.
func (g *Game) Resign(userID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	side := g.sideOf(userID)
	if side == variants.NoSide {
		return errNotPlayer
	}
	if g.status != StatusPlaying {
		return errNotPlaying
	}
	g.finish(variants.ResultFor(side.Opp()), TermResign)
	g.changed()
	return nil
}

// Abort cancels a game that has hardly begun: a waiting one (by its creator)
// or one in which each side has not yet had a turn.
func (g *Game) Abort(userID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.sideOf(userID) == variants.NoSide {
		return errNotPlayer
	}
	if g.status == StatusWaiting && userID == g.creator || g.status == StatusPlaying && g.turnsDone < 2 {
		g.finish("", TermAborted)
		g.changed()
		return nil
	}
	return errors.New("партию уже нельзя отменить, можно сдаться")
}

// OfferDraw offers a draw, or accepts the opponent's offer if there is one.
func (g *Game) OfferDraw(userID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	side := g.sideOf(userID)
	if side == variants.NoSide {
		return errNotPlayer
	}
	if g.status != StatusPlaying || g.turnsDone < 2 {
		return errors.New("ничью можно предложить после первых ходов")
	}
	if g.drawOffer == side.Opp() {
		g.finish(variants.ResultDraw, TermAgreement)
	} else {
		g.drawOffer = side
	}
	g.changed()
	return nil
}

// DeclineDraw turns the opponent's draw offer down (or withdraws one's own).
func (g *Game) DeclineDraw(userID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.sideOf(userID) == variants.NoSide {
		return errNotPlayer
	}
	if g.drawOffer == variants.NoSide {
		return nil
	}
	g.drawOffer = variants.NoSide
	g.changed()
	return nil
}

// Rematch records a wish for a rematch. When both sides want it, the new game
// `next` (colours swapped, a new seed, the same options otherwise) is returned
// for the caller to register; otherwise nil.
func (g *Game) Rematch(userID string, mk func() (*Game, error)) (*Game, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	side := g.sideOf(userID)
	if side == variants.NoSide {
		return nil, errNotPlayer
	}
	if g.status != StatusEnded {
		return nil, errors.New("реванш возможен после окончания партии")
	}
	if g.rematchID != "" {
		return nil, nil
	}
	switch g.rematchOffer {
	case variants.NoSide:
		g.rematchOffer = side
		g.changed()
		return nil, nil
	case side: // asked twice: take the offer back
		g.rematchOffer = variants.NoSide
		g.changed()
		return nil, nil
	}
	n, err := mk()
	if err != nil {
		return nil, err
	}
	n.creator = g.creator
	n.white, n.black = g.black, g.white
	n.start()
	g.rematchID = n.ID
	g.changed()
	return n, nil
}

// ------------------------------------------------------------------- clock

// flagLocked ends the game if the side to act has run out of time; it reports
// whether it did.
func (g *Game) flagLocked(now time.Time) bool {
	if !g.clockRunning() {
		return false
	}
	loser := g.st.Turn()
	if g.remaining(loser, now) > 0 {
		return false
	}
	result := variants.ResultFor(loser.Opp())
	if t, ok := g.st.(variants.Timeouter); ok {
		result = t.Timeout(loser)
	}
	g.finish(result, TermTimeout)
	g.changed()
	return true
}

// tick is called by the timer: someone may have run out of time or failed to
// make a first move.
func (g *Game) tick() {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	if g.status != StatusPlaying {
		return
	}
	if g.turnsDone < 2 {
		if now.Sub(g.turnAt) >= firstMoveTimeout {
			g.finish("", TermNoFirstMove)
			g.changed()
			return
		}
	} else if g.flagLocked(now) {
		return
	}
	g.armLocked()
}

// armLocked schedules the next moment at which something can happen on its
// own: a flag falling or the first-move limit.
func (g *Game) armLocked() {
	if g.timer != nil {
		g.timer.Stop()
		g.timer = nil
	}
	if g.status != StatusPlaying {
		return
	}
	var wait time.Duration
	if g.turnsDone < 2 {
		wait = firstMoveTimeout - g.now().Sub(g.turnAt)
	} else if g.timed() {
		wait = time.Duration(g.remaining(g.st.Turn(), g.now())) * time.Millisecond
	} else {
		return
	}
	if wait < 0 {
		wait = 0
	}
	g.timer = time.AfterFunc(wait+5*time.Millisecond, g.tick)
}

// Close stops the timer; the game is being forgotten.
func (g *Game) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.timer != nil {
		g.timer.Stop()
		g.timer = nil
	}
}

// ---------------------------------------------------------------- snapshots

// Clock is the clock part of a snapshot: the times left as of ServerNow
// (unix ms); the running side's time goes down from there.
type Clock struct {
	WhiteMs   int64 `json:"whiteMs"`
	BlackMs   int64 `json:"blackMs"`
	Running   Side  `json:"running"`
	ServerNow int64 `json:"serverNow"`
}

// Presence says who is connected right now.
type Presence struct {
	White    bool `json:"white"`
	Black    bool `json:"black"`
	Watching int  `json:"watching"`
}

// Rematch is the state of the "play again" wish.
type Rematch struct {
	Offer Side   `json:"offer,omitempty"`
	ID    string `json:"id,omitempty"` // the new game, once both agreed
}

// View is the game as one client sees it.
type View struct {
	ID          string             `json:"id"`
	Variant     string             `json:"variant"`
	VariantName string             `json:"variantName"`
	Params      map[string]any     `json:"params,omitempty"` // the lobby choices (the seed is not told)
	Status      string             `json:"status"`
	Result      string             `json:"result,omitempty"`
	Termination string             `json:"termination,omitempty"`
	White       *Player            `json:"white"`
	Black       *Player            `json:"black"`
	TC          TimeControl        `json:"tc"`
	Turn        Side               `json:"turn"`
	Position    variants.View      `json:"position"`
	Moves       []variants.MoveRec `json:"moves"`
	Clock       *Clock             `json:"clock,omitempty"`
	FirstMoveBy int64              `json:"firstMoveBy,omitempty"` // unix ms deadline while nobody has had two turns
	DrawOffer   Side               `json:"drawOffer,omitempty"`
	Rematch     *Rematch           `json:"rematch,omitempty"`
	Presence    Presence           `json:"presence"`
	Chat        []ChatMsg          `json:"chat,omitempty"`
	Created     int64              `json:"created"`
}

// Message is what the socket carries to a client.
type Message struct {
	Type string `json:"type"` // "state" or "error"
	Game *View  `json:"game,omitempty"`
	You  Side   `json:"you,omitempty"` // the colour of this viewer, "" for a spectator
	Err  string `json:"error,omitempty"`
}

// viewLocked builds the view for the viewer (NoSide: a spectator).
func (g *Game) viewLocked(you Side) *View {
	now := g.now()
	moves := append([]variants.MoveRec{}, g.moves...)
	if r, ok := g.st.(variants.MoveRedactor); ok {
		moves = r.RedactMoves(you, moves)
	}
	v := &View{
		ID: g.ID, Variant: g.vid, VariantName: g.vname, Params: g.opts.Params,
		Status: g.status, Result: g.result, Termination: g.term,
		White: g.white, Black: g.black, TC: g.tc, Turn: g.st.Turn(),
		Position: g.st.View(you), Moves: moves, DrawOffer: g.drawOffer, Created: g.created.UnixMilli(),
		Chat: g.chat,
	}
	if g.status != StatusPlaying { // nothing to play with while waiting or after the end
		v.Position.Legal = nil
	}
	if g.timed() {
		c := &Clock{WhiteMs: g.remaining(White, now), BlackMs: g.remaining(Black, now), ServerNow: now.UnixMilli()}
		if g.clockRunning() {
			c.Running = g.st.Turn()
		}
		c.WhiteMs, c.BlackMs = max(0, c.WhiteMs), max(0, c.BlackMs)
		v.Clock = c
	}
	if g.status == StatusPlaying && g.turnsDone < 2 {
		v.FirstMoveBy = g.turnAt.Add(firstMoveTimeout).UnixMilli()
	}
	if g.rematchOffer != variants.NoSide || g.rematchID != "" {
		v.Rematch = &Rematch{Offer: g.rematchOffer, ID: g.rematchID}
	}
	for c := range g.conns {
		switch g.sideOf(c.userID) {
		case White:
			v.Presence.White = true
		case Black:
			v.Presence.Black = true
		default:
			v.Presence.Watching++
		}
	}
	return v
}

// Snapshot is the message for one viewer.
func (g *Game) Snapshot(userID string) []byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.snapshotLocked(userID)
}

func (g *Game) snapshotLocked(userID string) []byte {
	you := g.sideOf(userID)
	b, _ := json.Marshal(Message{Type: "state", Game: g.viewLocked(you), You: you})
	return b
}

// changed publishes the new state: re-arms the timer, saves, tells everybody.
// Called with the lock held after every change.
func (g *Game) changed() {
	g.armLocked()
	if g.persist != nil {
		g.persist(g, g.stateLocked())
	}
	g.broadcastLocked()
}

func (g *Game) broadcastLocked() {
	for c := range g.conns {
		if !c.offer(g.snapshotLocked(c.userID)) { // too slow: it reconnects and gets a fresh snapshot
			c.shut()
			delete(g.conns, c)
		}
	}
}

// addConn registers a viewer and tells everybody the presence changed.
func (g *Game) addConn(c *conn) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.conns[c] = struct{}{}
	g.broadcastLocked()
}

func (g *Game) removeConn(c *conn) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.conns[c]; ok {
		delete(g.conns, c)
		g.broadcastLocked()
	}
}

// ------------------------------------------------------------------- state

// saved is the part of a game that is written to the database.
type saved struct {
	Variant      string             `json:"variant"`
	Options      variants.Options   `json:"options"`
	Status       string             `json:"status"`
	Result       string             `json:"result,omitempty"`
	Termination  string             `json:"termination,omitempty"`
	TC           TimeControl        `json:"tc"`
	Creator      string             `json:"creator"`
	White        *Player            `json:"white,omitempty"`
	Black        *Player            `json:"black,omitempty"`
	Moves        []variants.MoveRec `json:"moves"`
	TurnsDone    int                `json:"turnsDone"`
	WhiteMs      int64              `json:"whiteMs"`
	BlackMs      int64              `json:"blackMs"`
	TurnAtMs     int64              `json:"turnAtMs"`
	CreatedMs    int64              `json:"createdMs"`
	StartedMs    int64              `json:"startedMs,omitempty"`
	EndedMs      int64              `json:"endedMs,omitempty"`
	DrawOffer    Side               `json:"drawOffer,omitempty"`
	RematchOffer Side               `json:"rematchOffer,omitempty"`
	RematchID    string             `json:"rematchId,omitempty"`
	Chat         []ChatMsg          `json:"chat,omitempty"`
}

func (g *Game) stateLocked() []byte {
	s := saved{
		Variant: g.vid, Options: g.opts, Status: g.status, Result: g.result, Termination: g.term, TC: g.tc, Creator: g.creator,
		White: g.white, Black: g.black, Moves: g.moves, TurnsDone: g.turnsDone, WhiteMs: g.ms[White], BlackMs: g.ms[Black],
		TurnAtMs: g.turnAt.UnixMilli(), CreatedMs: g.created.UnixMilli(),
		DrawOffer: g.drawOffer, RematchOffer: g.rematchOffer, RematchID: g.rematchID, Chat: g.chat,
	}
	if !g.started.IsZero() {
		s.StartedMs = g.started.UnixMilli()
	}
	if !g.ended.IsZero() {
		s.EndedMs = g.ended.UnixMilli()
	}
	b, _ := json.Marshal(s)
	return b
}

// restore rebuilds a game from its saved state by replaying the actions.
func restore(id string, now func() time.Time, state []byte) (*Game, error) {
	var s saved
	if err := json.Unmarshal(state, &s); err != nil {
		return nil, err
	}
	v, ok := variants.Get(s.Variant)
	if !ok {
		return nil, fmt.Errorf("saved game %s: unknown variant %q", id, s.Variant)
	}
	g, err := blankGame(id, now, v, s.Options, s.TC)
	if err != nil {
		return nil, err
	}
	actions := make([]string, len(s.Moves))
	for i, m := range s.Moves {
		actions[i] = m.Action
	}
	st, err := variants.Replay(s.Variant, s.Options, actions)
	if err != nil {
		return nil, fmt.Errorf("saved game %s: %w", id, err)
	}
	g.st = st
	g.moves = s.Moves
	g.turnsDone = s.TurnsDone
	g.status, g.result, g.term, g.creator = s.Status, s.Result, s.Termination, s.Creator
	g.white, g.black = s.White, s.Black
	g.ms = map[Side]int64{White: s.WhiteMs, Black: s.BlackMs}
	g.turnAt = time.UnixMilli(s.TurnAtMs)
	g.created = time.UnixMilli(s.CreatedMs)
	if s.StartedMs != 0 {
		g.started = time.UnixMilli(s.StartedMs)
	}
	if s.EndedMs != 0 {
		g.ended = time.UnixMilli(s.EndedMs)
	}
	g.drawOffer, g.rematchOffer, g.rematchID = s.DrawOffer, s.RematchOffer, s.RematchID
	g.chat = s.Chat
	return g, nil
}

// Chat adds a line said by a player. Spectators read but do not write, and the
// text is cleaned and limited.
func (g *Game) Chat(userID, text string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	side := g.sideOf(userID)
	if side == variants.NoSide {
		return errors.New("писать в чат могут только игроки")
	}
	if g.white == nil || g.black == nil {
		return errors.New("чат откроется, когда придёт соперник")
	}
	if g.status != StatusPlaying && g.status != StatusEnded {
		return errors.New("чат недоступен")
	}
	text = cleanChat(text)
	if text == "" {
		return nil
	}
	now := g.now()
	if last, ok := g.lastSay[side]; ok && now.Sub(last) < chatGap {
		return errors.New("не так быстро")
	}
	g.lastSay[side] = now
	g.chat = append(g.chat, ChatMsg{Side: side, Text: text, At: now.UnixMilli()})
	if len(g.chat) > maxChatKept {
		g.chat = append([]ChatMsg(nil), g.chat[len(g.chat)-maxChatKept:]...)
	}
	g.changed()
	return nil
}

// cleanChat trims a chat line, drops control characters and cuts it to the limit.
func cleanChat(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || r == 0x2028 || r == 0x2029 {
			r = ' '
		}
		b.WriteRune(r)
		if n++; n >= maxChatRunes {
			break
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

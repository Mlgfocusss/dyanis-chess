// Package play runs games between two people: rooms, rules, clocks and the
// WebSocket that keeps both browsers (and anyone watching) in step.
//
// The server is the only source of truth. A client asks to move, resign or
// offer a draw; the server checks it against the rules of the engine
// facade and the clock, changes the game and sends everybody the new state
// as one whole snapshot. Whole snapshots (a few kilobytes) instead of
// patches make a reconnecting browser trivial: it is told the state, nothing
// else, and cannot fall out of step.
//
// A game lives in memory while it is played and is written to MySQL after
// every change, so that a restart of the server loses nothing: the next
// person to open the game's link finds it in the database.
package play

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"dyanis-chess/backend/internal/eco"
	"dyanis-chess/engine"
)

// Side is a colour.
type Side string

// The two colours.
const (
	White Side = "white"
	Black Side = "black"
)

func (s Side) opp() Side {
	if s == White {
		return Black
	}
	return White
}

// Game statuses.
const (
	StatusWaiting = "waiting" // created, the second player has not come yet
	StatusPlaying = "playing"
	StatusEnded   = "ended"   // has a result
	StatusAborted = "aborted" // cancelled before it began; no result
)

// How a game ended (Game.Termination).
const (
	TermCheckmate     = "checkmate"
	TermResign        = "resign"
	TermTimeout       = "timeout"
	TermStalemate     = "stalemate"
	TermAgreement     = "agreement"
	TermRepetition    = "repetition"
	TermFiftyMoves    = "fifty-moves"
	TermInsufficient  = "insufficient-material"
	TermAborted       = "aborted"
	TermNoFirstMove   = "no-first-move"
	TermWaitingGaveUp = "expired"
)

// Limits on what a game may ask for.
const (
	minBaseMs        = 10_000
	maxBaseMs        = 3 * 60 * 60 * 1000
	maxIncMs         = 60_000
	firstMoveTimeout = 60 * time.Second // each side's first move; past it the game is aborted
)

// Player is a seat's occupant as clients see them.
type Player struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`
	// Bot is the id of the computer opponent (see internal/bots) when this
	// seat is played by one, and Elo its rating; the ID then starts with "bot:".
	Bot string `json:"bot,omitempty"`
	Elo int    `json:"elo,omitempty"`
	// Prov says that Elo is a rating of a person still rough (few games): shown with "?".
	Prov bool `json:"prov,omitempty"`
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

// IsBot says whether a computer plays this seat.
func (p *Player) IsBot() bool { return p != nil && p.Bot != "" }

// TimeControl is the clock rule: BaseMs each, plus IncMs after every move.
// BaseMs 0 means no clock at all.
type TimeControl struct {
	BaseMs int64 `json:"baseMs"`
	IncMs  int64 `json:"incMs"`
}

// Validate reports a bad time control.
func (tc TimeControl) Validate() error {
	if tc.BaseMs == 0 {
		if tc.IncMs != 0 {
			return errors.New("без часов нельзя задать добавку")
		}
		return nil
	}
	if tc.BaseMs < minBaseMs || tc.BaseMs > maxBaseMs {
		return errors.New("время на партию: от 10 секунд до 3 часов")
	}
	if tc.IncMs < 0 || tc.IncMs > maxIncMs {
		return errors.New("добавка за ход: от 0 до 60 секунд")
	}
	return nil
}

// MoveRec is one played move.
type MoveRec struct {
	UCI string `json:"uci"`
	SAN string `json:"san"`
}

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

	tc          TimeControl
	status      string
	result      string // "1-0", "0-1", "1/2-1/2" once ended
	termination string
	creator     string // user id of whoever made the game
	white       *Player
	black       *Player

	pos     *engine.Position
	hashes  []uint64 // positions so far, including the current one
	moves   []MoveRec
	turn    Side
	ms      map[Side]int64 // time left at turnStart
	turnAt  time.Time      // when the side to move was given the move
	created time.Time
	started time.Time
	ended   time.Time

	drawOffer    Side // who offered a draw, "" for nobody
	rematchOffer Side
	rematchID    string

	conns map[*conn]struct{}
	timer *time.Timer

	// persist receives the saved state after every change (nil: nowhere).
	persist func(g *Game, state []byte)
	// after is called, with the lock held, after every change; the hub uses
	// it to make the computer opponent move, answer offers and record results.
	after func(g *Game)

	opening *eco.Opening // the last named opening the game has been through
	inBook  bool         // the position is still opening theory
	chat    []ChatMsg
	lastSay map[Side]time.Time

	recorded    bool // the result has been given to the bot progress
	rated       bool // counts for both people's rating (set at the start, only between two people)
	rateDone    bool // the rating update has been started (or there is none to do)
	change      *RatingChanges
	botBusy     bool // a bot is working out its move
	botReplying bool // a bot is answering a draw offer or a rematch wish
}

// newGame makes a waiting game with its creator seated as side.
func newGame(id string, now func() time.Time, tc TimeControl, creator Player, side Side) *Game {
	g := blankGame(id, now, tc)
	g.creator = creator.ID
	g.created = now()
	p := creator
	if side == White {
		g.white = &p
	} else {
		g.black = &p
	}
	return g
}

func blankGame(id string, now func() time.Time, tc TimeControl) *Game {
	pos := engine.NewPosition()
	return &Game{
		ID: id, now: now, tc: tc, status: StatusWaiting,
		pos: pos, hashes: []uint64{pos.Hash()}, turn: White,
		ms:    map[Side]int64{White: tc.BaseMs, Black: tc.BaseMs},
		conns: map[*conn]struct{}{}, lastSay: map[Side]time.Time{},
	}
}

func (g *Game) timed() bool { return g.tc.BaseMs > 0 }

// clockRunning: before both sides have moved once the clocks stand still
// (each side only has firstMoveTimeout to start).
func (g *Game) clockRunning() bool {
	return g.status == StatusPlaying && g.timed() && len(g.moves) >= 2
}

// remaining is the time the side has left right now.
func (g *Game) remaining(s Side, now time.Time) int64 {
	if g.clockRunning() && s == g.turn {
		return g.ms[s] - now.Sub(g.turnAt).Milliseconds()
	}
	return g.ms[s]
}

func (g *Game) sideOf(userID string) Side {
	if userID == "" {
		return ""
	}
	if g.white != nil && g.white.ID == userID {
		return White
	}
	if g.black != nil && g.black.ID == userID {
		return Black
	}
	return ""
}

// ----------------------------------------------------------------- actions

var (
	errNotPlaying = errors.New("партия не идёт")
	errNotYours   = errors.New("сейчас не ваш ход")
	errIllegal    = errors.New("такой ход невозможен")
	errNotPlayer  = errors.New("вы не участник партии")
)

// Join seats p in the empty place and starts the game.
func (g *Game) Join(p Player) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.status != StatusWaiting {
		return errors.New("в партию уже нельзя вступить")
	}
	if g.sideOf(p.ID) != "" {
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
}

// Move plays a move for the user.
func (g *Game) Move(userID, uci string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	side := g.sideOf(userID)
	if side == "" {
		return errNotPlayer
	}
	if g.status != StatusPlaying {
		return errNotPlaying
	}
	now := g.now()
	if g.flagLocked(now) {
		return errNotPlaying
	}
	if side != g.turn {
		return errNotYours
	}
	var mv engine.Move
	found := false
	for _, m := range g.pos.LegalMoves() {
		if m.String() == uci {
			mv, found = m, true
			break
		}
	}
	if !found {
		return errIllegal
	}
	if g.clockRunning() {
		g.ms[side] = g.ms[side] - now.Sub(g.turnAt).Milliseconds() + g.tc.IncMs
	}
	san := g.pos.SAN(mv)
	g.pos.Play(mv)
	g.hashes = append(g.hashes, g.pos.Hash())
	g.moves = append(g.moves, MoveRec{UCI: uci, SAN: san})
	g.trackOpening()
	g.turn = side.opp()
	g.turnAt = now
	g.drawOffer = ""

	if out := g.pos.Outcome(g.hashes); out.Over {
		switch {
		case out.Checkmate:
			g.finish(resultFor(side), TermCheckmate)
		case strings.Contains(out.Reason, "stalemate"):
			g.finish("1/2-1/2", TermStalemate)
		case strings.Contains(out.Reason, "repetition"):
			g.finish("1/2-1/2", TermRepetition)
		case strings.Contains(out.Reason, "50"):
			g.finish("1/2-1/2", TermFiftyMoves)
		default:
			g.finish("1/2-1/2", TermInsufficient)
		}
	}
	g.changed()
	return nil
}

func resultFor(winner Side) string {
	if winner == White {
		return "1-0"
	}
	return "0-1"
}

// finish ends the game with a result. Callers hold the lock and call changed.
func (g *Game) finish(result, termination string) {
	if g.clockRunning() { // stop the running clock where it stands, so the final times are true
		now := g.now()
		g.ms[g.turn] = max(0, g.ms[g.turn]-now.Sub(g.turnAt).Milliseconds())
		g.turnAt = now
	}
	g.status = StatusEnded
	if termination == TermAborted || termination == TermNoFirstMove || termination == TermWaitingGaveUp {
		g.status = StatusAborted
		result = ""
	}
	g.result, g.termination = result, termination
	g.ended = g.now()
	g.drawOffer = ""
}

// Resign gives the game to the opponent.
func (g *Game) Resign(userID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	side := g.sideOf(userID)
	if side == "" {
		return errNotPlayer
	}
	if g.status != StatusPlaying {
		return errNotPlaying
	}
	g.finish(resultFor(side.opp()), TermResign)
	g.changed()
	return nil
}

// Abort cancels a game that has hardly begun: a waiting one (by its creator)
// or one in which each side has not yet moved.
func (g *Game) Abort(userID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	side := g.sideOf(userID)
	if side == "" {
		return errNotPlayer
	}
	if g.status == StatusWaiting && userID == g.creator || g.status == StatusPlaying && len(g.moves) < 2 {
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
	if side == "" {
		return errNotPlayer
	}
	if g.status != StatusPlaying || len(g.moves) < 2 {
		return errors.New("ничью можно предложить после первых ходов")
	}
	if g.drawOffer == side.opp() {
		g.finish("1/2-1/2", TermAgreement)
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
	if g.sideOf(userID) == "" {
		return errNotPlayer
	}
	if g.drawOffer == "" {
		return nil
	}
	g.drawOffer = ""
	g.changed()
	return nil
}

// AcceptDraw accepts the opponent's draw offer; it does nothing (and says so)
// if there is none, so that a bot answering a stale offer cannot create one.
func (g *Game) AcceptDraw(userID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	side := g.sideOf(userID)
	if side == "" {
		return errNotPlayer
	}
	if g.status != StatusPlaying || g.drawOffer != side.opp() {
		return errors.New("предложения ничьей нет")
	}
	g.finish("1/2-1/2", TermAgreement)
	g.changed()
	return nil
}

// Rematch records a wish for a rematch. When both sides want it, newID is
// used for the new game (colours swapped), which is returned; otherwise nil.
// The caller registers the returned game.
func (g *Game) Rematch(userID, newID string) (*Game, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	side := g.sideOf(userID)
	if side == "" {
		return nil, errNotPlayer
	}
	if g.status != StatusEnded {
		return nil, errors.New("реванш возможен после окончания партии")
	}
	if g.rematchID != "" {
		return nil, nil
	}
	switch g.rematchOffer {
	case "":
		g.rematchOffer = side
		g.changed()
		return nil, nil
	case side: // asked twice: take the offer back
		g.rematchOffer = ""
		g.changed()
		return nil, nil
	}
	// the opponent had asked: play, with the colours swapped
	n := blankGame(newID, g.now, g.tc)
	n.creator = g.creator
	n.created = g.now()
	n.white, n.black = g.black, g.white
	n.rated = g.rated
	n.start()
	g.rematchID = newID
	g.changed()
	return n, nil
}

// ------------------------------------------------------------------- clock

// flagLocked ends the game if the side to move has run out of time; it
// reports whether it did.
func (g *Game) flagLocked(now time.Time) bool {
	if !g.clockRunning() || g.remaining(g.turn, now) > 0 {
		return false
	}
	loser := g.turn
	if !g.canMate(loser.opp()) { // a flag against a bare king is only a draw
		g.finish("1/2-1/2", TermTimeout)
	} else {
		g.finish(resultFor(loser.opp()), TermTimeout)
	}
	g.changed()
	return true
}

// canMate: does the side have enough material to ever checkmate?
func (g *Game) canMate(s Side) bool {
	placement := strings.Fields(g.pos.FEN())[0]
	minors := 0
	for _, r := range placement {
		own := (s == White && r >= 'A' && r <= 'Z') || (s == Black && r >= 'a' && r <= 'z')
		if !own {
			continue
		}
		switch strings.ToLower(string(r)) {
		case "p", "r", "q":
			return true
		case "n", "b":
			minors++
		}
	}
	return minors >= 2
}

// tick is called by the timer: someone may have run out of time or failed
// to make a first move.
func (g *Game) tick() {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	if g.status != StatusPlaying {
		return
	}
	if len(g.moves) < 2 {
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
	if len(g.moves) < 2 {
		wait = firstMoveTimeout - g.now().Sub(g.turnAt)
	} else if g.timed() {
		wait = time.Duration(g.remaining(g.turn, g.now())) * time.Millisecond
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

// View is the game as a client sees it.
type View struct {
	ID           string         `json:"id"`
	Status       string         `json:"status"`
	Result       string         `json:"result,omitempty"`
	Termination  string         `json:"termination,omitempty"`
	White        *Player        `json:"white"`
	Black        *Player        `json:"black"`
	TC           TimeControl    `json:"tc"`
	FEN          string         `json:"fen"`
	Turn         Side           `json:"turn"`
	Moves        []MoveRec      `json:"moves"`
	Legal        []string       `json:"legal,omitempty"` // for the side to move only, and only to that player
	Clock        *Clock         `json:"clock,omitempty"`
	FirstMoveBy  int64          `json:"firstMoveBy,omitempty"` // unix ms deadline while nobody has moved twice
	DrawOffer    Side           `json:"drawOffer,omitempty"`
	Rematch      *Rematch       `json:"rematch,omitempty"`
	Presence     Presence       `json:"presence"`
	Opening      *eco.Opening   `json:"opening,omitempty"` // the opening played so far, live
	Book         bool           `json:"book,omitempty"`    // the position is still theory
	Chat         []ChatMsg      `json:"chat,omitempty"`
	Created      int64          `json:"created"`
	Rated        bool           `json:"rated,omitempty"`
	RatingChange *RatingChanges `json:"ratingChange,omitempty"`
}

// Rematch is the state of the "play again" wish.
type Rematch struct {
	Offer Side   `json:"offer,omitempty"`
	ID    string `json:"id,omitempty"` // the new game, once both agreed
}

// Message is what the socket carries to a client.
type Message struct {
	Type string `json:"type"` // "state" or "error"
	Game *View  `json:"game,omitempty"`
	You  Side   `json:"you,omitempty"` // the colour of this viewer, "" for a spectator
	Err  string `json:"error,omitempty"`
}

// viewLocked builds the view; legal moves are added by the caller.
func (g *Game) viewLocked() *View {
	now := g.now()
	v := &View{
		ID: g.ID, Status: g.status, Result: g.result, Termination: g.termination,
		White: g.white, Black: g.black, TC: g.tc, FEN: g.pos.FEN(), Turn: g.turn,
		Moves: append([]MoveRec{}, g.moves...), DrawOffer: g.drawOffer, Created: g.created.UnixMilli(),
		Opening: g.opening, Book: g.inBook, Chat: g.chat,
		Rated: g.rated, RatingChange: g.change,
	}
	if g.timed() {
		c := &Clock{WhiteMs: g.remaining(White, now), BlackMs: g.remaining(Black, now), ServerNow: now.UnixMilli()}
		if g.clockRunning() {
			c.Running = g.turn
		}
		if c.WhiteMs < 0 {
			c.WhiteMs = 0
		}
		if c.BlackMs < 0 {
			c.BlackMs = 0
		}
		v.Clock = c
	}
	if g.status == StatusPlaying && len(g.moves) < 2 {
		v.FirstMoveBy = g.turnAt.Add(firstMoveTimeout).UnixMilli()
	}
	if g.rematchOffer != "" || g.rematchID != "" {
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

func (g *Game) legalLocked() []string {
	if g.status != StatusPlaying {
		return nil
	}
	ms := g.pos.LegalMoves()
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.String()
	}
	return out
}

// Snapshot is the message for one viewer.
func (g *Game) Snapshot(userID string) []byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.snapshotLocked(userID)
}

func (g *Game) snapshotLocked(userID string) []byte {
	you := g.sideOf(userID)
	v := g.viewLocked()
	if you != "" && you == g.turn {
		v.Legal = g.legalLocked()
	}
	b, _ := json.Marshal(Message{Type: "state", Game: v, You: you})
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
	if g.after != nil {
		g.after(g)
	}
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
	Status       string         `json:"status"`
	Result       string         `json:"result,omitempty"`
	Termination  string         `json:"termination,omitempty"`
	TC           TimeControl    `json:"tc"`
	Creator      string         `json:"creator"`
	White        *Player        `json:"white,omitempty"`
	Black        *Player        `json:"black,omitempty"`
	Moves        []MoveRec      `json:"moves"`
	WhiteMs      int64          `json:"whiteMs"`
	BlackMs      int64          `json:"blackMs"`
	TurnAtMs     int64          `json:"turnAtMs"`
	CreatedMs    int64          `json:"createdMs"`
	StartedMs    int64          `json:"startedMs,omitempty"`
	EndedMs      int64          `json:"endedMs,omitempty"`
	DrawOffer    Side           `json:"drawOffer,omitempty"`
	RematchOffer Side           `json:"rematchOffer,omitempty"`
	RematchID    string         `json:"rematchId,omitempty"`
	Chat         []ChatMsg      `json:"chat,omitempty"`
	Rated        bool           `json:"rated,omitempty"`
	RatingChange *RatingChanges `json:"ratingChange,omitempty"`
}

func (g *Game) stateLocked() []byte {
	s := saved{
		Status: g.status, Result: g.result, Termination: g.termination, TC: g.tc, Creator: g.creator,
		White: g.white, Black: g.black, Moves: g.moves, WhiteMs: g.ms[White], BlackMs: g.ms[Black],
		TurnAtMs: g.turnAt.UnixMilli(), CreatedMs: g.created.UnixMilli(),
		DrawOffer: g.drawOffer, RematchOffer: g.rematchOffer, RematchID: g.rematchID, Chat: g.chat,
		Rated: g.rated, RatingChange: g.change,
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

// restore rebuilds a game from its saved state by replaying the moves.
func restore(id string, now func() time.Time, state []byte) (*Game, error) {
	var s saved
	if err := json.Unmarshal(state, &s); err != nil {
		return nil, err
	}
	g := blankGame(id, now, s.TC)
	g.status, g.result, g.termination, g.creator = s.Status, s.Result, s.Termination, s.Creator
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
	g.recorded = g.status == StatusEnded || g.status == StatusAborted
	g.rated, g.change = s.Rated, s.RatingChange
	g.rateDone = g.recorded // a game that ended before a restart is not rated again
	for i, m := range s.Moves {
		mv, ok := findMove(g.pos, m.UCI)
		if !ok {
			return nil, fmt.Errorf("saved game %s: move %d (%s) is illegal", id, i+1, m.UCI)
		}
		g.pos.Play(mv)
		g.hashes = append(g.hashes, g.pos.Hash())
		g.moves = append(g.moves, m)
		g.trackOpening()
		g.turn = g.turn.opp()
	}
	g.chat = s.Chat
	return g, nil
}

func findMove(p *engine.Position, uci string) (engine.Move, bool) {
	for _, m := range p.LegalMoves() {
		if m.String() == uci {
			return m, true
		}
	}
	return engine.Move{}, false
}

// PGN writes the game for the review.
func (g *Game) PGN() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	name := func(p *Player) string {
		if p == nil {
			return "?"
		}
		return strings.NewReplacer(`"`, "'", `\`, "'", "\n", " ", "\r", " ").Replace(p.Name)
	}
	res := g.result
	if res == "" {
		res = "*"
	}
	var b strings.Builder
	date := g.created.Format("2006.01.02")
	fmt.Fprintf(&b, "[Event \"Онлайн-партия\"]\n[Site \"Dyanis Chess\"]\n[Date \"%s\"]\n[White \"%s\"]\n[Black \"%s\"]\n[Result \"%s\"]\n",
		date, name(g.white), name(g.black), res)
	if g.timed() {
		fmt.Fprintf(&b, "[TimeControl \"%d+%d\"]\n", g.tc.BaseMs/1000, g.tc.IncMs/1000)
	}
	if g.termination != "" {
		fmt.Fprintf(&b, "[Termination \"%s\"]\n", g.termination)
	}
	b.WriteString("\n")
	for i, m := range g.moves {
		if i%2 == 0 {
			fmt.Fprintf(&b, "%d. ", i/2+1)
		}
		b.WriteString(m.SAN + " ")
	}
	b.WriteString(res + "\n")
	return b.String()
}

// botSeat returns the seat a computer plays, if one does.
func (g *Game) botSeat() (Side, *Player) {
	switch {
	case g.white.IsBot():
		return White, g.white
	case g.black.IsBot():
		return Black, g.black
	}
	return "", nil
}

// humanOf returns the person playing against a bot.
func (g *Game) humanOf() (Side, *Player) {
	bs, _ := g.botSeat()
	switch bs {
	case White:
		return Black, g.black
	case Black:
		return White, g.white
	}
	return "", nil
}

// trackOpening updates the opening after a move has been played: the name of
// the last named position the game went through, and whether it is still theory.
func (g *Game) trackOpening() {
	h := g.hashes[len(g.hashes)-1]
	book := eco.Default()
	if o, ok := book.Lookup(h); ok {
		g.opening = &o
	}
	g.inBook = book.InBook(h)
}

// Chat adds a line said by a player. Spectators read but do not write, a bot
// does not chat, and the text is cleaned and limited.
func (g *Game) Chat(userID, text string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	side := g.sideOf(userID)
	if side == "" {
		return errors.New("писать в чат могут только игроки")
	}
	if g.white == nil || g.black == nil {
		return errors.New("чат откроется, когда придёт соперник")
	}
	if g.white.IsBot() || g.black.IsBot() {
		return errors.New("боты не пишут в чат")
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

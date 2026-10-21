package vplay

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"dyanis-chess/backend/internal/variants"
)

// Bughouse is the one game of the package that is not between two people: four players (two teams of two) on two
// boards, a clock for each of the four, one result for the whole game. A State (variants.State) has one board and
// two sides, so a bughouse game is its own type, BugGame, beside Game. It reuses what is shared: the WebSocket,
// the snapshots, the saver, the time control, the chat's cleaning, the first-move limit. The hub reaches both
// through the `room` interface below; Game itself is not touched (it only gets two tiny methods here).
//
// Seats: 0 board 1 White, 1 board 1 Black, 2 board 2 White, 3 board 2 Black (variants.BugSeat). Team 0 is seats
// 0 and 3 (result "1-0"), team 1 seats 1 and 2 ("0-1"). A player acts only on their own board; the server picks
// the board from the seat, a client never names one.

// room is what the hub's routes need from a game, of two seats or of four.
type room interface {
	Snapshot(userID string) []byte
	joinSeat(p Player, seat int) error // seat < 0: any free place
	addConn(c *conn)
	removeConn(c *conn)
	dispatch(h *Hub, userID string, in incoming) error
}

func (g *Game) joinSeat(p Player, _ int) error { return g.Join(p) }

func (g *Game) dispatch(h *Hub, uid string, in incoming) error { return h.handle(g, uid, in) }

// BugMove is one action of a bughouse game with the board and the seat it was made from.
type BugMove struct {
	Board  int    `json:"board"`
	Seat   int    `json:"seat"`
	Side   Side   `json:"side"`
	Action string `json:"action"`
	SAN    string `json:"san"`
}

// BugChatMsg is a line of the chat of the four.
type BugChatMsg struct {
	Seat int    `json:"seat"`
	Text string `json:"text"`
	At   int64  `json:"at"`
}

// BugGame is one game of bughouse. All fields are guarded by mu.
type BugGame struct {
	mu  sync.Mutex
	ID  string
	now func() time.Time

	vid, vname string
	opts       variants.Options
	st         *variants.Bug
	tc         TimeControl
	status     string
	result     string // "1-0": team 0, "0-1": team 1
	term       string
	creator    string
	seats      [variants.BugSeats]*Player

	moves   []BugMove
	plies   [2]int                   // actions made on each board
	ms      [variants.BugSeats]int64 // time left of each seat as of turnAt of its board
	turnAt  [2]time.Time             // when the side to act on each board was given the turn
	created time.Time
	started time.Time
	ended   time.Time

	rematchOffers [variants.BugSeats]bool
	rematchID     string

	conns map[*conn]struct{}
	timer *time.Timer

	persist func(g *BugGame, state []byte)

	chat    []BugChatMsg
	lastSay map[int]time.Time
}

func blankBugGame(id string, now func() time.Time, v variants.Variant, o variants.Options, tc TimeControl) (*BugGame, error) {
	bv, ok := v.(variants.BugVariant)
	if !ok {
		return nil, errors.New("это не игра на двух досках")
	}
	st, err := bv.NewBug(o)
	if err != nil {
		return nil, err
	}
	info := v.Info()
	g := &BugGame{
		ID: id, now: now, vid: info.ID, vname: info.Name, opts: o, st: st, tc: tc, status: StatusWaiting,
		conns: map[*conn]struct{}{}, lastSay: map[int]time.Time{}, created: now(),
	}
	for i := range g.ms {
		g.ms[i] = tc.BaseMs
	}
	return g, nil
}

// newBugGame makes a waiting game with its creator seated.
func newBugGame(id string, now func() time.Time, v variants.Variant, o variants.Options, tc TimeControl, creator Player, seat int) (*BugGame, error) {
	g, err := blankBugGame(id, now, v, o, tc)
	if err != nil {
		return nil, err
	}
	g.creator = creator.ID
	p := creator
	g.seats[seat] = &p
	return g, nil
}

func (g *BugGame) timed() bool { return g.tc.BaseMs > 0 }

// clockRunning: the clocks of a board stand still until both its players have moved once (each has only
// firstMoveTimeout to start).
func (g *BugGame) clockRunning(board int) bool {
	return g.status == StatusPlaying && g.timed() && g.plies[board] >= 2
}

func (g *BugGame) remaining(seat int, now time.Time) int64 {
	board, side := variants.BugPlace(seat)
	if g.clockRunning(board) && side == g.st.Turn(board) {
		return g.ms[seat] - now.Sub(g.turnAt[board]).Milliseconds()
	}
	return g.ms[seat]
}

func (g *BugGame) seatOf(userID string) int {
	if userID == "" {
		return -1
	}
	for i, p := range g.seats {
		if p != nil && p.ID == userID {
			return i
		}
	}
	return -1
}

func (g *BugGame) full() bool {
	for _, p := range g.seats {
		if p == nil {
			return false
		}
	}
	return true
}

// ----------------------------------------------------------------- actions

var errBugNoDraw = errors.New("в шведских шахматах ничьей по согласию нет")

// joinSeat seats p at the place (or at the first free one when seat < 0) and starts the game when all four are
// there. Someone already seated may take another free place while the game waits.
func (g *BugGame) joinSeat(p Player, seat int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.status != StatusWaiting {
		return errors.New("в партию уже нельзя вступить")
	}
	if seat >= variants.BugSeats {
		return errors.New("такого места нет")
	}
	mine := g.seatOf(p.ID)
	if mine >= 0 && seat < 0 {
		return errors.New("вы уже в этой партии: отправьте ссылку остальным")
	}
	if seat < 0 {
		for i, o := range g.seats {
			if o == nil {
				seat = i
				break
			}
		}
		if seat < 0 {
			return errors.New("все места заняты")
		}
	}
	if g.seats[seat] != nil {
		if mine == seat {
			return nil
		}
		return errors.New("это место уже занято")
	}
	if mine >= 0 {
		g.seats[mine] = nil
	}
	pc := p
	g.seats[seat] = &pc
	if g.full() {
		g.start()
	}
	g.changed()
	return nil
}

// leave frees the seat of a player while the game still waits.
func (g *BugGame) leave(userID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	seat := g.seatOf(userID)
	if seat < 0 {
		return errNotPlayer
	}
	if g.status != StatusWaiting {
		return errors.New("из начатой партии можно только сдаться")
	}
	g.seats[seat] = nil
	g.changed()
	return nil
}

func (g *BugGame) start() {
	now := g.now()
	g.status = StatusPlaying
	g.started = now
	g.turnAt = [2]time.Time{now, now}
}

// Act performs one action for the user, on the board of their seat.
func (g *BugGame) Act(userID, action string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	seat := g.seatOf(userID)
	if seat < 0 {
		return errNotPlayer
	}
	if g.status != StatusPlaying {
		return errNotPlaying
	}
	now := g.now()
	if g.flagLocked(now) {
		return errNotPlaying
	}
	board, side := variants.BugPlace(seat)
	if g.st.Turn(board) != side {
		return errNotYours
	}
	running := g.clockRunning(board)
	note, err := g.st.Apply(board, action)
	if err != nil {
		return err
	}
	g.moves = append(g.moves, BugMove{Board: board, Seat: seat, Side: side, Action: action, SAN: note})
	if running { // the turn on this board passed: the seat's clock stops, with the increment
		g.ms[seat] = g.ms[seat] - now.Sub(g.turnAt[board]).Milliseconds() + g.tc.IncMs
	}
	g.plies[board]++
	g.turnAt[board] = now
	if res, reason, over := g.st.Over(); over {
		g.finish(res, reason)
	}
	g.changed()
	return nil
}

// finish ends the game. Callers hold the lock and call changed.
func (g *BugGame) finish(result, termination string) {
	now := g.now()
	for b := 0; b < 2; b++ { // stop the running clocks where they stand, so the final times are true
		if g.clockRunning(b) {
			seat := variants.BugSeat(b, g.st.Turn(b))
			g.ms[seat] = max(0, g.ms[seat]-now.Sub(g.turnAt[b]).Milliseconds())
			g.turnAt[b] = now
		}
	}
	g.status = StatusEnded
	if termination == TermAborted || termination == TermNoFirstMove || termination == TermWaitingGaveUp {
		g.status = StatusAborted
		result = ""
	}
	g.result, g.term = result, termination
	g.ended = now
	if _, _, over := g.st.Over(); !over && g.status == StatusEnded {
		g.st.Finish(result, termination)
	}
}

// Resign gives the game to the other team.
func (g *BugGame) Resign(userID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	seat := g.seatOf(userID)
	if seat < 0 {
		return errNotPlayer
	}
	if g.status != StatusPlaying {
		return errNotPlaying
	}
	g.finish(variants.BugResultFor(1-variants.BugTeam(seat)), TermResign)
	g.changed()
	return nil
}

// Abort cancels a game that has not begun (by its creator) or in which no action was made yet (by any player).
func (g *BugGame) Abort(userID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	switch {
	case g.status == StatusWaiting && userID != "" && userID == g.creator,
		g.status == StatusPlaying && g.seatOf(userID) >= 0 && len(g.moves) == 0:
		g.finish("", TermAborted)
		g.changed()
		return nil
	case g.seatOf(userID) < 0 && userID != g.creator:
		return errNotPlayer
	}
	return errors.New("партию уже нельзя отменить, можно сдаться")
}

// Rematch records a wish to play again; when all four have it the new game (the same teams with the colours
// swapped on both boards) is returned for the caller to register.
func (g *BugGame) Rematch(userID string, mk func() (*BugGame, error)) (*BugGame, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	seat := g.seatOf(userID)
	if seat < 0 {
		return nil, errNotPlayer
	}
	if g.status != StatusEnded {
		return nil, errors.New("реванш возможен после окончания партии")
	}
	if g.rematchID != "" {
		return nil, nil
	}
	g.rematchOffers[seat] = !g.rematchOffers[seat]
	all := true
	for _, o := range g.rematchOffers {
		all = all && o
	}
	if !all {
		g.changed()
		return nil, nil
	}
	n, err := mk()
	if err != nil {
		g.rematchOffers[seat] = false
		return nil, err
	}
	n.mu.Lock() // n is already registered (mk does that), so a client that hears of it at once may read it
	n.creator = g.creator
	for i := range n.seats { // partners stay partners: seat i and seat i^1 trade places
		n.seats[i] = g.seats[i^1]
	}
	n.start()
	n.mu.Unlock()
	g.rematchID = n.ID
	g.changed()
	return n, nil
}

// ------------------------------------------------------------------- clock

// flagLocked ends the game if a player to act has run out of time; it reports whether it did.
func (g *BugGame) flagLocked(now time.Time) bool {
	loser, worst := -1, int64(1)
	for b := 0; b < 2; b++ {
		if !g.clockRunning(b) {
			continue
		}
		seat := variants.BugSeat(b, g.st.Turn(b))
		if r := g.remaining(seat, now); r <= 0 && (loser < 0 || r < worst) {
			loser, worst = seat, r
		}
	}
	if loser < 0 {
		return false
	}
	g.finish(variants.BugResultFor(1-variants.BugTeam(loser)), TermTimeout)
	g.changed()
	return true
}

func (g *BugGame) tick() {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	if g.status != StatusPlaying {
		return
	}
	for b := 0; b < 2; b++ {
		if g.plies[b] < 2 && now.Sub(g.turnAt[b]) >= firstMoveTimeout {
			g.finish("", TermNoFirstMove)
			g.changed()
			return
		}
	}
	if g.flagLocked(now) {
		return
	}
	g.armLocked()
}

// armLocked schedules the next moment something can happen on its own: a flag falling or the first-move limit.
func (g *BugGame) armLocked() {
	if g.timer != nil {
		g.timer.Stop()
		g.timer = nil
	}
	if g.status != StatusPlaying {
		return
	}
	now := g.now()
	wait, have := time.Duration(0), false
	consider := func(d time.Duration) {
		if !have || d < wait {
			wait, have = d, true
		}
	}
	for b := 0; b < 2; b++ {
		switch {
		case g.plies[b] < 2:
			consider(firstMoveTimeout - now.Sub(g.turnAt[b]))
		case g.timed():
			consider(time.Duration(g.remaining(variants.BugSeat(b, g.st.Turn(b)), now)) * time.Millisecond)
		}
	}
	if !have {
		return
	}
	wait = max(wait, 0)
	g.timer = time.AfterFunc(wait+5*time.Millisecond, g.tick)
}

// Close stops the timer; the game is being forgotten.
func (g *BugGame) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.timer != nil {
		g.timer.Stop()
		g.timer = nil
	}
}

// ---------------------------------------------------------------- snapshots

// BugClock is the clock part of a snapshot: the time left of each seat as of ServerNow (unix ms); the seats in
// Running (one per board whose clock goes) count down from there.
type BugClock struct {
	Ms        [variants.BugSeats]int64 `json:"ms"`
	Running   []int                    `json:"running"`
	ServerNow int64                    `json:"serverNow"`
}

// BugPresence says which seats are connected right now.
type BugPresence struct {
	Seats    [variants.BugSeats]bool `json:"seats"`
	Watching int                     `json:"watching"`
}

// BugRematch is the state of the "play again" wish of the four.
type BugRematch struct {
	Offers [variants.BugSeats]bool `json:"offers"`
	ID     string                  `json:"id,omitempty"`
}

// BugBoardView is one board of a snapshot.
type BugBoardView struct {
	Turn     Side          `json:"turn"`
	Position variants.View `json:"position"`
}

// BugView is a bughouse game as one client sees it. YourSeat is -1 for a spectator.
type BugView struct {
	ID          string                     `json:"id"`
	Variant     string                     `json:"variant"`
	VariantName string                     `json:"variantName"`
	Status      string                     `json:"status"`
	Result      string                     `json:"result,omitempty"`
	Termination string                     `json:"termination,omitempty"`
	Players     [variants.BugSeats]*Player `json:"players"`
	YourSeat    int                        `json:"yourSeat"`
	TC          TimeControl                `json:"tc"`
	Boards      [2]BugBoardView            `json:"boards"`
	Moves       []BugMove                  `json:"moves"`
	Clock       *BugClock                  `json:"clock,omitempty"`
	FirstMoveBy int64                      `json:"firstMoveBy,omitempty"`
	Rematch     *BugRematch                `json:"rematch,omitempty"`
	Presence    BugPresence                `json:"presence"`
	Chat        []BugChatMsg               `json:"chat,omitempty"`
	Created     int64                      `json:"created"`
}

// BugMessage is what the socket carries to a client (the same shape as Message).
type BugMessage struct {
	Type string   `json:"type"` // "state" or "error"
	Game *BugView `json:"game,omitempty"`
	You  Side     `json:"you,omitempty"` // the colour of the viewer's seat, "" for a spectator
	Err  string   `json:"error,omitempty"`
}

func (g *BugGame) viewLocked(seat int) *BugView {
	now := g.now()
	v := &BugView{
		ID: g.ID, Variant: g.vid, VariantName: g.vname, Status: g.status, Result: g.result, Termination: g.term,
		Players: g.seats, YourSeat: seat, TC: g.tc, Moves: append([]BugMove{}, g.moves...), Chat: g.chat, Created: g.created.UnixMilli(),
	}
	myBoard, mySide := -1, variants.NoSide
	if seat >= 0 {
		myBoard, mySide = variants.BugPlace(seat)
	}
	for b := 0; b < 2; b++ {
		viewer := variants.NoSide
		if b == myBoard {
			viewer = mySide
		}
		v.Boards[b] = BugBoardView{Turn: g.st.Turn(b), Position: g.st.View(b, viewer)}
		if g.status != StatusPlaying {
			v.Boards[b].Position.Legal = nil
		}
	}
	if g.timed() {
		c := &BugClock{ServerNow: now.UnixMilli(), Running: []int{}}
		for s := range c.Ms {
			c.Ms[s] = max(0, g.remaining(s, now))
		}
		for b := 0; b < 2; b++ {
			if g.clockRunning(b) {
				c.Running = append(c.Running, variants.BugSeat(b, g.st.Turn(b)))
			}
		}
		v.Clock = c
	}
	if g.status == StatusPlaying {
		for b := 0; b < 2; b++ {
			if g.plies[b] < 2 {
				if by := g.turnAt[b].Add(firstMoveTimeout).UnixMilli(); v.FirstMoveBy == 0 || by < v.FirstMoveBy {
					v.FirstMoveBy = by
				}
			}
		}
	}
	for _, o := range g.rematchOffers {
		if o || g.rematchID != "" {
			v.Rematch = &BugRematch{Offers: g.rematchOffers, ID: g.rematchID}
			break
		}
	}
	for c := range g.conns {
		if s := g.seatOf(c.userID); s >= 0 {
			v.Presence.Seats[s] = true
		} else {
			v.Presence.Watching++
		}
	}
	return v
}

// Snapshot is the message for one viewer.
func (g *BugGame) Snapshot(userID string) []byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.snapshotLocked(userID)
}

func (g *BugGame) snapshotLocked(userID string) []byte {
	seat := g.seatOf(userID)
	m := BugMessage{Type: "state", Game: g.viewLocked(seat)}
	if seat >= 0 {
		_, m.You = variants.BugPlace(seat)
	}
	b, _ := json.Marshal(m)
	return b
}

func (g *BugGame) changed() {
	g.armLocked()
	if g.persist != nil {
		g.persist(g, g.stateLocked())
	}
	g.broadcastLocked()
}

func (g *BugGame) broadcastLocked() {
	for c := range g.conns {
		if !c.offer(g.snapshotLocked(c.userID)) {
			c.shut()
			delete(g.conns, c)
		}
	}
}

func (g *BugGame) addConn(c *conn) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.conns[c] = struct{}{}
	g.broadcastLocked()
}

func (g *BugGame) removeConn(c *conn) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.conns[c]; ok {
		delete(g.conns, c)
		g.broadcastLocked()
	}
}

// Chat adds a line said by one of the four (spectators read only).
func (g *BugGame) Chat(userID, text string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	seat := g.seatOf(userID)
	if seat < 0 {
		return errors.New("писать в чат могут только игроки")
	}
	if g.status != StatusPlaying && g.status != StatusEnded {
		return errors.New("чат откроется, когда соберутся все четверо")
	}
	text = cleanChat(text)
	if text == "" {
		return nil
	}
	now := g.now()
	if last, ok := g.lastSay[seat]; ok && now.Sub(last) < chatGap {
		return errors.New("не так быстро")
	}
	g.lastSay[seat] = now
	g.chat = append(g.chat, BugChatMsg{Seat: seat, Text: text, At: now.UnixMilli()})
	if len(g.chat) > maxChatKept {
		g.chat = append([]BugChatMsg(nil), g.chat[len(g.chat)-maxChatKept:]...)
	}
	g.changed()
	return nil
}

// ------------------------------------------------------------------- state

// bugSaved is the part of a bughouse game that is written to the database. The "seats" key tells it from the
// document of a game of two (saved).
type bugSaved struct {
	Variant       string                     `json:"variant"`
	Options       variants.Options           `json:"options"`
	Status        string                     `json:"status"`
	Result        string                     `json:"result,omitempty"`
	Termination   string                     `json:"termination,omitempty"`
	TC            TimeControl                `json:"tc"`
	Creator       string                     `json:"creator"`
	Seats         [variants.BugSeats]*Player `json:"seats"`
	Moves         []BugMove                  `json:"moves"`
	Ms            [variants.BugSeats]int64   `json:"ms"`
	TurnAtMs      [2]int64                   `json:"turnAtMs"`
	CreatedMs     int64                      `json:"createdMs"`
	StartedMs     int64                      `json:"startedMs,omitempty"`
	EndedMs       int64                      `json:"endedMs,omitempty"`
	RematchOffers [variants.BugSeats]bool    `json:"rematchOffers"`
	RematchID     string                     `json:"rematchId,omitempty"`
	Chat          []BugChatMsg               `json:"chat,omitempty"`
}

func (g *BugGame) stateLocked() []byte {
	s := bugSaved{
		Variant: g.vid, Options: g.opts, Status: g.status, Result: g.result, Termination: g.term, TC: g.tc, Creator: g.creator,
		Seats: g.seats, Moves: g.moves, Ms: g.ms, TurnAtMs: [2]int64{g.turnAt[0].UnixMilli(), g.turnAt[1].UnixMilli()},
		CreatedMs: g.created.UnixMilli(), RematchOffers: g.rematchOffers, RematchID: g.rematchID, Chat: g.chat,
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

// isBugState says whether a saved document is that of a bughouse game.
func isBugState(state []byte) bool {
	var probe struct {
		Seats json.RawMessage `json:"seats"`
	}
	return json.Unmarshal(state, &probe) == nil && len(probe.Seats) > 0
}

// restoreBug rebuilds a game from its saved state by replaying the actions.
func restoreBug(id string, now func() time.Time, state []byte) (*BugGame, error) {
	var s bugSaved
	if err := json.Unmarshal(state, &s); err != nil {
		return nil, err
	}
	v, ok := variants.Get(s.Variant)
	if !ok {
		return nil, fmt.Errorf("saved game %s: unknown variant %q", id, s.Variant)
	}
	g, err := blankBugGame(id, now, v, s.Options, s.TC)
	if err != nil {
		return nil, err
	}
	actions := make([]variants.BugAction, len(s.Moves))
	for i, m := range s.Moves {
		actions[i] = variants.BugAction{Board: m.Board, Action: m.Action}
		g.plies[m.Board]++
	}
	st, err := variants.ReplayBug(s.Options, actions)
	if err != nil {
		return nil, fmt.Errorf("saved game %s: %w", id, err)
	}
	g.st = st
	g.moves = s.Moves
	g.status, g.result, g.term, g.creator = s.Status, s.Result, s.Termination, s.Creator
	g.seats = s.Seats
	g.ms = s.Ms
	g.turnAt = [2]time.Time{time.UnixMilli(s.TurnAtMs[0]), time.UnixMilli(s.TurnAtMs[1])}
	g.created = time.UnixMilli(s.CreatedMs)
	if s.StartedMs != 0 {
		g.started = time.UnixMilli(s.StartedMs)
	}
	if s.EndedMs != 0 {
		g.ended = time.UnixMilli(s.EndedMs)
	}
	g.rematchOffers, g.rematchID = s.RematchOffers, s.RematchID
	g.chat = s.Chat
	if g.status == StatusEnded {
		if _, _, over := g.st.Over(); !over { // ended by a resignation or a flag
			g.st.Finish(g.result, g.term)
		}
	}
	return g, nil
}

// dispatch performs what a client asked for.
func (g *BugGame) dispatch(h *Hub, uid string, in incoming) error {
	switch in.Type {
	case "move":
		return g.Act(uid, in.Action)
	case "resign":
		return g.Resign(uid)
	case "abort":
		return g.Abort(uid)
	case "chat":
		return g.Chat(uid, in.Text)
	case "draw", "drawDecline":
		return errBugNoDraw
	case "leave":
		return g.leave(uid)
	case "seat": // a seated player takes another free place while the game waits
		if in.Seat == nil {
			return errors.New("не указано место")
		}
		g.mu.Lock()
		seat := g.seatOf(uid)
		var p Player
		if seat >= 0 {
			p = *g.seats[seat]
		}
		g.mu.Unlock()
		if seat < 0 {
			return errNotPlayer
		}
		return g.joinSeat(p, *in.Seat)
	case "rematch":
		return h.rematchBug(g, uid)
	}
	return nil
}

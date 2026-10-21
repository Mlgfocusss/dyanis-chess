package vplay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dyanis-chess/backend/internal/variants"
)

const bugWhite = `{"variant":"bughouse","color":"white","timeMs":0}`

func bugSnapshot(t *testing.T, srv *httptest.Server, user, id string) BugMessage {
	t.Helper()
	code, b := call(t, srv, user, "GET", "/api/variants/games/"+id, "")
	var m BugMessage
	if code != 200 || json.Unmarshal(b, &m) != nil {
		t.Fatalf("snapshot: %d %s", code, b)
	}
	return m
}

func joinAt(t *testing.T, srv *httptest.Server, user, id string, seat int) int {
	t.Helper()
	body := ""
	if seat >= 0 {
		body = string(mustJSON(map[string]int{"seat": seat}))
	}
	code, _ := call(t, srv, user, "POST", "/api/variants/games/"+id+"/join", body)
	return code
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// bugSocket is a socket that reads the snapshots of a game of four.
type bugSocket struct{ *socket }

func dialBug(t *testing.T, srv *httptest.Server, user, id string) bugSocket {
	return bugSocket{dial(t, srv, user, id)}
}

func (s bugSocket) state(ok func(*BugView) bool) BugMessage {
	s.t.Helper()
	for i := 0; i < 60; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, data, err := s.c.Read(ctx)
		cancel()
		if err != nil {
			s.t.Fatalf("read: %v", err)
		}
		var m BugMessage
		if err := json.Unmarshal(data, &m); err != nil {
			s.t.Fatalf("bad message %s: %v", data, err)
		}
		if m.Type == "state" && ok(m.Game) {
			return m
		}
	}
	s.t.Fatal("the awaited snapshot never came")
	return BugMessage{}
}

// four seats the four of a game: anna board 1 White (seat 0), carol board 1 Black (1), dave board 2 White (2),
// boris board 2 Black (3).
func fourPlayers(t *testing.T, srv *httptest.Server) (id string, s [4]bugSocket) {
	t.Helper()
	id = createGame(t, srv, "anna", bugWhite)
	if m := bugSnapshot(t, srv, "anna", id); m.Game.Status != StatusWaiting || m.Game.Players[0] == nil || m.Game.YourSeat != 0 {
		t.Fatalf("a created game: %+v", m.Game)
	}
	if code := joinAt(t, srv, "boris", id, 3); code != http.StatusOK {
		t.Fatalf("boris takes seat 3: %d", code)
	}
	if code := joinAt(t, srv, "carol", id, -1); code != http.StatusOK { // the first free place: seat 1
		t.Fatalf("carol: %d", code)
	}
	if m := bugSnapshot(t, srv, "carol", id); m.Game.YourSeat != 1 || m.Game.Status != StatusWaiting {
		t.Fatalf("carol sits at %d, status %s", m.Game.YourSeat, m.Game.Status)
	}
	if code := joinAt(t, srv, "dave", id, 2); code != http.StatusOK {
		t.Fatalf("dave: %d", code)
	}
	for i, u := range []string{"anna", "carol", "dave", "boris"} {
		s[i] = dialBug(t, srv, u, id)
		s[i].state(func(v *BugView) bool { return v.Status == StatusPlaying })
	}
	return id, s
}

func TestBughouseSeatsAreTakenOneByOne(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	id := createGame(t, srv, "anna", bugWhite)
	if joinAt(t, srv, "anna", id, -1) != http.StatusConflict {
		t.Error("the creator is already seated")
	}
	if joinAt(t, srv, "boris", id, 0) != http.StatusConflict {
		t.Error("seat 0 is taken")
	}
	if joinAt(t, srv, "boris", id, 7) != http.StatusConflict {
		t.Error("there is no seat 7")
	}
	joinAt(t, srv, "boris", id, 2)
	// a seated player may move to another free place while the game waits, over the socket
	b := dialBug(t, srv, "boris", id)
	b.send("seat", map[string]any{"seat": 3})
	m := b.state(func(v *BugView) bool { return v.YourSeat == 3 })
	if m.Game.Players[2] != nil || m.Game.Players[3] == nil || m.Game.Players[3].ID != "boris" {
		t.Errorf("players after the move: %v", m.Game.Players)
	}
	b.send("seat", map[string]any{"seat": 0})
	b.state(func(v *BugView) bool { return v.YourSeat == 3 && v.Players[0].ID == "anna" }) // taken: stays
	b.send("leave", nil)
	m = b.state(func(v *BugView) bool { return v.YourSeat == -1 })
	if m.Game.Players[3] != nil {
		t.Error("the seat was not freed")
	}
	joinAt(t, srv, "boris", id, -1)
	joinAt(t, srv, "carol", id, -1)
	if code := joinAt(t, srv, "dave", id, -1); code != http.StatusOK {
		t.Fatalf("the last player: %d", code)
	}
	if got := bugSnapshot(t, srv, "anna", id).Game.Status; got != StatusPlaying {
		t.Errorf("all four are seated, the status is %s", got)
	}
	if joinAt(t, srv, "erin", id, -1) != http.StatusConflict {
		t.Error("a fifth player cannot join")
	}
	if code, _ := call(t, srv, "anna", "POST", "/api/variants/seek", `{"variant":"bughouse","baseMs":60000,"incMs":0}`); code != http.StatusBadRequest {
		t.Errorf("the quick game for bughouse: %d", code)
	}
}

func TestBughouseCaptureReachesThePartnerOverTheSockets(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	id, s := fourPlayers(t, srv)
	anna, carol, dave, boris := s[0], s[1], s[2], s[3]

	act := func(who bugSocket, action string, moves int) {
		t.Helper()
		who.send("move", map[string]any{"action": action})
		for _, o := range s {
			o.state(func(v *BugView) bool { return len(v.Moves) == moves })
		}
	}
	act(anna, "e2e4", 1)
	act(carol, "d7d5", 2)
	act(anna, "e4d5", 3) // anna takes a pawn: boris, her partner on the other board, gets it

	m := bugSnapshot(t, srv, "boris", id)
	var extra variants.PocketExtra
	raw, _ := json.Marshal(m.Game.Boards[1].Position.Extra)
	json.Unmarshal(raw, &extra)
	if extra.Black["P"] != 1 {
		t.Fatalf("boris holds %v", extra)
	}
	// only the player to move on a board gets legal actions, and only on their own board
	if len(m.Game.Boards[0].Position.Legal) != 0 || len(m.Game.Boards[1].Position.Legal) != 0 {
		t.Errorf("boris is not to move: %v %v", m.Game.Boards[0].Position.Legal, m.Game.Boards[1].Position.Legal)
	}
	act(dave, "e2e4", 4)
	m = bugSnapshot(t, srv, "boris", id)
	drop := false
	for _, a := range m.Game.Boards[1].Position.Legal {
		drop = drop || a == "P@e5"
	}
	if !drop || len(m.Game.Boards[0].Position.Legal) != 0 {
		t.Fatalf("boris may drop the pawn on his own board only: %v", m.Game.Boards[1].Position.Legal)
	}
	// the partner of a board cannot play on it: carol's e7e5 sent from boris's seat is a move on board 2
	act(boris, "P@e5", 5)
	last := bugSnapshot(t, srv, "anna", id).Game.Moves[4]
	if last.Board != 1 || last.Seat != 3 || last.SAN != "P@e5" {
		t.Errorf("record of the drop: %+v", last)
	}
}

func TestBughouseMateOnOneBoardEndsForAll(t *testing.T) {
	h, srv := newTestHub(t, &memStore{})
	id, s := fourPlayers(t, srv)
	dave, boris := s[2], s[3]
	steps := []struct {
		who    bugSocket
		action string
	}{{dave, "f2f3"}, {boris, "e7e5"}, {dave, "g2g4"}, {boris, "d8h4"}}
	for i, st := range steps {
		st.who.send("move", map[string]any{"action": st.action})
		for _, o := range s {
			o.state(func(v *BugView) bool { return len(v.Moves) == i+1 })
		}
	}
	for i, o := range s {
		m := bugSnapshot(t, srv, []string{"anna", "carol", "dave", "boris"}[i], id)
		_ = o
		// boris is seat 3: team 0 (with anna) wins
		if m.Game.Result != "1-0" || m.Game.Termination != "checkmate" {
			t.Errorf("seat %d sees %s %s", i, m.Game.Result, m.Game.Termination)
		}
		if len(m.Game.Boards[0].Position.Legal)+len(m.Game.Boards[1].Position.Legal) != 0 {
			t.Errorf("seat %d is offered moves after the end", i)
		}
	}
	if code, _ := call(t, srv, "anna", "POST", "/api/variants/games/"+id+"/join", ""); code != http.StatusConflict {
		t.Error("joining an ended game")
	}
	// rematch needs all four; partners stay partners and the colours swap on both boards
	for i, o := range s {
		o.send("rematch", nil)
		if i < 3 {
			s[0].state(func(v *BugView) bool { return v.Rematch != nil && v.Rematch.Offers[i] })
		}
	}
	m := s[0].state(func(v *BugView) bool { return v.Rematch != nil && v.Rematch.ID != "" })
	next := bugSnapshot(t, srv, "anna", m.Game.Rematch.ID).Game
	if next.Status != StatusPlaying || next.Players[0].ID != "carol" || next.Players[1].ID != "anna" || next.Players[2].ID != "boris" || next.Players[3].ID != "dave" {
		t.Errorf("rematch seats: %v %v %v %v", next.Players[0], next.Players[1], next.Players[2], next.Players[3])
	}
	_ = h
}

func TestBughouseResignChatAbortAndNoDraw(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	id, s := fourPlayers(t, srv)
	anna, carol := s[0], s[1]

	anna.send("draw", nil)
	var errMsg string
	for i := 0; i < 5 && errMsg == ""; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, data, err := anna.c.Read(ctx)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		var m BugMessage
		json.Unmarshal(data, &m)
		errMsg = m.Err
	}
	if errMsg == "" {
		t.Error("a draw offer must be refused")
	}

	carol.send("chat", map[string]any{"text": "  привет  всем "})
	m := anna.state(func(v *BugView) bool { return len(v.Chat) == 1 })
	if m.Game.Chat[0].Seat != 1 || m.Game.Chat[0].Text != "привет всем" {
		t.Errorf("chat: %+v", m.Game.Chat)
	}
	// a spectator cannot write
	watcher := dialBug(t, srv, "", id)
	watcher.send("chat", map[string]any{"text": "я смотрю"})
	watcher.send("resign", nil)
	if got := bugSnapshot(t, srv, "", id).Game; len(got.Chat) != 1 || got.Status != StatusPlaying || got.YourSeat != -1 {
		t.Errorf("a spectator wrote or resigned: %+v", got)
	}

	// the move of a board is made from its seat only: the partner (seat 2, board 2) cannot move on board 1,
	// and seat 1 (Black) cannot move first
	carol.send("move", map[string]any{"action": "e7e5"})
	anna.send("move", map[string]any{"action": "e2e4"})
	anna.state(func(v *BugView) bool { return len(v.Moves) == 1 })
	// abort is for a game without actions; now there is one
	anna.send("abort", nil)
	// resign: seat 2 is in team 1, so team 0 wins
	s[2].send("resign", nil)
	m = anna.state(func(v *BugView) bool { return v.Status == StatusEnded })
	if m.Game.Result != "1-0" || m.Game.Termination != TermResign {
		t.Errorf("resign: %s %s", m.Game.Result, m.Game.Termination)
	}
}

func TestBughouseAbortBeforeTheFirstMove(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	_, s := fourPlayers(t, srv)
	s[3].send("abort", nil)
	m := s[0].state(func(v *BugView) bool { return v.Status == StatusAborted })
	if m.Game.Result != "" {
		t.Errorf("an aborted game has no result: %q", m.Game.Result)
	}
}

// ------------------------------------------------------------- clocks, on a fake clock

func clockedBug(t *testing.T, tc TimeControl) (*BugGame, *fakeClock) {
	t.Helper()
	clk := &fakeClock{t: time.Unix(1_800_000_000, 0)}
	h := New(Config{User: testUser, Now: clk.now})
	t.Cleanup(h.Close)
	v, opts, err := variants.Prepare("bughouse", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	g, err := newBugGame("0000000000000002", clk.now, v, opts, tc, Player{ID: "anna", Name: "Anna"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	h.registerBug(g)
	for i, u := range []string{"carol", "dave", "boris"} {
		if err := g.joinSeat(Player{ID: u, Name: u}, -1); err != nil {
			t.Fatalf("seat %d: %v", i+1, err)
		}
	}
	if g.status != StatusPlaying {
		t.Fatalf("status %s", g.status)
	}
	return g, clk
}

func TestBughouseClocksRunPerBoardAfterTwoPlies(t *testing.T) {
	g, clk := clockedBug(t, TimeControl{BaseMs: 60_000, IncMs: 2_000})
	act := func(u, a string) {
		t.Helper()
		if err := g.Act(u, a); err != nil {
			t.Fatalf("%s %s: %v", u, a, err)
		}
	}
	snap := func() *BugView {
		var m BugMessage
		json.Unmarshal(g.Snapshot("anna"), &m)
		return m.Game
	}
	if c := snap().Clock; len(c.Running) != 0 {
		t.Fatalf("clocks run before any move: %v", c.Running)
	}
	act("anna", "e2e4")
	clk.add(5 * time.Second)
	act("carol", "d7d5")
	if c := snap().Clock; len(c.Running) != 1 || c.Running[0] != 0 || c.Ms[0] != 60_000 {
		t.Fatalf("board 1 has had two plies, so its clock starts now, board 2 stands still: %+v", c)
	}
	// board 2 is slower: its clocks start after its own second ply, independently
	act("dave", "e2e4")
	act("boris", "e7e5")
	c := snap().Clock
	if len(c.Running) != 2 || c.Running[0] != 0 || c.Running[1] != 2 {
		t.Fatalf("white of both boards is to move and both clocks go: %v", c.Running)
	}
	clk.add(7 * time.Second)
	act("anna", "g1f3") // anna's clock stops with the increment; dave's keeps going
	c = snap().Clock
	if c.Ms[0] != 60_000-7_000+2_000 {
		t.Errorf("anna's time: %d", c.Ms[0])
	}
	if c.Ms[2] != 60_000-7_000 { // dave's clock has gone for the same seven seconds
		t.Errorf("dave's time as of the snapshot: %d", c.Ms[2])
	}
	// the flag of one player ends the game for his team
	clk.add(61 * time.Second)
	g.tick()
	g.mu.Lock()
	status, result, term := g.status, g.result, g.term
	g.mu.Unlock()
	if status != StatusEnded || term != TermTimeout {
		t.Fatalf("after a minute of silence: %s %s", status, term)
	}
	// dave (seat 2, team 1) and carol (seat 1, team 1) are to move; whoever fell, team 1 loses: "1-0"
	if result != "1-0" {
		t.Errorf("result %s", result)
	}
}

func TestBughouseNoFirstMoveAbortsTheGame(t *testing.T) {
	g, clk := clockedBug(t, TimeControl{BaseMs: 60_000})
	clk.add(59 * time.Second)
	g.tick()
	if g.status != StatusPlaying {
		t.Fatal("not yet")
	}
	clk.add(2 * time.Second)
	g.tick()
	if g.status != StatusAborted || g.term != TermNoFirstMove || g.result != "" {
		t.Errorf("%s %s %q", g.status, g.term, g.result)
	}
}

func TestBughouseSurvivesARestart(t *testing.T) {
	db := &memStore{}
	h, srv := newTestHub(t, db)
	_, s := fourPlayers(t, srv)
	for i, a := range []struct {
		who    int
		action string
	}{{0, "e2e4"}, {1, "d7d5"}, {0, "e4d5"}} {
		s[a.who].send("move", map[string]any{"action": a.action})
		s[3].state(func(v *BugView) bool { return len(v.Moves) == i+1 })
	}
	id := ""
	h.mu.Lock()
	for k := range h.bugs {
		id = k
	}
	h.mu.Unlock()
	h.Close() // flushes the saver

	h2 := New(Config{Store: db, User: testUser})
	srv2 := newServerFor(t, h2)
	t.Cleanup(h2.Close)
	m := bugSnapshot(t, srv2, "boris", id)
	if m.Game.Status != StatusPlaying || len(m.Game.Moves) != 3 || m.Game.YourSeat != 3 {
		t.Fatalf("restored: status %s, %d moves, seat %d", m.Game.Status, len(m.Game.Moves), m.Game.YourSeat)
	}
	raw, _ := json.Marshal(m.Game.Boards[1].Position.Extra)
	var extra variants.PocketExtra
	json.Unmarshal(raw, &extra)
	if extra.Black["P"] != 1 {
		t.Errorf("the pawn in boris's pocket after the restart: %v", extra)
	}
	// and the list of games has the four players
	code, b := call(t, srv2, "anna", "GET", "/api/variants/games", "")
	var list struct{ Items []Listed }
	json.Unmarshal(b, &list)
	if code != 200 || len(list.Items) != 1 || len(list.Items[0].Players) != 4 || list.Items[0].Seat == nil || *list.Items[0].Seat != 0 {
		t.Errorf("list: %d %s", code, b)
	}
}

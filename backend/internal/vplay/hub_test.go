package vplay

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"dyanis-chess/backend/internal/store"
	"dyanis-chess/backend/internal/variants"
)

// memStore keeps games in a map.
type memStore struct {
	mu   sync.Mutex
	rows map[string]*store.VariantGame
}

func (m *memStore) SaveVariantGame(_ context.Context, g *store.VariantGame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rows == nil {
		m.rows = map[string]*store.VariantGame{}
	}
	c := *g
	m.rows[g.ID] = &c
	return nil
}

func (m *memStore) LoadVariantGame(_ context.Context, id string) (*store.VariantGame, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if g, ok := m.rows[id]; ok {
		c := *g
		return &c, nil
	}
	return nil, store.ErrNotFound
}

func (m *memStore) ListVariantGames(_ context.Context, userID string, _ int) ([]*store.VariantGame, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*store.VariantGame
	for _, g := range m.rows {
		if (g.WhiteID == userID || g.BlackID == userID) && (g.Status == StatusPlaying || g.Status == StatusEnded) {
			c := *g
			out = append(out, &c)
		}
	}
	return out, nil
}

// the signed-in user of a request is named by its "u" cookie
func testUser(r *http.Request) *store.User {
	c, err := r.Cookie("u")
	if err != nil || c.Value == "" {
		return nil
	}
	return &store.User{ID: c.Value, Name: strings.ToUpper(c.Value[:1]) + c.Value[1:]}
}

func newTestHub(t *testing.T, db Store) (*Hub, *httptest.Server) {
	t.Helper()
	h := New(Config{Store: db, User: testUser})
	srv := httptest.NewServer(h.Handler())
	t.Cleanup(func() {
		srv.Close()
		h.Close()
	})
	return h, srv
}

func call(t *testing.T, srv *httptest.Server, user, method, path, body string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if user != "" {
		req.AddCookie(&http.Cookie{Name: "u", Value: user})
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func createGame(t *testing.T, srv *httptest.Server, user, body string) string {
	t.Helper()
	code, b := call(t, srv, user, "POST", "/api/variants/games", body)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, b)
	}
	var out struct{ ID string }
	json.Unmarshal(b, &out)
	return out.ID
}

// socket is one WebSocket client.
type socket struct {
	t *testing.T
	c *websocket.Conn
}

func dial(t *testing.T, srv *httptest.Server, user, id string, hdr ...string) *socket {
	t.Helper()
	h := http.Header{}
	if user != "" {
		h.Add("Cookie", "u="+user)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		h.Set(hdr[i], hdr[i+1])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/api/variants/games/"+id+"/ws", &websocket.DialOptions{HTTPHeader: h})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.CloseNow() })
	return &socket{t: t, c: c}
}

func (s *socket) send(typ string, extra map[string]any) {
	s.t.Helper()
	m := map[string]any{"type": typ}
	for k, v := range extra {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.c.Write(ctx, websocket.MessageText, b); err != nil {
		s.t.Fatal(err)
	}
}

func (s *socket) next() Message {
	s.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := s.c.Read(ctx)
	if err != nil {
		s.t.Fatalf("read: %v", err)
	}
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		s.t.Fatalf("bad message %s: %v", data, err)
	}
	return m
}

// until reads messages until one satisfies ok (a snapshot or an error).
func (s *socket) until(ok func(Message) bool) Message {
	s.t.Helper()
	for i := 0; i < 50; i++ {
		if m := s.next(); ok(m) {
			return m
		}
	}
	s.t.Fatal("the awaited message never came")
	return Message{}
}

func (s *socket) state(ok func(*View) bool) Message {
	s.t.Helper()
	return s.until(func(m Message) bool { return m.Type == "state" && ok(m.Game) })
}

// twoPlayers starts a game of the variant between "anna" (White) and "boris".
func twoPlayers(t *testing.T, srv *httptest.Server, body string) (id string, anna, boris *socket) {
	t.Helper()
	id = createGame(t, srv, "anna", body)
	if code, b := call(t, srv, "boris", "POST", "/api/variants/games/"+id+"/join", ""); code != http.StatusOK {
		t.Fatalf("join: %d %s", code, b)
	}
	anna, boris = dial(t, srv, "anna", id), dial(t, srv, "boris", id)
	anna.state(func(v *View) bool { return v.Status == StatusPlaying })
	boris.state(func(v *View) bool { return v.Status == StatusPlaying })
	return id, anna, boris
}

const classicWhite = `{"variant":"classic","color":"white","timeMs":0}`

func TestFoolsMateOverTheSockets(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	_, anna, boris := twoPlayers(t, srv, classicWhite)

	play := func(who *socket, action string, moves int) {
		t.Helper()
		who.send("move", map[string]any{"action": action})
		anna.state(func(v *View) bool { return len(v.Moves) == moves })
		boris.state(func(v *View) bool { return len(v.Moves) == moves })
	}
	play(anna, "f2f3", 1)
	play(boris, "e7e5", 2)
	play(anna, "g2g4", 3)
	boris.send("move", map[string]any{"action": "d8h4"})
	m := anna.state(func(v *View) bool { return v.Status == StatusEnded })
	if m.Game.Result != "0-1" || m.Game.Termination != variants.ReasonCheckmate {
		t.Errorf("result %q by %q", m.Game.Result, m.Game.Termination)
	}
	if got := m.Game.Moves[3]; got.SAN != "Qh4#" || got.Side != variants.Black || got.Action != "d8h4" {
		t.Errorf("the last record: %+v", got)
	}
	if m.You != White || len(m.Game.Position.Legal) != 0 || m.Game.Position.Board["h4"] != "bQ" {
		t.Errorf("you %v, legal %v, h4 %q", m.You, m.Game.Position.Legal, m.Game.Position.Board["h4"])
	}
	if len(m.Game.Position.Check) != 1 || m.Game.Position.Check[0] != "e1" {
		t.Errorf("check: %v", m.Game.Position.Check)
	}
	// after the end no action is accepted
	anna.send("move", map[string]any{"action": "a2a3"})
	if e := anna.until(func(m Message) bool { return m.Type == "error" }); e.Err == "" {
		t.Error("empty error")
	}
}

func TestOnlyTheLegalMovesOfTheSideToMove(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	id, anna, boris := twoPlayers(t, srv, classicWhite)

	s := snapshot(t, srv, "anna", id)
	if len(s.Game.Position.Legal) != 20 || s.Game.Turn != White {
		t.Errorf("White's legal moves: %d, turn %v", len(s.Game.Position.Legal), s.Game.Turn)
	}
	b := snapshot(t, srv, "boris", id)
	if len(b.Game.Position.Legal) != 0 {
		t.Errorf("Black is told the moves of White: %v", b.Game.Position.Legal)
	}
	for _, c := range []struct {
		who    *socket
		action string
		want   string
	}{
		{boris, "e7e5", "сейчас не ваш ход"},
		{anna, "e2e5", "такой ход невозможен"},
		{anna, "", "такой ход невозможен"},
	} {
		c.who.send("move", map[string]any{"action": c.action})
		e := c.who.until(func(m Message) bool { return m.Type == "error" })
		if e.Err != c.want {
			t.Errorf("%q: error %q, want %q", c.action, e.Err, c.want)
		}
		// the true state follows the error
		c.who.state(func(v *View) bool { return len(v.Moves) == 0 })
	}
}

// snapshot reads the state of a game over HTTP as the user sees it.
func snapshot(t *testing.T, srv *httptest.Server, user, id string) Message {
	t.Helper()
	code, b := call(t, srv, user, "GET", "/api/variants/games/"+id, "")
	var m Message
	if code != 200 || json.Unmarshal(b, &m) != nil {
		t.Fatalf("snapshot: %d %s", code, b)
	}
	return m
}

func TestSpectatorAndPermissions(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	id, anna, _ := twoPlayers(t, srv, classicWhite)
	spec := dial(t, srv, "", id)
	m := spec.state(func(v *View) bool { return true })
	if m.You != variants.NoSide || len(m.Game.Position.Legal) != 0 || len(m.Game.Position.Board) != 32 {
		t.Errorf("spectator: you %q, legal %d, board %d", m.You, len(m.Game.Position.Legal), len(m.Game.Position.Board))
	}
	spec.send("move", map[string]any{"action": "e2e4"})
	if e := spec.until(func(m Message) bool { return m.Type == "error" }); e.Err != "вы не участник партии" {
		t.Errorf("spectator's move: %q", e.Err)
	}
	spec.send("resign", nil)
	spec.until(func(m Message) bool { return m.Type == "error" })
	anna.state(func(v *View) bool { return v.Presence.Watching == 1 && v.Status == StatusPlaying })

	// unknown game and bad ids
	if code, _ := call(t, srv, "anna", "GET", "/api/variants/games/0123456789abcdef", ""); code != http.StatusNotFound {
		t.Errorf("unknown game: %d", code)
	}
	if code, _ := call(t, srv, "anna", "GET", "/api/variants/games/xyz", ""); code != http.StatusNotFound {
		t.Errorf("bad id accepted")
	}
	// a third person cannot join
	if code, _ := call(t, srv, "carl", "POST", "/api/variants/games/"+id+"/join", ""); code != http.StatusConflict {
		t.Errorf("third player: %d", code)
	}
}

func TestCreateValidation(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	for _, c := range []struct {
		user, body string
		code       int
	}{
		{"", classicWhite, http.StatusUnauthorized},
		{"anna", `{"variant":"nope","color":"white"}`, http.StatusBadRequest},
		{"anna", `{"variant":"classic","color":"green"}`, http.StatusBadRequest},
		{"anna", `{"variant":"classic","color":"white","timeMs":5}`, http.StatusBadRequest},
		{"anna", `{"variant":"classic","color":"white","timeMs":0,"incMs":3000}`, http.StatusBadRequest},
		{"anna", `{"variant":"classic","options":{"x":1}}`, http.StatusBadRequest}, // classic takes no options
		{"anna", `not json`, http.StatusBadRequest},
		{"anna", `{"variant":"classic","color":"random","timeMs":180000,"incMs":2000}`, http.StatusCreated},
	} {
		if code, b := call(t, srv, c.user, "POST", "/api/variants/games", c.body); code != c.code {
			t.Errorf("%s %q: %d %s, want %d", c.user, c.body, code, b, c.code)
		}
	}
	// at most five open invitations
	for i := 0; i < 4; i++ {
		createGame(t, srv, "dora", classicWhite)
	}
	createGame(t, srv, "dora", classicWhite)
	if code, _ := call(t, srv, "dora", "POST", "/api/variants/games", classicWhite); code != http.StatusTooManyRequests {
		t.Errorf("sixth invitation: %d", code)
	}
}

func TestCatalogue(t *testing.T) {
	_, srv := newTestHub(t, nil)
	code, b := call(t, srv, "", "GET", "/api/variants", "")
	var out struct{ Variants []variants.Info }
	if code != 200 || json.Unmarshal(b, &out) != nil {
		t.Fatalf("%d %s", code, b)
	}
	found := false
	for _, v := range out.Variants {
		if v.ID == "classic" && v.Hidden && v.Name != "" {
			found = true
		}
	}
	if !found {
		t.Errorf("classic is not listed: %s", b)
	}
}

func TestResignAbortAndDraw(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	_, anna, boris := twoPlayers(t, srv, classicWhite)

	// no draw offer before both sides have moved
	anna.send("draw", nil)
	anna.until(func(m Message) bool { return m.Type == "error" })
	// the game can be aborted at the start
	for i, a := range []struct {
		who    *socket
		action string
	}{{anna, "e2e4"}, {boris, "e7e5"}} {
		a.who.send("move", map[string]any{"action": a.action})
		anna.state(func(v *View) bool { return len(v.Moves) == i+1 })
	}
	anna.send("abort", nil)
	anna.until(func(m Message) bool { return m.Type == "error" })

	anna.send("draw", nil)
	m := boris.state(func(v *View) bool { return v.DrawOffer == White })
	if m.You != Black {
		t.Fatal("wrong seat")
	}
	boris.send("drawDecline", nil)
	boris.state(func(v *View) bool { return v.DrawOffer == variants.NoSide })
	boris.send("draw", nil)
	anna.state(func(v *View) bool { return v.DrawOffer == Black })
	anna.send("draw", nil) // accepts
	d := anna.state(func(v *View) bool { return v.Status == StatusEnded })
	if d.Game.Result != "1/2-1/2" || d.Game.Termination != TermAgreement {
		t.Errorf("draw: %q %q", d.Game.Result, d.Game.Termination)
	}

	// resignation in a new game
	_, a2, b2 := twoPlayers(t, srv, classicWhite)
	b2.send("resign", nil)
	r := a2.state(func(v *View) bool { return v.Status == StatusEnded })
	if r.Game.Result != "1-0" || r.Game.Termination != TermResign {
		t.Errorf("resign: %q %q", r.Game.Result, r.Game.Termination)
	}
}

func TestAbortBeforeTheSecondTurn(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	_, anna, _ := twoPlayers(t, srv, classicWhite)
	anna.send("abort", nil)
	m := anna.state(func(v *View) bool { return v.Status == StatusAborted })
	if m.Game.Result != "" || m.Game.Termination != TermAborted {
		t.Errorf("abort: %q %q", m.Game.Result, m.Game.Termination)
	}
	// a waiting invitation is cancelled by its creator only
	id := createGame(t, srv, "erik", classicWhite)
	s := dial(t, srv, "erik", id)
	s.state(func(v *View) bool { return v.Status == StatusWaiting })
	s.send("abort", nil)
	s.state(func(v *View) bool { return v.Status == StatusAborted })
}

func TestRematchSwapsColoursAndKeepsTheVariant(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	_, anna, boris := twoPlayers(t, srv, classicWhite)
	boris.send("resign", nil)
	anna.state(func(v *View) bool { return v.Status == StatusEnded })
	boris.state(func(v *View) bool { return v.Status == StatusEnded })

	anna.send("rematch", nil)
	boris.state(func(v *View) bool { return v.Rematch != nil && v.Rematch.Offer == White })
	boris.send("rematch", nil)
	m := anna.state(func(v *View) bool { return v.Rematch != nil && v.Rematch.ID != "" })
	next := dial(t, srv, "boris", m.Game.Rematch.ID)
	s := next.state(func(v *View) bool { return v.Status == StatusPlaying })
	if s.You != White || s.Game.Variant != "classic" || s.Game.White.ID != "boris" || s.Game.Black.ID != "anna" {
		t.Errorf("rematch: you %q, %s, %s vs %s", s.You, s.Game.Variant, s.Game.White.ID, s.Game.Black.ID)
	}
}

func TestChat(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	id, anna, boris := twoPlayers(t, srv, classicWhite)
	anna.send("chat", map[string]any{"text": "  привет\n<b>там</b>  "})
	m := boris.state(func(v *View) bool { return len(v.Chat) == 1 })
	if m.Game.Chat[0].Text != "привет <b>там</b>" || m.Game.Chat[0].Side != White {
		t.Errorf("chat: %+v", m.Game.Chat)
	}
	spec := dial(t, srv, "", id)
	spec.send("chat", map[string]any{"text": "hi"})
	if e := spec.until(func(m Message) bool { return m.Type == "error" }); !strings.Contains(e.Err, "игроки") {
		t.Errorf("spectator chat: %q", e.Err)
	}
	// too fast
	boris.send("chat", map[string]any{"text": "a"})
	boris.send("chat", map[string]any{"text": "b"})
	boris.until(func(m Message) bool { return m.Type == "error" && m.Err == "не так быстро" })
}

// A game is saved after each change and found again by a new hub, with the
// same position, the same record of moves and the clock still running.
func TestRestartRestoresAGame(t *testing.T) {
	db := &memStore{}
	h, srv := newTestHub(t, db)
	id, anna, boris := twoPlayers(t, srv, `{"variant":"classic","color":"white","timeMs":300000,"incMs":2000}`)
	for i, a := range []string{"e2e4", "e7e5", "g1f3", "b8c6"} {
		who := []*socket{anna, boris}[i%2]
		who.send("move", map[string]any{"action": a})
		anna.state(func(v *View) bool { return len(v.Moves) == i+1 })
	}
	anna.send("chat", map[string]any{"text": "до рестарта"})
	anna.state(func(v *View) bool { return len(v.Chat) == 1 })
	before := string(h.mustGame(t, id).Snapshot("anna"))
	h.Close() // flushes the saver

	h2 := New(Config{Store: db, User: testUser})
	defer h2.Close()
	srv2 := httptest.NewServer(h2.Handler())
	defer srv2.Close()
	code, body := call(t, srv2, "anna", "GET", "/api/variants/games/"+id, "")
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var m Message
	json.Unmarshal(body, &m)
	if m.Game.Status != StatusPlaying || len(m.Game.Moves) != 4 || m.Game.Moves[2].SAN != "Nf3" || m.Game.Turn != White || len(m.Game.Chat) != 1 {
		t.Fatalf("restored: %+v", m.Game)
	}
	if m.Game.Position.Board["f3"] != "wN" || m.Game.Position.Board["c6"] != "bN" || len(m.Game.Position.Legal) == 0 {
		t.Errorf("restored board: %v", m.Game.Position.Board)
	}
	if m.Game.Clock == nil || m.Game.Clock.Running != White {
		t.Errorf("the clock should run for White: %+v", m.Game.Clock)
	}
	var first, second Message
	json.Unmarshal([]byte(before), &first)
	first.Game.Clock, m.Game.Clock = nil, nil
	first.Game.Presence, m.Game.Presence = Presence{}, Presence{}
	a, _ := json.Marshal(first.Game)
	b, _ := json.Marshal(m.Game)
	if string(a) != string(b) {
		t.Errorf("snapshots differ after the restart:\n%s\n%s", a, b)
	}
	_ = second
	// and the game goes on
	ws := dial(t, srv2, "anna", id)
	ws.state(func(v *View) bool { return true })
	ws.send("move", map[string]any{"action": "f1c4"})
	ws.state(func(v *View) bool { return len(v.Moves) == 5 })
	// the list of my games names the variant
	code, body = call(t, srv2, "anna", "GET", "/api/variants/games", "")
	var list struct{ Items []Listed }
	json.Unmarshal(body, &list)
	if code != 200 || len(list.Items) != 1 || list.Items[0].Variant != "classic" || list.Items[0].VariantName == "" || list.Items[0].You != White {
		t.Errorf("list: %d %s", code, body)
	}
}

func (h *Hub) mustGame(t *testing.T, id string) *Game {
	t.Helper()
	g, err := h.find(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestCrossOriginSocketIsRefused(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	id := createGame(t, srv, "anna", classicWhite)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/api/variants/games/"+id+"/ws",
		&websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://evil.example"}}})
	if err == nil {
		t.Error("a socket from a foreign origin was accepted")
	}
}

// newServerFor serves a hub that the test built itself.
func newServerFor(t *testing.T, h *Hub) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h.Handler())
	t.Cleanup(srv.Close)
	return srv
}

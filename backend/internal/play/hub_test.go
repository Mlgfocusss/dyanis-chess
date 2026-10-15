package play

import (
	"context"
	"encoding/json"
	"io"
	mrand "math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"dyanis-chess/backend/internal/bots"
	"dyanis-chess/backend/internal/store"
)

// memStore keeps games in a map.
type memStore struct {
	mu   sync.Mutex
	rows map[string]*store.Game
	bots map[string]*store.BotRecord // "user/bot"
	// recorded lists every result handed over, in order
	recorded []string
}

func (m *memStore) RecordBotResult(_ context.Context, userID, botID, outcome string, points int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.bots == nil {
		m.bots = map[string]*store.BotRecord{}
	}
	r := m.bots[userID+"/"+botID]
	if r == nil {
		r = &store.BotRecord{BotID: botID}
		m.bots[userID+"/"+botID] = r
	}
	first := false
	switch outcome {
	case "win":
		first = r.Wins == 0
		r.Wins++
		if first {
			r.Points = points
		}
	case "draw":
		r.Draws++
	default:
		r.Losses++
	}
	m.recorded = append(m.recorded, userID+" "+botID+" "+outcome)
	return first, nil
}

func (m *memStore) BotProgress(_ context.Context, userID string) ([]store.BotRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []store.BotRecord
	for k, r := range m.bots {
		if len(k) > len(userID) && k[:len(userID)+1] == userID+"/" {
			out = append(out, *r)
		}
	}
	return out, nil
}

func (m *memStore) SaveGame(_ context.Context, g *store.Game) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rows == nil {
		m.rows = map[string]*store.Game{}
	}
	c := *g
	m.rows[g.ID] = &c
	return nil
}

func (m *memStore) LoadGame(_ context.Context, id string) (*store.Game, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if g, ok := m.rows[id]; ok {
		c := *g
		return &c, nil
	}
	return nil, store.ErrNotFound
}

func (m *memStore) ListGames(_ context.Context, userID string, limit int) ([]*store.Game, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*store.Game
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
	h := New(Config{Store: db, User: testUser,
		BotThink: func(*bots.Bot, *mrand.Rand) time.Duration { return 0 }}) // bots answer at once in tests
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
	code, b := call(t, srv, user, "POST", "/api/games", body)
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
		h.Set("Cookie", "u="+user)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		h.Set(hdr[i], hdr[i+1])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/api/games/"+id+"/ws", &websocket.DialOptions{HTTPHeader: h})
	if err != nil {
		t.Fatalf("dial as %q: %v", user, err)
	}
	t.Cleanup(func() { c.CloseNow() })
	return &socket{t: t, c: c}
}

func (s *socket) send(typ, uci string) {
	s.t.Helper()
	b, _ := json.Marshal(incoming{Type: typ, UCI: uci})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.c.Write(ctx, websocket.MessageText, b); err != nil {
		s.t.Fatal(err)
	}
}

// until reads messages until one satisfies ok.
func (s *socket) until(what string, ok func(Message) bool) Message {
	s.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, data, err := s.c.Read(ctx)
		if err != nil {
			s.t.Fatalf("waiting for %s: %v", what, err)
		}
		var m Message
		if err := json.Unmarshal(data, &m); err != nil {
			s.t.Fatal(err)
		}
		if ok(m) {
			return m
		}
	}
}

func (s *socket) untilMoves(n int) Message {
	return s.until("move count", func(m Message) bool { return m.Game != nil && len(m.Game.Moves) >= n })
}

func TestCreateJoinAndPlayOverWebSockets(t *testing.T) {
	db := &memStore{}
	_, srv := newTestHub(t, db)
	id := createGame(t, srv, "anna", `{"color":"white","timeMs":300000,"incMs":2000}`)

	anna := dial(t, srv, "anna", id)
	first := anna.until("state", func(m Message) bool { return m.Type == "state" })
	if first.Game.Status != StatusWaiting || first.You != White || first.Game.White.Name != "Anna" || first.Game.Black != nil {
		t.Fatalf("waiting room: %+v you=%s", first.Game, first.You)
	}

	// Boris opens the link: a stranger can watch, then joins.
	if code, _ := call(t, srv, "", "POST", "/api/games/"+id+"/join", ""); code != http.StatusUnauthorized {
		t.Errorf("anonymous join: %d", code)
	}
	if code, b := call(t, srv, "anna", "POST", "/api/games/"+id+"/join", ""); code != http.StatusConflict {
		t.Errorf("joining your own game: %d %s", code, b)
	}
	if code, b := call(t, srv, "boris", "POST", "/api/games/"+id+"/join", ""); code != 200 {
		t.Fatalf("join: %d %s", code, b)
	}
	started := anna.until("start", func(m Message) bool { return m.Game != nil && m.Game.Status == StatusPlaying })
	if started.Game.Black.Name != "Boris" || len(started.Game.Legal) != 20 {
		t.Errorf("after join: %+v", started.Game)
	}

	boris := dial(t, srv, "boris", id)
	watcher := dial(t, srv, "", id)
	st := boris.until("state", func(m Message) bool { return m.Type == "state" })
	if st.You != Black || st.Game.Presence.White != true {
		t.Errorf("boris sees you=%s presence=%+v", st.You, st.Game.Presence)
	}
	watcher.until("presence of the watcher", func(m Message) bool {
		return m.Game != nil && m.You == "" && m.Game.Presence.Watching == 1
	})

	// Fool's mate.
	anna.send("move", "f2f3")
	boris.untilMoves(1)
	boris.send("move", "e7e5")
	anna.untilMoves(2)
	anna.send("move", "g2g4")
	boris.untilMoves(3)
	boris.send("move", "d8h4")
	for name, s := range map[string]*socket{"anna": anna, "boris": boris, "watcher": watcher} {
		end := s.until(name+" sees the end", func(m Message) bool { return m.Game != nil && m.Game.Status == StatusEnded })
		if end.Game.Result != "0-1" || end.Game.Termination != TermCheckmate || len(end.Game.Moves) != 4 {
			t.Errorf("%s: %+v", name, end.Game)
		}
	}

	// The record: PGN for the review, the list of games, the stored row.
	code, pgnText := call(t, srv, "anna", "GET", "/api/games/"+id+"/pgn", "")
	if code != 200 || !strings.Contains(string(pgnText), "1. f3 e5 2. g4 Qh4# 0-1") {
		t.Errorf("pgn: %d %s", code, pgnText)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		row, err := db.LoadGame(context.Background(), id)
		if err == nil && row.Status == StatusEnded {
			if row.Result != "0-1" || row.WhiteID != "anna" || row.BlackID != "boris" {
				t.Errorf("stored row: %+v", row)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the finished game never reached the store")
		}
		time.Sleep(10 * time.Millisecond)
	}
	code, b := call(t, srv, "boris", "GET", "/api/games", "")
	var list struct{ Items []Listed }
	json.Unmarshal(b, &list)
	if code != 200 || len(list.Items) != 1 || list.Items[0].You != Black || list.Items[0].Result != "0-1" || list.Items[0].Plies != 4 {
		t.Errorf("list: %d %s", code, b)
	}
	if code, _ := call(t, srv, "", "GET", "/api/games", ""); code != http.StatusUnauthorized {
		t.Errorf("anonymous list: %d", code)
	}
}

func TestBadMovesAreAnsweredWithAnErrorAndTheTrueState(t *testing.T) {
	_, srv := newTestHub(t, nil)
	id := createGame(t, srv, "anna", `{"color":"white"}`)
	call(t, srv, "boris", "POST", "/api/games/"+id+"/join", "")
	anna, watcher := dial(t, srv, "anna", id), dial(t, srv, "", id)
	anna.until("state", func(m Message) bool { return m.Type == "state" })

	anna.send("move", "e2e5")
	e := anna.until("error", func(m Message) bool { return m.Type == "error" })
	if e.Err == "" {
		t.Error("empty error text")
	}
	st := anna.until("state after the error", func(m Message) bool { return m.Type == "state" && len(m.Game.Moves) == 0 })
	if st.Game.Turn != White {
		t.Errorf("turn %s", st.Game.Turn)
	}

	// A spectator cannot act.
	watcher.until("state", func(m Message) bool { return m.Type == "state" })
	watcher.send("move", "e2e4")
	watcher.send("resign", "")
	watcher.until("error", func(m Message) bool { return m.Type == "error" })
	if _, b := call(t, srv, "", "GET", "/api/games/"+id, ""); !strings.Contains(string(b), `"moves":[]`) || !strings.Contains(string(b), `"playing"`) {
		t.Errorf("the spectator changed the game: %s", b)
	}
}

func TestDrawResignAndRematchOverTheSocket(t *testing.T) {
	_, srv := newTestHub(t, nil)
	id := createGame(t, srv, "anna", `{"color":"white"}`)
	call(t, srv, "boris", "POST", "/api/games/"+id+"/join", "")
	anna, boris := dial(t, srv, "anna", id), dial(t, srv, "boris", id)
	anna.until("state", func(m Message) bool { return m.Type == "state" })

	anna.send("move", "e2e4")
	boris.untilMoves(1)
	boris.send("move", "e7e5")
	anna.untilMoves(2)

	anna.send("draw", "")
	boris.until("offer", func(m Message) bool { return m.Game != nil && m.Game.DrawOffer == White })
	boris.send("drawDecline", "")
	anna.until("offer withdrawn", func(m Message) bool { return m.Game != nil && m.Game.DrawOffer == "" && len(m.Game.Moves) == 2 })

	boris.send("draw", "")
	anna.until("offer", func(m Message) bool { return m.Game != nil && m.Game.DrawOffer == Black })
	anna.send("draw", "") // accepts
	end := boris.until("draw", func(m Message) bool { return m.Game != nil && m.Game.Status == StatusEnded })
	if end.Game.Result != "1/2-1/2" || end.Game.Termination != TermAgreement {
		t.Errorf("draw: %+v", end.Game)
	}

	anna.send("rematch", "")
	boris.until("rematch offer", func(m Message) bool { return m.Game != nil && m.Game.Rematch != nil && m.Game.Rematch.Offer == White })
	boris.send("rematch", "")
	m := anna.until("rematch id", func(m Message) bool { return m.Game != nil && m.Game.Rematch != nil && m.Game.Rematch.ID != "" })
	newID := m.Game.Rematch.ID

	code, b := call(t, srv, "boris", "GET", "/api/games/"+newID, "")
	var n Message
	json.Unmarshal(b, &n)
	if code != 200 || n.Game.Status != StatusPlaying || n.Game.White.Name != "Boris" || n.You != White || len(n.Game.Legal) != 20 {
		t.Errorf("rematch game: %d %s", code, b)
	}
}

func TestCreateValidation(t *testing.T) {
	_, srv := newTestHub(t, nil)
	cases := []struct {
		user, body string
		want       int
	}{
		{"", `{"color":"white"}`, 401},
		{"anna", `nonsense`, 400},
		{"anna", `{"color":"green"}`, 400},
		{"anna", `{"color":"white","timeMs":1000}`, 400},
		{"anna", `{"color":"white","timeMs":60000,"incMs":999999}`, 400},
		{"anna", `{"color":"random","timeMs":180000,"incMs":2000}`, 201},
		{"anna", `{}`, 201},
	}
	for _, c := range cases {
		if code, b := call(t, srv, c.user, "POST", "/api/games", c.body); code != c.want {
			t.Errorf("%q as %q: %d %s, want %d", c.body, c.user, code, b, c.want)
		}
	}
	// Too many open invitations (two are open already).
	for i := 0; i < maxWaitingPerUsr-2; i++ {
		createGame(t, srv, "anna", `{}`)
	}
	if code, _ := call(t, srv, "anna", "POST", "/api/games", `{}`); code != http.StatusTooManyRequests {
		t.Errorf("invitation flood: %d", code)
	}
	if code, _ := call(t, srv, "boris", "POST", "/api/games", `{}`); code != 201 {
		t.Errorf("someone else is blocked too: %d", code)
	}
	for _, id := range []string{"nope", "0123456789abcdeg", "0123456789abcdef"} {
		if code, _ := call(t, srv, "anna", "GET", "/api/games/"+id, ""); code != 404 {
			t.Errorf("game %q: %d", id, code)
		}
	}
}

func TestWebSocketRefusesAForeignOrigin(t *testing.T) {
	_, srv := newTestHub(t, nil)
	id := createGame(t, srv, "anna", `{}`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h := http.Header{}
	h.Set("Origin", "http://evil.example")
	h.Set("Cookie", "u=anna")
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/api/games/"+id+"/ws", &websocket.DialOptions{HTTPHeader: h})
	if err == nil {
		c.CloseNow()
		t.Fatal("a page from another site opened the socket (cross-site WebSocket hijacking)")
	}
}

// A game survives the server: another hub on the same store serves it, with
// the clock still running.
func TestGameSurvivesARestart(t *testing.T) {
	db := &memStore{}
	h1, srv1 := newTestHub(t, db)
	id := createGame(t, srv1, "anna", `{"color":"white","timeMs":300000}`)
	call(t, srv1, "boris", "POST", "/api/games/"+id+"/join", "")
	anna := dial(t, srv1, "anna", id)
	anna.until("state", func(m Message) bool { return m.Type == "state" })
	anna.send("move", "e2e4")
	anna.untilMoves(1)
	srv1.Close()
	h1.Close() // flushes the pending saves

	_, srv2 := newTestHub(t, db)
	code, b := call(t, srv2, "boris", "GET", "/api/games/"+id, "")
	var m Message
	json.Unmarshal(b, &m)
	if code != 200 || m.Game.Status != StatusPlaying || len(m.Game.Moves) != 1 || m.You != Black || m.Game.Turn != Black {
		t.Fatalf("after restart: %d %s", code, b)
	}
	if len(m.Game.Legal) != 20 {
		t.Errorf("black has %d legal moves", len(m.Game.Legal))
	}
	boris := dial(t, srv2, "boris", id)
	boris.until("state", func(m Message) bool { return m.Type == "state" })
	boris.send("move", "e7e5")
	boris.untilMoves(2)
}

// newServerFor serves an already built hub; the server and the hub are closed with the test.
func newServerFor(t *testing.T, h *Hub) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h.Handler())
	t.Cleanup(func() {
		srv.Close()
		h.Close()
	})
	return srv
}

// sendRaw writes a ready message.
func (s *socket) sendRaw(b []byte) {
	s.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.c.Write(ctx, websocket.MessageText, b); err != nil {
		s.t.Fatal(err)
	}
}

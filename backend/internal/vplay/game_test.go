package vplay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dyanis-chess/backend/internal/variants"
)

// ---------------------------------------------------------------- test variants

// t-twice: every turn is two moves of one side (the duck, the dice: a turn of several actions).
type twiceState struct {
	*variants.Game
	step int
}

func (s *twiceState) Apply(action string) (string, error) {
	note, err := s.Game.Apply(action)
	if err != nil {
		return "", err
	}
	s.step++
	if _, _, over := s.Game.Over(); !over && s.step%2 == 1 {
		s.Game.Edit(func(p *variants.Position) { p.Turn = p.Turn.Opp() })
	}
	return note, nil
}

type twiceVariant struct{}

func (twiceVariant) Info() variants.Info {
	return variants.Info{ID: "t-twice", Name: "Тест: два хода", Hidden: true}
}
func (twiceVariant) New(o variants.Options) (variants.State, error) {
	g, err := variants.NewGame(variants.ClassicRules{}, o)
	return &twiceState{Game: g}, err
}

// t-fog: a viewer sees only their own pieces and moves, until the game is over.
type fogState struct{ *variants.Game }

func (f fogState) View(viewer variants.Side) variants.View {
	v := f.Game.View(viewer)
	if _, _, over := f.Game.Over(); over {
		return v
	}
	for sq, code := range v.Board {
		if pc, _ := variants.ParseCode(code); pc.Side != viewer {
			delete(v.Board, sq)
		}
	}
	v.Last = nil
	return v
}

func (f fogState) RedactMoves(viewer variants.Side, moves []variants.MoveRec) []variants.MoveRec {
	if _, _, over := f.Game.Over(); over {
		return moves
	}
	out := make([]variants.MoveRec, len(moves))
	for i, m := range moves {
		if m.Side != viewer {
			m.Action, m.SAN = "", "?"
		}
		out[i] = m
	}
	return out
}

type fogVariant struct{}

func (fogVariant) Info() variants.Info {
	return variants.Info{ID: "t-fog", Name: "Тест: туман", Hidden: true}
}
func (fogVariant) New(o variants.Options) (variants.State, error) {
	g, err := variants.NewGame(variants.ClassicRules{}, o)
	return fogState{g}, err
}

// t-opts: takes a lobby choice "n" from 1 to 3 and a seed.
type optsVariant struct{}

func (optsVariant) Info() variants.Info {
	return variants.Info{ID: "t-opts", Name: "Тест: настройки"}
}
func (optsVariant) CheckOptions(o *variants.Options) error {
	if o.Params == nil {
		o.Params = map[string]any{"n": float64(1)}
	}
	if n := o.Int("n", 0); n < 1 || n > 3 || len(o.Params) != 1 {
		return fmt.Errorf("n от 1 до 3")
	}
	return nil
}
func (optsVariant) New(o variants.Options) (variants.State, error) {
	return variants.NewGame(variants.ClassicRules{}, o)
}

func init() {
	variants.Register(twiceVariant{})
	variants.Register(fogVariant{})
	variants.Register(optsVariant{})
}

// ------------------------------------------------------------------ a fake clock

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// clocked is a hub on a fake clock, and the way to start a game without sockets.
func clocked(t *testing.T, variant string, tc TimeControl) (*Hub, *Game, *fakeClock) {
	t.Helper()
	clk := &fakeClock{t: time.Unix(1_800_000_000, 0)}
	h := New(Config{User: testUser, Now: clk.now})
	t.Cleanup(h.Close)
	v, opts, err := variants.Prepare(variant, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	g, err := newGame("0000000000000001", clk.now, v, opts, tc, Player{ID: "anna", Name: "Anna"}, White)
	if err != nil {
		t.Fatal(err)
	}
	h.register(g)
	if err := g.Join(Player{ID: "boris", Name: "Boris"}); err != nil {
		t.Fatal(err)
	}
	return h, g, clk
}

func clockOf(g *Game) (white, black int64, running Side) {
	var m Message
	json.Unmarshal(g.Snapshot("anna"), &m)
	return m.Game.Clock.WhiteMs, m.Game.Clock.BlackMs, m.Game.Clock.Running
}

// ------------------------------------------------------------------------- clocks

func TestClocksStandUntilBothSidesHaveHadATurn(t *testing.T) {
	_, g, clk := clocked(t, "classic", TimeControl{BaseMs: 60_000, IncMs: 1000})
	clk.add(20 * time.Second)
	if err := g.Act("anna", "e2e4"); err != nil {
		t.Fatal(err)
	}
	clk.add(30 * time.Second)
	if w, b, run := clockOf(g); w != 60_000 || b != 60_000 || run != variants.NoSide {
		t.Fatalf("the clocks run before the second turn: %d %d %v", w, b, run)
	}
	if err := g.Act("boris", "e7e5"); err != nil {
		t.Fatal(err)
	}
	if _, _, run := clockOf(g); run != White {
		t.Fatalf("White's clock should start now: %v", run)
	}
	clk.add(20 * time.Second)
	if w, _, _ := clockOf(g); w != 40_000 {
		t.Errorf("White has %d ms after 20 s", w)
	}
	if err := g.Act("anna", "g1f3"); err != nil {
		t.Fatal(err)
	}
	// 20 s spent, 1 s given back
	if w, b, run := clockOf(g); w != 41_000 || b != 60_000 || run != Black {
		t.Errorf("after White's move: %d %d %v", w, b, run)
	}
	// Black thinks too long: the flag falls, and a move after it is refused
	clk.add(61 * time.Second)
	if err := g.Act("boris", "b8c6"); err == nil {
		t.Fatal("a move after the flag fell was accepted")
	}
	var m Message
	json.Unmarshal(g.Snapshot("anna"), &m)
	if m.Game.Status != StatusEnded || m.Game.Result != "1-0" || m.Game.Termination != TermTimeout {
		t.Errorf("flag: %s %q %q", m.Game.Status, m.Game.Result, m.Game.Termination)
	}
	if m.Game.Clock.BlackMs != 0 || m.Game.Clock.Running != variants.NoSide {
		t.Errorf("final clock: %+v", m.Game.Clock)
	}
}

func TestTheTimerEndsAGameNobodyMovesIn(t *testing.T) {
	_, g, clk := clocked(t, "classic", TimeControl{BaseMs: 30_000})
	g.Act("anna", "e2e4")
	g.Act("boris", "e7e5")
	clk.add(31 * time.Second)
	g.tick() // what the timer does
	var m Message
	json.Unmarshal(g.Snapshot("boris"), &m)
	if m.Game.Status != StatusEnded || m.Game.Result != "0-1" || m.Game.Termination != TermTimeout {
		t.Errorf("flag by the timer: %+v", m.Game)
	}
}

func TestNoFirstMoveAbortsTheGame(t *testing.T) {
	_, g, clk := clocked(t, "classic", TimeControl{BaseMs: 60_000})
	clk.add(30 * time.Second)
	g.tick()
	if g.status != StatusPlaying {
		t.Fatal("aborted too early")
	}
	clk.add(31 * time.Second)
	g.tick()
	if g.status != StatusAborted || g.result != "" || g.term != TermNoFirstMove {
		t.Errorf("after a minute: %s %q %q", g.status, g.result, g.term)
	}
	// the same without a clock: a game nobody starts is aborted too
	_, g2, clk2 := clocked(t, "classic", TimeControl{})
	clk2.add(61 * time.Second)
	g2.tick()
	if g2.status != StatusAborted {
		t.Errorf("without a clock: %s", g2.status)
	}
}

// A flag against a lone knight is a draw: the variant's State decides (here the base).
func TestFlagUsesTheStatesTimeout(t *testing.T) {
	_, g, clk := clocked(t, "classic", TimeControl{BaseMs: 30_000})
	g.Act("anna", "e2e4")
	g.Act("boris", "e7e5")
	g.st.(*variants.Game).Edit(func(p *variants.Position) {
		*p = *variantsFEN("8/8/8/4k3/8/8/8/3NK3 w - - 0 1")
	})
	clk.add(31 * time.Second)
	g.tick() // White's flag falls; Black has a king alone: a draw
	if g.result != "1/2-1/2" || g.term != TermTimeout {
		t.Errorf("flag against a bare king: %q %q", g.result, g.term)
	}
}

func variantsFEN(fen string) *variants.Position {
	p, err := variants.ParseFEN(fen)
	if err != nil {
		panic(err)
	}
	return p
}

// ------------------------------------------------- turns of several actions

func TestTheClockChangesHandsOnlyWhenTheTurnPasses(t *testing.T) {
	_, g, clk := clocked(t, "t-twice", TimeControl{BaseMs: 60_000, IncMs: 2000})
	act := func(user, a string) {
		t.Helper()
		if err := g.Act(user, a); err != nil {
			t.Fatalf("%s %s: %v", user, a, err)
		}
	}
	act("anna", "e2e4")
	if g.turnsDone != 0 || g.st.Turn() != White {
		t.Fatalf("the first action of a turn passed the turn: %d %v", g.turnsDone, g.st.Turn())
	}
	if err := g.Act("boris", "e7e5"); err == nil || err != errNotYours {
		t.Errorf("Black acted in White's turn: %v", err)
	}
	act("anna", "d2d4")
	act("boris", "e7e5")
	act("boris", "d7d5")
	if g.turnsDone != 2 {
		t.Fatalf("turns done: %d", g.turnsDone)
	}
	if len(g.moves) != 4 {
		t.Fatalf("moves recorded: %d", len(g.moves))
	}
	// White's clock runs through both of its actions
	clk.add(10 * time.Second)
	act("anna", "g1f3")
	if w, _, run := clockOf(g); w != 50_000 || run != White {
		t.Errorf("after the first action (10 s in): %d %v", w, run)
	}
	clk.add(5 * time.Second)
	act("anna", "b1c3")
	if w, b, run := clockOf(g); w != 47_000 || b != 60_000 || run != Black { // 60 - 15 + 2
		t.Errorf("after the second action: white %d black %d running %v", w, b, run)
	}
	// the snapshot gives the actor the moves of the step and nobody else
	var m Message
	json.Unmarshal(g.Snapshot("boris"), &m)
	if m.Game.Turn != Black || len(m.Game.Position.Legal) == 0 {
		t.Errorf("Black must act: %+v", m.Game.Turn)
	}
	// moves of one turn are consecutive records of one side (the list groups them)
	if m.Game.Moves[0].Side != White || m.Game.Moves[1].Side != White || m.Game.Moves[2].Side != Black {
		t.Errorf("sides in the list: %+v", m.Game.Moves)
	}
}

// ------------------------------------------------------ hidden information

func TestEachViewerSeesTheirOwnPosition(t *testing.T) {
	h, srv := newTestHub(t, &memStore{})
	_ = h
	id, anna, boris := twoPlayers(t, srv, `{"variant":"t-fog","color":"white","timeMs":0}`)
	anna.send("move", map[string]any{"action": "e2e4"})
	anna.state(func(v *View) bool { return len(v.Moves) == 1 })
	boris.send("move", map[string]any{"action": "e7e5"})
	anna.state(func(v *View) bool { return len(v.Moves) == 2 })

	a := snapshot(t, srv, "anna", id)
	b := snapshot(t, srv, "boris", id)
	s := snapshot(t, srv, "", id)
	for sq, code := range a.Game.Position.Board {
		if code[0] != 'w' {
			t.Errorf("White sees %s on %s", code, sq)
		}
	}
	for sq, code := range b.Game.Position.Board {
		if code[0] != 'b' {
			t.Errorf("Black sees %s on %s", code, sq)
		}
	}
	if len(s.Game.Position.Board) != 0 || len(s.Game.Position.Legal) != 0 {
		t.Errorf("a spectator sees %v", s.Game.Position.Board)
	}
	if a.Game.Moves[1].SAN != "?" || a.Game.Moves[1].Action != "" || a.Game.Moves[0].SAN != "e4" {
		t.Errorf("White's list: %+v", a.Game.Moves)
	}
	if b.Game.Moves[0].SAN != "?" || b.Game.Moves[1].SAN != "e5" {
		t.Errorf("Black's list: %+v", b.Game.Moves)
	}
	if raw := call2(t, srv, "", id); strings.Contains(raw, "e2e4") || strings.Contains(raw, `"wP"`) || strings.Contains(raw, `"bP"`) {
		t.Errorf("a spectator's snapshot leaks: %s", raw)
	}

	// a resignation ends the game and opens everything
	boris.send("resign", nil)
	anna.state(func(v *View) bool { return v.Status == StatusEnded })
	s = snapshot(t, srv, "", id)
	if len(s.Game.Position.Board) != 32 || s.Game.Moves[1].SAN != "e5" || s.Game.Moves[0].Action != "e2e4" {
		t.Errorf("after the end: %d pieces, %+v", len(s.Game.Position.Board), s.Game.Moves)
	}
}

func call2(t *testing.T, srv *httptest.Server, user, id string) string {
	t.Helper()
	_, b := call(t, srv, user, "GET", "/api/variants/games/"+id, "")
	return string(b)
}

// ---------------------------------------------------------------- options

func TestOptionsAreCheckedAndKept(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	for body, want := range map[string]int{
		`{"variant":"t-opts","color":"white"}`:                         http.StatusCreated,
		`{"variant":"t-opts","color":"white","options":{"n":3}}`:       http.StatusCreated,
		`{"variant":"t-opts","color":"white","options":{"n":4}}`:       http.StatusBadRequest,
		`{"variant":"t-opts","color":"white","options":{"n":1,"x":2}}`: http.StatusBadRequest,
	} {
		if code, b := call(t, srv, "anna", "POST", "/api/variants/games", body); code != want {
			t.Errorf("%s: %d %s", body, code, b)
		}
	}
	id := createGame(t, srv, "boris", `{"variant":"t-opts","color":"white","options":{"n":2}}`)
	m := snapshot(t, srv, "boris", id)
	if m.Game.Params["n"] != float64(2) || m.Game.Variant != "t-opts" || m.Game.VariantName != "Тест: настройки" {
		t.Errorf("params in the snapshot: %+v", m.Game)
	}
	if strings.Contains(call2(t, srv, "boris", id), "seed") {
		t.Error("the seed is shown to the client")
	}
}

// The seed is drawn per game and survives a restart (it is how the rolls and setups come back).
func TestTheSeedIsKeptWithTheGame(t *testing.T) {
	db := &memStore{}
	h, srv := newTestHub(t, db)
	id := createGame(t, srv, "anna", `{"variant":"classic","color":"white"}`)
	g := h.mustGame(t, id)
	seed := g.opts.Seed
	if seed == 0 {
		t.Fatal("no seed was drawn")
	}
	other := h.mustGame(t, createGame(t, srv, "anna", `{"variant":"classic","color":"white"}`))
	if other.opts.Seed == seed {
		t.Error("two games share a seed")
	}
	h.Close()
	row, _ := db.LoadVariantGame(context.Background(), id)
	h2 := New(Config{Store: db, User: testUser})
	defer h2.Close()
	back, err := h2.find(context.Background(), id)
	if err != nil || back.opts.Seed != seed || row.Variant != "classic" {
		t.Errorf("restored seed %v (%v), want %v", back, err, seed)
	}
}

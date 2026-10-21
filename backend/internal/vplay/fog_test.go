package vplay

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dyanis-chess/backend/internal/variants"
)

// The real fog of war (variants/fog.go) over the real server: every raw frame
// a player or a spectator receives is read and searched for what they must not know.

const fogWhite = `{"variant":"fog","color":"white","timeMs":0}`

// rawFrames reads the frames of a socket without parsing them into the typed snapshot.
type rawFrames struct {
	s      *socket
	frames [][]byte
}

func (r *rawFrames) drain(until func(Message) bool) Message {
	r.s.t.Helper()
	for i := 0; i < 80; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, data, err := r.s.c.Read(ctx)
		cancel()
		if err != nil {
			r.s.t.Fatalf("read: %v", err)
		}
		r.frames = append(r.frames, data)
		var m Message
		if json.Unmarshal(data, &m) == nil && until(m) {
			return m
		}
	}
	r.s.t.Fatal("the awaited frame never came")
	return Message{}
}

func fogHidden(v variants.View) map[string]bool {
	out := map[string]bool{}
	list, _ := v.Extra.(map[string]any)["fog"].([]any)
	for _, s := range list {
		out[s.(string)] = true
	}
	return out
}

func TestFogOverTheSocketsLeaksNothing(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	id, anna, boris := twoPlayers(t, srv, fogWhite)
	ra, rb := &rawFrames{s: anna}, &rawFrames{s: boris}

	// 1.e4 f6 2.Qh5 a6 3.Qxe8 takes the king
	script := []struct {
		who    *socket
		action string
	}{{anna, "e2e4"}, {boris, "f7f6"}, {anna, "d1h5"}, {boris, "a7a6"}}
	for i, st := range script {
		st.who.send("move", map[string]any{"action": st.action})
		n := i + 1
		ra.drain(func(m Message) bool { return m.Type == "state" && len(m.Game.Moves) == n })
		rb.drain(func(m Message) bool { return m.Type == "state" && len(m.Game.Moves) == n })
	}
	// while the game goes on: White's frames never carry a black action, Black's never a white one,
	// and each side's board holds only pieces in view
	a := snapshot(t, srv, "anna", id)
	b := snapshot(t, srv, "boris", id)
	s := snapshot(t, srv, "", id)
	for _, c := range []struct {
		name string
		m    Message
		own  byte
		opp  []string // actions of the other side that must not be there
	}{
		{"anna", a, 'w', []string{"f7f6", "a7a6"}},
		{"boris", b, 'b', []string{"e2e4", "d1h5"}},
	} {
		hidden := fogHidden(c.m.Game.Position)
		for sq, code := range c.m.Game.Position.Board {
			if code[0] != c.own && hidden[sq] {
				t.Errorf("%s sees %s on the fogged %s", c.name, code, sq)
			}
		}
		for _, mv := range c.m.Game.Moves {
			if mv.Side == c.m.You && mv.SAN == "…" {
				t.Errorf("%s: own move hidden: %+v", c.name, mv)
			}
			if mv.Side != c.m.You && mv.Action != "" {
				t.Errorf("%s: the opponent's action in the list: %+v", c.name, mv)
			}
		}
		raw := string([]byte(call2(t, srv, c.name, id)))
		for _, act := range c.opp {
			if strings.Contains(raw, act) {
				t.Errorf("%s: the snapshot mentions %s", c.name, act)
			}
		}
	}
	// the spectator sees no board, no moves, no legal actions
	if len(s.Game.Position.Board) != 0 || len(s.Game.Position.Legal) != 0 {
		t.Errorf("the spectator sees %v", s.Game.Position.Board)
	}
	for _, mv := range s.Game.Moves {
		if mv.SAN != "…" || mv.Action != "" {
			t.Errorf("the spectator is told a move: %+v", mv)
		}
	}
	raw := string([]byte(call2(t, srv, "", id)))
	for _, bad := range []string{"e2e4", "d1h5", "f7f6", `"wP"`, `"bP"`, `"wQ"`} {
		if strings.Contains(raw, bad) {
			t.Errorf("the spectator's snapshot contains %s: %s", bad, raw)
		}
	}
	// a spectator on the socket gets the same
	spec := dial(t, srv, "", id)
	rs := &rawFrames{s: spec}
	rs.drain(func(m Message) bool { return m.Type == "state" })

	// the queen takes the king
	anna.send("move", map[string]any{"action": "h5e8"})
	end := ra.drain(func(m Message) bool { return m.Type == "state" && m.Game.Status == StatusEnded })
	rb.drain(func(m Message) bool { return m.Type == "state" && m.Game.Status == StatusEnded })
	rs.drain(func(m Message) bool { return m.Type == "state" && m.Game.Status == StatusEnded })
	if end.Game.Result != "1-0" || end.Game.Termination != variants.ReasonKingCaptured {
		t.Fatalf("result %q by %q", end.Game.Result, end.Game.Termination)
	}

	// during the game no frame of Black held a white action or a white piece code at a square it was
	// told is fogged; after it, everything is open
	for _, f := range rb.frames[:len(rb.frames)-1] {
		var m Message
		json.Unmarshal(f, &m)
		if m.Game == nil || m.Game.Status != StatusPlaying {
			continue
		}
		hidden := fogHidden(m.Game.Position)
		for sq, code := range m.Game.Position.Board {
			if code[0] == 'w' && hidden[sq] {
				t.Fatalf("Black's frame shows %s on the fogged %s: %s", code, sq, f)
			}
		}
		for _, bad := range []string{`"e2e4"`, `"d1h5"`} {
			if strings.Contains(string(f), bad) {
				t.Fatalf("Black's frame holds %s: %s", bad, f)
			}
		}
	}
	for _, f := range rs.frames[:len(rs.frames)-1] {
		if strings.Contains(string(f), `"wP"`) || strings.Contains(string(f), `"action":"e`) {
			t.Fatalf("a spectator's frame leaks: %s", f)
		}
	}
	for _, who := range []string{"anna", "boris", ""} {
		m := snapshot(t, srv, who, id)
		if len(m.Game.Position.Board) != 31 { // the black king is gone
			t.Errorf("%q sees %d pieces after the end", who, len(m.Game.Position.Board))
		}
		if m.Game.Moves[1].Action != "f7f6" || m.Game.Moves[4].SAN != "Qxe8" {
			t.Errorf("%q: the list after the end: %+v", who, m.Game.Moves)
		}
		if m.Game.Position.Extra != nil {
			t.Errorf("%q: fog data after the end: %v", who, m.Game.Position.Extra)
		}
	}
}

func TestFogOpensWhenTheGameIsGivenUpOrAborted(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	// resignation
	id, anna, boris := twoPlayers(t, srv, fogWhite)
	anna.send("move", map[string]any{"action": "e2e4"})
	boris.state(func(v *View) bool { return len(v.Moves) == 1 })
	if n := len(snapshot(t, srv, "boris", id).Game.Position.Board); n != 16 {
		t.Fatalf("Black sees %d pieces after 1.e4 (its own 16; the pawn e4 is out of view)", n)
	}
	boris.send("resign", nil)
	anna.state(func(v *View) bool { return v.Status == StatusEnded })
	if n := len(snapshot(t, srv, "boris", id).Game.Position.Board); n != 32 {
		t.Errorf("after the resignation Black sees %d pieces", n)
	}
	// aborted within the first two turns: also open
	id2, anna2, boris2 := twoPlayers(t, srv, fogWhite)
	anna2.send("move", map[string]any{"action": "d2d4"})
	boris2.state(func(v *View) bool { return len(v.Moves) == 1 })
	anna2.send("abort", nil)
	boris2.state(func(v *View) bool { return v.Status == StatusAborted })
	m := snapshot(t, srv, "boris", id2)
	if len(m.Game.Position.Board) != 32 || len(m.Game.Position.Legal) != 0 {
		t.Errorf("aborted game: %d pieces, legal %v", len(m.Game.Position.Board), m.Game.Position.Legal)
	}
}

func TestFogGameSurvivesARestart(t *testing.T) {
	db := &memStore{}
	h, srv := newTestHub(t, db)
	id, anna, boris := twoPlayers(t, srv, fogWhite)
	anna.send("move", map[string]any{"action": "g1f3"})
	boris.state(func(v *View) bool { return len(v.Moves) == 1 })
	boris.send("move", map[string]any{"action": "b8c6"})
	anna.state(func(v *View) bool { return len(v.Moves) == 2 })
	snap := func(srv *httptest.Server, user string) string {
		var m Message
		json.Unmarshal([]byte(call2(t, srv, user, id)), &m)
		m.Game.Presence = Presence{} // who is connected differs
		b, _ := json.Marshal(m)
		return string(b)
	}
	beforeA, beforeB := snap(srv, "anna"), snap(srv, "boris")
	h.Close()

	h2 := New(Config{Store: db, User: testUser})
	defer h2.Close()
	srv2 := httptest.NewServer(h2.Handler())
	defer srv2.Close()
	if got := snap(srv2, "anna"); got != beforeA {
		t.Errorf("White's snapshot differs after the restart: %s != %s", beforeA, got)
	}
	if got := snap(srv2, "boris"); got != beforeB {
		t.Errorf("Black's snapshot differs after the restart: %s != %s", beforeB, got)
	}
	// the fog is rebuilt: Black still does not see the knight on f3
	var m Message
	json.Unmarshal([]byte(call2(t, srv2, "boris", id)), &m)
	if _, ok := m.Game.Position.Board["f3"]; ok {
		t.Error("the restored game shows the knight f3 to Black")
	}
	if m.Game.Moves[0].SAN != "…" && m.Game.Moves[0].SAN != "Nf3" || m.Game.Moves[0].Action != "" {
		t.Errorf("restored list: %+v", m.Game.Moves[0])
	}
}

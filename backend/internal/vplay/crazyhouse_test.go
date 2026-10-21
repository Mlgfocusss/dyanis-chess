package vplay

import (
	"encoding/json"
	"testing"
)

// A game of Crazyhouse through the server: a capture fills the pocket, the pocket reaches both players in the
// snapshot, a drop is an action like any other, and a game restored from its actions has the same pockets.
func TestCrazyhouseThroughTheServer(t *testing.T) {
	_, g, _ := clocked(t, "crazyhouse", TimeControl{})
	for i, a := range []struct{ who, action string }{
		{"anna", "e2e4"}, {"boris", "d7d5"}, {"anna", "e4d5"}, {"boris", "g8f6"},
	} {
		if err := g.Act(a.who, a.action); err != nil {
			t.Fatalf("action %d %s: %v", i, a.action, err)
		}
	}
	var m Message
	if err := json.Unmarshal(g.Snapshot("anna"), &m); err != nil {
		t.Fatal(err)
	}
	extra, _ := m.Game.Position.Extra.(map[string]any)
	white, _ := extra["white"].(map[string]any)
	if white["P"] != float64(1) {
		t.Fatalf("the white pocket in the snapshot: %v", extra)
	}
	hasDrop := false
	for _, a := range m.Game.Position.Legal {
		if a == "P@e4" {
			hasDrop = true
		}
	}
	if !hasDrop {
		t.Error("the snapshot of the player to move offers no drop P@e4")
	}
	if err := g.Act("boris", "P@e4"); err == nil {
		t.Error("Black dropped a pawn out of turn and out of its empty pocket")
	}
	if err := g.Act("anna", "P@e4"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if got := g.moves[len(g.moves)-1]; got.Action != "P@e4" || got.SAN != "P@e4" {
		t.Errorf("record of the drop: %+v", got)
	}

	back, err := restore(g.ID, g.now, g.stateLocked())
	if err != nil {
		t.Fatal(err)
	}
	var m2 Message
	_ = json.Unmarshal(back.Snapshot("anna"), &m2)
	if m2.Game.Position.Board["e4"] != "wP" {
		t.Errorf("restored board: %v", m2.Game.Position.Board)
	}
}

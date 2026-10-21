package vplay

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"dyanis-chess/backend/internal/variants"
)

// Dice chess hands the turn on by itself when a side's three dice cannot be played: one action of White
// may give the turn to Black and straight back. The clock must charge the actor once, give the skipped
// side nothing, and count both turns as done.

func clockedSeed(t *testing.T, variant string, seed int64, tc TimeControl) (*Game, *fakeClock) {
	t.Helper()
	clk := &fakeClock{t: time.Unix(1_800_000_000, 0)}
	h := New(Config{User: testUser, Now: clk.now})
	t.Cleanup(h.Close)
	v, opts, err := variants.Prepare(variant, nil, seed)
	if err != nil {
		t.Fatal(err)
	}
	g, err := newGame("0000000000000002", clk.now, v, opts, tc, Player{ID: "anna", Name: "Anna"}, White)
	if err != nil {
		t.Fatal(err)
	}
	h.register(g)
	if err := g.Join(Player{ID: "boris", Name: "Boris"}); err != nil {
		t.Fatal(err)
	}
	return g, clk
}

func TestDiceSkippedTurnsKeepTheClockHonest(t *testing.T) {
	const inc = 2000
	skippedTurns := 0
	for seed := int64(1); seed <= 12; seed++ {
		g, clk := clockedSeed(t, "dice", seed, TimeControl{BaseMs: 600_000, IncMs: inc})
		counter, ok := g.st.(variants.TurnCounter)
		if !ok {
			t.Fatal("the dice state does not count turns")
		}
		startPasses := counter.TurnsPassed()
		for step := 0; step < 80 && g.status == StatusPlaying; step++ {
			actor := g.st.Turn()
			user := map[Side]string{White: "anna", Black: "boris"}[actor]
			legal := g.st.Legal()
			if len(legal) == 0 {
				t.Fatalf("seed %d: the side to act has nothing to play", seed)
			}
			w0, b0, _ := clockOf(g)
			ran := g.clockRunning()
			passedBefore := counter.TurnsPassed()
			clk.add(time.Second)
			if err := g.Act(user, legal[0]); err != nil {
				t.Fatalf("seed %d: %v", seed, err)
			}
			if g.status != StatusPlaying {
				break
			}
			passed := counter.TurnsPassed() - passedBefore
			if g.turnsDone != counter.TurnsPassed()-startPasses {
				t.Fatalf("seed %d: turns done %d, the state has passed %d (from %d)", seed, g.turnsDone, counter.TurnsPassed(), startPasses)
			}
			if passed >= 2 {
				skippedTurns++
			}
			if (passed%2 == 0) != (g.st.Turn() == actor) {
				t.Fatalf("seed %d: %d passes but the turn is %v after %v", seed, passed, g.st.Turn(), actor)
			}
			w1, b1, run := clockOf(g)
			own0, own1, other0, other1 := w0, w1, b0, b1
			if actor == Black {
				own0, own1, other0, other1 = b0, b1, w0, w1
			}
			if other1 != other0 {
				t.Fatalf("seed %d: the side that did not act lost or won time: %d -> %d (passed %d)", seed, other0, other1, passed)
			}
			if ran {
				want := own0 - 1000
				if passed > 0 {
					want += inc // the increment once, whatever number of turns the skips cost
				}
				if own1 != want {
					t.Fatalf("seed %d: the actor's clock %d, want %d (passed %d)", seed, own1, want, passed)
				}
			} else if own1 != own0 {
				t.Fatalf("seed %d: a clock that must stand moved: %d -> %d", seed, own0, own1)
			}
			if g.clockRunning() && run != g.st.Turn() {
				t.Fatalf("seed %d: the clock of %v runs but %v is to act", seed, run, g.st.Turn())
			}
		}
	}
	if skippedTurns == 0 {
		t.Error("no skipped turn met in the games played")
	}
}

// After the restart of the server a dice game is rebuilt by replaying its actions: the dice are the same.
func TestDiceGameIsRestoredWithItsDice(t *testing.T) {
	g, _ := clockedSeed(t, "dice", 7, TimeControl{})
	for step := 0; step < 40 && g.status == StatusPlaying; step++ {
		user := map[Side]string{White: "anna", Black: "boris"}[g.st.Turn()]
		if err := g.Act(user, g.st.Legal()[0]); err != nil {
			t.Fatal(err)
		}
	}
	var actions []string
	for _, m := range g.moves {
		actions = append(actions, m.Action)
	}
	again, err := variants.Replay("dice", g.opts, actions)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(g.st.View(g.st.Turn()))
	b, _ := json.Marshal(again.View(again.Turn()))
	if string(a) != string(b) {
		t.Errorf("restored view differs:\n%s\n%s", a, b)
	}
}

// The three dice are in the snapshot of both players and of a spectator, the seed is in none, and a game
// restored from its saved state in the middle of a turn shows the same dice with the same ones used.
func TestDiceSnapshotShowsAllThreeAndSurvivesARestore(t *testing.T) {
	g, clk := clockedSeed(t, "dice", 11, TimeControl{BaseMs: 300_000})
	diceOf := func(uid string) []map[string]any {
		var m struct {
			Game struct {
				Position struct {
					Extra struct {
						Dice []map[string]any `json:"dice"`
						Left int              `json:"left"`
					} `json:"extra"`
				} `json:"position"`
			} `json:"game"`
		}
		raw := g.Snapshot(uid)
		if strings.Contains(string(raw), "seed") {
			t.Fatalf("the snapshot tells of a seed: %s", raw)
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		return m.Game.Position.Extra.Dice
	}
	for _, uid := range []string{"anna", "boris", ""} {
		if d := diceOf(uid); len(d) != 3 {
			t.Fatalf("%q sees %d dice", uid, len(d))
		}
	}
	// play until a move leaves dice of the same turn
	for step := 0; step < 60 && g.status == StatusPlaying; step++ {
		actor := g.st.Turn()
		passes := turnsPassed(g.st)
		user := map[Side]string{White: "anna", Black: "boris"}[actor]
		clk.add(time.Second)
		if err := g.Act(user, g.st.Legal()[0]); err != nil {
			t.Fatal(err)
		}
		if turnsPassed(g.st) != passes || g.st.Turn() != actor {
			continue
		}
		// in the middle of a turn: one die is used, and the opponent sees the same
		used := 0
		for _, f := range diceOf("boris") {
			if f["state"] == "used" {
				used++
			}
		}
		if used == 0 {
			t.Fatal("a move was made and no die is used")
		}
		restored, err := restore(g.ID, clk.now, g.stateLocked())
		if err != nil {
			t.Fatal(err)
		}
		for _, uid := range []string{"anna", "boris", ""} {
			a, b := g.Snapshot(uid), restored.Snapshot(uid)
			var ma, mb map[string]any
			_ = json.Unmarshal(a, &ma)
			_ = json.Unmarshal(b, &mb)
			pa, _ := json.Marshal(ma["game"].(map[string]any)["position"])
			pb, _ := json.Marshal(mb["game"].(map[string]any)["position"])
			if string(pa) != string(pb) {
				t.Fatalf("the restored position differs for %q:\n%s\n%s", uid, pa, pb)
			}
		}
		return
	}
	t.Fatal("never stood in the middle of a turn")
}

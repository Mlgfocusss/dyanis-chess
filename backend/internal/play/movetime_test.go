package play

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"dyanis-chess/backend/internal/pgn"
)

// move plays one move for whoever has the turn after the clock has run for d.
func moveAfter(t *testing.T, g *Game, clk *fakeClock, d time.Duration, uci string) {
	t.Helper()
	clk.advance(d)
	play(t, g, uci)
}

func TestMoveTimesAreKeptInTheSnapshotAndTheState(t *testing.T) {
	g, clk := started(t, TimeControl{BaseMs: 300_000, IncMs: 2_000})
	moveAfter(t, g, clk, 2300*time.Millisecond, "e2e4")
	moveAfter(t, g, clk, 5*time.Second, "e7e5")
	moveAfter(t, g, clk, 65*time.Second, "g1f3")

	var m Message
	if err := json.Unmarshal(g.Snapshot("anna"), &m); err != nil {
		t.Fatal(err)
	}
	mv := m.Game.Moves
	if len(mv) != 3 || mv[0].Ms != 2300 || mv[1].Ms != 5000 || mv[2].Ms != 65_000 {
		t.Fatalf("times in the snapshot: %+v", mv)
	}
	// the first two moves run before the clocks start: nothing is taken from them, only the increment-free base
	if mv[0].Left != 300_000 || mv[1].Left != 300_000 {
		t.Errorf("clock left after the first moves: %d, %d", mv[0].Left, mv[1].Left)
	}
	// the third move cost 65 s and earned 2 s
	if mv[2].Left != 300_000-65_000+2_000 {
		t.Errorf("clock left after the third move: %d", mv[2].Left)
	}

	g.mu.Lock()
	raw := g.stateLocked()
	g.mu.Unlock()
	r, err := restore(g.ID, clk.now, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.moves[2].Ms != 65_000 || r.moves[0].Ms != 2300 {
		t.Errorf("times lost over a restore: %+v", r.moves)
	}
}

func TestAnOldSavedGameWithoutTimesStillOpens(t *testing.T) {
	g, clk := started(t, TimeControl{BaseMs: 300_000})
	play(t, g, "e2e4", "e7e5")
	g.mu.Lock()
	raw := string(g.stateLocked())
	g.mu.Unlock()
	// strip the new fields, as the games saved before them look
	raw = strings.NewReplacer(`,"ms":1`, ``, `,"left":300000`, ``).Replace(raw)
	if strings.Contains(raw, `"ms":`) && strings.Contains(raw, `"left":`) {
		t.Fatalf("the test did not remove the times: %s", raw)
	}
	r, err := restore(g.ID, clk.now, []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if len(r.moves) != 2 || r.moves[0].Ms != 0 {
		t.Fatalf("old game: %+v", r.moves)
	}
	if strings.Contains(r.PGN(), "[%emt") || strings.Contains(r.PGN(), "[%clk") {
		t.Errorf("an old game has no times to write:\n%s", r.PGN())
	}
}

func TestPGNCarriesClockAndMoveTime(t *testing.T) {
	g, clk := started(t, TimeControl{BaseMs: 300_000, IncMs: 2_000})
	moveAfter(t, g, clk, 2500*time.Millisecond, "f2f3")
	moveAfter(t, g, clk, 4*time.Second, "e7e5")
	moveAfter(t, g, clk, 10*time.Second, "g2g4")
	moveAfter(t, g, clk, 1*time.Second, "d8h4")
	text := g.PGN()
	for _, want := range []string{"{[%clk 0:05:00] [%emt 0:00:02.5]}", "1... e5", "[%clk 0:04:52] [%emt 0:00:10]}"} {
		if !strings.Contains(text, want) {
			t.Errorf("PGN lacks %q:\n%s", want, text)
		}
	}
	games, err := pgn.ParseString(text)
	if err != nil || len(games) != 1 {
		t.Fatalf("parse: %v\n%s", err, text)
	}
	mv := games[0].Moves
	if len(mv) != 4 || mv[0].Spent != 2500*time.Millisecond || mv[0].Clock != 300*time.Second ||
		mv[2].Spent != 10*time.Second || mv[2].Clock != 292*time.Second || mv[3].Spent != time.Second {
		t.Errorf("parsed times: %+v", mv)
	}
}

func TestCorrespondenceKeepsTheRealWaitAndNoClock(t *testing.T) {
	g, clk := started(t, corr3)
	moveAfter(t, g, clk, 30*time.Minute, "e2e4")
	moveAfter(t, g, clk, 50*time.Hour+20*time.Minute+7*time.Second, "e7e5")
	g.mu.Lock()
	mv := append([]MoveRec(nil), g.moves...)
	g.mu.Unlock()
	if mv[0].Ms != (30*time.Minute).Milliseconds() || mv[1].Ms != (50*time.Hour+20*time.Minute+7*time.Second).Milliseconds() || mv[1].Left != 0 {
		t.Fatalf("correspondence times: %+v", mv)
	}
	text := g.PGN()
	if strings.Contains(text, "[%clk") || !strings.Contains(text, "[%emt 50:20:07]") || !strings.Contains(text, `[TimeControl "1/259200"]`) {
		t.Errorf("correspondence PGN:\n%s", text)
	}
	if _, err := pgn.ParseString(text); err != nil {
		t.Errorf("parse: %v", err)
	}
}

func TestMoveTimeIsAtLeastOneMillisecond(t *testing.T) {
	g, _ := started(t, TimeControl{})
	play(t, g, "e2e4") // the fake clock has not moved at all
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.moves[0].Ms != 1 || g.moves[0].Left != 0 {
		t.Errorf("an instant move: %+v", g.moves[0])
	}
}

package play

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const day = 24 * time.Hour

var corr3 = TimeControl{Days: 3}

func TestCorrespondenceTimeControlValidate(t *testing.T) {
	for _, d := range CorrespondenceDays {
		if err := (TimeControl{Days: d}).Validate(); err != nil {
			t.Errorf("%d days: %v", d, err)
		}
	}
	for _, tc := range []TimeControl{{Days: 4}, {Days: -1}, {Days: 30}, {Days: 3, BaseMs: 60_000}, {Days: 3, IncMs: 1000}} {
		if err := tc.Validate(); err == nil {
			t.Errorf("%+v should be refused", tc)
		}
	}
	if (TimeControl{Days: 3}).Validate() != nil || (TimeControl{BaseMs: 300_000, IncMs: 2000}).Validate() != nil {
		t.Error("the usual controls stay valid")
	}
}

func TestCorrespondenceHasNoClockAndDays(t *testing.T) {
	g, clk := started(t, corr3)
	if g.timed() || !g.corr() {
		t.Fatal("a correspondence game has days, not a clock")
	}
	play(t, g, "e2e4")
	clk.advance(2 * day) // a long think is fine
	play(t, g, "e7e5", "g1f3")

	var m Message
	if err := json.Unmarshal(g.Snapshot("anna"), &m); err != nil {
		t.Fatal(err)
	}
	if m.Game.Clock != nil || m.Game.FirstMoveBy != 0 {
		t.Errorf("no clock and no 60 seconds: %+v", m.Game)
	}
	if want := clk.now().Add(3 * day).UnixMilli(); m.Game.Due != want || m.Game.TC.Days != 3 || m.Game.Claimable {
		t.Errorf("due %d, want %d (%+v)", m.Game.Due, want, m.Game)
	}
}

func TestCorrespondenceTimeoutAfterTheDays(t *testing.T) {
	g, clk := started(t, corr3)
	play(t, g, "e2e4", "e7e5", "g1f3")
	clk.advance(3*day - time.Minute)
	g.tick()
	if st, _, _ := state(g); st != StatusPlaying {
		t.Fatalf("still within the days: %s", st)
	}
	clk.advance(2 * time.Minute)
	g.tick()
	st, res, term := state(g)
	if st != StatusEnded || res != "1-0" || term != TermTimeout {
		t.Errorf("Black ran out of time: %s %s %s", st, res, term)
	}
}

func TestCorrespondenceMoveAfterTheDeadlineIsRefusedWhenClaimedAtOnce(t *testing.T) {
	g, clk := started(t, corr3)
	play(t, g, "e2e4", "e7e5")
	clk.advance(4 * day)
	if err := g.Move("anna", "g1f3"); err == nil {
		t.Error("a move after the deadline was taken")
	}
	if _, res, term := state(g); res != "0-1" || term != TermTimeout {
		t.Errorf("White lost on time: %s %s", res, term)
	}
}

func TestCorrespondenceFirstMoveNeverMade(t *testing.T) {
	g, clk := started(t, corr3)
	clk.advance(4 * day)
	g.tick()
	if st, res, term := state(g); st != StatusAborted || res != "" || term != TermNoFirstMove {
		t.Errorf("nobody started: %s %q %s", st, res, term)
	}
}

func TestCorrespondenceClaimByHand(t *testing.T) {
	g, clk := started(t, corr3)
	g.prefs = func(id string) CorrPrefs { return CorrPrefs{AutoClaim: id != "anna"} } // Anna claims by hand
	play(t, g, "e2e4", "e7e5", "g1f3")                                                // Black to move; Anna is waiting
	if err := g.Claim("anna"); err == nil {
		t.Error("claimed while the opponent still has time")
	}
	clk.advance(3*day + time.Hour)
	g.tick()
	if st, _, _ := state(g); st != StatusPlaying {
		t.Fatalf("not ended by itself when the winner claims by hand: %s", st)
	}
	var m Message
	_ = json.Unmarshal(g.Snapshot("anna"), &m)
	if !m.Game.Claimable {
		t.Error("the snapshot should offer the claim")
	}
	if err := g.Claim("boris"); err == nil {
		t.Error("the one out of time claimed their own win")
	}
	if err := g.Claim("anna"); err != nil {
		t.Fatal(err)
	}
	if st, res, term := state(g); st != StatusEnded || res != "1-0" || term != TermTimeout {
		t.Errorf("after the claim: %s %s %s", st, res, term)
	}
}

func TestCorrespondenceLateMoveBeforeTheClaim(t *testing.T) {
	g, clk := started(t, corr3)
	g.prefs = func(id string) CorrPrefs { return CorrPrefs{AutoClaim: id != "anna"} }
	play(t, g, "e2e4", "e7e5", "g1f3")
	clk.advance(4 * day) // late, but Anna has not claimed
	if err := g.Move("boris", "b8c6"); err != nil {
		t.Fatalf("a late move before the claim: %v", err)
	}
	if st, _, _ := state(g); st != StatusPlaying {
		t.Errorf("the game goes on: %s", st)
	}
}

func TestCorrespondenceGivenUpAfterTheGrace(t *testing.T) {
	g, clk := started(t, corr3)
	g.prefs = func(id string) CorrPrefs { return CorrPrefs{AutoClaim: id != "anna"} }
	play(t, g, "e2e4", "e7e5", "g1f3")
	clk.advance(3*day + claimGrace + time.Minute)
	g.tick()
	if st, res, _ := state(g); st != StatusEnded || res != "1-0" {
		t.Errorf("a claim nobody makes is made after the grace: %s %s", st, res)
	}
}

func TestCorrespondenceVacationStopsTheClock(t *testing.T) {
	g, clk := started(t, corr3)
	play(t, g, "e2e4", "e7e5", "g1f3") // Black to move, due in 3 days
	until := clk.now().Add(10 * day)
	g.prefs = func(id string) CorrPrefs {
		if id == "boris" {
			return CorrPrefs{AutoClaim: true, Vacation: until}
		}
		return CorrPrefs{AutoClaim: true}
	}
	clk.advance(5 * day)
	g.tick()
	if st, _, _ := state(g); st != StatusPlaying {
		t.Fatalf("on vacation: %s", st)
	}
	clk.advance(5*day + time.Hour) // the vacation is over, a day of grace is not yet
	g.tick()
	if st, _, _ := state(g); st != StatusPlaying {
		t.Fatalf("a day after the vacation is still left: %s", st)
	}
	clk.advance(day)
	g.tick()
	if st, res, _ := state(g); st != StatusEnded || res != "1-0" {
		t.Errorf("after the vacation: %s %s", st, res)
	}
}

func TestCorrespondenceSurvivesARestart(t *testing.T) {
	g, clk := started(t, corr3)
	play(t, g, "e2e4", "e7e5")
	g.mu.Lock()
	raw := g.stateLocked()
	g.mu.Unlock()
	if !strings.Contains(string(raw), `"days":3`) {
		t.Fatalf("the days are saved: %s", raw)
	}
	clk.advance(2 * day)
	back, err := restore("0123456789abcdef", clk.now, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer back.Close()
	if !back.corr() || back.tc.Days != 3 {
		t.Fatalf("restored: %+v", back.tc)
	}
	// The deadline is that of the move, not of the restart.
	if want := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC).Add(3 * day); !back.dueAt().Equal(want) {
		t.Errorf("due %v, want %v", back.dueAt(), want)
	}
	clk.advance(2 * day)
	back.tick()
	if st, res, _ := state(back); st != StatusEnded || res != "0-1" {
		t.Errorf("White ran out of time while the server was down: %s %s", st, res)
	}
}

func TestCorrespondenceRematchKeepsTheDays(t *testing.T) {
	g, _ := started(t, corr3)
	play(t, g, "e2e4")
	if err := g.Resign("anna"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Rematch("anna", "1111111111111111"); err != nil {
		t.Fatal(err)
	}
	n, err := g.Rematch("boris", "1111111111111111")
	if err != nil || n == nil {
		t.Fatalf("rematch: %v %v", n, err)
	}
	defer n.Close()
	if n.tc.Days != 3 || n.white.ID != "boris" {
		t.Errorf("rematch: %+v white %v", n.tc, n.white)
	}
}

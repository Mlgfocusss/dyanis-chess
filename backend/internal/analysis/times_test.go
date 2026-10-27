package analysis

import (
	"encoding/json"
	"strings"
	"testing"

	"dyanis-chess/backend/internal/pgn"
)

func timesOf(t *testing.T, text string) []int64 {
	t.Helper()
	g, err := pgn.ParseOne(text)
	if err != nil {
		t.Fatal(err)
	}
	return moveTimes(g, startsBlack(g))
}

func eq(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTimesFromClocksAndIncrement(t *testing.T) {
	// 300+3: white 300 -> 297 (3 s spent: 300+3-300... see below), each value worked out by hand
	got := timesOf(t, `[TimeControl "300+3"]

1. e4 {[%clk 0:05:00]} e5 {[%clk 0:04:58]} 2. Nf3 {[%clk 0:04:53]} Nc6 {[%clk 0:04:59]} 1-0`)
	// white: 300+3-300 = 3 s; black: 300+3-298 = 5 s; white: 300+3-293 = 10 s; black: 298+3-299 = 2 s
	if want := []int64{3000, 5000, 10000, 2000}; !eq(got, want) {
		t.Errorf("times %v, want %v", got, want)
	}
}

func TestTimesWithoutIncrementInTheTag(t *testing.T) {
	got := timesOf(t, `[TimeControl "600"]

1. e4 {[%clk 0:09:58]} e5 {[%clk 0:09:55]} 2. Nf3 {[%clk 0:09:50]} Nc6 {[%clk 0:09:54]} 1-0`)
	if want := []int64{2000, 5000, 8000, 1000}; !eq(got, want) {
		t.Errorf("times %v, want %v", got, want)
	}
}

func TestTimesGuessTheIncrementFromThePairs(t *testing.T) {
	// no TimeControl tag: the first moves are unknown, the increment (2 s) shows as the clock rising
	got := timesOf(t, `[Event "x"]

1. e4 {[%clk 0:03:00]} e5 {[%clk 0:03:00]} 2. Nf3 {[%clk 0:03:02]} Nc6 {[%clk 0:02:55]} 3. Bb5 {[%clk 0:02:59]} a6 {[%clk 0:02:50]} 1-0`)
	// the white clock rose 2 s (180 -> 182): an increment of 2 s. White move 2: 180+2-182 = 0; move 3: 182+2-179 = 5.
	// Black move 2: 180+2-175 = 7; move 3: 175+2-170 = 7. The first moves of both have no time.
	if want := []int64{-1, -1, 0, 7000, 5000, 7000}; !eq(got, want) {
		t.Errorf("times %v, want %v", got, want)
	}
}

func TestTimesPreferTheMoveTimeAnnotation(t *testing.T) {
	got := timesOf(t, `[TimeControl "300+3"]

1. e4 {[%clk 0:05:00] [%emt 0:00:02.4]} e5 {[%emt 0:00:01]} 1/2-1/2`)
	if want := []int64{2400, 1000}; !eq(got, want) {
		t.Errorf("times %v, want %v", got, want)
	}
}

func TestNoClocksNoTimes(t *testing.T) {
	got := timesOf(t, `[TimeControl "-"]

1. e4 e5 2. Nf3 1-0`)
	if want := []int64{-1, -1, -1}; !eq(got, want) {
		t.Errorf("times %v, want %v", got, want)
	}
}

func TestAMissingClockDoesNotPoisonTheNextMove(t *testing.T) {
	got := timesOf(t, `[TimeControl "300"]

1. e4 {[%clk 0:04:55]} e5 {[%clk 0:04:58]} 2. Nf3 Nc6 3. Bb5 {[%clk 0:04:40]} a6 {[%clk 0:04:50]} 4. Ba4 {[%clk 0:04:30]} b5 {[%clk 0:04:45]} 1-0`)
	// the moves without a clock have no time, and the move after them has no clock to start from either;
	// from the next one on the times are known again
	if want := []int64{5000, 2000, -1, -1, -1, -1, 10000, 5000}; !eq(got, want) {
		t.Errorf("times %v, want %v", got, want)
	}
}

func TestReportCarriesTheMoveTimes(t *testing.T) {
	g, err := pgn.ParseOne(`[TimeControl "300+2"]

1. e4 {[%clk 0:04:58]} e5 {[%clk 0:05:00]} 1/2-1/2`)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Analyze(g, Options{Depth: 4, Workers: 1, NoSpecial: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Moves[0].TimeMs == nil || *rep.Moves[0].TimeMs != 4000 || rep.Moves[1].TimeMs == nil || *rep.Moves[1].TimeMs != 2000 {
		t.Fatalf("move times: %+v %+v", rep.Moves[0].TimeMs, rep.Moves[1].TimeMs)
	}
	// ... and a game without clocks leaves the field out of the JSON, as an old report did
	g2, _ := pgn.ParseOne("1. e4 e5 1/2-1/2")
	rep2, err := Analyze(g2, Options{Depth: 4, Workers: 1, NoSpecial: true})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(rep2.Moves[0])
	if strings.Contains(string(b), "timeMs") {
		t.Errorf("no times were known: %s", b)
	}
}

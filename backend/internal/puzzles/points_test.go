package puzzles

import (
	"encoding/json"
	"testing"
)

func TestPuzzlePoints(t *testing.T) {
	cases := []struct {
		rating, streak int
		hint           bool
		want           int
	}{
		{1900, 0, false, 19}, // rating / 100
		{1949, 0, false, 19},
		{1950, 0, false, 20}, // rounded
		{400, 0, false, 4},
		{100, 0, false, 1},
		{20, 0, false, 1},  // never below the minimum
		{1900, 0, true, 9}, // the hint halves it, rounded down
		{300, 0, true, 1},  // 3 / 2 = 1
		{100, 0, true, 1},  // not below the minimum
		{1900, 3, false, 19},
		{1900, 4, false, 20}, // the fifth in a row earns the bonus
		{1900, 5, false, 19},
		{1900, 9, false, 20}, // the tenth
		{1900, 4, true, 10},  // the hint first, then the bonus
	}
	for _, c := range cases {
		if got := PuzzlePoints(c.rating, c.streak, c.hint); got != c.want {
			t.Errorf("PuzzlePoints(%d, %d, %v) = %d, want %d", c.rating, c.streak, c.hint, got, c.want)
		}
	}
}

func TestResultInPointsMode(t *testing.T) {
	f, h := newTest("u1") // puzzle 00008 has rating 1900, the person 1200
	var got resultResponse
	post := func(body string) {
		rec := do(h, "POST", "/api/puzzles/00008/result", body)
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", body, rec.Code, rec.Body)
		}
		got = resultResponse{}
		json.Unmarshal(rec.Body.Bytes(), &got)
	}
	post(`{"solved":true,"mode":"points"}`)
	if !got.Counted || got.After != 1200 || got.You.Rating != 1200 || got.You.Attempts != 31 {
		t.Errorf("the rating must stay: %+v", got)
	}
	if got.Points == nil || got.Points.Gained != 19 || got.Points.Total != 19 || got.Points.Streak != 1 {
		t.Errorf("points: %+v", got.Points)
	}
	if len(f.recorded) != 1 || !f.recorded[0] {
		t.Errorf("the try is recorded as the first: %v", f.recorded)
	}
	post(`{"solved":true,"mode":"points","hint":true}`)
	if got.Points.Gained != 9 || got.Points.Total != 28 || got.Points.Streak != 2 {
		t.Errorf("with the hint: %+v", got.Points)
	}
	post(`{"solved":false,"mode":"points"}`)
	if got.Points.Gained != 0 || got.Points.Total != 28 || got.Points.Streak != 0 || got.Points.Best != 2 {
		t.Errorf("a failure takes nothing and ends the series: %+v", got.Points)
	}
	f.tried = true // a repeat: nothing earned, the standing is shown
	post(`{"solved":true,"mode":"points"}`)
	if got.Counted || got.Points == nil || got.Points.Gained != 0 || got.Points.Total != 28 {
		t.Errorf("a repeat: %+v", got)
	}
	// the browser cannot send points of its own
	post(`{"solved":true,"mode":"points","points":1000,"gained":1000}`)
	if got.Points.Total != 28 {
		t.Errorf("client points taken: %+v", got.Points)
	}
	if rec := do(h, "POST", "/api/puzzles/00008/result", `{"solved":true,"mode":"zzz"}`); rec.Code != 400 {
		t.Errorf("unknown mode: %d", rec.Code)
	}
}

func TestNextAsksForPoints(t *testing.T) {
	f, h := newTest("u1")
	f.points.Total = 77
	rec := do(h, "GET", "/api/puzzles/next?mode=points", "")
	var got nextResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Points == nil || got.Points.Total != 77 {
		t.Errorf("points: %+v", got.Points)
	}
	rec = do(h, "GET", "/api/puzzles/next", "")
	got = nextResponse{}
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Points != nil {
		t.Errorf("points without asking: %+v", got.Points)
	}
}

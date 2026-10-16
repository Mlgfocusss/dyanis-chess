package puzzles

import (
	"strings"
	"testing"
)

func TestRushNext(t *testing.T) {
	f, h := newTest("")
	rec := do(h, "GET", "/api/puzzles/rush/next?rating=700", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"id":"00008"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if len(f.picked) == 0 || f.picked[0][0] != 600 || f.picked[0][1] != 800 {
		t.Errorf("window around 700: %v", f.picked)
	}
	// the rating is clamped, a bad one means the easiest
	f.picked = nil
	do(h, "GET", "/api/puzzles/rush/next?rating=99999", "")
	do(h, "GET", "/api/puzzles/rush/next?rating=oops", "")
	if f.picked[0][0] != 2900 || f.picked[1][0] != 300 {
		t.Errorf("clamp: %v", f.picked)
	}
	// puzzles of the run are not offered again when another can be had (here every pick is the same, so it gives up)
	f.picked = nil
	rec = do(h, "GET", "/api/puzzles/rush/next?rating=700&exclude=00008", "")
	if rec.Code != 200 || len(f.picked) != rushTries {
		t.Errorf("exclude: %d, %d picks", rec.Code, len(f.picked))
	}
}

func TestRushResult(t *testing.T) {
	f, h := newTest("u1")
	rec := do(h, "POST", "/api/puzzles/rush/result", `{"mode":"3min","score":17}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"best":12`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if len(f.rush) != 1 || f.rush[0] != "3min 17" {
		t.Errorf("recorded: %v", f.rush)
	}
	if len(f.recorded) != 0 {
		t.Error("the storm moved the puzzle rating")
	}
	for _, bad := range []string{`{"mode":"9min","score":1}`, `{"mode":"3min","score":-1}`, `{"mode":"3min","score":9999}`, `oops`} {
		if rec := do(h, "POST", "/api/puzzles/rush/result", bad); rec.Code != 400 {
			t.Errorf("%s: %d", bad, rec.Code)
		}
	}
	_, anon := newTest("")
	if rec := do(anon, "POST", "/api/puzzles/rush/result", `{"mode":"3min","score":1}`); rec.Code != 401 {
		t.Errorf("nobody signed in: %d", rec.Code)
	}
	if rec := do(anon, "GET", "/api/puzzles/rush/best", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"best":{}`) {
		t.Errorf("best for nobody: %d %s", rec.Code, rec.Body)
	}
}

func TestLevelRating(t *testing.T) {
	rnd := func(n int) int { return n - 1 } // the top of the range
	for _, c := range []struct {
		rating int
		level  string
		want   int
	}{
		{1200, "", 1200}, {1200, "normal", 1200}, {1200, "oops", 1200},
		{1200, "easier", 1050}, {1200, "harder", 1350},
		{1200, "random", 1500},                       // rnd gives 600, shifted by -300 to +300
		{450, "easier", 400}, {2950, "harder", 3000}, // the ends
	} {
		if got := levelRating(c.rating, c.level, rnd); got != c.want {
			t.Errorf("%d %q: %d, want %d", c.rating, c.level, got, c.want)
		}
	}
	if got := levelRating(1200, "random", func(int) int { return 0 }); got != 900 {
		t.Errorf("the bottom of the random range: %d", got)
	}
}

func TestNextUsesTheLevel(t *testing.T) {
	f, h := newTest("u1") // the person's rating is 1200
	for url, want := range map[string]int{
		"/api/puzzles/next":              1100, // the window is 100 either way: 1100..1300
		"/api/puzzles/next?level=harder": 1250, // 1350 - 100
		"/api/puzzles/next?level=easier": 950,  // 1050 - 100
	} {
		f.picked = nil
		do(h, "GET", url, "")
		if len(f.picked) == 0 || f.picked[0][0] != want {
			t.Errorf("%s: window starts at %v, want %d", url, f.picked, want)
		}
	}
}

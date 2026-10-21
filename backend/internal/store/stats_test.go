package store

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

var (
	statsSeq  atomic.Int64
	statsBase = time.Now().UnixNano() & 0xffffff // differs between runs, so parallel test runs do not meet
)

// statsID is an id that no other test uses, at most 12 characters: a prefix, a number that is
// fixed for this run and a counter.
func statsID(prefix string) string {
	return fmt.Sprintf("%s%06x%03x", prefix, statsBase, statsSeq.Add(1)&0xfff)
}

func statsUser(t *testing.T, s *Store) *User {
	t.Helper()
	u := &User{ID: statsID("st"), Kind: KindGuest, Name: "Статистика"}
	if err := s.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.db.Exec("DELETE FROM users WHERE id = ?", u.ID) })
	return u
}

func TestStatsGameAndDuelTally(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	me, other := statsUser(t, s), statsUser(t, s)
	now := time.Now().UTC()

	save := func(white, black, status, result string) {
		t.Helper()
		g := &Game{ID: statsID("sg"), WhiteID: white, BlackID: black, Status: status, Result: result, State: []byte(`{}`), CreatedAt: now, UpdatedAt: now}
		if err := s.SaveGame(ctx, g); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.db.Exec("DELETE FROM games WHERE id = ?", g.ID) })
	}
	// against a person: a win as White, a loss as Black, a draw as White
	save(me.ID, other.ID, "ended", "1-0")
	save(other.ID, me.ID, "ended", "1-0")
	save(me.ID, other.ID, "ended", "1/2-1/2")
	// against a bot (its seat is empty): a win as Black, a win as White, a loss as White
	save("", me.ID, "ended", "0-1")
	save(me.ID, "", "ended", "1-0")
	save(me.ID, "", "ended", "0-1")
	// not counted: still going, cancelled
	save(me.ID, other.ID, "playing", "")
	save(me.ID, "", "aborted", "")

	humans, bots, err := s.GameTally(ctx, me.ID)
	if err != nil {
		t.Fatal(err)
	}
	if humans != (Tally{Wins: 1, Draws: 1, Losses: 1}) || bots != (Tally{Wins: 2, Losses: 1}) {
		t.Errorf("humans %+v bots %+v", humans, bots)
	}
	if humans.Total() != 3 || bots.Total() != 3 {
		t.Errorf("totals %d %d", humans.Total(), bots.Total())
	}
	// the other person's view: they won as White and lost as Black, drew once
	oh, ob, err := s.GameTally(ctx, other.ID)
	if err != nil || oh != (Tally{Wins: 1, Draws: 1, Losses: 1}) || ob.Total() != 0 {
		t.Errorf("other: %+v %+v (%v)", oh, ob, err)
	}

	// duels: a win as A, a loss as B, a draw as A
	duel := func(a, b, result string) {
		t.Helper()
		id := statsID("sd")
		if err := s.SaveDuel(ctx, &DuelRow{ID: id, AID: a, BID: b, Result: result, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.db.Exec("DELETE FROM duels WHERE id = ?", id) })
	}
	duel(me.ID, other.ID, "a")
	duel(other.ID, me.ID, "a")
	duel(me.ID, other.ID, "draw")
	d, err := s.DuelTally(ctx, me.ID)
	if err != nil || d != (Tally{Wins: 1, Draws: 1, Losses: 1}) {
		t.Errorf("duels: %+v (%v)", d, err)
	}
}

func TestStatsReviews(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	me, other := statsUser(t, s), statsUser(t, s)
	base := time.Now().UTC().Truncate(time.Millisecond).Add(-time.Hour)

	if n, err := s.ReviewCount(ctx, me.ID); err != nil || n != 0 {
		t.Fatalf("count before: %d (%v)", n, err)
	}
	save := func(owner string, i int, status, summary string) string {
		t.Helper()
		id := statsID("sa")
		a := &Analysis{ID: id, UserID: owner, Status: status, Depth: 10, Game: []byte(`{"white":"A","black":"B"}`),
			CreatedAt: base.Add(time.Duration(i) * time.Minute), FinishedAt: base.Add(time.Duration(i) * time.Minute)}
		if status == "done" {
			a.Report = []byte(`{}`)
		}
		if summary != "" {
			a.Summary = []byte(summary)
		}
		if err := s.SaveAnalysis(ctx, a); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.db.Exec("DELETE FROM analyses WHERE id = ?", id) })
		return id
	}
	first := save(me.ID, 0, "done", `{"accuracy":[80,60],"plies":30}`)
	second := save(me.ID, 1, "done", `{"accuracy":[70,90],"plies":40}`)
	save(me.ID, 2, "done", "")                                   // no summary: counted, not sampled
	save(me.ID, 3, "error", "")                                  // failed: neither
	save(other.ID, 4, "done", `{"accuracy":[10,10],"plies":10}`) // someone else's

	if n, err := s.ReviewCount(ctx, me.ID); err != nil || n != 3 {
		t.Errorf("count: %d (%v)", n, err)
	}
	got, err := s.RecentReviews(ctx, me.ID, 10)
	if err != nil || len(got) != 2 || got[0].ID != second || got[1].ID != first {
		t.Fatalf("recent: %+v (%v)", got, err)
	}
	if string(got[0].Game) == "" || string(got[0].Summary) == "" || !got[1].CreatedAt.Equal(base) {
		t.Errorf("fields: %+v", got[0])
	}
	if got, _ := s.RecentReviews(ctx, me.ID, 1); len(got) != 1 || got[0].ID != second {
		t.Errorf("limit: %+v", got)
	}
}

func TestStatsPuzzles(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	me, other := statsUser(t, s), statsUser(t, s)

	ids := map[string]string{}
	var batch []Puzzle
	for _, p := range []struct{ key, themes string }{
		{"fork1", "fork short"}, {"fork2", "fork middlegame"}, {"pin1", "pin"}, {"fork3", "fork"},
	} {
		id := statsID("p")
		ids[p.key] = id
		batch = append(batch, Puzzle{ID: id, FEN: "8/8/8/8/8/8/8/K6k w - - 0 1", Moves: "a1a2 h1h2", Rating: 1500, Themes: p.themes})
	}
	if _, err := s.ImportPuzzles(ctx, batch); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, id := range ids {
			s.db.Exec("DELETE FROM puzzles WHERE id = ?", id)
		}
	})

	// fork1 failed, fork2 failed, pin1 solved, fork3 solved; the other person failed pin1
	rate := 1000
	for _, a := range []struct {
		key    string
		solved bool
	}{{"fork1", false}, {"fork2", false}, {"pin1", true}, {"fork3", true}} {
		next := rate - 10
		if a.solved {
			next = rate + 10
		}
		if _, err := s.RecordPuzzleAttempt(ctx, me.ID, ids[a.key], a.solved, func(int, int) int { return next }, nil); err != nil {
			t.Fatal(err)
		}
		rate = next
	}
	if _, err := s.RecordPuzzleAttempt(ctx, other.ID, ids["pin1"], false, func(r, _ int) int { return r - 10 }, nil); err != nil {
		t.Fatal(err)
	}

	fails, err := s.PuzzleThemeFailures(ctx, me.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ThemeFailures{}
	for _, f := range fails {
		got[f.Theme] = f
	}
	if got["fork"] != (ThemeFailures{"fork", 3, 2}) || got["short"] != (ThemeFailures{"short", 1, 1}) ||
		got["middlegame"] != (ThemeFailures{"middlegame", 1, 1}) {
		t.Errorf("failures: %+v", got)
	}
	if _, ok := got["pin"]; ok {
		t.Errorf("a theme without failures is listed: %+v", got["pin"])
	}

	hist, err := s.RecentPuzzleAttempts(ctx, me.ID, 3)
	if err != nil || len(hist) != 3 {
		t.Fatalf("history: %+v (%v)", hist, err)
	}
	if hist[0].PuzzleID != ids["fork3"] || !hist[0].Solved || hist[0].After-hist[0].Before != 10 || hist[2].PuzzleID != ids["fork2"] {
		t.Errorf("history order: %+v", hist)
	}
}

func TestStatsPuzzleDays(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	me, other := statsUser(t, s), statsUser(t, s)

	at := func(day string, hour int) time.Time {
		d, err := time.Parse("2006-01-02", day)
		if err != nil {
			t.Fatal(err)
		}
		return d.Add(time.Duration(hour) * time.Hour)
	}
	attempt := func(user string, when time.Time) {
		t.Helper()
		if _, err := s.db.Exec(`INSERT INTO puzzle_attempts (user_id, puzzle_id, solved, rating_before, rating_after, created_at)
			VALUES (?, ?, 1, 1000, 1010, ?)`, user, statsID("x"), when); err != nil {
			t.Fatal(err)
		}
	}
	// 2020-03-01: two tries, the second one just before midnight UTC
	attempt(me.ID, at("2020-03-01", 8))
	attempt(me.ID, at("2020-03-01", 23).Add(59*time.Minute))
	attempt(other.ID, at("2020-03-05", 1))
	// 2020-03-02: the puzzle of the day only
	if _, err := s.db.Exec(`INSERT INTO daily_puzzle_results (user_id, day, puzzle_id, solved, created_at) VALUES (?, '2020-03-02', 'abc', 1, ?)`,
		me.ID, at("2020-03-02", 5)); err != nil {
		t.Fatal(err)
	}
	// 2020-03-03: a storm run and a duel as the second player
	if err := s.RecordRush(ctx, me.ID, "3min", 12); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE rush_results SET created_at = ? WHERE user_id = ?", at("2020-03-03", 12), me.ID); err != nil {
		t.Fatal(err)
	}
	duel := statsID("sd")
	if err := s.SaveDuel(ctx, &DuelRow{ID: duel, AID: other.ID, BID: me.ID, Result: "b", CreatedAt: at("2020-03-03", 13)}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.db.Exec("DELETE FROM duels WHERE id = ?", duel) })

	days, err := s.PuzzleDays(ctx, me.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"2020-03-01": 2, "2020-03-02": 1, "2020-03-03": 2}
	if len(days) != len(want) {
		t.Fatalf("days: %v", days)
	}
	for k, v := range want {
		if days[k] != v {
			t.Errorf("day %s: %d, want %d (%v)", k, days[k], v, days)
		}
	}
	if empty, err := s.PuzzleDays(ctx, statsUser(t, s).ID); err != nil || len(empty) != 0 {
		t.Errorf("a person with no puzzles: %v (%v)", empty, err)
	}
}

package store

import (
	"context"
	"testing"
	"time"
)

func TestTrainerPracticeAndPuzzlesSince(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	me, other := statsUser(t, s), statsUser(t, s)

	ids := map[string]string{}
	var batch []Puzzle
	for _, p := range []struct{ key, themes string }{{"fork1", "fork short"}, {"fork2", "fork"}, {"pin1", "pin"}, {"fork3", "fork"}} {
		id := statsID("tp")
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

	now := time.Now().UTC()
	dayStart := now.Truncate(24 * time.Hour)
	attempt := func(user, puzzle string, at time.Time) {
		t.Helper()
		if _, err := s.db.Exec(`INSERT INTO puzzle_attempts (user_id, puzzle_id, solved, rating_before, rating_after, created_at)
			VALUES (?, ?, 1, 1000, 1010, ?)`, user, puzzle, at); err != nil {
			t.Fatal(err)
		}
	}
	// today: a rated try at a fork puzzle and one at a pin puzzle; yesterday: a rated fork try (not today's)
	attempt(me.ID, ids["fork1"], dayStart.Add(time.Second))
	attempt(me.ID, ids["pin1"], dayStart.Add(2*time.Second))
	attempt(me.ID, ids["fork3"], dayStart.Add(-time.Hour))
	attempt(other.ID, ids["fork1"], dayStart.Add(time.Second))

	if all, on, err := s.PuzzlesSince(ctx, me.ID, dayStart, "fork"); err != nil || all != 2 || on != 1 {
		t.Fatalf("before practice: %d %d (%v)", all, on, err)
	}

	// practice: a fork puzzle, recorded twice (the second is ignored), and a pin one; and another day's
	if err := s.RecordPractice(ctx, me.ID, ids["fork2"], false, now); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordPractice(ctx, me.ID, ids["fork2"], true, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordPractice(ctx, me.ID, ids["pin1"], true, now); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordPractice(ctx, me.ID, ids["fork3"], true, dayStart.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordPractice(ctx, other.ID, ids["fork2"], true, now); err != nil {
		t.Fatal(err)
	}
	var solved bool
	if err := s.db.QueryRow("SELECT solved FROM trainer_practice WHERE user_id = ? AND puzzle_id = ?", me.ID, ids["fork2"]).Scan(&solved); err != nil || solved {
		t.Errorf("the first try of the day counts: solved=%v (%v)", solved, err)
	}

	all, on, err := s.PuzzlesSince(ctx, me.ID, dayStart, "fork")
	if err != nil || all != 4 || on != 2 {
		t.Errorf("fork: %d %d (%v)", all, on, err)
	}
	if all, on, err := s.PuzzlesSince(ctx, me.ID, dayStart, "pin"); err != nil || all != 4 || on != 2 {
		t.Errorf("pin: %d %d (%v)", all, on, err)
	}
	if all, on, err := s.PuzzlesSince(ctx, me.ID, dayStart, ""); err != nil || all != 4 || on != 0 {
		t.Errorf("no theme: %d %d (%v)", all, on, err)
	}
	if all, on, err := s.PuzzlesSince(ctx, me.ID, dayStart, "endgame"); err != nil || all != 4 || on != 0 {
		t.Errorf("a theme nobody did: %d %d (%v)", all, on, err)
	}
	// a longer look-back sees yesterday as well
	if all, on, err := s.PuzzlesSince(ctx, me.ID, dayStart.AddDate(0, 0, -1), "fork"); err != nil || all != 6 || on != 4 {
		t.Errorf("two days: %d %d (%v)", all, on, err)
	}
	if all, _, err := s.PuzzlesSince(ctx, statsUser(t, s).ID, dayStart, "fork"); err != nil || all != 0 {
		t.Errorf("a stranger: %d (%v)", all, err)
	}

	// the practice goes away with the person
	if _, err := s.db.Exec("DELETE FROM users WHERE id = ?", other.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM trainer_practice WHERE user_id = ?", other.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("practice of a deleted person: %d (%v)", n, err)
	}
}

func TestTrainerRepertoireReviewedSince(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	me, other := statsUser(t, s), statsUser(t, s)
	day := time.Now().UTC().Truncate(24 * time.Hour)

	rep, err := s.CreateRepertoire(ctx, me.ID, "Тренер", "white")
	if err != nil {
		t.Fatal(err)
	}
	otherRep, err := s.CreateRepertoire(ctx, other.ID, "Чужой", "white")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.DeleteRepertoire(ctx, me.ID, rep.ID)
		s.DeleteRepertoire(ctx, other.ID, otherRep.ID)
	})
	add := func(r *Repertoire, uci string, parent int64, ply int, mine bool) RepMove {
		t.Helper()
		m, _, err := s.AddRepMove(ctx, RepMove{RepertoireID: r.ID, ParentID: parent, UCI: uci, SAN: uci, FEN: "x", Ply: ply, Mine: mine})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	a := add(rep, "e2e4", 0, 1, true)
	b := add(rep, "e7e5", a.ID, 2, false)
	c := add(rep, "g1f3", b.ID, 3, true)
	d := add(rep, "b1c3", b.ID, 3, true)
	foreign := add(otherRep, "e2e4", 0, 1, true)

	if n, err := s.RepertoireReviewedSince(ctx, me.ID, day); err != nil || n != 0 {
		t.Fatalf("before: %d (%v)", n, err)
	}
	next := func(box, lapses int) Card { return Card{Box: 1, DueAt: time.Now().Add(time.Hour), Lapses: lapses} }
	for _, m := range []RepMove{a, c} {
		if _, err := s.ReviewRepMove(ctx, rep.ID, m.ID, next); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.ReviewRepMove(ctx, rep.ID, a.ID, next); err != nil { // the same card again: still one
		t.Fatal(err)
	}
	if _, err := s.ReviewRepMove(ctx, otherRep.ID, foreign.ID, next); err != nil {
		t.Fatal(err)
	}
	if n, err := s.RepertoireReviewedSince(ctx, me.ID, day); err != nil || n != 2 {
		t.Errorf("today: %d (%v)", n, err)
	}
	// d was answered yesterday
	if _, err := s.db.Exec("UPDATE rep_moves SET box = 1, due_at = ?, reviewed_at = ? WHERE id = ?", day, day.Add(-time.Hour), d.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.RepertoireReviewedSince(ctx, me.ID, day); n != 2 {
		t.Errorf("yesterday's answer counted: %d", n)
	}
	if n, _ := s.RepertoireReviewedSince(ctx, me.ID, day.AddDate(0, 0, -1)); n != 3 {
		t.Errorf("two days: %d", n)
	}
}

func TestTrainerRecentReports(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	me, other := statsUser(t, s), statsUser(t, s)
	base := time.Now().UTC().Truncate(time.Millisecond).Add(-time.Hour)
	save := func(owner string, minute int, status string, report string) string {
		t.Helper()
		id := statsID("tr")
		a := &Analysis{ID: id, UserID: owner, Status: status, Depth: 10, Game: []byte(`{"white":"A","black":"B"}`),
			CreatedAt: base.Add(time.Duration(minute) * time.Minute), FinishedAt: base.Add(time.Duration(minute) * time.Minute)}
		if report != "" {
			a.Report = []byte(report)
		}
		if err := s.SaveAnalysis(ctx, a); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.db.Exec("DELETE FROM analyses WHERE id = ?", id) })
		return id
	}
	first := save(me.ID, 0, "done", `{"moves":[],"accuracy":[80,70]}`)
	second := save(me.ID, 1, "done", `{"moves":[],"accuracy":[60,50]}`)
	save(me.ID, 2, "error", "")
	save(me.ID, 3, "done", "") // no report
	save(other.ID, 4, "done", `{"moves":[]}`)

	got, err := s.RecentReports(ctx, me.ID, 10)
	if err != nil || len(got) != 2 || got[0].ID != second || got[1].ID != first {
		t.Fatalf("reports: %+v (%v)", got, err)
	}
	if string(got[0].Game) == "" || len(got[0].Report) == 0 || !got[1].CreatedAt.Equal(base) {
		t.Errorf("fields: %+v", got[0])
	}
	if one, _ := s.RecentReports(ctx, me.ID, 1); len(one) != 1 || one[0].ID != second {
		t.Errorf("limit: %+v", one)
	}
}

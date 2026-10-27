package store

import (
	"context"
	"testing"
	"time"
)

func TestPuzzlePointsRecordMergeDelete(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	guest, google := lessonUser(t, s), lessonUser(t, s)
	keep := func(r, _ int) int { return r }
	award := func(streak int) int { return 10 + streak }
	try := func(u *User, puzzle string, solved bool) PuzzleAttempt {
		t.Helper()
		at, err := s.RecordPuzzleAttempt(ctx, u.ID, puzzle, solved, keep, award)
		if err != nil {
			t.Fatal(err)
		}
		return at
	}

	at := try(guest, "pp-a", true) // 10, series 1
	if !at.Counted || at.Gained != 10 || at.Points.Total != 10 || at.Points.Streak != 1 || at.Points.Today != 10 || at.After != at.Before {
		t.Fatalf("first: %+v %+v", at, at.Points)
	}
	at = try(guest, "pp-b", true) // 11, series 2
	if at.Gained != 11 || at.Points.Total != 21 || at.Points.Streak != 2 || at.Points.BestStreak != 2 {
		t.Fatalf("second: %+v %+v", at, at.Points)
	}
	at = try(guest, "pp-c", false) // nothing, series over
	if at.Gained != 0 || at.Points.Total != 21 || at.Points.Streak != 0 || at.Points.BestStreak != 2 {
		t.Fatalf("failure: %+v %+v", at, at.Points)
	}
	at = try(guest, "pp-a", true) // tried before: nothing changes
	if at.Counted || at.Points != nil {
		t.Fatalf("repeat: %+v", at)
	}
	day := time.Now().UTC().Format("2006-01-02")
	if p, err := s.PuzzlePointsOf(ctx, guest.ID, day); err != nil || p.Total != 21 || p.Today != 21 {
		t.Fatalf("PuzzlePointsOf: %+v %v", p, err)
	}
	if p, _ := s.PuzzlePointsOf(ctx, guest.ID, "2001-01-01"); p.Today != 0 || p.Total != 21 {
		t.Errorf("another day: %+v", p)
	}
	// the rating stayed, the tries are recorded
	if r, _ := s.PuzzleRatingOf(ctx, guest.ID); r.Rating != StartPuzzleRating || r.Attempts != 3 || r.Solved != 2 {
		t.Errorf("rating record: %+v", r)
	}

	// the other account has points and a better series
	for i, id := range []string{"pp-d", "pp-e", "pp-f"} {
		try(google, id, true)
		_ = i
	}
	try(google, "pp-g", false)
	try(google, "pp-h", true) // series 1 now, best 3
	if err := s.MergeUsers(ctx, guest.ID, google.ID); err != nil {
		t.Fatal(err)
	}
	p, err := s.PuzzlePointsOf(ctx, google.ID, day)
	if err != nil {
		t.Fatal(err)
	}
	// google: 10 + 11 + 12 + 10 = 43; guest: 21
	if p.Total != 64 || p.Today != 64 || p.BestStreak != 3 || p.Streak != 1 {
		t.Errorf("merged: %+v", p)
	}

	// deleting a user takes the rows with them
	if err := s.DeleteUser(ctx, google.ID); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range []string{"puzzle_points", "puzzle_points_days"} {
		var n int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM "+tbl+" WHERE user_id IN (?, ?)", guest.ID, google.ID).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s: %d rows left, %v", tbl, n, err)
		}
	}
}

// The rated puzzles move the rating and count the points in the one transaction.
func TestPuzzleAttemptMovesRatingAndPoints(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u := lessonUser(t, s)
	up := func(r, _ int) int { return r + 18 }
	down := func(r, _ int) int { return r - 26 }
	award := func(streak int) int { return 7 + streak }

	at, err := s.RecordPuzzleAttempt(ctx, u.ID, "rp-a", true, up, award)
	if err != nil {
		t.Fatal(err)
	}
	if !at.Counted || at.Before != StartPuzzleRating || at.After != StartPuzzleRating+18 || at.Gained != 7 || at.Points.Total != 7 || at.Points.Streak != 1 {
		t.Fatalf("solved: %+v %+v", at, at.Points)
	}
	at, err = s.RecordPuzzleAttempt(ctx, u.ID, "rp-b", false, down, award)
	if err != nil {
		t.Fatal(err)
	}
	if at.After != StartPuzzleRating+18-26 || at.Gained != 0 || at.Points.Total != 7 || at.Points.Streak != 0 || at.Points.BestStreak != 1 {
		t.Fatalf("failed: %+v %+v", at, at.Points)
	}
	// a repeat changes neither
	at, err = s.RecordPuzzleAttempt(ctx, u.ID, "rp-a", true, up, award)
	if err != nil || at.Counted || at.Points != nil || at.After != at.Before {
		t.Fatalf("repeat: %+v %v", at, err)
	}
	day := time.Now().UTC().Format("2006-01-02")
	if p, err := s.PuzzlePointsOf(ctx, u.ID, day); err != nil || p.Total != 7 || p.Today != 7 {
		t.Errorf("points: %+v %v", p, err)
	}
	if r, _ := s.PuzzleRatingOf(ctx, u.ID); r.Rating != StartPuzzleRating-8 || r.Attempts != 2 || r.Solved != 1 {
		t.Errorf("rating: %+v", r)
	}
}

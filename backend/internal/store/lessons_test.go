package store

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

var (
	lessonSeq  atomic.Int64
	lessonBase = time.Now().UnixNano() & 0xffffff // differs between runs, so parallel test runs do not meet
)

func lessonUser(t *testing.T, s *Store) *User {
	t.Helper()
	u := &User{ID: fmt.Sprintf("ls%06x%03x", lessonBase, lessonSeq.Add(1)&0xfff), Kind: KindGuest, Name: "Ученик"}
	if err := s.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.db.Exec("DELETE FROM users WHERE id = ?", u.ID) })
	return u
}

func TestLessonProgress(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u := lessonUser(t, s)

	if m, err := s.LessonProgressOf(ctx, u.ID); err != nil || len(m) != 0 {
		t.Fatalf("a new person has no progress: %v %v", m, err)
	}
	p, err := s.SaveLessonProgress(ctx, u.ID, "rook", 2, 1, false)
	if err != nil || p.Step != 2 || p.Mistakes != 1 || p.Completed() {
		t.Fatalf("first save: %+v %v", p, err)
	}
	// forward: the step and the mistakes of the further run
	p, _ = s.SaveLessonProgress(ctx, u.ID, "rook", 4, 3, false)
	if p.Step != 4 || p.Mistakes != 3 {
		t.Errorf("forward: %+v", p)
	}
	// a person who starts over reports a smaller step: nothing goes back
	p, _ = s.SaveLessonProgress(ctx, u.ID, "rook", 1, 0, false)
	if p.Step != 4 || p.Mistakes != 3 {
		t.Errorf("a shorter run must not undo the progress: %+v", p)
	}
	// completion
	p, err = s.SaveLessonProgress(ctx, u.ID, "rook", 6, 4, true)
	if err != nil || !p.Completed() || p.Step != 6 || p.Mistakes != 4 {
		t.Fatalf("completion: %+v %v", p, err)
	}
	first := *p.CompletedAt
	// a replay: completion and its time stay, the best run's mistakes count
	p, _ = s.SaveLessonProgress(ctx, u.ID, "rook", 3, 9, false)
	if !p.Completed() || p.Step != 6 || p.Mistakes != 4 {
		t.Errorf("replay in the middle: %+v", p)
	}
	p, _ = s.SaveLessonProgress(ctx, u.ID, "rook", 6, 1, true)
	if !p.Completed() || p.Mistakes != 1 || !p.CompletedAt.Equal(first) {
		t.Errorf("a cleaner replay: %+v (first %v)", p, first)
	}
	p, _ = s.SaveLessonProgress(ctx, u.ID, "rook", 6, 5, true)
	if p.Mistakes != 1 {
		t.Errorf("a worse replay must not raise the mistakes: %+v", p)
	}

	// another lesson and the reading
	if _, err := s.SaveLessonProgress(ctx, u.ID, "fork", 1, 0, false); err != nil {
		t.Fatal(err)
	}
	m, err := s.LessonProgressOf(ctx, u.ID)
	if err != nil || len(m) != 2 || !m["rook"].Completed() || m["rook"].Mistakes != 1 || m["fork"].Step != 1 || m["fork"].Completed() {
		t.Fatalf("read back: %+v %v", m, err)
	}
	if m["rook"].UpdatedAt.IsZero() {
		t.Error("no update time")
	}
}

func TestLessonProgressIsPerPerson(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	a, b := lessonUser(t, s), lessonUser(t, s)
	if _, err := s.SaveLessonProgress(ctx, a.ID, "queen", 3, 0, false); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.LessonProgressOf(ctx, b.ID); len(m) != 0 {
		t.Errorf("progress leaked to another person: %v", m)
	}
}

func TestLessonProgressGoesWithTheUser(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u := lessonUser(t, s)
	if _, err := s.SaveLessonProgress(ctx, u.ID, "queen", 3, 0, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM users WHERE id = ?", u.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM lesson_progress WHERE user_id = ?", u.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("the rows must go with the user (ON DELETE CASCADE): %d %v", n, err)
	}
}

func TestMergeUsersCombinesLessonProgress(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	guest, google := lessonUser(t, s), lessonUser(t, s)
	save := func(u *User, lesson string, step, mistakes int, done bool) {
		t.Helper()
		if _, err := s.SaveLessonProgress(ctx, u.ID, lesson, step, mistakes, done); err != nil {
			t.Fatal(err)
		}
	}
	save(guest, "only-guest", 3, 2, false)
	save(guest, "both-further-guest", 5, 4, false)
	save(google, "both-further-guest", 2, 1, false)
	save(guest, "both-done", 6, 5, true)
	save(google, "both-done", 6, 2, true)
	save(guest, "done-guest", 7, 0, true)
	save(google, "done-guest", 3, 1, false)
	save(google, "only-google", 1, 0, false)

	if err := s.MergeUsers(ctx, guest.ID, google.ID); err != nil {
		t.Fatal(err)
	}
	m, err := s.LessonProgressOf(ctx, google.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 5 {
		t.Fatalf("%d lessons after the merge: %+v", len(m), m)
	}
	if p := m["only-guest"]; p.Step != 3 || p.Mistakes != 2 || p.Completed() {
		t.Errorf("only-guest: %+v", p)
	}
	if p := m["both-further-guest"]; p.Step != 5 || p.Mistakes != 4 || p.Completed() {
		t.Errorf("the further run counts: %+v", p)
	}
	if p := m["both-done"]; !p.Completed() || p.Mistakes != 2 {
		t.Errorf("both done, the fewer mistakes: %+v", p)
	}
	if p := m["done-guest"]; !p.Completed() || p.Step != 7 || p.Mistakes != 0 {
		t.Errorf("done on the guest account: %+v", p)
	}
	if p := m["only-google"]; p.Step != 1 {
		t.Errorf("only-google: %+v", p)
	}
	if left, _ := s.LessonProgressOf(ctx, guest.ID); len(left) != 0 {
		t.Errorf("the guest's rows must be gone: %v", left)
	}
}

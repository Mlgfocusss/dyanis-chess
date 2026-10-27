package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBlockEndsTiesAndCascades(t *testing.T) {
	s := openTest(t)
	ctx, now := context.Background(), time.Now()
	a, b, c := mkUser(t, s, "a"), mkUser(t, s, "b"), mkUser(t, s, "c")

	// a and b are friends, talked, and each has a challenge open towards the other; c asked a for friendship.
	s.RequestFriend(ctx, a.ID, b.ID, now)
	s.AcceptFriend(ctx, b.ID, a.ID, now)
	s.AddMessage(ctx, b.ID, a.ID, "unread", now)
	s.RequestFriend(ctx, c.ID, a.ID, now)
	for i, p := range [][2]string{{a.ID, b.ID}, {b.ID, a.ID}} {
		err := s.CreateChallenge(ctx, &ChallengeRow{ID: []string{"blkchal0000000a1", "blkchal0000000b1"}[i], FromID: p[0], ToID: p[1], Color: "white", BaseMs: 60000,
			Created: now, Expires: now.Add(time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		s.db.ExecContext(context.Background(), "DELETE FROM challenges WHERE id IN ('blkchal0000000a1', 'blkchal0000000b1')")
	})

	if fresh, err := s.Block(ctx, a.ID, b.ID, now); err != nil || !fresh {
		t.Fatalf("block: %v %v", fresh, err)
	}
	if fresh, err := s.Block(ctx, a.ID, b.ID, now); err != nil || fresh {
		t.Errorf("blocking again is not new: %v %v", fresh, err)
	}
	if fr, _ := s.Friends(ctx, a.ID); len(fr) != 0 {
		t.Errorf("a's friends: %+v", fr)
	}
	if fr, _ := s.Friends(ctx, b.ID); len(fr) != 0 {
		t.Errorf("b's friends: %+v", fr)
	}
	if in, _ := s.IncomingRequests(ctx, a.ID); len(in) != 1 || in[0].UserID != c.ID {
		t.Errorf("the request of somebody else is not touched: %+v", in)
	}
	if n, _ := s.CountIncomingChallenges(ctx, a.ID, now); n != 0 {
		t.Errorf("open challenges to a: %d", n)
	}
	if n, _ := s.CountPendingChallenges(ctx, a.ID, "", now); n != 0 {
		t.Errorf("open challenges from a: %d", n)
	}
	if n, _ := s.CountUnread(ctx, a.ID); n != 0 {
		t.Errorf("unread from the blocked person: %d", n)
	}

	st, err := s.BlockBetween(ctx, a.ID, b.ID)
	if err != nil || !st.ByMe || st.ByThem || !st.Any() {
		t.Errorf("a's view: %+v %v", st, err)
	}
	st, _ = s.BlockBetween(ctx, b.ID, a.ID)
	if st.ByMe || !st.ByThem {
		t.Errorf("b's view: %+v", st)
	}
	if st, _ = s.BlockBetween(ctx, a.ID, c.ID); st.Any() {
		t.Errorf("a and c: %+v", st)
	}
	if n, _ := s.CountBlocks(ctx, a.ID); n != 1 {
		t.Errorf("count: %d", n)
	}
	mine, _ := s.BlockedIDs(ctx, a.ID)
	theirs, _ := s.BlockerIDs(ctx, b.ID)
	both, _ := s.BlockedWith(ctx, b.ID)
	if !mine[b.ID] || !theirs[a.ID] || !both[a.ID] || len(both) != 1 {
		t.Errorf("sets: %v %v %v", mine, theirs, both)
	}
	list, _ := s.Blocks(ctx, a.ID)
	if len(list) != 1 || list[0].BlockedID != b.ID {
		t.Errorf("list: %+v", list)
	}

	// Search: b does not find a, a finds b.
	if found, _ := s.SearchUsers(ctx, "a", b.ID, 20); contains(found, a.ID) {
		t.Error("the blocked person finds the blocker")
	}
	if found, _ := s.SearchUsers(ctx, "b", a.ID, 20); !contains(found, b.ID) {
		t.Error("the blocker does not find the blocked person")
	}

	if ok, err := s.Unblock(ctx, a.ID, b.ID); err != nil || !ok {
		t.Fatalf("unblock: %v %v", ok, err)
	}
	if ok, _ := s.Unblock(ctx, a.ID, b.ID); ok {
		t.Error("unblocked twice")
	}
	if st, _ = s.BlockBetween(ctx, a.ID, b.ID); st.Any() {
		t.Errorf("after unblocking: %+v", st)
	}

	// The account going takes the blocks with it, from both ends.
	s.Block(ctx, a.ID, b.ID, now)
	s.Block(ctx, c.ID, a.ID, now)
	if err := s.DeleteUser(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.BlockerIDs(ctx, b.ID); len(got) != 0 {
		t.Errorf("a block of a deleted account is left: %v", got)
	}
	if got, _ := s.BlockedIDs(ctx, c.ID); len(got) != 0 {
		t.Errorf("a block on a deleted account is left: %v", got)
	}
}

func contains(list []*PublicUser, id string) bool {
	for _, u := range list {
		if u.ID == id {
			return true
		}
	}
	return false
}

func TestMergeMovesBlocks(t *testing.T) {
	s := openTest(t)
	ctx, now := context.Background(), time.Now()
	guest, acct, x, y, z := mkUser(t, s, "guest"), mkUser(t, s, "acct"), mkUser(t, s, "x"), mkUser(t, s, "y"), mkUser(t, s, "z")

	s.Block(ctx, guest.ID, x.ID, now)    // moves
	s.Block(ctx, acct.ID, x.ID, now)     // the same on the account: one stays
	s.Block(ctx, y.ID, guest.ID, now)    // y blocked the guest: now blocks the account
	s.Block(ctx, guest.ID, acct.ID, now) // would be a block on oneself: dropped
	// z is a friend of the account, and the guest has blocked z: the merged account cannot keep both.
	s.RequestFriend(ctx, acct.ID, z.ID, now)
	s.AcceptFriend(ctx, z.ID, acct.ID, now)
	s.Block(ctx, guest.ID, z.ID, now)
	// (blocking z by the guest has not touched the account's friendship with z)
	if fr, _ := s.Friends(ctx, acct.ID); len(fr) != 1 {
		t.Fatalf("setup: %+v", fr)
	}

	if err := s.MergeUsers(ctx, guest.ID, acct.ID); err != nil {
		t.Fatal(err)
	}
	mine, _ := s.BlockedIDs(ctx, acct.ID)
	if !mine[x.ID] || !mine[z.ID] || mine[acct.ID] || len(mine) != 2 {
		t.Errorf("the account blocks %v", mine)
	}
	if by, _ := s.BlockerIDs(ctx, acct.ID); !by[y.ID] || len(by) != 1 {
		t.Errorf("blocked by %v", by)
	}
	if fr, _ := s.Friends(ctx, acct.ID); len(fr) != 0 {
		t.Errorf("a friendship next to a block was kept: %+v", fr)
	}
	if fr, _ := s.Friends(ctx, z.ID); len(fr) != 0 {
		t.Errorf("z still has the friendship: %+v", fr)
	}
	if n, _ := s.CountBlocks(ctx, guest.ID); n != 0 {
		t.Errorf("the deleted guest still has blocks: %d", n)
	}
}

func TestReportsStore(t *testing.T) {
	s := openTest(t)
	ctx, now := context.Background(), time.Now()
	guest, acct, target := mkUser(t, s, "guest"), mkUser(t, s, "acct"), mkUser(t, s, "target")

	mk := func(by *User, kind, tid string) (uint64, error) {
		return s.CreateReport(ctx, &ComplaintRow{ReporterID: by.ID, ReporterName: by.Name, Kind: kind, TargetID: tid, TargetUserID: target.ID, TargetName: "target",
			Reason: "spam", Note: "n", Snapshot: "text of " + tid, Created: now})
	}
	id1, err := mk(guest, ReportUser, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeleteReport(context.Background(), id1) })
	if _, err := mk(guest, ReportUser, target.ID); !errors.Is(err, ErrReportExists) {
		t.Errorf("a duplicate: %v", err)
	}
	id2, err := mk(acct, ReportUser, target.ID) // another person on the same target
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeleteReport(context.Background(), id2) })

	// Merging the guest into the account: the guest's report is a duplicate of the account's, so it stays
	// with the guest and loses its author; another one moves.
	id3, err := mk(guest, ReportMessage, "12345")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeleteReport(context.Background(), id3) })
	if err := s.MergeUsers(ctx, guest.ID, acct.ID); err != nil {
		t.Fatal(err)
	}
	moved, err := s.Report(ctx, id3)
	if err != nil || moved.ReporterID != acct.ID {
		t.Errorf("the report moved to the account: %+v %v", moved, err)
	}
	left, err := s.Report(ctx, id1)
	if err != nil || left.ReporterID != "" || left.ReporterName != "guest" || left.Snapshot != "text of "+target.ID {
		t.Errorf("the duplicate lost its author but not its evidence: %+v %v", left, err)
	}

	// Closing closes every open report on the target.
	if n, err := s.CountOpenReports(ctx); err != nil || n < 3 {
		t.Errorf("open: %d %v", n, err)
	}
	n, err := s.CloseReports(ctx, ReportUser, target.ID, acct.ID, "none", now)
	if err != nil || n != 2 {
		t.Fatalf("close: %d %v", n, err)
	}
	r, _ := s.Report(ctx, id2)
	if r.Status != ReportClosed || r.ClosedBy != acct.ID || r.Action != "none" || r.ClosedAt.IsZero() {
		t.Errorf("closed report: %+v", r)
	}
	// A closed report no longer blocks a new one from the same person.
	id4, err := mk(acct, ReportUser, target.ID)
	if err != nil {
		t.Fatalf("a new report after closing: %v", err)
	}
	t.Cleanup(func() { s.DeleteReport(context.Background(), id4) })
	if _, err := s.Report(ctx, 1<<40); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown report: %v", err)
	}

	// The reported account goes; the report remembers the name.
	if err := s.DeleteUser(ctx, target.ID); err != nil {
		t.Fatal(err)
	}
	gone, _ := s.Report(ctx, id4)
	if gone.TargetUserID != "" || gone.TargetName != "target" {
		t.Errorf("after the account is gone: %+v", gone)
	}
}

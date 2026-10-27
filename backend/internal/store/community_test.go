package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func mkUser(t *testing.T, s *Store, name string) *User {
	t.Helper()
	u := &User{ID: fmt.Sprintf("sc%014d", time.Now().UnixNano()%1e14), Kind: KindGuest, Name: name}
	if err := s.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.db.ExecContext(context.Background(), "DELETE FROM users WHERE id = ?", u.ID) })
	time.Sleep(2 * time.Millisecond) // ids come from the clock
	return u
}

func TestFriendsAndRequests(t *testing.T) {
	s := openTest(t)
	ctx, now := context.Background(), time.Now()
	a, b, c := mkUser(t, s, "a"), mkUser(t, s, "b"), mkUser(t, s, "c")

	if acc, err := s.RequestFriend(ctx, a.ID, b.ID, now); err != nil || acc {
		t.Fatalf("request: %v %v", acc, err)
	}
	if _, err := s.RequestFriend(ctx, a.ID, b.ID, now); !errors.Is(err, ErrRequestExists) {
		t.Errorf("twice: %v", err)
	}
	if err := s.AcceptFriend(ctx, b.ID, c.ID, now); !errors.Is(err, ErrNotFound) {
		t.Errorf("accepting what was never sent: %v", err)
	}
	if err := s.AcceptFriend(ctx, b.ID, a.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RequestFriend(ctx, b.ID, a.ID, now); !errors.Is(err, ErrAlreadyFriends) {
		t.Errorf("between friends: %v", err)
	}
	fa, err := s.Friends(ctx, a.ID)
	fb, _ := s.Friends(ctx, b.ID)
	if err != nil || len(fa) != 1 || fa[0].FriendID != b.ID || len(fb) != 1 || fb[0].FriendID != a.ID {
		t.Fatalf("friends: %+v %+v %v", fa, fb, err)
	}

	if err := s.MuteFriend(ctx, a.ID, c.ID, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("muting a stranger: %v", err)
	}
	if err := s.MuteFriend(ctx, a.ID, b.ID, true); err != nil {
		t.Fatal(err)
	}
	ra, _ := s.Friend(ctx, a.ID, b.ID)
	rb, _ := s.Friend(ctx, b.ID, a.ID)
	if !ra.MuteChallenges || rb.MuteChallenges {
		t.Errorf("a mute is one-sided: %+v %+v", ra, rb)
	}

	// Ending a friendship removes both rows; the account going removes the rest.
	if ok, err := s.RemoveFriend(ctx, b.ID, a.ID); err != nil || !ok {
		t.Fatalf("remove: %v %v", ok, err)
	}
	if ok, _ := s.RemoveFriend(ctx, b.ID, a.ID); ok {
		t.Error("removed twice")
	}
}

func TestMergeKeepsFriendsMessagesAndForum(t *testing.T) {
	s := openTest(t)
	ctx, now := context.Background(), time.Now()
	guest, acct, f1, f2 := mkUser(t, s, "guest"), mkUser(t, s, "acct"), mkUser(t, s, "f1"), mkUser(t, s, "f2")

	// The guest has f1 and f2 as friends; the account has f2 already. A message with each and a topic.
	for _, f := range []*User{f1, f2} {
		s.RequestFriend(ctx, guest.ID, f.ID, now)
		s.AcceptFriend(ctx, f.ID, guest.ID, now)
		s.AddMessage(ctx, f.ID, guest.ID, "to guest from "+f.Name, now)
	}
	s.RequestFriend(ctx, acct.ID, f2.ID, now)
	s.AcceptFriend(ctx, f2.ID, acct.ID, now)
	s.AddMessage(ctx, guest.ID, acct.ID, "between the two", now) // must not turn into a message to oneself
	topic, err := s.ForumCreateTopic(ctx, &ForumTopic{Category: "general", AuthorID: guest.ID, AuthorName: "guest", Title: "t", Created: now}, "body")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.ForumDeleteTopic(context.Background(), topic) })

	if err := s.MergeUsers(ctx, guest.ID, acct.ID); err != nil {
		t.Fatal(err)
	}
	fr, err := s.Friends(ctx, acct.ID)
	if err != nil || len(fr) != 2 {
		t.Fatalf("friends after the merge: %+v %v", fr, err)
	}
	for _, f := range []*User{f1, f2} {
		back, _ := s.Friends(ctx, f.ID)
		if len(back) != 1 || back[0].FriendID != acct.ID {
			t.Errorf("%s sees %+v", f.Name, back)
		}
	}
	n, _ := s.CountUnread(ctx, acct.ID)
	if n != 2 {
		t.Errorf("the guest's two messages are the account's now (the one between the two is gone): %d", n)
	}
	tp, err := s.ForumTopicByID(ctx, topic)
	if err != nil || tp.AuthorID != acct.ID {
		t.Errorf("the topic: %+v %v", tp, err)
	}
}

func TestCorrespondenceColumns(t *testing.T) {
	s := openTest(t)
	ctx, now := context.Background(), time.Now().UTC().Truncate(time.Millisecond)
	a, b := mkUser(t, s, "a"), mkUser(t, s, "b")
	id := fmt.Sprintf("%016x", time.Now().UnixNano())
	t.Cleanup(func() { s.db.ExecContext(context.Background(), "DELETE FROM games WHERE id = ?", id) })
	g := &Game{ID: id, WhiteID: a.ID, BlackID: b.ID, Status: "playing", State: []byte(`{}`), CreatedAt: now, UpdatedAt: now,
		Corr: true, TurnID: b.ID, DueAt: now.Add(72 * time.Hour)}
	if err := s.SaveGame(ctx, g); err != nil {
		t.Fatal(err)
	}
	if n, err := s.CountMyTurn(ctx, b.ID); err != nil || n != 1 {
		t.Errorf("b's turn: %d %v", n, err)
	}
	if n, _ := s.CountMyTurn(ctx, a.ID); n != 0 {
		t.Errorf("a's turn: %d", n)
	}
	list, err := s.CorrespondenceGames(ctx, a.ID)
	if err != nil || len(list) != 1 || list[0].ID != id {
		t.Fatalf("list: %+v %v", list, err)
	}
	ids, err := s.ActiveCorrespondenceIDs(ctx)
	found := false
	for _, x := range ids {
		found = found || x == id
	}
	if err != nil || !found {
		t.Errorf("active ids: %v %v", ids, err)
	}
	g.Status, g.TurnID = "ended", ""
	if err := s.SaveGame(ctx, g); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountMyTurn(ctx, b.ID); n != 0 {
		t.Errorf("an ended game has nobody's turn: %d", n)
	}
	if list, _ := s.CorrespondenceGames(ctx, a.ID); len(list) != 0 {
		t.Errorf("an ended game is not listed: %+v", list)
	}
}

func TestDeleteUserWaitsForCorrespondenceGame(t *testing.T) {
	s := openTest(t)
	ctx, now := context.Background(), time.Now().UTC()
	a, b := mkUser(t, s, "a"), mkUser(t, s, "b")
	id := fmt.Sprintf("%016x", time.Now().UnixNano())
	t.Cleanup(func() { s.db.ExecContext(context.Background(), "DELETE FROM games WHERE id = ?", id) })
	old := now.Add(-10 * 24 * time.Hour) // far outside the window of a game with a clock
	if err := s.SaveGame(ctx, &Game{ID: id, WhiteID: a.ID, BlackID: b.ID, Status: "playing", State: []byte(`{}`), CreatedAt: old, UpdatedAt: old, Corr: true, TurnID: a.ID, DueAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(ctx, a.ID); !errors.Is(err, ErrActiveGame) {
		t.Errorf("deleting in the middle of a correspondence game: %v", err)
	}
}

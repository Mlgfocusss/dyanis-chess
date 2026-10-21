package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestVariantGames(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	anna, boris := newUserID(t, s, KindGuest), newUserID(t, s, KindGuest)
	id := fmt.Sprintf("v%015d", time.Now().UnixNano()%1_000_000_000_000_000)
	t.Cleanup(func() { s.db.Exec("DELETE FROM variant_games WHERE id = ?", id) })
	now := time.Now().UTC().Truncate(time.Millisecond)

	if _, err := s.LoadVariantGame(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing game: %v", err)
	}
	g := &VariantGame{ID: id, Variant: "atomic", WhiteID: anna.ID, Status: "waiting", State: []byte(`{"actions":[]}`), CreatedAt: now, UpdatedAt: now}
	if err := s.SaveVariantGame(ctx, g); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListVariantGames(ctx, anna.ID, 10); len(list) != 0 {
		t.Errorf("a game nobody joined is listed: %d", len(list))
	}

	// The server saves after every change: the same row is updated.
	g.BlackID, g.Status, g.UpdatedAt, g.Result = boris.ID, "ended", now.Add(time.Minute), "1-0"
	g.State = []byte(`{"actions":["e2e4"],"name":"Борис"}`)
	if err := s.SaveVariantGame(ctx, g); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadVariantGame(ctx, id)
	if err != nil || got.Variant != "atomic" || got.WhiteID != anna.ID || got.BlackID != boris.ID || got.Result != "1-0" || !got.CreatedAt.Equal(now) {
		t.Fatalf("LoadVariantGame: %+v (%v)", got, err)
	}
	for _, u := range []*User{anna, boris} {
		if list, err := s.ListVariantGames(ctx, u.ID, 10); err != nil || len(list) != 1 || list[0].ID != id {
			t.Errorf("list for %s: %v (%v)", u.ID, list, err)
		}
	}

	// A merged account takes its variant games along; a deleted one leaves them anonymous.
	if err := s.MergeUsers(ctx, anna.ID, boris.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = s.LoadVariantGame(ctx, id)
	if got.WhiteID != boris.ID || got.BlackID != boris.ID {
		t.Errorf("after merge: white %s black %s", got.WhiteID, got.BlackID)
	}
	s.db.Exec("DELETE FROM users WHERE id = ?", boris.ID)
	if got, err := s.LoadVariantGame(ctx, id); err != nil || got.WhiteID != "" || got.BlackID != "" {
		t.Errorf("after the user was deleted: %+v (%v)", got, err)
	}
}

// Deleting an account: a variant game with another person that is going stops it, an open invitation goes.
func TestDeleteUserAndVariantGames(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	anna, boris := newUserID(t, s, KindGuest), newUserID(t, s, KindGuest)
	live := fmt.Sprintf("v%015d", time.Now().UnixNano()%1_000_000_000_000_000)
	open := live[:15] + "x"
	t.Cleanup(func() {
		s.db.Exec("DELETE FROM variant_games WHERE id IN (?, ?)", live, open)
		s.db.Exec("DELETE FROM users WHERE id IN (?, ?)", anna.ID, boris.ID)
	})
	now := time.Now().UTC()
	must := func(g *VariantGame) {
		t.Helper()
		if err := s.SaveVariantGame(ctx, g); err != nil {
			t.Fatal(err)
		}
	}
	must(&VariantGame{ID: live, Variant: "atomic", WhiteID: anna.ID, BlackID: boris.ID, Status: "playing", State: []byte(`{}`), CreatedAt: now, UpdatedAt: now})
	if err := s.DeleteUser(ctx, anna.ID); !errors.Is(err, ErrActiveGame) {
		t.Fatalf("a game is on: %v", err)
	}
	must(&VariantGame{ID: live, Variant: "atomic", WhiteID: anna.ID, BlackID: boris.ID, Status: "ended", Result: "1-0", State: []byte(`{}`), CreatedAt: now, UpdatedAt: now})
	must(&VariantGame{ID: open, Variant: "atomic", WhiteID: anna.ID, Status: "waiting", State: []byte(`{}`), CreatedAt: now, UpdatedAt: now})
	if err := s.DeleteUser(ctx, anna.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadVariantGame(ctx, open); !errors.Is(err, ErrNotFound) {
		t.Errorf("the open invitation survived: %v", err)
	}
	if g, err := s.LoadVariantGame(ctx, live); err != nil || g.WhiteID != "" || g.BlackID != boris.ID {
		t.Errorf("the finished game stays for the opponent: %+v (%v)", g, err)
	}
}

// Bughouse seats: the third and fourth players are stored, listed, merged and survive their account's deletion.
func TestVariantGamesFourSeats(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u := []*User{newUserID(t, s, KindGuest), newUserID(t, s, KindGuest), newUserID(t, s, KindGuest), newUserID(t, s, KindGuest)}
	id := fmt.Sprintf("v%015d", time.Now().UnixNano()%1_000_000_000_000_000)
	t.Cleanup(func() {
		s.db.Exec("DELETE FROM variant_games WHERE id = ?", id)
		for _, x := range u {
			s.db.Exec("DELETE FROM users WHERE id = ?", x.ID)
		}
	})
	now := time.Now().UTC().Truncate(time.Millisecond)
	g := &VariantGame{ID: id, Variant: "bughouse", WhiteID: u[0].ID, BlackID: u[1].ID, ThirdID: u[2].ID, FourthID: u[3].ID,
		Status: "playing", State: []byte(`{}`), CreatedAt: now, UpdatedAt: now}
	if err := s.SaveVariantGame(ctx, g); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadVariantGame(ctx, id)
	if err != nil || got.ThirdID != u[2].ID || got.FourthID != u[3].ID {
		t.Fatalf("load: %+v (%v)", got, err)
	}
	for _, x := range u {
		if list, err := s.ListVariantGames(ctx, x.ID, 10); err != nil || len(list) != 1 {
			t.Errorf("list for %s: %v (%v)", x.ID, list, err)
		}
	}
	if err := s.DeleteUser(ctx, u[3].ID); !errors.Is(err, ErrActiveGame) {
		t.Errorf("the fourth player has a game on: %v", err)
	}
	if err := s.MergeUsers(ctx, u[2].ID, u[0].ID); err != nil {
		t.Fatal(err)
	}
	got, _ = s.LoadVariantGame(ctx, id)
	if got.ThirdID != u[0].ID {
		t.Errorf("after the merge the third seat is %s", got.ThirdID)
	}
}

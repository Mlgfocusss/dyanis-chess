package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestLeaders(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	stamp := time.Now().UnixNano()
	// A category no other test uses, so that rows of other tests do not get in.
	cat := "lead" + fmt.Sprint(stamp%100000)

	mk := func(name string) *User {
		u := &User{ID: fmt.Sprintf("ld%014d", time.Now().UnixNano()%1e14), Kind: KindGoogle, GoogleSub: fmt.Sprintf("sub-%s-%d", name, stamp),
			Name: name, Email: name + "@example.test", AvatarURL: "https://example.test/" + name + ".png"}
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.db.ExecContext(context.Background(), "DELETE FROM users WHERE id = ?", u.ID) })
		time.Sleep(2 * time.Millisecond) // ids come from the clock
		return u
	}
	a, b, c, d := mk("ann"), mk("bob"), mk("cat"), mk("dan")
	save := func(u *User, rating, rd float64, games int) {
		err := s.SaveRating(ctx, &RatingRow{UserID: u.ID, Category: cat, Rating: rating, RD: rd, Volatility: 0.06, Games: games, UpdatedAt: time.Now().UTC()})
		if err != nil {
			t.Fatal(err)
		}
	}
	save(a, 1500, 60, 10)
	save(b, 1800, 70, 20)
	save(c, 2000, 60, 3)   // too few games
	save(d, 1900, 200, 50) // rough

	rows, err := s.LeaderCandidates(ctx, cat, 5, 110, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].UserID != b.ID || rows[1].UserID != a.ID || rows[0].Games != 20 || rows[0].Rating != 1800 {
		t.Fatalf("candidates: %+v", rows)
	}
	if rows, _ = s.LeaderCandidates(ctx, cat, 5, 110, 1, ""); len(rows) != 1 {
		t.Errorf("limit: %+v", rows)
	}
	if rows, _ = s.LeaderCandidates(ctx, cat, 1, 400, 100, ""); len(rows) != 4 || rows[0].UserID != c.ID {
		t.Errorf("loose filters: %+v", rows)
	}

	users, err := s.PublicUsers(ctx, []string{a.ID, b.ID, "nobody"})
	if err != nil || len(users) != 2 || users[a.ID].Name != "ann" || users[b.ID].AvatarURL != "https://example.test/bob.png" || users[a.ID].Kind != KindGoogle {
		t.Fatalf("public users: %+v %v", users, err)
	}
	if users, err = s.PublicUsers(ctx, nil); err != nil || len(users) != 0 {
		t.Errorf("no ids: %+v %v", users, err)
	}
	one, err := s.PublicUser(ctx, c.ID)
	if err != nil || one.Name != "cat" || one.CreatedAt.IsZero() {
		t.Errorf("public user: %+v %v", one, err)
	}
	if _, err := s.PublicUser(ctx, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown user: %v", err)
	}

	n, err := s.CountEndedGames(ctx, a.ID)
	if err != nil || n != 0 {
		t.Fatalf("no games yet: %d %v", n, err)
	}
	now := time.Now().UTC()
	for i, status := range []string{"ended", "ended", "playing", "waiting"} {
		g := &Game{ID: fmt.Sprintf("lg%02d%012d", i, stamp%1e12), Status: status, State: []byte("{}"), CreatedAt: now, UpdatedAt: now}
		if i%2 == 0 {
			g.WhiteID, g.BlackID = a.ID, b.ID
		} else {
			g.WhiteID, g.BlackID = b.ID, a.ID
		}
		if err := s.SaveGame(ctx, g); err != nil {
			t.Fatal(err)
		}
		id := g.ID
		t.Cleanup(func() { s.db.ExecContext(context.Background(), "DELETE FROM games WHERE id = ?", id) })
	}
	if n, err = s.CountEndedGames(ctx, a.ID); err != nil || n != 2 {
		t.Errorf("ended games: %d %v", n, err)
	}
}

package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

// profileUser makes a user with a random id (other tests run on the same
// database at the same time) and removes it afterwards.
func profileUser(t *testing.T, s *Store, name string) *User {
	t.Helper()
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	u := &User{ID: hex.EncodeToString(b[:]), Kind: KindGuest, Name: name}
	if err := s.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.db.Exec("DELETE FROM users WHERE id = ?", u.ID) })
	return u
}

func profileRandomID() string {
	var b [8]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func count(t *testing.T, s *Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSetName(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u := profileUser(t, s, "Старое имя")

	if err := s.SetName(ctx, u.ID, "Новое имя"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.UserByID(ctx, u.ID); got.Name != "Новое имя" {
		t.Errorf("name after SetName: %q", got.Name)
	}
	// the same name again is not an error (MySQL reports no changed rows)
	if err := s.SetName(ctx, u.ID, "Новое имя"); err != nil {
		t.Errorf("same name: %v", err)
	}
	if err := s.SetName(ctx, "nobody-here", "X"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown user: %v", err)
	}
}

func TestDeleteUserRemovesTheirData(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u := profileUser(t, s, "Удаляемый")
	other := profileUser(t, s, "Соперник")
	hash := hex.EncodeToString(append([]byte(u.ID), make([]byte, 24)...))[:64]
	if err := s.CreateSession(ctx, hash, u.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSettings(ctx, u.ID, []byte(`{"sound":false}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordBotResult(ctx, u.ID, "bot-x", "win", 10); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRating(ctx, &RatingRow{UserID: u.ID, Category: "rapid", Rating: 1500, RD: 350, Volatility: 0.06, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	// a review stays, without its owner
	aid := profileRandomID()
	t.Cleanup(func() { s.db.Exec("DELETE FROM analyses WHERE id = ?", aid) })
	if err := s.SaveAnalysis(ctx, &Analysis{ID: aid, UserID: u.ID, Status: "done", Game: []byte(`{}`), Report: []byte(`{}`), CreatedAt: time.Now(), FinishedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// a finished game against a person stays for the opponent
	gHuman, gBot, gWait := profileRandomID(), profileRandomID(), profileRandomID()
	for _, g := range []*Game{
		{ID: gHuman, WhiteID: u.ID, BlackID: other.ID, Status: "ended", Result: "1-0", State: []byte(`{}`), CreatedAt: now, UpdatedAt: now},
		{ID: gBot, WhiteID: u.ID, Status: "playing", State: []byte(`{}`), CreatedAt: now, UpdatedAt: now},
		{ID: gWait, BlackID: u.ID, Status: "waiting", State: []byte(`{}`), CreatedAt: now, UpdatedAt: now},
	} {
		if err := s.SaveGame(ctx, g); err != nil {
			t.Fatal(err)
		}
		id := g.ID
		t.Cleanup(func() { s.db.Exec("DELETE FROM games WHERE id = ?", id) })
	}
	// a duel against a person stays, a duel whose other side is gone does not
	dBoth := profileRandomID()
	t.Cleanup(func() { s.db.Exec("DELETE FROM duels WHERE id = ?", dBoth) })
	if err := s.SaveDuel(ctx, &DuelRow{ID: dBoth, AID: u.ID, BID: other.ID, Result: "draw", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := s.UserByID(ctx, u.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("user still there: %v", err)
	}
	if _, err := s.SessionUser(ctx, hash); !errors.Is(err, ErrNotFound) {
		t.Errorf("session survived: %v", err)
	}
	if n := count(t, s, "SELECT COUNT(*) FROM sessions WHERE user_id = ?", u.ID); n != 0 {
		t.Errorf("%d sessions left", n)
	}
	if n := count(t, s, "SELECT COUNT(*) FROM bot_progress WHERE user_id = ?", u.ID); n != 0 {
		t.Errorf("%d bot_progress rows left", n)
	}
	if n := count(t, s, "SELECT COUNT(*) FROM ratings WHERE user_id = ?", u.ID); n != 0 {
		t.Errorf("%d rating rows left", n)
	}
	// anonymous, not deleted
	if a, err := s.LoadAnalysis(ctx, aid); err != nil || a.UserID != "" {
		t.Errorf("review after deletion: %+v (%v)", a, err)
	}
	if g, err := s.LoadGame(ctx, gHuman); err != nil || g.WhiteID != "" || g.BlackID != other.ID {
		t.Errorf("game with a person after deletion: %+v (%v)", g, err)
	}
	// nobody left on either side: gone
	for _, id := range []string{gBot, gWait} {
		if _, err := s.LoadGame(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("game %s with nobody left was kept: %v", id, err)
		}
	}
	if n := count(t, s, "SELECT COUNT(*) FROM duels WHERE id = ? AND a_id IS NULL AND b_id = ?", dBoth, other.ID); n != 1 {
		t.Errorf("duel with a person after deletion: %d matching rows", n)
	}

	// deleting the opponent too leaves nothing of the shared game and duel
	if err := s.DeleteUser(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadGame(ctx, gHuman); !errors.Is(err, ErrNotFound) {
		t.Errorf("game of two deleted users was kept: %v", err)
	}
	if n := count(t, s, "SELECT COUNT(*) FROM duels WHERE id = ?", dBoth); n != 0 {
		t.Errorf("duel of two deleted users was kept")
	}
	if err := s.DeleteUser(ctx, other.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting twice: %v", err)
	}
}

func TestDeleteUserRefusedDuringAGame(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u := profileUser(t, s, "Играет")
	other := profileUser(t, s, "Тоже играет")
	now := time.Now()
	gid := profileRandomID()
	t.Cleanup(func() { s.db.Exec("DELETE FROM games WHERE id = ?", gid) })
	if err := s.SaveGame(ctx, &Game{ID: gid, WhiteID: other.ID, BlackID: u.ID, Status: "playing", State: []byte(`{}`), CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(ctx, u.ID); !errors.Is(err, ErrActiveGame) {
		t.Fatalf("DeleteUser during a game: %v", err)
	}
	if _, err := s.UserByID(ctx, u.ID); err != nil {
		t.Errorf("a refused deletion removed the user: %v", err)
	}

	// the game ends: now it works
	if err := s.SaveGame(ctx, &Game{ID: gid, WhiteID: other.ID, BlackID: u.ID, Status: "ended", Result: "0-1", State: []byte(`{}`), CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteUser after the game: %v", err)
	}

	// a game left "playing" for days is a leftover and does not block
	third := profileUser(t, s, "Третий")
	gid2 := profileRandomID()
	t.Cleanup(func() { s.db.Exec("DELETE FROM games WHERE id = ?", gid2) })
	old := now.Add(-ActiveGameWindow - time.Hour)
	if err := s.SaveGame(ctx, &Game{ID: gid2, WhiteID: other.ID, BlackID: third.ID, Status: "playing", State: []byte(`{}`), CreatedAt: old, UpdatedAt: old}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(ctx, third.ID); err != nil {
		t.Fatalf("a stale game blocked the deletion: %v", err)
	}
}

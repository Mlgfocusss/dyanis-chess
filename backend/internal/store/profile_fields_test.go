package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func ptr(s string) *string { return &s }

// fieldsUser makes a user with a random id and removes it afterwards.
func fieldsUser(t *testing.T, s *Store, u User) *User {
	t.Helper()
	u.ID = profileRandomID()
	if u.Kind == "" {
		u.Kind = KindGuest
	}
	if u.Name == "" {
		u.Name = "Игрок"
	}
	if err := s.CreateUser(context.Background(), &u); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.db.ExecContext(context.Background(), "DELETE FROM users WHERE id = ?", u.ID) })
	return &u
}

func TestUpdateProfile(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u := fieldsUser(t, s, User{Name: "Старое имя"})

	got, err := s.UserByID(ctx, u.ID)
	if err != nil || got.FirstName != "" || got.Country != "" || got.About != "" || got.GoogleAvatarURL != "" {
		t.Fatalf("a new user has no profile fields: %+v %v", got, err)
	}

	about := "Люблю шахматы.\nИграю вечерами. 🙂"
	err = s.UpdateProfile(ctx, u.ID, ProfileUpdate{
		Name: ptr("Новое имя"), FirstName: ptr("Анна"), LastName: ptr("Иванова"), Location: ptr("Минск"),
		About: ptr(about), Country: ptr("BY"), AvatarURL: ptr("preset:crown"),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ = s.UserByID(ctx, u.ID)
	if got.Name != "Новое имя" || got.FirstName != "Анна" || got.LastName != "Иванова" || got.Location != "Минск" ||
		got.About != about || got.Country != "BY" || got.AvatarURL != "preset:crown" {
		t.Errorf("after update: %+v", got)
	}

	// a nil field is left alone; the rest changes
	if err := s.UpdateProfile(ctx, u.ID, ProfileUpdate{Location: ptr("Гродно")}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.UserByID(ctx, u.ID)
	if got.Location != "Гродно" || got.FirstName != "Анна" || got.Country != "BY" || got.Name != "Новое имя" {
		t.Errorf("partial update: %+v", got)
	}

	// an empty string clears (NULL), the name stays whatever else is sent
	if err := s.UpdateProfile(ctx, u.ID, ProfileUpdate{FirstName: ptr(""), Country: ptr(""), AvatarURL: ptr(""), About: ptr("")}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.UserByID(ctx, u.ID)
	if got.FirstName != "" || got.Country != "" || got.AvatarURL != "" || got.About != "" || got.LastName != "Иванова" {
		t.Errorf("after clearing: %+v", got)
	}
	if n := count(t, s, "SELECT COUNT(*) FROM users WHERE id = ? AND first_name IS NULL AND country IS NULL AND about IS NULL AND avatar_url IS NULL", u.ID); n != 1 {
		t.Error("cleared fields must be NULL, not empty strings")
	}

	// the same values again are not an error (MySQL reports no changed rows), nor is an empty update
	if err := s.UpdateProfile(ctx, u.ID, ProfileUpdate{Location: ptr("Гродно")}); err != nil {
		t.Errorf("same value: %v", err)
	}
	if err := s.UpdateProfile(ctx, u.ID, ProfileUpdate{}); err != nil {
		t.Errorf("nothing to change: %v", err)
	}
	if err := s.UpdateProfile(ctx, "nobody-here", ProfileUpdate{Location: ptr("X")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown user: %v", err)
	}
	if err := s.UpdateProfile(ctx, "nobody-here", ProfileUpdate{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown user, nothing to change: %v", err)
	}

	// the column sizes are the limits the server checks: one more character does not fit
	if err := s.UpdateProfile(ctx, u.ID, ProfileUpdate{About: ptr(strings.Repeat("я", 500))}); err != nil {
		t.Errorf("500 characters: %v", err)
	}
	if err := s.UpdateProfile(ctx, u.ID, ProfileUpdate{About: ptr(strings.Repeat("я", 501))}); err == nil {
		t.Error("501 characters fit the column")
	}
	if err := s.UpdateProfile(ctx, u.ID, ProfileUpdate{Country: ptr("BYY")}); err == nil {
		t.Error("a three-letter country fit the column")
	}
	if err := s.UpdateProfile(ctx, u.ID, ProfileUpdate{FirstName: ptr(strings.Repeat("я", 41))}); err == nil {
		t.Error("a 41-character name fit the column")
	}
}

func TestCountryIndex(t *testing.T) {
	s := openTest(t)
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = 'users' AND index_name = 'idx_users_country' AND column_name = 'country'`).Scan(&n)
	if err != nil || n != 1 {
		t.Fatalf("idx_users_country: %d, %v", n, err)
	}
	// codes are compared exactly: "by" is not "BY"
	u := fieldsUser(t, s, User{})
	if err := s.UpdateProfile(context.Background(), u.ID, ProfileUpdate{Country: ptr("BY")}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT COUNT(*) FROM users WHERE id = ? AND country = 'by'", u.ID); n != 0 {
		t.Error("country comparison ignores case")
	}
	if n := count(t, s, "SELECT COUNT(*) FROM users WHERE id = ? AND country = 'BY'", u.ID); n != 1 {
		t.Error("country does not match its own code")
	}
}

func TestLinkGoogleKeepsTheChosenAvatar(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	link := func(u *User, photo string) *User {
		t.Helper()
		if err := s.LinkGoogle(ctx, u.ID, "sub-"+u.ID, "Имя", "a@example.test", photo); err != nil {
			t.Fatal(err)
		}
		got, err := s.UserByID(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	// a guest who chose nothing gets the photo shown and remembered
	g := fieldsUser(t, s, User{})
	if got := link(g, "https://img.test/1.png"); got.AvatarURL != "https://img.test/1.png" || got.GoogleAvatarURL != "https://img.test/1.png" || got.Kind != KindGoogle {
		t.Errorf("plain link: %+v", got)
	}
	// still showing the photo: a new photo replaces it
	if got := link(g, "https://img.test/2.png"); got.AvatarURL != "https://img.test/2.png" || got.GoogleAvatarURL != "https://img.test/2.png" {
		t.Errorf("photo refresh: %+v", got)
	}
	// a ready-made avatar is chosen: later photos are only remembered
	if err := s.UpdateProfile(ctx, g.ID, ProfileUpdate{AvatarURL: ptr("preset:bolt")}); err != nil {
		t.Fatal(err)
	}
	if got := link(g, "https://img.test/3.png"); got.AvatarURL != "preset:bolt" || got.GoogleAvatarURL != "https://img.test/3.png" {
		t.Errorf("preset overridden: %+v", got)
	}
	// the letter is a choice too
	if err := s.UpdateProfile(ctx, g.ID, ProfileUpdate{AvatarURL: ptr("")}); err != nil {
		t.Fatal(err)
	}
	if got := link(g, "https://img.test/4.png"); got.AvatarURL != "" || got.GoogleAvatarURL != "https://img.test/4.png" {
		t.Errorf("letter overridden: %+v", got)
	}
	// back to the photo
	if err := s.UpdateProfile(ctx, g.ID, ProfileUpdate{AvatarURL: ptr("https://img.test/4.png")}); err != nil {
		t.Fatal(err)
	}
	if got := link(g, "https://img.test/5.png"); got.AvatarURL != "https://img.test/5.png" {
		t.Errorf("photo not refreshed: %+v", got)
	}

	// a guest with a ready-made avatar who links Google keeps it
	p := fieldsUser(t, s, User{AvatarURL: "preset:star"})
	if got := link(p, "https://img.test/p.png"); got.AvatarURL != "preset:star" || got.GoogleAvatarURL != "https://img.test/p.png" {
		t.Errorf("guest preset: %+v", got)
	}
	// a Google user created with a photo shows it (CreateUser keeps both)
	n := fieldsUser(t, s, User{Kind: KindGoogle, GoogleSub: "sub-new-" + profileRandomID(), AvatarURL: "https://img.test/n.png", GoogleAvatarURL: "https://img.test/n.png"})
	if got, _ := s.UserByID(ctx, n.ID); got.AvatarURL != "https://img.test/n.png" || got.GoogleAvatarURL != "https://img.test/n.png" {
		t.Errorf("created: %+v", got)
	}
}

func TestSessionUserHasTheProfileFields(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u := fieldsUser(t, s, User{FirstName: "Анна", LastName: "Иванова", Location: "Минск", About: "Привет", Country: "BY", AvatarURL: "preset:duel", GoogleAvatarURL: "https://img.test/g.png"})
	hash := "ses" + profileRandomID() + strings.Repeat("0", 64)
	hash = hash[:64]
	if err := s.CreateSession(ctx, hash, u.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := s.SessionUser(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.FirstName != "Анна" || got.LastName != "Иванова" || got.Location != "Минск" || got.About != "Привет" || got.Country != "BY" ||
		got.AvatarURL != "preset:duel" || got.GoogleAvatarURL != "https://img.test/g.png" {
		t.Errorf("%+v", got)
	}
}

func TestPublicUserHasTheOpenProfile(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	a := fieldsUser(t, s, User{Name: "Анна", FirstName: "Анна", LastName: "Иванова", Location: "Минск", About: "Привет\nмир", Country: "BY", AvatarURL: "preset:coach"})
	b := fieldsUser(t, s, User{Name: "Борис"})

	got, err := s.PublicUser(ctx, a.ID)
	if err != nil || got.FirstName != "Анна" || got.LastName != "Иванова" || got.Location != "Минск" || got.About != "Привет\nмир" ||
		got.Country != "BY" || got.AvatarURL != "preset:coach" {
		t.Fatalf("public user: %+v %v", got, err)
	}
	users, err := s.PublicUsers(ctx, []string{a.ID, b.ID})
	if err != nil || len(users) != 2 || users[a.ID].Country != "BY" || users[b.ID].Country != "" || users[b.ID].About != "" {
		t.Fatalf("public users: %+v %v", users, err)
	}
}

func TestLeaderCandidatesByCountry(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	cat := "cty" + fmt.Sprint(time.Now().UnixNano()%100000) // a category no other test uses

	mk := func(country string, rating float64, games int) *User {
		u := fieldsUser(t, s, User{Country: country})
		err := s.SaveRating(ctx, &RatingRow{UserID: u.ID, Category: cat, Rating: rating, RD: 60, Volatility: 0.06, Games: games, UpdatedAt: time.Now().UTC()})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	by1, by2, pl, none := mk("BY", 1500, 10), mk("BY", 1800, 20), mk("PL", 2000, 30), mk("", 1900, 40)
	byFew := mk("BY", 2100, 2) // too few games

	rows, err := s.LeaderCandidates(ctx, cat, 5, 110, 100, "BY")
	if err != nil || len(rows) != 2 || rows[0].UserID != by2.ID || rows[1].UserID != by1.ID {
		t.Fatalf("Belarus: %+v %v", rows, err)
	}
	rows, _ = s.LeaderCandidates(ctx, cat, 1, 110, 100, "BY")
	if len(rows) != 3 || rows[0].UserID != byFew.ID {
		t.Errorf("Belarus with fewer games needed: %+v", rows)
	}
	if rows, _ = s.LeaderCandidates(ctx, cat, 5, 110, 1, "BY"); len(rows) != 1 || rows[0].UserID != by2.ID {
		t.Errorf("limit: %+v", rows)
	}
	if rows, _ = s.LeaderCandidates(ctx, cat, 5, 110, 100, "PL"); len(rows) != 1 || rows[0].UserID != pl.ID {
		t.Errorf("Poland: %+v", rows)
	}
	if rows, _ = s.LeaderCandidates(ctx, cat, 5, 110, 100, "JP"); len(rows) != 0 {
		t.Errorf("an empty country: %+v", rows)
	}
	if rows, _ = s.LeaderCandidates(ctx, cat, 5, 110, 100, "by"); len(rows) != 0 {
		t.Errorf("lower case matched: %+v", rows)
	}
	// no country: everybody, including those who named none
	rows, _ = s.LeaderCandidates(ctx, cat, 5, 110, 100, "")
	if len(rows) != 4 || rows[0].UserID != pl.ID || rows[1].UserID != none.ID {
		t.Errorf("all: %+v", rows)
	}
}

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestLoadMigrations(t *testing.T) {
	migs, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) == 0 || migs[0].version != 1 {
		t.Fatalf("migrations: %+v", migs)
	}
	for i := 1; i < len(migs); i++ {
		if migs[i].version != migs[i-1].version+1 {
			t.Errorf("gap between migration %d and %d", migs[i-1].version, migs[i].version)
		}
	}
}

// The tests below need a real MySQL: point DYANIS_TEST_DSN at one, e.g.
//
//	docker compose up -d db
//	DYANIS_TEST_DSN='dyanis:PASSWORD@tcp(127.0.0.1:3307)/dyanis' go test ./internal/store
func openTest(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("DYANIS_TEST_DSN")
	if dsn == "" {
		t.Skip("DYANIS_TEST_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s, err := Open(ctx, dsn, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.migrate(ctx); err != nil { // second run: nothing to do
		t.Fatal(err)
	}
	var n, max int
	if err := s.db.QueryRow("SELECT COUNT(*), MAX(version) FROM schema_migrations").Scan(&n, &max); err != nil {
		t.Fatal(err)
	}
	migs, _ := loadMigrations()
	if n != len(migs) || max != migs[len(migs)-1].version {
		t.Errorf("recorded %d migrations up to %d, have %d", n, max, len(migs))
	}
}

func TestAnalysisRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	id := fmt.Sprintf("t%015d", time.Now().UnixNano()%1_000_000_000_000_000) // 16 chars, like real ids
	t.Cleanup(func() { s.db.Exec("DELETE FROM analyses WHERE id = ?", id) })

	if _, err := s.LoadAnalysis(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing row: %v", err)
	}
	game, _ := json.Marshal(map[string]string{"white": "Анна", "black": "Борис 🙂"})
	report := []byte(`{"accuracy":[83.1,94.3],"moves":[]}`)
	created := time.Now().UTC().Truncate(time.Millisecond)
	a := &Analysis{ID: id, Status: "done", Depth: 14, Game: game, Report: report,
		CreatedAt: created, FinishedAt: created.Add(2 * time.Second)}
	if err := s.SaveAnalysis(ctx, a); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadAnalysis(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var g map[string]string
	if err := json.Unmarshal(got.Game, &g); err != nil || g["white"] != "Анна" || g["black"] != "Борис 🙂" {
		t.Errorf("game round trip: %s (%v)", got.Game, err)
	}
	var r struct{ Accuracy [2]float64 }
	if err := json.Unmarshal(got.Report, &r); err != nil || r.Accuracy[1] != 94.3 {
		t.Errorf("report round trip: %s (%v)", got.Report, err)
	}
	if got.Status != "done" || got.Depth != 14 || got.Error != "" || !got.CreatedAt.Equal(created) {
		t.Errorf("fields: %+v", got)
	}

	// An error result has no report; saving again replaces the row.
	a.Status, a.Report, a.Error = "error", nil, "boom"
	if err := s.SaveAnalysis(ctx, a); err != nil {
		t.Fatal(err)
	}
	got, err = s.LoadAnalysis(ctx, id)
	if err != nil || got.Status != "error" || got.Report != nil || got.Error != "boom" {
		t.Errorf("error row: %+v (%v)", got, err)
	}
}

func newUserID(t *testing.T, s *Store, kind string) *User {
	t.Helper()
	id := fmt.Sprintf("u%015d", time.Now().UnixNano()%1_000_000_000_000_000)
	u := &User{ID: id, Kind: kind, Name: "Тест 🙂"}
	if err := s.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.db.Exec("DELETE FROM users WHERE id = ?", id) })
	return u
}

func TestUsersAndSessions(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u := newUserID(t, s, KindGuest)

	got, err := s.UserByID(ctx, u.ID)
	if err != nil || got.Kind != KindGuest || got.Name != "Тест 🙂" || got.GoogleSub != "" || got.Settings != nil {
		t.Fatalf("UserByID: %+v (%v)", got, err)
	}
	if _, err := s.UserByID(ctx, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}

	// Sessions: live, expired, deleted.
	hash := fmt.Sprintf("%064x", time.Now().UnixNano())
	t.Cleanup(func() { s.db.Exec("DELETE FROM sessions WHERE token_hash = ?", hash) })
	if err := s.CreateSession(ctx, hash, u.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if su, err := s.SessionUser(ctx, hash); err != nil || su.ID != u.ID {
		t.Fatalf("SessionUser: %+v (%v)", su, err)
	}
	s.db.Exec("UPDATE sessions SET expires_at = ? WHERE token_hash = ?", time.Now().Add(-time.Minute).UTC(), hash)
	if _, err := s.SessionUser(ctx, hash); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired session still works: %v", err)
	}
	if err := s.PurgeExpiredSessions(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	s.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE token_hash = ?", hash).Scan(&n)
	if n != 0 {
		t.Error("expired session was not purged")
	}
	s.CreateSession(ctx, hash, u.ID, time.Now().Add(time.Hour))
	s.DeleteSession(ctx, hash)
	if _, err := s.SessionUser(ctx, hash); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted session still works: %v", err)
	}

	// Settings round trip, then upgrade to Google in place.
	if err := s.SetSettings(ctx, u.ID, []byte(`{"boardTheme":"green"}`)); err != nil {
		t.Fatal(err)
	}
	sub := "sub-" + u.ID
	if err := s.LinkGoogle(ctx, u.ID, sub, "Анна", "a@example.com", "https://img/a.png"); err != nil {
		t.Fatal(err)
	}
	got, err = s.UserByGoogleSub(ctx, sub)
	if err != nil || got.ID != u.ID || got.Kind != KindGoogle || got.Email != "a@example.com" || got.AvatarURL != "https://img/a.png" {
		t.Fatalf("UserByGoogleSub: %+v (%v)", got, err)
	}
	var st map[string]string
	if err := json.Unmarshal(got.Settings, &st); err != nil || st["boardTheme"] != "green" {
		t.Errorf("settings after link: %s (%v)", got.Settings, err)
	}
	// One Google account cannot belong to two users.
	other := newUserID(t, s, KindGuest)
	if err := s.LinkGoogle(ctx, other.ID, sub, "X", "", ""); err == nil {
		t.Error("the same google_sub was linked to two users")
	}
}

func TestAnalysesOfAUser(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	alice, bob := newUserID(t, s, KindGuest), newUserID(t, s, KindGuest)
	base := time.Now().UTC().Truncate(time.Millisecond)

	save := func(owner string, age time.Duration, withSummary bool) string {
		id := fmt.Sprintf("a%015d", time.Now().UnixNano()%1_000_000_000_000_000)
		time.Sleep(2 * time.Millisecond)
		a := &Analysis{ID: id, UserID: owner, Status: "done", Depth: 14,
			Game: []byte(`{"white":"A"}`), Report: []byte(`{"moves":[]}`),
			CreatedAt: base.Add(-age), FinishedAt: base}
		if withSummary {
			a.Summary = []byte(`{"accuracy":[90,80],"plies":40}`)
		}
		if err := s.SaveAnalysis(ctx, a); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.db.Exec("DELETE FROM analyses WHERE id = ?", id) })
		return id
	}
	old := save(alice.ID, time.Hour, false)
	recent := save(alice.ID, time.Minute, true)
	bobs := save(bob.ID, 0, true)
	anon := save("", 0, true)

	list, err := s.ListAnalyses(ctx, alice.ID, 10)
	if err != nil || len(list) != 2 || list[0].ID != recent || list[1].ID != old {
		t.Fatalf("alice's list (newest first): %v (%v)", list, err)
	}
	var sm struct{ Plies int }
	if json.Unmarshal(list[0].Summary, &sm) != nil || sm.Plies != 40 || list[1].Summary != nil || list[0].Report != nil {
		t.Errorf("summary/report columns: %s | %s | %s", list[0].Summary, list[1].Summary, list[0].Report)
	}
	if got, _ := s.ListAnalyses(ctx, alice.ID, 1); len(got) != 1 {
		t.Errorf("limit ignored: %d rows", len(got))
	}
	if got, err := s.LoadAnalysis(ctx, bobs); err != nil || got.UserID != bob.ID {
		t.Errorf("owner not loaded: %+v (%v)", got, err)
	}
	if got, err := s.LoadAnalysis(ctx, anon); err != nil || got.UserID != "" {
		t.Errorf("anonymous row: %+v (%v)", got, err)
	}

	// Only the owner can delete.
	if err := s.DeleteAnalysis(ctx, bob.ID, recent); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign delete: %v", err)
	}
	if err := s.DeleteAnalysis(ctx, alice.ID, recent); err != nil {
		t.Errorf("own delete: %v", err)
	}
	if _, err := s.LoadAnalysis(ctx, recent); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted row still loads: %v", err)
	}

	// Merging moves the games and removes the source user.
	if err := s.MergeUsers(ctx, alice.ID, bob.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListAnalyses(ctx, bob.ID, 10); len(got) != 2 {
		t.Errorf("after merge bob has %d reviews, want 2", len(got))
	}
	if _, err := s.UserByID(ctx, alice.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("merged-away user still exists: %v", err)
	}

	// Deleting a user keeps their reviews, as anonymous ones (their links still work).
	s.db.Exec("DELETE FROM users WHERE id = ?", bob.ID)
	if got, err := s.LoadAnalysis(ctx, bobs); err != nil || got.UserID != "" {
		t.Errorf("review after its owner was deleted: %+v (%v)", got, err)
	}
}

func TestGames(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	anna, boris := newUserID(t, s, KindGuest), newUserID(t, s, KindGuest)
	id := fmt.Sprintf("g%015d", time.Now().UnixNano()%1_000_000_000_000_000)
	t.Cleanup(func() { s.db.Exec("DELETE FROM games WHERE id = ?", id) })
	now := time.Now().UTC().Truncate(time.Millisecond)

	if _, err := s.LoadGame(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing game: %v", err)
	}
	g := &Game{ID: id, WhiteID: anna.ID, Status: "waiting", State: []byte(`{"moves":[]}`), CreatedAt: now, UpdatedAt: now}
	if err := s.SaveGame(ctx, g); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListGames(ctx, anna.ID, 10); len(list) != 0 {
		t.Errorf("a game nobody joined is listed: %d", len(list))
	}

	// Saving again updates the row (the server saves after every move).
	g.BlackID, g.Status, g.UpdatedAt = boris.ID, "ended", now.Add(time.Minute)
	g.Result, g.State = "1-0", []byte(`{"moves":[{"uci":"e2e4","san":"e4"}],"name":"Борис 🙂"}`)
	if err := s.SaveGame(ctx, g); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadGame(ctx, id)
	if err != nil || got.WhiteID != anna.ID || got.BlackID != boris.ID || got.Status != "ended" || got.Result != "1-0" || !got.CreatedAt.Equal(now) {
		t.Fatalf("LoadGame: %+v (%v)", got, err)
	}
	var doc struct{ Name string }
	if json.Unmarshal(got.State, &doc) != nil || doc.Name != "Борис 🙂" {
		t.Errorf("state: %s", got.State)
	}
	for _, u := range []*User{anna, boris} {
		if list, err := s.ListGames(ctx, u.ID, 10); err != nil || len(list) != 1 || list[0].ID != id {
			t.Errorf("list for %s: %v (%v)", u.ID, list, err)
		}
	}

	// A merged account takes its games along; a deleted one leaves them anonymous.
	if err := s.MergeUsers(ctx, anna.ID, boris.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = s.LoadGame(ctx, id)
	if got.WhiteID != boris.ID || got.BlackID != boris.ID {
		t.Errorf("after merge: white %s black %s", got.WhiteID, got.BlackID)
	}
	s.db.Exec("DELETE FROM users WHERE id = ?", boris.ID)
	if got, err := s.LoadGame(ctx, id); err != nil || got.WhiteID != "" || got.BlackID != "" {
		t.Errorf("after the user was deleted: %+v (%v)", got, err)
	}
}

func TestBotProgress(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	anna, boris := newUserID(t, s, KindGuest), newUserID(t, s, KindGuest)

	if got, err := s.BotProgress(ctx, anna.ID); err != nil || len(got) != 0 {
		t.Fatalf("empty progress: %v (%v)", got, err)
	}
	// A loss, a draw, the first win (earns the points), a second win (earns nothing more).
	steps := []struct {
		outcome string
		first   bool
	}{{"loss", false}, {"draw", false}, {"win", true}, {"win", false}}
	for _, st := range steps {
		first, err := s.RecordBotResult(ctx, anna.ID, "timur", st.outcome, 20)
		if err != nil || first != st.first {
			t.Fatalf("%s: first=%v err=%v, want first=%v", st.outcome, first, err, st.first)
		}
	}
	got, _ := s.BotProgress(ctx, anna.ID)
	if len(got) != 1 || got[0].BotID != "timur" || got[0].Wins != 2 || got[0].Draws != 1 || got[0].Losses != 1 || got[0].Points != 20 || got[0].FirstWinAt.IsZero() {
		t.Fatalf("progress: %+v", got)
	}
	if first, _ := s.RecordBotResult(ctx, anna.ID, "misha", "loss", 10); first {
		t.Error("a loss was a first win")
	}
	if got, _ := s.BotProgress(ctx, anna.ID); len(got) != 2 || got[0].BotID != "misha" || got[0].Wins != 0 || got[0].Points != 0 || !got[0].FirstWinAt.IsZero() {
		t.Errorf("a bot never beaten: %+v", got)
	}

	// Merging two accounts combines the records and counts points once.
	s.RecordBotResult(ctx, boris.ID, "timur", "win", 20)
	s.RecordBotResult(ctx, boris.ID, "anna", "win", 30)
	if err := s.MergeUsers(ctx, boris.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = s.BotProgress(ctx, anna.ID)
	byBot := map[string]BotRecord{}
	for _, r := range got {
		byBot[r.BotID] = r
	}
	if byBot["timur"].Wins != 3 || byBot["timur"].Points != 20 || byBot["anna"].Wins != 1 || byBot["anna"].Points != 30 {
		t.Errorf("after merge: %+v", got)
	}
	if left, _ := s.BotProgress(ctx, boris.ID); len(left) != 0 {
		t.Errorf("the merged-away user still has progress: %v", left)
	}
}

func TestPuzzles(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	anna, boris := newUserID(t, s, KindGuest), newUserID(t, s, KindGuest)

	// Ratings far from the real set, so the test does not meet imported puzzles.
	const fen = "6k1/5ppp/8/8/8/8/5PPP/3R2K1 w - - 0 1"
	a := Puzzle{ID: "tZaAa", FEN: fen, Moves: "g8f8 d1d8", Rating: 5001, Popularity: 90, Plays: 10, Themes: "mate mateIn1 backRankMate"}
	b := Puzzle{ID: "tzaaa", FEN: fen, Moves: "g8f8 d1d8", Rating: 5002, Popularity: 90, Plays: 10, Themes: "mate"} // differs from a only by case
	t.Cleanup(func() { s.db.Exec("DELETE FROM puzzles WHERE id IN ('tZaAa', 'tzaaa')") })
	if n, err := s.ImportPuzzles(ctx, []Puzzle{a, b}); err != nil || n != 2 {
		t.Fatalf("import: %d (%v)", n, err)
	}
	if n, err := s.ImportPuzzles(ctx, []Puzzle{a}); err != nil || n != 0 {
		t.Fatalf("second import: %d (%v)", n, err)
	}
	got, err := s.PuzzleByID(ctx, "tZaAa")
	if err != nil || *got != a {
		t.Fatalf("PuzzleByID: %+v (%v)", got, err)
	}
	if _, err := s.PuzzleByID(ctx, "nothere"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing puzzle: %v", err)
	}

	// Picking by rating window and by theme.
	if p, err := s.PickPuzzle(ctx, anna.ID, "", 5001, 5001, true); err != nil || p.ID != "tZaAa" {
		t.Errorf("pick by rating: %+v (%v)", p, err)
	}
	if p, err := s.PickPuzzle(ctx, anna.ID, "backRankMate", 5000, 5003, true); err != nil || p.ID != "tZaAa" {
		t.Errorf("pick by theme: %+v (%v)", p, err)
	}
	if _, err := s.PickPuzzle(ctx, anna.ID, "backRankMate", 5002, 5003, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("a window without the theme: %v", err)
	}
	themes, err := s.PuzzleThemes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, th := range themes {
		seen[th.Theme] = true
	}
	if !seen["mateIn1"] || !seen["backRankMate"] {
		t.Errorf("themes: %v", themes)
	}

	// The first try counts, later ones do not; a person never rated starts at 1000.
	if r, err := s.PuzzleRatingOf(ctx, anna.ID); err != nil || r.Rating != StartPuzzleRating || r.Attempts != 0 {
		t.Fatalf("new person: %+v (%v)", r, err)
	}
	up := func(rating, attempts int) int { return rating + 20 }
	at, err := s.RecordPuzzleAttempt(ctx, anna.ID, "tZaAa", true, up)
	if err != nil || !at.Counted || at.Before != 1000 || at.After != 1020 || at.Record.Solved != 1 {
		t.Fatalf("first attempt: %+v (%v)", at, err)
	}
	at, err = s.RecordPuzzleAttempt(ctx, anna.ID, "tZaAa", false, up)
	if err != nil || at.Counted || at.After != 1020 || at.Record.Attempts != 1 {
		t.Fatalf("repeat attempt: %+v (%v)", at, err)
	}
	if _, err := s.PickPuzzle(ctx, anna.ID, "", 5001, 5001, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("a tried puzzle was picked again: %v", err)
	}
	if p, err := s.PickPuzzle(ctx, anna.ID, "", 5001, 5001, false); err != nil || p.ID != "tZaAa" {
		t.Errorf("tried puzzles allowed: %+v (%v)", p, err)
	}

	// Merging adds the records up and keeps the rating of the one with more attempts.
	s.RecordPuzzleAttempt(ctx, boris.ID, "tzaaa", false, func(r, n int) int { return r - 10 })
	s.RecordPuzzleAttempt(ctx, boris.ID, "other", false, func(r, n int) int { return r - 10 })
	if err := s.MergeUsers(ctx, anna.ID, boris.ID); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.PuzzleRatingOf(ctx, boris.ID); r.Attempts != 3 || r.Solved != 1 || r.Rating != 980 {
		t.Errorf("after merge: %+v", r)
	}
	if at, _ := s.RecordPuzzleAttempt(ctx, boris.ID, "tZaAa", true, up); at.Counted {
		t.Error("the attempt of the merged-away account was forgotten")
	}
}

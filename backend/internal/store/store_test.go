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
	at, err := s.RecordPuzzleAttempt(ctx, anna.ID, "tZaAa", true, up, nil)
	if err != nil || !at.Counted || at.Before != 1000 || at.After != 1020 || at.Record.Solved != 1 {
		t.Fatalf("first attempt: %+v (%v)", at, err)
	}
	at, err = s.RecordPuzzleAttempt(ctx, anna.ID, "tZaAa", false, up, nil)
	if err != nil || at.Counted || at.After != 1020 || at.Record.Attempts != 1 {
		t.Fatalf("repeat attempt: %+v (%v)", at, err)
	}
	// The puzzle of the day is practice: its first try of a day is kept, the rating is not touched.
	if tried, _, err := s.DailyPuzzleResult(ctx, anna.ID, "2026-10-16"); err != nil || tried {
		t.Errorf("a day never played: %v (%v)", tried, err)
	}
	for _, solved := range []bool{true, false} { // the second write of the day changes nothing
		if err := s.RecordDailyPuzzle(ctx, anna.ID, "2026-10-16", "tZaAa", solved); err != nil {
			t.Fatal(err)
		}
	}
	if tried, solved, err := s.DailyPuzzleResult(ctx, anna.ID, "2026-10-16"); err != nil || !tried || !solved {
		t.Errorf("first try of the day: %v %v (%v)", tried, solved, err)
	}
	if tried, _, _ := s.DailyPuzzleResult(ctx, boris.ID, "2026-10-16"); tried {
		t.Error("a result of anna shows for boris")
	}

	// The tactics storm keeps every run and shows the best by mode.
	for _, run := range []struct {
		mode  string
		score int
	}{{"3min", 11}, {"3min", 17}, {"3min", 9}, {"5min", 20}} {
		if err := s.RecordRush(ctx, anna.ID, run.mode, run.score); err != nil {
			t.Fatal(err)
		}
	}
	bests, err := s.RushBests(ctx, anna.ID)
	if err != nil || bests["3min"] != (RushBest{Best: 17, Runs: 3}) || bests["5min"] != (RushBest{Best: 20, Runs: 1}) {
		t.Errorf("rush bests: %+v (%v)", bests, err)
	}
	if bests, _ := s.RushBests(ctx, boris.ID); len(bests) != 0 {
		t.Errorf("a record of anna shows for boris: %+v", bests)
	}

	// Glicko-2 ratings: one row per person and category, replaced by each save.
	if _, err := s.Rating(ctx, anna.ID, "blitz"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a rating never made: %v", err)
	}
	at0 := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	if err := s.SaveRating(ctx, &RatingRow{UserID: anna.ID, Category: "blitz", Rating: 1234.5, RD: 180.25, Volatility: 0.06, Games: 1, Wins: 1, UpdatedAt: at0}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRating(ctx, &RatingRow{UserID: anna.ID, Category: "blitz", Rating: 1250.75, RD: 160, Volatility: 0.0599, Games: 2, Wins: 1, Losses: 1, UpdatedAt: at0.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRating(ctx, &RatingRow{UserID: anna.ID, Category: "duel", Rating: 1200, RD: 350, Volatility: 0.06, UpdatedAt: at0}); err != nil {
		t.Fatal(err)
	}
	rr, err := s.Rating(ctx, anna.ID, "blitz")
	if err != nil || rr.Rating != 1250.75 || rr.RD != 160 || rr.Games != 2 || rr.Losses != 1 || !rr.UpdatedAt.Equal(at0.Add(time.Hour)) {
		t.Errorf("rating: %+v (%v)", rr, err)
	}
	if all, err := s.Ratings(ctx, anna.ID); err != nil || len(all) != 2 || all[0].Category != "blitz" || all[1].Category != "duel" {
		t.Errorf("ratings: %v (%v)", all, err)
	}
	if all, _ := s.Ratings(ctx, boris.ID); len(all) != 0 {
		t.Errorf("a rating of anna shows for boris: %v", all)
	}
	// a finished duel is written once
	duel := &DuelRow{ID: "d000000000000001", AID: anna.ID, BID: boris.ID, AScore: 7, BScore: 5, AMistakes: 1, BMistakes: 2, Result: "a", CreatedAt: at0}
	for i := 0; i < 2; i++ {
		if err := s.SaveDuel(ctx, duel); err != nil {
			t.Fatal(err)
		}
	}
	if r, _ := s.PuzzleRatingOf(ctx, anna.ID); r.Attempts != 1 || r.Rating != 1020 { // only the one rated try above
		t.Errorf("practice moved the rating: %+v", r)
	}
	s.RecordDailyPuzzle(ctx, boris.ID, "2026-10-17", "tzaaa", false)
	s.RecordDailyPuzzle(ctx, boris.ID, "2026-10-16", "tZaAa", false)
	s.RecordDailyPuzzle(ctx, anna.ID, "2026-10-15", "tZaAa", true)
	// The puzzle of the day: the seed picks by id order among the matching ones.
	for seed, want := range map[uint64]string{0: "tZaAa", 1: "tzaaa", 2: "tZaAa", 7: "tzaaa"} {
		if p, err := s.DailyPuzzle(ctx, 5001, 5002, 90, seed); err != nil || p.ID != want {
			t.Errorf("daily seed %d: %+v, want %s (%v)", seed, p, want, err)
		}
	}
	if _, err := s.DailyPuzzle(ctx, 5001, 5002, 91, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("daily puzzle with too high a popularity bar: %v", err)
	}
	if _, err := s.PickPuzzle(ctx, anna.ID, "", 5001, 5001, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("a tried puzzle was picked again: %v", err)
	}
	if p, err := s.PickPuzzle(ctx, anna.ID, "", 5001, 5001, false); err != nil || p.ID != "tZaAa" {
		t.Errorf("tried puzzles allowed: %+v (%v)", p, err)
	}

	// Merging adds the records up and keeps the rating of the one with more attempts.
	s.RecordPuzzleAttempt(ctx, boris.ID, "tzaaa", false, func(r, n int) int { return r - 10 }, nil)
	s.RecordPuzzleAttempt(ctx, boris.ID, "other", false, func(r, n int) int { return r - 10 }, nil)
	if err := s.MergeUsers(ctx, anna.ID, boris.ID); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.PuzzleRatingOf(ctx, boris.ID); r.Attempts != 3 || r.Solved != 1 || r.Rating != 980 {
		t.Errorf("after merge: %+v", r)
	}
	if tried, solved, _ := s.DailyPuzzleResult(ctx, boris.ID, "2026-10-16"); !tried || solved {
		t.Errorf("daily result of a day both played: the target's must stay (tried %v, solved %v)", tried, solved)
	}
	if tried, _, _ := s.DailyPuzzleResult(ctx, boris.ID, "2026-10-17"); !tried {
		t.Error("a daily result of the target was lost in the merge")
	}
	if tried, solved, _ := s.DailyPuzzleResult(ctx, boris.ID, "2026-10-15"); !tried || !solved {
		t.Error("a daily result of the merged-away account was lost")
	}
	if at, _ := s.RecordPuzzleAttempt(ctx, boris.ID, "tZaAa", true, up, nil); at.Counted {
		t.Error("the attempt of the merged-away account was forgotten")
	}
}

func TestRepertoires(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	anna, boris := newUserID(t, s, KindGuest), newUserID(t, s, KindGuest)

	rep, err := s.CreateRepertoire(ctx, anna.ID, "Итальянка", "white")
	if err != nil || rep.ID == 0 {
		t.Fatal(rep, err)
	}
	if _, err := s.Repertoire(ctx, boris.ID, rep.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("somebody else's repertoire: %v", err)
	}

	// 1.e4 (mine) e5 2.Nf3 (mine); the same edge twice is one row
	add := func(parent int64, uci, san string, ply int, mine bool) RepMove {
		t.Helper()
		m, created, err := s.AddRepMove(ctx, RepMove{RepertoireID: rep.ID, ParentID: parent, UCI: uci, SAN: san, FEN: "fen " + uci, Ply: ply, Mine: mine})
		if err != nil || !created || m.ID == 0 {
			t.Fatalf("add %s: %+v created=%v err=%v", uci, m, created, err)
		}
		return m
	}
	m1 := add(0, "e2e4", "e4", 1, true)
	m2 := add(m1.ID, "e7e5", "e5", 2, false)
	m3 := add(m2.ID, "g1f3", "Nf3", 3, true)
	// a batch: one move under an existing move, one under a move of the batch
	before, _ := s.RepMoves(ctx, rep.ID)
	if err := s.AddRepMovesBulk(ctx, rep.ID, []NewRepMove{
		{ParentID: m3.ID, ParentIndex: -1, UCI: "b8c6", SAN: "Nc6", FEN: "fen b8c6", Ply: 4},
		{ParentIndex: 0, UCI: "f1c4", SAN: "Bc4", FEN: "fen f1c4", Ply: 5, Mine: true},
	}); err != nil {
		t.Fatal(err)
	}
	all, _ := s.RepMoves(ctx, rep.ID)
	if len(all) != len(before)+2 {
		t.Fatalf("bulk add: %d moves, was %d", len(all), len(before))
	}
	nc6, bc4 := all[len(all)-2], all[len(all)-1]
	if nc6.ParentID != m3.ID || bc4.ParentID != nc6.ID || !bc4.Mine || nc6.Mine {
		t.Errorf("bulk parents: %+v %+v", nc6, bc4)
	}
	if err := s.AddRepMovesBulk(ctx, rep.ID, []NewRepMove{{ParentIndex: 3, UCI: "a2a3", SAN: "a3", FEN: "x", Ply: 1}}); err == nil {
		t.Error("a parent that comes later in the batch was accepted")
	}
	if err := s.AddRepMovesBulk(ctx, rep.ID, make([]NewRepMove, MaxRepMoves)); !errors.Is(err, ErrLimit) {
		t.Errorf("over the limit: %v", err)
	}
	if after, _ := s.RepMoves(ctx, rep.ID); len(after) != len(all) {
		t.Errorf("a refused batch left %d moves behind", len(after)-len(all))
	}
	if _, err := s.DeleteRepMove(ctx, rep.ID, nc6.ID); err != nil { // the two bulk moves go, as they were added to the tree
		t.Fatal(err)
	}
	again, created, err := s.AddRepMove(ctx, RepMove{RepertoireID: rep.ID, ParentID: 0, UCI: "e2e4", SAN: "e4", FEN: "x", Ply: 1, Mine: true})
	if err != nil || created || again.ID != m1.ID {
		t.Errorf("duplicate edge: %+v created=%v err=%v", again, created, err)
	}

	list, err := s.ListRepertoires(ctx, anna.ID, now)
	if err != nil || len(list) != 1 || list[0].Counts != (RepertoireCounts{Moves: 3, Mine: 2, New: 2}) {
		t.Fatalf("list: %+v (%v)", list, err)
	}
	if other, _ := s.ListRepertoires(ctx, boris.ID, now); len(other) != 0 {
		t.Errorf("boris sees %v", other)
	}

	// a card: first answer asks again tomorrow, becomes due after that
	c, err := s.ReviewRepMove(ctx, rep.ID, m1.ID, func(box, lapses int) Card {
		return Card{Box: box + 1, DueAt: now.Add(-time.Minute), Lapses: lapses}
	})
	if err != nil || c.Box != 1 {
		t.Fatalf("review: %+v (%v)", c, err)
	}
	if _, err := s.ReviewRepMove(ctx, rep.ID, m2.ID, func(b, l int) Card { return Card{} }); !errors.Is(err, ErrNotFound) {
		t.Errorf("the opponent's move is not a card: %v", err)
	}
	list, _ = s.ListRepertoires(ctx, anna.ID, now)
	if got := list[0].Counts; got != (RepertoireCounts{Moves: 3, Mine: 2, New: 1, Due: 1}) {
		t.Errorf("after one review: %+v", got)
	}
	moves, _ := s.RepMoves(ctx, rep.ID)
	if len(moves) != 3 || moves[0].ID != m1.ID || moves[2].ID != m3.ID || !moves[0].DueAt.Valid || moves[0].Reviews != 1 {
		t.Errorf("moves: %+v", moves)
	}

	// merging two accounts moves the repertoires
	if err := s.MergeUsers(ctx, anna.ID, boris.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Repertoire(ctx, boris.ID, rep.ID); err != nil || got.Name != "Итальянка" {
		t.Errorf("after the merge: %+v (%v)", got, err)
	}

	// deleting a move takes the lines after it
	n, err := s.DeleteRepMove(ctx, rep.ID, m2.ID)
	if err != nil || n != 2 {
		t.Errorf("delete subtree: %d (%v)", n, err)
	}
	if _, err := s.DeleteRepMove(ctx, rep.ID, m2.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete again: %v", err)
	}
	if left, _ := s.RepMoves(ctx, rep.ID); len(left) != 1 {
		t.Errorf("left: %+v", left)
	}

	// rename, then delete the whole repertoire
	if err := s.RenameRepertoire(ctx, boris.ID, rep.ID, "Новое"); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameRepertoire(ctx, boris.ID, rep.ID, "Новое"); err != nil { // the same name changes no rows
		t.Errorf("rename to the same name: %v", err)
	}
	if err := s.DeleteRepertoire(ctx, boris.ID, rep.ID); err != nil {
		t.Fatal(err)
	}
	if left, _ := s.RepMoves(ctx, rep.ID); len(left) != 0 {
		t.Errorf("moves of a deleted repertoire: %+v", left)
	}
	if err := s.DeleteRepertoire(ctx, boris.ID, rep.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete again: %v", err)
	}
}

func TestCoachNotes(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	hash := fmt.Sprintf("%064x", time.Now().UnixNano())
	if _, ok, err := s.LoadCoachNote(ctx, hash); ok || err != nil {
		t.Fatalf("unknown note: %v %v", ok, err)
	}
	text := "Ферзь без защиты: 12…Qxb2 😀"
	if err := s.SaveCoachNote(ctx, hash, "some/model", text); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveCoachNote(ctx, hash, "other/model", "second"); err != nil { // the first one stays
		t.Fatal(err)
	}
	got, ok, err := s.LoadCoachNote(ctx, hash)
	if err != nil || !ok || got != text {
		t.Errorf("got %q %v %v", got, ok, err)
	}
}

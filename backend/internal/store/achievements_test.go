package store

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

var (
	achSeq  atomic.Int64
	achBase = time.Now().UnixNano() & 0xffffff
)

func achUser(t *testing.T, s *Store) *User {
	t.Helper()
	u := &User{ID: fmt.Sprintf("ac%06x%03x", achBase, achSeq.Add(1)&0xfff), Kind: KindGuest, Name: "Знаток"}
	if err := s.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.db.Exec("DELETE FROM games WHERE white_id = ? OR black_id = ?", u.ID, u.ID)
		s.db.Exec("DELETE FROM users WHERE id = ?", u.ID)
	})
	return u
}

func TestAchievementRowsAreWrittenOnce(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u := achUser(t, s)

	if rows, err := s.Achievements(ctx, u.ID); err != nil || len(rows) != 0 {
		t.Fatalf("a new person has none: %v %v", rows, err)
	}
	t1 := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	added, err := s.AddAchievements(ctx, u.ID, []string{"first_game", "games_10"}, t1)
	if err != nil || len(added) != 2 {
		t.Fatalf("add: %v %v", added, err)
	}
	// the same badge again, later: not written, the date stays; only the new one is reported
	added, err = s.AddAchievements(ctx, u.ID, []string{"first_game", "mate_win"}, t1.Add(time.Hour))
	if err != nil || len(added) != 1 || added[0] != "mate_win" {
		t.Fatalf("second add: %v %v", added, err)
	}
	if added, _ = s.AddAchievements(ctx, u.ID, nil, t1); len(added) != 0 {
		t.Errorf("nothing to add: %v", added)
	}
	rows, _ := s.Achievements(ctx, u.ID)
	if len(rows) != 3 || rows[0].Key != "mate_win" { // newest first
		t.Fatalf("rows: %+v", rows)
	}
	for _, r := range rows {
		if r.Seen {
			t.Errorf("%s is new, so unseen", r.Key)
		}
		if r.Key == "first_game" && !r.UnlockedAt.Equal(t1) {
			t.Errorf("the date moved: %v", r.UnlockedAt)
		}
	}

	if err := s.MarkAchievementsSeen(ctx, u.ID, []string{"first_game"}); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.Achievements(ctx, u.ID)
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.Key] = r.Seen
	}
	if !seen["first_game"] || seen["games_10"] || seen["mate_win"] {
		t.Errorf("seen: %v", seen)
	}
	if err := s.MarkAchievementsSeen(ctx, u.ID, nil); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.Achievements(ctx, u.ID)
	for _, r := range rows {
		if !r.Seen {
			t.Errorf("%s: all should be seen", r.Key)
		}
	}
}

func TestMergeUsersCombinesAchievements(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	guest, account := achUser(t, s), achUser(t, s)
	early := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	late := early.Add(48 * time.Hour)
	if _, err := s.AddAchievements(ctx, guest.ID, []string{"first_game", "mate_win"}, early); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddAchievements(ctx, account.ID, []string{"first_game", "first_friend"}, late); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkAchievementsSeen(ctx, account.ID, []string{"first_friend"}); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeUsers(ctx, guest.ID, account.ID); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Achievements(ctx, account.ID)
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows: %+v %v", rows, err)
	}
	for _, r := range rows {
		switch r.Key {
		case "first_game":
			if !r.UnlockedAt.Equal(early) {
				t.Errorf("the earlier date wins: %v", r.UnlockedAt)
			}
		case "mate_win":
			if !r.UnlockedAt.Equal(early) {
				t.Errorf("mate_win: %v", r.UnlockedAt)
			}
		case "first_friend":
			if !r.Seen || !r.UnlockedAt.Equal(late) {
				t.Errorf("the account's own badge is untouched: %+v", r)
			}
		default:
			t.Errorf("unexpected %s", r.Key)
		}
	}
	if left, _ := s.Achievements(ctx, guest.ID); len(left) != 0 {
		t.Errorf("the deleted guest keeps rows: %+v", left)
	}
}

func saveGame(t *testing.T, s *Store, id, white, black, result, state string, corr bool) {
	t.Helper()
	g := &Game{ID: id, WhiteID: white, BlackID: black, Status: "ended", Result: result, State: []byte(state),
		CreatedAt: time.Now(), UpdatedAt: time.Now(), Corr: corr}
	if err := s.SaveGame(context.Background(), g); err != nil {
		t.Fatal(err)
	}
}

// moves makes the JSON array of n half-moves.
func moves(n int) string {
	out := "["
	for i := 0; i < n; i++ {
		if i > 0 {
			out += ","
		}
		out += `{"uci":"a2a3","san":"a3"}`
	}
	return out + "]"
}

func TestAchievementFactsCountGames(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	me, other := achUser(t, s), achUser(t, s)
	q := AchievementQuery{MinRatedGames: 5, MinPuzzleAttempts: 10, LongGamePlies: 79}
	id := func(n int) string { return fmt.Sprintf("%sg%02d", me.ID, n) } // 14 characters

	// 1. rated mate win with White against a person, short
	saveGame(t, s, id(1), me.ID, other.ID, "1-0", `{"termination":"checkmate","rated":true,"moves":`+moves(30)+`}`, false)
	// 2. a win on time with Black against a bot, long (79 half-moves)
	saveGame(t, s, id(2), "", me.ID, "0-1", `{"termination":"timeout","moves":`+moves(79)+`}`, false)
	// 3. a draw with Black against a person, 78 half-moves: not long
	saveGame(t, s, id(3), other.ID, me.ID, "1/2-1/2", `{"termination":"agreement","moves":`+moves(78)+`}`, false)
	// 4. a loss with White against a bot
	saveGame(t, s, id(4), me.ID, "", "0-1", `{"termination":"checkmate","moves":`+moves(20)+`}`, false)
	// 5. a won correspondence game by resignation against a person
	saveGame(t, s, id(5), me.ID, other.ID, "1-0", `{"termination":"resign","moves":`+moves(10)+`}`, true)
	// 6. an unfinished game and an aborted one do not count
	g := &Game{ID: id(6), WhiteID: me.ID, BlackID: other.ID, Status: "playing", State: []byte(`{"moves":[]}`), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.SaveGame(ctx, g); err != nil {
		t.Fatal(err)
	}

	f, err := s.AchievementFacts(ctx, me.ID, q)
	if err != nil {
		t.Fatal(err)
	}
	if f.Games != 5 || f.Wins != 3 || f.Draws != 1 {
		t.Errorf("games %d wins %d draws %d", f.Games, f.Wins, f.Draws)
	}
	if f.HumanWins != 2 || f.RatedWins != 1 {
		t.Errorf("human wins %d rated %d", f.HumanWins, f.RatedWins)
	}
	if f.MateWins != 1 || f.TimeWins != 1 {
		t.Errorf("mate %d time %d", f.MateWins, f.TimeWins)
	}
	if f.LongGames != 1 || f.CorrGames != 1 {
		t.Errorf("long %d corr %d", f.LongGames, f.CorrGames)
	}
	// the other side sees the same games from the other seat
	fo, err := s.AchievementFacts(ctx, other.ID, q)
	if err != nil {
		t.Fatal(err)
	}
	if fo.Games != 3 || fo.Wins != 0 || fo.Draws != 1 || fo.HumanWins != 0 {
		t.Errorf("the other person: %+v", fo)
	}
}

func TestAchievementFactsCountTheRest(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	me, friend := achUser(t, s), achUser(t, s)
	q := AchievementQuery{MinRatedGames: 5, MinPuzzleAttempts: 10, LongGamePlies: 79}
	now := time.Now().UTC()

	// bots: Максимум beaten, Миша only lost to
	if _, err := s.RecordBotResult(ctx, me.ID, "max", "win", 150); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordBotResult(ctx, me.ID, "misha", "loss", 10); err != nil {
		t.Fatal(err)
	}
	// ratings: blitz counts (6 games), bullet does not yet (2 games)
	for cat, r := range map[string]struct {
		rating float64
		games  int
	}{"blitz": {1650, 6}, "bullet": {1900, 2}} {
		if err := s.SaveRating(ctx, &RatingRow{UserID: me.ID, Category: cat, Rating: r.rating, RD: 80, Volatility: 0.06, Games: r.games, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	// puzzles: 3 solved of 4; a rating with too few tries does not count
	for i := 0; i < 4; i++ {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO puzzle_attempts (user_id, puzzle_id, solved, rating_before, rating_after, created_at)
			VALUES (?, ?, ?, 1000, 1010, ?)`, me.ID, fmt.Sprintf("pz%d", i), i != 0, now); err != nil {
			t.Fatal(err)
		}
	}
	s.db.ExecContext(ctx, "INSERT INTO puzzle_ratings (user_id, rating, attempts, solved, updated_at) VALUES (?, 1700, 4, 3, ?)", me.ID, now)
	s.db.ExecContext(ctx, "INSERT INTO puzzle_points (user_id, points, streak, best_streak, updated_at) VALUES (?, 50, 2, 7, ?)", me.ID, now)
	s.db.ExecContext(ctx, "INSERT INTO rush_results (user_id, mode, score, created_at) VALUES (?, '3min', 12, ?), (?, '5min', 31, ?)", me.ID, now, me.ID, now)
	// a duel won as side b and one lost as side a
	s.db.ExecContext(ctx, "INSERT INTO duels (id, a_id, b_id, a_score, b_score, a_mistakes, b_mistakes, result, created_at) VALUES (?, ?, ?, 3, 5, 1, 0, 'b', ?)",
		me.ID+"d1", friend.ID, me.ID, now)
	s.db.ExecContext(ctx, "INSERT INTO duels (id, a_id, b_id, a_score, b_score, a_mistakes, b_mistakes, result, created_at) VALUES (?, ?, ?, 3, 5, 1, 0, 'b', ?)",
		me.ID+"d2", me.ID, friend.ID, now)
	// a repository with three moves, one of them learned, one only a reply (not mine)
	res, err := s.db.ExecContext(ctx, "INSERT INTO repertoires (user_id, name, color, created_at) VALUES (?, 'Итальянка', 'white', ?)", me.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	rid, _ := res.LastInsertId()
	for i, m := range []struct {
		uci  string
		mine bool
		box  int
	}{{"e2e4", true, LearnedBox}, {"e7e5", false, 0}, {"g1f3", true, 1}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO rep_moves (repertoire_id, parent_id, uci, san, fen, ply, mine, box, created_at)
			VALUES (?, ?, ?, 'x', 'f', ?, ?, ?, ?)`, rid, i, m.uci, i+1, m.mine, m.box, now); err != nil {
			t.Fatal(err)
		}
	}
	// reviews: one done, one failed
	for i, st := range []string{"done", "failed"} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO analyses (id, status, depth, game, created_at, user_id) VALUES (?, ?, 10, '{}', ?, ?)`,
			fmt.Sprintf("%sr%d", me.ID, i), st, now, me.ID); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { s.db.Exec("DELETE FROM analyses WHERE user_id = ?", me.ID) })
	// community: a friend (two rows) and a forum topic with a deleted and a live post
	s.db.ExecContext(ctx, "INSERT INTO friends (user_id, friend_id, created_at) VALUES (?, ?, ?), (?, ?, ?)", me.ID, friend.ID, now, friend.ID, me.ID, now)
	rt, err := s.db.ExecContext(ctx, "INSERT INTO forum_topics (category, author_id, author_name, title, created_at, last_post_at) VALUES ('general', ?, 'Знаток', 'Тема', ?, ?)", me.ID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	tid, _ := rt.LastInsertId()
	t.Cleanup(func() { s.db.Exec("DELETE FROM forum_topics WHERE id = ?", tid) })
	s.db.ExecContext(ctx, "INSERT INTO forum_posts (topic_id, author_id, author_name, body, created_at, deleted) VALUES (?, ?, 'Знаток', 'a', ?, 0), (?, ?, 'Знаток', 'b', ?, 1)", tid, me.ID, now, tid, me.ID, now)
	// lessons: one finished, one begun
	s.SaveLessonProgress(ctx, me.ID, "rook", 5, 0, true)
	s.SaveLessonProgress(ctx, me.ID, "bishop", 2, 0, false)

	f, err := s.AchievementFacts(ctx, me.ID, q)
	if err != nil {
		t.Fatal(err)
	}
	if f.BotsBeaten != 1 || !f.MaxBeaten || f.DionisDown {
		t.Errorf("bots: %d %v %v", f.BotsBeaten, f.MaxBeaten, f.DionisDown)
	}
	if f.RatingMax != 1650 {
		t.Errorf("rating counted only with enough games: %d", f.RatingMax)
	}
	if f.PuzzlesSolved != 3 || f.PuzzleRating != 0 || f.BestPointsStreak != 7 || f.RushBest != 31 {
		t.Errorf("puzzles: solved %d rating %d streak %d rush %d", f.PuzzlesSolved, f.PuzzleRating, f.BestPointsStreak, f.RushBest)
	}
	if f.DuelGames != 2 || f.DuelWins != 1 {
		t.Errorf("duels: %d games %d won", f.DuelGames, f.DuelWins)
	}
	if f.Repertoires != 1 || f.RepMoves != 3 || f.LearnedMoves != 1 {
		t.Errorf("repository: %d %d %d", f.Repertoires, f.RepMoves, f.LearnedMoves)
	}
	if f.Reviews != 1 || f.Friends != 1 || f.ForumPosts != 1 {
		t.Errorf("reviews %d friends %d posts %d", f.Reviews, f.Friends, f.ForumPosts)
	}
	if len(f.LessonsDone) != 1 || f.LessonsDone[0] != "rook" {
		t.Errorf("lessons: %v", f.LessonsDone)
	}

	// with enough tries the puzzle rating counts
	s.db.ExecContext(ctx, "UPDATE puzzle_ratings SET attempts = 12 WHERE user_id = ?", me.ID)
	if f, _ = s.AchievementFacts(ctx, me.ID, q); f.PuzzleRating != 1700 {
		t.Errorf("puzzle rating: %d", f.PuzzleRating)
	}

	// the reviews with a high accuracy and with brilliant moves
	report := `{"moves":[{"class":"best"},{"class":"great"},{"class":"brilliant"},{"class":"best"},{"class":"brilliant"}]}`
	if _, err := s.db.ExecContext(ctx, `UPDATE analyses SET game = ?, summary = ?, report = ? WHERE user_id = ? AND status = 'done'`,
		`{"white":"Знаток","black":"Миша"}`, `{"accuracy":[91.5,60],"plies":40}`, report, me.ID); err != nil {
		t.Fatal(err)
	}
	acc, err := s.AccuracyReviews(ctx, me.ID, 90, 20, 100)
	if err != nil || len(acc) != 1 {
		t.Fatalf("accuracy rows: %v %v", acc, err)
	}
	if acc, _ = s.AccuracyReviews(ctx, me.ID, 92, 20, 100); len(acc) != 0 {
		t.Errorf("91.5 is not 92: %v", acc)
	}
	if acc, _ = s.AccuracyReviews(ctx, me.ID, 90, 41, 100); len(acc) != 0 {
		t.Errorf("a short game does not count: %v", acc)
	}
	bril, err := s.BrilliantReviews(ctx, me.ID, 100)
	if err != nil || len(bril) != 1 || len(bril[0].Moves) != 2 || bril[0].Moves[0] != 2 || bril[0].Moves[1] != 4 {
		t.Fatalf("brilliant rows: %+v %v", bril, err)
	}
	s.db.ExecContext(ctx, `UPDATE analyses SET report = '{"moves":[{"class":"best"}]}' WHERE user_id = ?`, me.ID)
	if bril, _ = s.BrilliantReviews(ctx, me.ID, 100); len(bril) != 0 {
		t.Errorf("no brilliant move: %+v", bril)
	}
}

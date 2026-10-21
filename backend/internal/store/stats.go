package store

import (
	"context"
	"time"
)

// Queries behind the profile's statistics tab (internal/stats). Each one reads a
// single person's rows through an index that starts with their id.

// Tally is a record of wins, draws and losses.
type Tally struct {
	Wins   int
	Draws  int
	Losses int
}

// Total is the number of games in the record.
func (t Tally) Total() int { return t.Wins + t.Draws + t.Losses }

// add counts one game from the point of view of a side that played White (white true) or Black.
// result is the game's result as stored ("1-0", "0-1", "1/2-1/2"); anything else is not counted.
func (t *Tally) add(white bool, result string, n int) {
	switch result {
	case "1/2-1/2":
		t.Draws += n
	case "1-0":
		if white {
			t.Wins += n
		} else {
			t.Losses += n
		}
	case "0-1":
		if white {
			t.Losses += n
		} else {
			t.Wins += n
		}
	}
}

// GameTally counts the person's finished online games (not those that were given up before they
// began): against other people and against bots. A bot is not a user, so its seat is NULL; an
// opponent whose account was deleted has a NULL seat too and counts as a bot, which is rare enough.
func (s *Store) GameTally(ctx context.Context, userID string) (humans, bots Tally, err error) {
	for _, side := range []struct {
		white        bool
		mine, theirs string
	}{{true, "white_id", "black_id"}, {false, "black_id", "white_id"}} {
		rows, qerr := s.db.QueryContext(ctx, "SELECT "+side.theirs+" IS NULL, result, COUNT(*) FROM games WHERE "+side.mine+
			" = ? AND status = 'ended' AND result IS NOT NULL GROUP BY 1, 2", userID)
		if qerr != nil {
			return humans, bots, qerr
		}
		for rows.Next() {
			var vsBot bool
			var result string
			var n int
			if err = rows.Scan(&vsBot, &result, &n); err != nil {
				rows.Close()
				return humans, bots, err
			}
			if vsBot {
				bots.add(side.white, result, n)
			} else {
				humans.add(side.white, result, n)
			}
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return humans, bots, err
		}
		rows.Close()
	}
	return humans, bots, nil
}

// DuelTally counts the person's finished puzzle duels.
func (s *Store) DuelTally(ctx context.Context, userID string) (Tally, error) {
	var t Tally
	for _, side := range []struct{ mine, wins string }{{"a_id", "a"}, {"b_id", "b"}} {
		rows, err := s.db.QueryContext(ctx, "SELECT result, COUNT(*) FROM duels WHERE "+side.mine+" = ? GROUP BY result", userID)
		if err != nil {
			return t, err
		}
		for rows.Next() {
			var result string
			var n int
			if err := rows.Scan(&result, &n); err != nil {
				rows.Close()
				return t, err
			}
			switch result {
			case "draw":
				t.Draws += n
			case side.wins:
				t.Wins += n
			default:
				t.Losses += n
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return t, err
		}
		rows.Close()
	}
	return t, nil
}

// ReviewBrief is a finished review as the statistics need it: who played, and the summary with
// the accuracy of both sides.
type ReviewBrief struct {
	ID        string
	Game      []byte // JSON, api.GameInfo
	Summary   []byte // JSON, api.Summary
	CreatedAt time.Time
}

// ReviewCount is the number of finished reviews the person has.
func (s *Store) ReviewCount(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM analyses WHERE user_id = ? AND status = 'done'", userID).Scan(&n)
	return n, err
}

// RecentReviews returns the person's latest finished reviews that have a summary, newest first.
func (s *Store) RecentReviews(ctx context.Context, userID string, limit int) ([]ReviewBrief, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, game, summary, created_at FROM analyses
		WHERE user_id = ? AND status = 'done' AND summary IS NOT NULL ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReviewBrief
	for rows.Next() {
		var r ReviewBrief
		if err := rows.Scan(&r.ID, &r.Game, &r.Summary, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.CreatedAt = r.CreatedAt.UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

// ThemeFailures is how a person fared in puzzles of one theme.
type ThemeFailures struct {
	Theme    string
	Attempts int
	Failed   int
}

// PuzzleThemeFailures returns, for every theme the person has met, the number of puzzles tried
// and failed on the first go (only first tries are recorded). Themes without a failure are left out.
func (s *Store) PuzzleThemeFailures(ctx context.Context, userID string) ([]ThemeFailures, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.theme, COUNT(*), CAST(SUM(a.solved = 0) AS SIGNED)
		FROM puzzle_attempts a JOIN puzzle_themes t ON t.puzzle_id = a.puzzle_id
		WHERE a.user_id = ? GROUP BY t.theme HAVING SUM(a.solved = 0) > 0`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ThemeFailures
	for rows.Next() {
		var t ThemeFailures
		if err := rows.Scan(&t.Theme, &t.Attempts, &t.Failed); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// AttemptRow is one counted try at a puzzle.
type AttemptRow struct {
	PuzzleID string
	Solved   bool
	Before   int
	After    int
	At       time.Time
}

// RecentPuzzleAttempts returns the person's latest tries at puzzles, newest first.
func (s *Store) RecentPuzzleAttempts(ctx context.Context, userID string, limit int) ([]AttemptRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT puzzle_id, solved, rating_before, rating_after, created_at
		FROM puzzle_attempts WHERE user_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AttemptRow
	for rows.Next() {
		var a AttemptRow
		if err := rows.Scan(&a.PuzzleID, &a.Solved, &a.Before, &a.After, &a.At); err != nil {
			return nil, err
		}
		a.At = a.At.UTC()
		out = append(out, a)
	}
	return out, rows.Err()
}

// PuzzleDays counts, for every UTC day the person did puzzles on, how many things they did: tries
// at rated puzzles, the puzzle of the day, runs of the tactics storm and puzzle duels. The key is
// "YYYY-MM-DD". Days are stored in UTC, so no time zone conversion happens in the query.
func (s *Store) PuzzleDays(ctx context.Context, userID string) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT d, CAST(SUM(n) AS SIGNED) FROM (
			SELECT DATE_FORMAT(created_at, '%Y-%m-%d') AS d, COUNT(*) AS n FROM puzzle_attempts WHERE user_id = ? GROUP BY 1
			UNION ALL
			SELECT DATE_FORMAT(day, '%Y-%m-%d'), COUNT(*) FROM daily_puzzle_results WHERE user_id = ? GROUP BY 1
			UNION ALL
			SELECT DATE_FORMAT(created_at, '%Y-%m-%d'), COUNT(*) FROM rush_results WHERE user_id = ? GROUP BY 1
			UNION ALL
			SELECT DATE_FORMAT(created_at, '%Y-%m-%d'), COUNT(*) FROM duels WHERE a_id = ? GROUP BY 1
			UNION ALL
			SELECT DATE_FORMAT(created_at, '%Y-%m-%d'), COUNT(*) FROM duels WHERE b_id = ? GROUP BY 1
		) t GROUP BY d`, userID, userID, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var d string
		var n int
		if err := rows.Scan(&d, &n); err != nil {
			return nil, err
		}
		out[d] = n
	}
	return out, rows.Err()
}

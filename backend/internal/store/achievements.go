package store

import (
	"context"
	"regexp"
	"strconv"
	"time"
)

// Queries behind the achievements (internal/achievements). What earns a badge is counted here from the
// tables that already exist, so no other package has to know about achievements; the table
// `achievements` only remembers what was earned.

// AchievementQuery holds the thresholds that the counting queries need; they live in
// internal/achievements and are passed in.
type AchievementQuery struct {
	MinRatedGames     int // games in a category before its rating counts
	MinPuzzleAttempts int // tries at puzzles before the puzzle rating counts
	LongGamePlies     int // half-moves of a "long game"
}

// AchievementFacts are the counters of one person that the achievements are decided by.
type AchievementFacts struct {
	// finished online games (against people and bots)
	Games, Wins, Draws    int
	HumanWins, RatedWins  int
	MateWins, TimeWins    int
	LongGames, CorrGames  int
	BotsBeaten            int
	MaxBeaten, DionisDown bool
	// ratings
	RatingMax int // the best Glicko-2 rating over the categories with enough games
	DuelGames int
	DuelWins  int
	// puzzles
	PuzzlesSolved    int
	PuzzleRating     int // 0 until enough tries
	BestPointsStreak int
	RushBest         int
	// reviews
	Reviews int
	// opening repositories
	Repertoires  int
	RepMoves     int
	LearnedMoves int
	// community
	Friends    int
	ForumPosts int
	// lessons the person has finished
	LessonsDone []string
}

// AchievementFacts counts everything in four small queries, each through an index that starts with
// the person's id.
func (s *Store) AchievementFacts(ctx context.Context, userID string, q AchievementQuery) (AchievementFacts, error) {
	var f AchievementFacts
	if err := s.achievementGames(ctx, userID, q, &f); err != nil {
		return f, err
	}
	var rating float64
	var maxBeaten, dionis int
	err := s.db.QueryRowContext(ctx, `SELECT
		COALESCE((SELECT COUNT(*) FROM bot_progress WHERE user_id = ? AND wins > 0), 0),
		COALESCE((SELECT COUNT(*) FROM bot_progress WHERE user_id = ? AND wins > 0 AND bot_id = 'max'), 0),
		COALESCE((SELECT COUNT(*) FROM bot_progress WHERE user_id = ? AND wins > 0 AND bot_id = 'dionis'), 0),
		COALESCE((SELECT MAX(rating) FROM ratings WHERE user_id = ? AND games >= ?), 0),
		COALESCE((SELECT COUNT(*) FROM duels WHERE a_id = ?), 0) + COALESCE((SELECT COUNT(*) FROM duels WHERE b_id = ?), 0),
		COALESCE((SELECT COUNT(*) FROM duels WHERE a_id = ? AND result = 'a'), 0) + COALESCE((SELECT COUNT(*) FROM duels WHERE b_id = ? AND result = 'b'), 0),
		COALESCE((SELECT COUNT(*) FROM puzzle_attempts WHERE user_id = ? AND solved = 1), 0),
		COALESCE((SELECT IF(attempts >= ?, rating, 0) FROM puzzle_ratings WHERE user_id = ?), 0),
		COALESCE((SELECT best_streak FROM puzzle_points WHERE user_id = ?), 0),
		COALESCE((SELECT MAX(score) FROM rush_results WHERE user_id = ?), 0),
		COALESCE((SELECT COUNT(*) FROM analyses WHERE user_id = ? AND status = 'done'), 0),
		COALESCE((SELECT COUNT(*) FROM repertoires WHERE user_id = ?), 0),
		COALESCE((SELECT COUNT(*) FROM rep_moves m JOIN repertoires r ON r.id = m.repertoire_id WHERE r.user_id = ?), 0),
		COALESCE((SELECT COUNT(*) FROM rep_moves m JOIN repertoires r ON r.id = m.repertoire_id WHERE r.user_id = ? AND m.mine = 1 AND m.box >= ?), 0),
		COALESCE((SELECT COUNT(*) FROM friends WHERE user_id = ?), 0),
		COALESCE((SELECT COUNT(*) FROM forum_posts WHERE author_id = ? AND deleted = 0), 0)`,
		userID, userID, userID, userID, q.MinRatedGames, userID, userID, userID, userID, userID, q.MinPuzzleAttempts, userID,
		userID, userID, userID, userID, userID, userID, LearnedBox, userID, userID).Scan(
		&f.BotsBeaten, &maxBeaten, &dionis, &rating, &f.DuelGames, &f.DuelWins,
		&f.PuzzlesSolved, &f.PuzzleRating, &f.BestPointsStreak, &f.RushBest,
		&f.Reviews, &f.Repertoires, &f.RepMoves, &f.LearnedMoves, &f.Friends, &f.ForumPosts)
	if err != nil {
		return f, err
	}
	f.MaxBeaten, f.DionisDown, f.RatingMax = maxBeaten > 0, dionis > 0, int(rating)

	rows, err := s.db.QueryContext(ctx, "SELECT lesson_id FROM lesson_progress WHERE user_id = ? AND completed_at IS NOT NULL", userID)
	if err != nil {
		return f, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return f, err
		}
		f.LessonsDone = append(f.LessonsDone, id)
	}
	return f, rows.Err()
}

// achievementGames reads the finished games of the person grouped by what the badges tell apart:
// the side, whether the opponent is a bot, the result, how it ended, whether it was long, rated or by
// post. The grouping runs inside MySQL, so a person with thousands of games still costs a handful of
// rows to read.
func (s *Store) achievementGames(ctx context.Context, userID string, q AchievementQuery, f *AchievementFacts) error {
	for _, side := range []struct {
		white        bool
		mine, theirs string
	}{{true, "white_id", "black_id"}, {false, "black_id", "white_id"}} {
		rows, err := s.db.QueryContext(ctx, `SELECT `+side.theirs+` IS NULL, result,
				COALESCE(JSON_UNQUOTE(JSON_EXTRACT(state, '$.termination')), ''),
				COALESCE(JSON_LENGTH(JSON_EXTRACT(state, '$.moves')), 0) >= ?,
				COALESCE(JSON_UNQUOTE(JSON_EXTRACT(state, '$.rated')), '') = 'true',
				corr, COUNT(*)
			FROM games WHERE `+side.mine+` = ? AND status = 'ended' AND result IS NOT NULL
			GROUP BY 1, 2, 3, 4, 5, 6`, q.LongGamePlies, userID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var vsBot, long, rated, corr bool
			var result, term string
			var n int
			if err := rows.Scan(&vsBot, &result, &term, &long, &rated, &corr, &n); err != nil {
				rows.Close()
				return err
			}
			won := (side.white && result == "1-0") || (!side.white && result == "0-1")
			f.Games += n
			if result == "1/2-1/2" {
				f.Draws += n
			}
			if long {
				f.LongGames += n
			}
			if corr {
				f.CorrGames += n
			}
			if won {
				f.Wins += n
				if !vsBot {
					f.HumanWins += n
					if rated {
						f.RatedWins += n
					}
				}
				switch term {
				case "checkmate":
					f.MateWins += n
				case "timeout":
					f.TimeWins += n
				}
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// AccuracyReviews returns the person's latest finished reviews in which one of the two sides played with
// accuracy of at least min over at least minPlies half-moves; the caller finds out which side was theirs.
func (s *Store) AccuracyReviews(ctx context.Context, userID string, min float64, minPlies, limit int) ([]ReviewBrief, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, game, summary, created_at FROM analyses
		WHERE user_id = ? AND status = 'done' AND summary IS NOT NULL
			AND JSON_EXTRACT(summary, '$.plies') >= ?
			AND (JSON_EXTRACT(summary, '$.accuracy[0]') >= ? OR JSON_EXTRACT(summary, '$.accuracy[1]') >= ?)
		ORDER BY created_at DESC LIMIT ?`, userID, minPlies, min, min, limit)
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

// BrilliantReview is a review with brilliant moves: the game and the indexes of those moves in the
// report (0 is the first half-move, so even numbers are White's).
type BrilliantReview struct {
	ID    string
	Game  []byte // JSON, api.GameInfo
	Moves []int
}

var moveIndex = regexp.MustCompile(`\[(\d+)\]`)

// BrilliantReviews looks for moves of the class "brilliant" in the person's latest limit finished
// reviews. The search runs inside MySQL (JSON_SEARCH over the class of every move), so the reports
// themselves are not read into the server.
func (s *Store) BrilliantReviews(ctx context.Context, userID string, limit int) ([]BrilliantReview, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, game, CAST(JSON_SEARCH(report, 'all', 'brilliant', NULL, '$.moves[*].class') AS CHAR)
		FROM (SELECT id, game, report FROM analyses
			WHERE user_id = ? AND status = 'done' AND report IS NOT NULL ORDER BY created_at DESC LIMIT ?) t
		WHERE JSON_SEARCH(report, 'one', 'brilliant', NULL, '$.moves[*].class') IS NOT NULL`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BrilliantReview
	for rows.Next() {
		var b BrilliantReview
		var paths string
		if err := rows.Scan(&b.ID, &b.Game, &paths); err != nil {
			return nil, err
		}
		for _, m := range moveIndex.FindAllStringSubmatch(paths, -1) {
			if i, err := strconv.Atoi(m[1]); err == nil {
				b.Moves = append(b.Moves, i)
			}
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// AchievementRow is a badge a person has earned.
type AchievementRow struct {
	Key        string
	UnlockedAt time.Time
	Seen       bool
}

// Achievements returns the badges a person has earned, the newest first.
func (s *Store) Achievements(ctx context.Context, userID string) ([]AchievementRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ach_key, unlocked_at, seen FROM achievements
		WHERE user_id = ? ORDER BY unlocked_at DESC, ach_key`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AchievementRow
	for rows.Next() {
		var r AchievementRow
		if err := rows.Scan(&r.Key, &r.UnlockedAt, &r.Seen); err != nil {
			return nil, err
		}
		r.UnlockedAt = r.UnlockedAt.UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddAchievements records badges as earned at the given time (not yet seen). A badge that is already
// recorded keeps its date; the returned keys are the ones that were new.
func (s *Store) AddAchievements(ctx context.Context, userID string, keys []string, at time.Time) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var added []string
	for _, k := range keys {
		res, err := tx.ExecContext(ctx, "INSERT IGNORE INTO achievements (user_id, ach_key, unlocked_at, seen) VALUES (?, ?, ?, 0)", userID, k, at.UTC())
		if err != nil {
			return nil, err
		}
		if n, err := res.RowsAffected(); err == nil && n == 1 {
			added = append(added, k)
		}
	}
	return added, tx.Commit()
}

// MarkAchievementsSeen says that the browser has shown the notices of these badges; no keys means all.
func (s *Store) MarkAchievementsSeen(ctx context.Context, userID string, keys []string) error {
	if len(keys) == 0 {
		_, err := s.db.ExecContext(ctx, "UPDATE achievements SET seen = 1 WHERE user_id = ? AND seen = 0", userID)
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, k := range keys {
		if _, err := tx.ExecContext(ctx, "UPDATE achievements SET seen = 1 WHERE user_id = ? AND ach_key = ?", userID, k); err != nil {
			return err
		}
	}
	return tx.Commit()
}

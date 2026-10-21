package store

import (
	"context"
	"time"
)

// Queries behind the trainer page (internal/trainer): what a person has done today, and the full
// reports of their latest reviews.

const dayFormat = "2006-01-02"

// RecordPractice remembers a puzzle done as practice (outside the rating) on the day of at (UTC).
// A puzzle already recorded for that day is left as it was: the first try counts.
func (s *Store) RecordPractice(ctx context.Context, userID, puzzleID string, solved bool, at time.Time) error {
	at = at.UTC()
	_, err := s.db.ExecContext(ctx, `INSERT IGNORE INTO trainer_practice (user_id, day, puzzle_id, solved, created_at)
		VALUES (?, ?, ?, ?, ?)`, userID, at.Format(dayFormat), puzzleID, solved, at)
	return err
}

// PuzzlesSince counts the puzzles the person tried from since on: rated first tries and practice
// ones together (all), and those of them whose puzzle has the theme (onTheme; theme "" gives 0).
// since is the start of a UTC day.
func (s *Store) PuzzlesSince(ctx context.Context, userID string, since time.Time, theme string) (all, onTheme int, err error) {
	since = since.UTC()
	day := since.Format(dayFormat)
	err = s.db.QueryRowContext(ctx, `SELECT
			(SELECT COUNT(*) FROM puzzle_attempts WHERE user_id = ? AND created_at >= ?)
			+ (SELECT COUNT(*) FROM trainer_practice WHERE user_id = ? AND day >= ?)`,
		userID, since, userID, day).Scan(&all)
	if err != nil || theme == "" {
		return all, 0, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT
			(SELECT COUNT(*) FROM puzzle_attempts a JOIN puzzle_themes t ON t.puzzle_id = a.puzzle_id
				WHERE a.user_id = ? AND a.created_at >= ? AND t.theme = ?)
			+ (SELECT COUNT(*) FROM trainer_practice p JOIN puzzle_themes t ON t.puzzle_id = p.puzzle_id
				WHERE p.user_id = ? AND p.day >= ? AND t.theme = ?)`,
		userID, since, theme, userID, day, theme).Scan(&onTheme)
	return all, onTheme, err
}

// RepertoireReviewedSince counts the moves of the person's repositories that were answered from
// since on (a move answered several times counts once: only the last answer is kept).
func (s *Store) RepertoireReviewedSince(ctx context.Context, userID string, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rep_moves m JOIN repertoires r ON r.id = m.repertoire_id
		WHERE r.user_id = ? AND m.mine = 1 AND m.reviewed_at >= ?`, userID, since.UTC()).Scan(&n)
	return n, err
}

// ReportRow is a finished review with its full report.
type ReportRow struct {
	ID        string
	Game      []byte // JSON, api.GameInfo
	Report    []byte // JSON, analysis.Report
	CreatedAt time.Time
}

// RecentReports returns the person's latest finished reviews with their reports, newest first. The
// reports are large: ask for as few as the page needs.
func (s *Store) RecentReports(ctx context.Context, userID string, limit int) ([]ReportRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, game, report, created_at FROM analyses
		WHERE user_id = ? AND status = 'done' AND report IS NOT NULL ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReportRow
	for rows.Next() {
		var r ReportRow
		if err := rows.Scan(&r.ID, &r.Game, &r.Report, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.CreatedAt = r.CreatedAt.UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

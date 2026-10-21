package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// LessonProgress is how far a person has got in one lesson.
type LessonProgress struct {
	LessonID    string
	Step        int // steps done; the lesson goes on from here
	Mistakes    int // wrong moves, of the furthest run (the fewest, once completed)
	CompletedAt *time.Time
	UpdatedAt   time.Time
}

// Completed says whether the lesson was finished at least once.
func (p LessonProgress) Completed() bool { return p.CompletedAt != nil }

// LessonProgressOf returns a person's progress in every lesson they have started, by lesson id.
func (s *Store) LessonProgressOf(ctx context.Context, userID string) (map[string]LessonProgress, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT lesson_id, step_index, mistakes, completed_at, updated_at
		FROM lesson_progress WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]LessonProgress{}
	for rows.Next() {
		var p LessonProgress
		var done sql.NullTime
		if err := rows.Scan(&p.LessonID, &p.Step, &p.Mistakes, &done, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if done.Valid {
			t := done.Time
			p.CompletedAt = &t
		}
		out[p.LessonID] = p
	}
	return out, rows.Err()
}

// SaveLessonProgress records that a person has done `step` steps of a lesson with `mistakes` wrong
// moves, and finished it when done is set. Progress only moves forward: a shorter run (a person who
// replays a lesson) leaves the step as it was, and a lesson once completed stays completed. The
// mistakes follow the furthest run, and after a completion they only fall (the best run counts).
// It returns the stored progress.
func (s *Store) SaveLessonProgress(ctx context.Context, userID, lessonID string, step, mistakes int, done bool) (LessonProgress, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return LessonProgress{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Truncate(time.Millisecond) // the column keeps milliseconds
	p := LessonProgress{LessonID: lessonID, Step: step, Mistakes: mistakes, UpdatedAt: now}
	if done {
		p.CompletedAt = &now
	}
	var old LessonProgress
	var oldDone sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT step_index, mistakes, completed_at FROM lesson_progress
		WHERE user_id = ? AND lesson_id = ? FOR UPDATE`, userID, lessonID).Scan(&old.Step, &old.Mistakes, &oldDone)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `INSERT INTO lesson_progress (user_id, lesson_id, step_index, mistakes, completed_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`, userID, lessonID, p.Step, p.Mistakes, p.CompletedAt, now); err != nil {
			return LessonProgress{}, err
		}
	case err != nil:
		return LessonProgress{}, err
	default:
		if oldDone.Valid {
			t := oldDone.Time
			old.CompletedAt = &t
			p.CompletedAt = &t // the first completion stays
		}
		switch {
		case old.CompletedAt != nil:
			if done {
				p.Mistakes = min(old.Mistakes, mistakes)
			} else {
				p.Mistakes = old.Mistakes
			}
		case step >= old.Step:
			p.Mistakes = mistakes
		default:
			p.Mistakes = old.Mistakes
		}
		p.Step = max(old.Step, step)
		if _, err := tx.ExecContext(ctx, `UPDATE lesson_progress SET step_index = ?, mistakes = ?, completed_at = ?, updated_at = ?
			WHERE user_id = ? AND lesson_id = ?`, p.Step, p.Mistakes, p.CompletedAt, now, userID, lessonID); err != nil {
			return LessonProgress{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return LessonProgress{}, err
	}
	return p, nil
}

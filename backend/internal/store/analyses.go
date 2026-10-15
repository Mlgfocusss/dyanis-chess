package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Analysis is a finished review as stored: the JSON documents are kept
// opaque here, their shape belongs to internal/api and internal/analysis.
type Analysis struct {
	ID         string
	UserID     string // owner; empty = anonymous
	Status     string // "done" or "error"
	Depth      int
	Game       []byte // JSON
	Report     []byte // JSON; nil when Status is "error"
	Summary    []byte // JSON, a few figures for lists; may be nil
	Error      string
	CreatedAt  time.Time
	FinishedAt time.Time
}

// nullIfEmpty maps "" to SQL NULL.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// SaveAnalysis inserts or replaces a review.
func (s *Store) SaveAnalysis(ctx context.Context, a *Analysis) error {
	var report, summary any
	if a.Report != nil {
		report = string(a.Report)
	}
	if a.Summary != nil {
		summary = string(a.Summary)
	}
	_, err := s.db.ExecContext(ctx, `REPLACE INTO analyses
		(id, user_id, status, depth, game, report, summary, error, created_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, nullIfEmpty(a.UserID), a.Status, a.Depth, string(a.Game), report, summary,
		nullIfEmpty(a.Error), a.CreatedAt.UTC(), a.FinishedAt.UTC())
	return err
}

// LoadAnalysis returns the review with this id, or ErrNotFound.
func (s *Store) LoadAnalysis(ctx context.Context, id string) (*Analysis, error) {
	var a Analysis
	var game, report, summary []byte
	var errText, owner sql.NullString
	var finished sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT id, user_id, status, depth, game, report, summary, error, created_at, finished_at
		FROM analyses WHERE id = ?`, id).
		Scan(&a.ID, &owner, &a.Status, &a.Depth, &game, &report, &summary, &errText, &a.CreatedAt, &finished)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.UserID, a.Game, a.Report, a.Summary, a.Error = owner.String, game, report, summary, errText.String
	a.FinishedAt = finished.Time
	return &a, nil
}

// ListAnalyses returns the user's reviews, newest first, without the
// (large) report documents.
func (s *Store) ListAnalyses(ctx context.Context, userID string, limit int) ([]*Analysis, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, status, depth, game, summary, created_at
		FROM analyses WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Analysis
	for rows.Next() {
		a := &Analysis{UserID: userID}
		var game, summary []byte
		if err := rows.Scan(&a.ID, &a.Status, &a.Depth, &game, &summary, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Game, a.Summary = game, summary
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAnalysis removes a review owned by userID; ErrNotFound if there is
// no such review of theirs.
func (s *Store) DeleteAnalysis(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM analyses WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

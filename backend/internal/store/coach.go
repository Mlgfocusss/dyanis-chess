package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// LoadCoachNote returns the explanation saved under this hash.
func (s *Store) LoadCoachNote(ctx context.Context, hash string) (string, bool, error) {
	var text string
	err := s.db.QueryRowContext(ctx, `SELECT text FROM coach_notes WHERE hash = ?`, hash).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return text, err == nil, err
}

// SaveCoachNote keeps an explanation; saving the same hash again changes nothing.
func (s *Store) SaveCoachNote(ctx context.Context, hash, model, text string) error {
	_, err := s.db.ExecContext(ctx, `INSERT IGNORE INTO coach_notes (hash, model, text, created_at) VALUES (?, ?, ?, ?)`,
		hash, model, text, time.Now().UTC())
	return err
}

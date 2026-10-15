package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Game is an online game as stored: the columns that lists need and the
// JSON document with everything else (its shape belongs to internal/play).
type Game struct {
	ID        string
	WhiteID   string // "" while nobody sits there
	BlackID   string
	Status    string
	Result    string
	State     []byte
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SaveGame inserts or updates a game.
func (s *Store) SaveGame(ctx context.Context, g *Game) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO games
		(id, white_id, black_id, status, result, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE white_id = VALUES(white_id), black_id = VALUES(black_id),
			status = VALUES(status), result = VALUES(result), state = VALUES(state), updated_at = VALUES(updated_at)`,
		g.ID, nullIfEmpty(g.WhiteID), nullIfEmpty(g.BlackID), g.Status, nullIfEmpty(g.Result), string(g.State),
		g.CreatedAt.UTC(), g.UpdatedAt.UTC())
	return err
}

func scanGame(r rowScanner) (*Game, error) {
	var g Game
	var white, black, result sql.NullString
	var state []byte
	if err := r.Scan(&g.ID, &white, &black, &g.Status, &result, &state, &g.CreatedAt, &g.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	g.WhiteID, g.BlackID, g.Result, g.State = white.String, black.String, result.String, state
	return &g, nil
}

const gameColumns = "id, white_id, black_id, status, result, state, created_at, updated_at"

// LoadGame returns a game or ErrNotFound.
func (s *Store) LoadGame(ctx context.Context, id string) (*Game, error) {
	return scanGame(s.db.QueryRowContext(ctx, "SELECT "+gameColumns+" FROM games WHERE id = ?", id))
}

// ListGames returns the games a user took part in that have begun, newest
// first (a game nobody joined is not a game yet).
func (s *Store) ListGames(ctx context.Context, userID string, limit int) ([]*Game, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+gameColumns+` FROM games
		WHERE (white_id = ? OR black_id = ?) AND status IN ('playing', 'ended')
		ORDER BY updated_at DESC LIMIT ?`, userID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Game
	for rows.Next() {
		g, err := scanGame(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

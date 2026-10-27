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
	// Correspondence games (days per move): who is to move and until when. Written by SaveGame,
	// not read back with the game (the state document has it all).
	Corr   bool
	TurnID string // the person to move, "" for a bot or nobody
	DueAt  time.Time
}

// SaveGame inserts or updates a game.
func (s *Store) SaveGame(ctx context.Context, g *Game) error {
	var due any
	if !g.DueAt.IsZero() {
		due = g.DueAt.UTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO games
		(id, white_id, black_id, status, result, state, created_at, updated_at, corr, turn_id, due_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE white_id = VALUES(white_id), black_id = VALUES(black_id),
			status = VALUES(status), result = VALUES(result), state = VALUES(state), updated_at = VALUES(updated_at),
			corr = VALUES(corr), turn_id = VALUES(turn_id), due_at = VALUES(due_at)`,
		g.ID, nullIfEmpty(g.WhiteID), nullIfEmpty(g.BlackID), g.Status, nullIfEmpty(g.Result), string(g.State),
		g.CreatedAt.UTC(), g.UpdatedAt.UTC(), g.Corr, nullIfEmpty(g.TurnID), due)
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

// CorrespondenceGames returns the unfinished correspondence games of a user, the nearest deadline first.
func (s *Store) CorrespondenceGames(ctx context.Context, userID string) ([]*Game, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+gameColumns+` FROM games
		WHERE corr = 1 AND status = 'playing' AND (white_id = ? OR black_id = ?) ORDER BY due_at`, userID, userID)
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

// CountMyTurn counts the correspondence games in which the user is to move.
func (s *Store) CountMyTurn(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM games WHERE corr = 1 AND status = 'playing' AND turn_id = ?", userID).Scan(&n)
	return n, err
}

// ActiveCorrespondenceIDs lists every unfinished correspondence game, to bring them back into memory
// (and their deadlines back to life) when the server starts.
func (s *Store) ActiveCorrespondenceIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM games WHERE corr = 1 AND status = 'playing'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

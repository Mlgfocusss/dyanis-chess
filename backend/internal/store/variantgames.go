package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// VariantGame is a game of a chess variant as stored: the columns that lists
// need and the JSON document with everything else (its shape belongs to
// internal/vplay).
type VariantGame struct {
	ID      string
	Variant string
	WhiteID string // "" while nobody sits there
	BlackID string
	// ThirdID and FourthID are the seats of board 2 (White, Black) in a bughouse game, "" for every other variant.
	ThirdID   string
	FourthID  string
	Status    string
	Result    string
	State     []byte
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SaveVariantGame inserts or updates a game.
func (s *Store) SaveVariantGame(ctx context.Context, g *VariantGame) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO variant_games
		(id, variant, white_id, black_id, third_id, fourth_id, status, result, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE white_id = VALUES(white_id), black_id = VALUES(black_id),
			third_id = VALUES(third_id), fourth_id = VALUES(fourth_id),
			status = VALUES(status), result = VALUES(result), state = VALUES(state), updated_at = VALUES(updated_at)`,
		g.ID, g.Variant, nullIfEmpty(g.WhiteID), nullIfEmpty(g.BlackID), nullIfEmpty(g.ThirdID), nullIfEmpty(g.FourthID), g.Status, nullIfEmpty(g.Result), string(g.State),
		g.CreatedAt.UTC(), g.UpdatedAt.UTC())
	return err
}

const variantGameColumns = "id, variant, white_id, black_id, third_id, fourth_id, status, result, state, created_at, updated_at"

func scanVariantGame(r rowScanner) (*VariantGame, error) {
	var g VariantGame
	var white, black, third, fourth, result sql.NullString
	var state []byte
	if err := r.Scan(&g.ID, &g.Variant, &white, &black, &third, &fourth, &g.Status, &result, &state, &g.CreatedAt, &g.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	g.WhiteID, g.BlackID, g.ThirdID, g.FourthID = white.String, black.String, third.String, fourth.String
	g.Result, g.State = result.String, state
	return &g, nil
}

// LoadVariantGame returns a game or ErrNotFound.
func (s *Store) LoadVariantGame(ctx context.Context, id string) (*VariantGame, error) {
	return scanVariantGame(s.db.QueryRowContext(ctx, "SELECT "+variantGameColumns+" FROM variant_games WHERE id = ?", id))
}

// ListVariantGames returns the games a user took part in that have begun,
// newest first.
func (s *Store) ListVariantGames(ctx context.Context, userID string, limit int) ([]*VariantGame, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+variantGameColumns+` FROM variant_games
		WHERE (white_id = ? OR black_id = ? OR third_id = ? OR fourth_id = ?) AND status IN ('playing', 'ended')
		ORDER BY updated_at DESC LIMIT ?`, userID, userID, userID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*VariantGame
	for rows.Next() {
		g, err := scanVariantGame(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

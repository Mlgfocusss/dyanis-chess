package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// RatingRow is a person's Glicko-2 rating in one category (see internal/ratings).
type RatingRow struct {
	UserID     string
	Category   string
	Rating     float64
	RD         float64
	Volatility float64
	Games      int
	Wins       int
	Draws      int
	Losses     int
	UpdatedAt  time.Time
}

const ratingColumns = "user_id, category, rating, rd, volatility, games, wins, draws, losses, updated_at"

func scanRating(r rowScanner) (*RatingRow, error) {
	var x RatingRow
	err := r.Scan(&x.UserID, &x.Category, &x.Rating, &x.RD, &x.Volatility, &x.Games, &x.Wins, &x.Draws, &x.Losses, &x.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	x.UpdatedAt = x.UpdatedAt.UTC()
	return &x, nil
}

// Rating returns the person's rating in a category; ErrNotFound when they have not played it.
func (s *Store) Rating(ctx context.Context, userID, category string) (*RatingRow, error) {
	return scanRating(s.db.QueryRowContext(ctx, "SELECT "+ratingColumns+" FROM ratings WHERE user_id = ? AND category = ?", userID, category))
}

// Ratings returns every rating of the person.
func (s *Store) Ratings(ctx context.Context, userID string) ([]*RatingRow, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+ratingColumns+" FROM ratings WHERE user_id = ? ORDER BY category", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*RatingRow
	for rows.Next() {
		x, err := scanRating(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// SaveRating writes a rating, replacing the person's earlier one in the category.
func (s *Store) SaveRating(ctx context.Context, x *RatingRow) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO ratings (`+ratingColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE rating = VALUES(rating), rd = VALUES(rd), volatility = VALUES(volatility),
		games = VALUES(games), wins = VALUES(wins), draws = VALUES(draws), losses = VALUES(losses), updated_at = VALUES(updated_at)`,
		x.UserID, x.Category, x.Rating, x.RD, x.Volatility, x.Games, x.Wins, x.Draws, x.Losses, x.UpdatedAt.UTC())
	return err
}

// DuelRow is a finished puzzle duel.
type DuelRow struct {
	ID                   string
	AID, BID             string
	AScore, BScore       int
	AMistakes, BMistakes int
	Result               string // "a", "b" or "draw"
	CreatedAt            time.Time
}

// SaveDuel records a finished duel.
func (s *Store) SaveDuel(ctx context.Context, d *DuelRow) error {
	_, err := s.db.ExecContext(ctx, `INSERT IGNORE INTO duels (id, a_id, b_id, a_score, b_score, a_mistakes, b_mistakes, result, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, d.ID, d.AID, d.BID, d.AScore, d.BScore, d.AMistakes, d.BMistakes, d.Result, d.CreatedAt.UTC())
	return err
}

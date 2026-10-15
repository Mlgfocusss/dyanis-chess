package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// BotRecord is a person's record against one bot.
type BotRecord struct {
	BotID      string
	Wins       int
	Draws      int
	Losses     int
	Points     int
	FirstWinAt time.Time
}

// RecordBotResult adds one finished game against a bot (outcome "win",
// "draw" or "loss"). The first win earns points (the given amount, once);
// firstWin reports it.
func (s *Store) RecordBotResult(ctx context.Context, userID, botID, outcome string, points int) (firstWin bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()

	var wins, draws, losses, earned int
	err = tx.QueryRowContext(ctx, "SELECT wins, draws, losses, points FROM bot_progress WHERE user_id = ? AND bot_id = ? FOR UPDATE",
		userID, botID).Scan(&wins, &draws, &losses, &earned)
	exists := !errors.Is(err, sql.ErrNoRows)
	if err != nil && exists {
		return false, err
	}
	switch outcome {
	case "win":
		firstWin = wins == 0
		wins++
	case "draw":
		draws++
	default:
		losses++
	}
	if firstWin {
		earned = points
	}
	if !exists {
		_, err = tx.ExecContext(ctx, `INSERT INTO bot_progress (user_id, bot_id, wins, draws, losses, points, first_win_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, userID, botID, wins, draws, losses, earned, nullTime(firstWin, now), now)
	} else if firstWin {
		_, err = tx.ExecContext(ctx, `UPDATE bot_progress SET wins = ?, draws = ?, losses = ?, points = ?, first_win_at = ?, updated_at = ?
			WHERE user_id = ? AND bot_id = ?`, wins, draws, losses, earned, now, now, userID, botID)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE bot_progress SET wins = ?, draws = ?, losses = ?, updated_at = ?
			WHERE user_id = ? AND bot_id = ?`, wins, draws, losses, now, userID, botID)
	}
	if err != nil {
		return false, err
	}
	return firstWin, tx.Commit()
}

func nullTime(ok bool, t time.Time) any {
	if !ok {
		return nil
	}
	return t
}

// BotProgress returns the person's record against every bot they have played.
func (s *Store) BotProgress(ctx context.Context, userID string) ([]BotRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT bot_id, wins, draws, losses, points, first_win_at
		FROM bot_progress WHERE user_id = ? ORDER BY bot_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BotRecord
	for rows.Next() {
		var r BotRecord
		var first sql.NullTime
		if err := rows.Scan(&r.BotID, &r.Wins, &r.Draws, &r.Losses, &r.Points, &first); err != nil {
			return nil, err
		}
		r.FirstWinAt = first.Time
		out = append(out, r)
	}
	return out, rows.Err()
}

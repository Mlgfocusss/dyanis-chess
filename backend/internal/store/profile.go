package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ErrActiveGame is returned by DeleteUser while the user is in the middle of a
// game against another person: deleting the account would end it for them.
var ErrActiveGame = errors.New("store: the user has an unfinished game")

// ActiveGameWindow is how recently a game with a person must have been touched
// to count as unfinished. A game left "playing" for longer is a leftover (the
// server stopped before the clock could end it) and does not block anything.
const ActiveGameWindow = 24 * time.Hour

// SetName changes the display name of a user (ErrNotFound if there is none).
func (s *Store) SetName(ctx context.Context, id, name string) error {
	res, err := s.db.ExecContext(ctx, "UPDATE users SET name = ? WHERE id = ?", name, id)
	if err != nil {
		return err
	}
	// MySQL reports 0 affected rows when the name is unchanged, so look again.
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		if _, err := s.UserByID(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// ProfileUpdate holds the profile fields to change; a nil field is left alone. An empty string
// clears a field (stored as NULL), except Name, which must not be empty. The caller validates.
type ProfileUpdate struct {
	Name      *string
	FirstName *string
	LastName  *string
	Location  *string
	About     *string
	Country   *string // ISO 3166-1 alpha-2
	AvatarURL *string // "", "preset:<id>" or a photo address
}

// UpdateProfile changes the given fields in one statement (ErrNotFound if there is no such user).
func (s *Store) UpdateProfile(ctx context.Context, id string, p ProfileUpdate) error {
	var set []string
	var args []any
	add := func(col string, v *string, nullable bool) {
		if v == nil {
			return
		}
		set = append(set, col+" = ?")
		if nullable {
			args = append(args, nullIfEmpty(*v))
		} else {
			args = append(args, *v)
		}
	}
	add("name", p.Name, false)
	add("first_name", p.FirstName, true)
	add("last_name", p.LastName, true)
	add("location", p.Location, true)
	add("about", p.About, true)
	add("country", p.Country, true)
	add("avatar_url", p.AvatarURL, true)
	if len(set) == 0 {
		_, err := s.UserByID(ctx, id)
		return err
	}
	args = append(args, id)
	res, err := s.db.ExecContext(ctx, "UPDATE users SET "+strings.Join(set, ", ")+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 { // unchanged values also report 0
		if _, err := s.UserByID(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// DeleteUser removes a user and what is theirs. The foreign keys do most of
// it: sessions, bot progress, puzzle ratings, attempts, rush and daily results,
// ratings and repertoires (with their moves) go with the user (ON DELETE
// CASCADE), while reviews, and games and duels with another person, stay and
// lose only the owner (ON DELETE SET NULL), so the opponent keeps their history.
// What would be left with nobody on either side (games against a bot, open
// invitations, duels whose other side is already gone) is deleted here.
//
// A game with another person that is still going (a correspondence game, however long it has
// waited) gives ErrActiveGame and nothing is deleted.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// FOR UPDATE on the user row makes two deletions of one account wait for each other.
	var one int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM users WHERE id = ? FOR UPDATE", id).Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM games
		WHERE status = 'playing' AND white_id IS NOT NULL AND black_id IS NOT NULL AND (white_id = ? OR black_id = ?)
		AND (updated_at > ? OR corr = 1)`, id, id, time.Now().Add(-ActiveGameWindow).UTC()).Scan(&active); err != nil {
		return err
	}
	var activeVariants int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM variant_games
		WHERE status = 'playing' AND white_id IS NOT NULL AND black_id IS NOT NULL
		AND (white_id = ? OR black_id = ? OR third_id = ? OR fourth_id = ?)
		AND updated_at > ?`, id, id, id, id, time.Now().Add(-ActiveGameWindow).UTC()).Scan(&activeVariants); err != nil {
		return err
	}
	if active > 0 || activeVariants > 0 {
		return ErrActiveGame
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM games
		WHERE (white_id = ? AND black_id IS NULL) OR (black_id = ? AND white_id IS NULL)`, id, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM variant_games
		WHERE third_id IS NULL AND fourth_id IS NULL
		AND ((white_id = ? AND black_id IS NULL) OR (black_id = ? AND white_id IS NULL))`, id, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM duels
		WHERE (a_id = ? AND b_id IS NULL) OR (b_id = ? AND a_id IS NULL)`, id, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

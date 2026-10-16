package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// User kinds.
const (
	KindGuest  = "guest"
	KindGoogle = "google"
)

// User is an account. Settings is the settings page's JSON, opaque here.
type User struct {
	ID         string
	Kind       string
	GoogleSub  string
	Name       string
	Email      string
	AvatarURL  string
	Settings   []byte // JSON object or nil
	CreatedAt  time.Time
	LastSeenAt time.Time
}

const userColumns = "id, kind, google_sub, name, email, avatar_url, settings, created_at, last_seen_at"

type rowScanner interface{ Scan(dest ...any) error }

func scanUser(r rowScanner) (*User, error) {
	var u User
	var sub, email, avatar sql.NullString
	var settings []byte
	if err := r.Scan(&u.ID, &u.Kind, &sub, &u.Name, &email, &avatar, &settings, &u.CreatedAt, &u.LastSeenAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.GoogleSub, u.Email, u.AvatarURL, u.Settings = sub.String, email.String, avatar.String, settings
	return &u, nil
}

// CreateUser inserts a user; CreatedAt and LastSeenAt are set to now.
func (s *Store) CreateUser(ctx context.Context, u *User) error {
	now := time.Now().UTC()
	u.CreatedAt, u.LastSeenAt = now, now
	var settings any
	if u.Settings != nil {
		settings = string(u.Settings)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO users
		(id, kind, google_sub, name, email, avatar_url, settings, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Kind, nullIfEmpty(u.GoogleSub), u.Name, nullIfEmpty(u.Email), nullIfEmpty(u.AvatarURL),
		settings, now, now)
	return err
}

// UserByID returns the user or ErrNotFound.
func (s *Store) UserByID(ctx context.Context, id string) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE id = ?", id))
}

// UserByGoogleSub returns the user signed in with this Google account, or
// ErrNotFound.
func (s *Store) UserByGoogleSub(ctx context.Context, sub string) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE google_sub = ?", sub))
}

// LinkGoogle turns a guest into a Google user in place (games and settings
// stay), or refreshes the profile of an existing Google user.
func (s *Store) LinkGoogle(ctx context.Context, id, sub, name, email, avatarURL string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users
		SET kind = ?, google_sub = ?, name = ?, email = ?, avatar_url = ? WHERE id = ?`,
		KindGoogle, sub, name, nullIfEmpty(email), nullIfEmpty(avatarURL), id)
	return err
}

// SetSettings replaces the user's settings document.
func (s *Store) SetSettings(ctx context.Context, id string, settings []byte) error {
	_, err := s.db.ExecContext(ctx, "UPDATE users SET settings = ? WHERE id = ?", string(settings), id)
	return err
}

// MergeUsers moves the reviews of one user to another and deletes the first
// (with their sessions). Used when a guest signs in to a Google account
// that already exists.
func (s *Store) MergeUsers(ctx context.Context, from, to string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE analyses SET user_id = ? WHERE user_id = ?", to, from); err != nil {
		return err
	}
	for _, col := range []string{"white_id", "black_id"} {
		if _, err := tx.ExecContext(ctx, "UPDATE games SET "+col+" = ? WHERE "+col+" = ?", to, from); err != nil {
			return err
		}
	}
	// Progress against bots is combined; a bot beaten on either account stays
	// beaten, and its points are counted once.
	if _, err := tx.ExecContext(ctx, `INSERT INTO bot_progress (user_id, bot_id, wins, draws, losses, points, first_win_at, updated_at)
		SELECT ?, src.bot_id, src.wins, src.draws, src.losses, src.points, src.first_win_at, src.updated_at
		FROM (SELECT bot_id, wins, draws, losses, points, first_win_at, updated_at FROM bot_progress WHERE user_id = ?) AS src
		ON DUPLICATE KEY UPDATE wins = bot_progress.wins + src.wins, draws = bot_progress.draws + src.draws,
			losses = bot_progress.losses + src.losses, points = GREATEST(bot_progress.points, src.points),
			first_win_at = COALESCE(LEAST(bot_progress.first_win_at, src.first_win_at), bot_progress.first_win_at, src.first_win_at),
			updated_at = src.updated_at`, to, from); err != nil {
		return err
	}
	// Puzzle attempts all move over. The rating of the account with more
	// attempts stays; the counts are added up.
	if _, err := tx.ExecContext(ctx, "UPDATE puzzle_attempts SET user_id = ? WHERE user_id = ?", to, from); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE puzzle_ratings t JOIN puzzle_ratings f ON f.user_id = ? AND t.user_id = ?
		SET t.rating = IF(f.attempts > t.attempts, f.rating, t.rating), t.attempts = t.attempts + f.attempts,
			t.solved = t.solved + f.solved, t.updated_at = GREATEST(t.updated_at, f.updated_at)`, from, to); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE IGNORE puzzle_ratings SET user_id = ? WHERE user_id = ?", to, from); err != nil {
		return err
	}
	// Results of the puzzle of the day: on a day both accounts played, the target's stays.
	if _, err := tx.ExecContext(ctx, "UPDATE IGNORE daily_puzzle_results SET user_id = ? WHERE user_id = ?", to, from); err != nil {
		return err
	}
	// Repertoires (with their moves) move over as they are.
	if _, err := tx.ExecContext(ctx, "UPDATE repertoires SET user_id = ? WHERE user_id = ?", to, from); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = ?", from); err != nil {
		return err
	}
	return tx.Commit()
}

// CreateSession records a session; tokenHash is the hex SHA-256 of the
// token given to the browser.
func (s *Store) CreateSession(ctx context.Context, tokenHash, userID string, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, created_at, expires_at)
		VALUES (?, ?, ?, ?)`, tokenHash, userID, time.Now().UTC(), expires.UTC())
	return err
}

// SessionUser returns the owner of a live session, or ErrNotFound when the
// session is unknown or expired. It also records that the user was seen.
func (s *Store) SessionUser(ctx context.Context, tokenHash string) (*User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT u.id, u.kind, u.google_sub, u.name, u.email, u.avatar_url,
		u.settings, u.created_at, u.last_seen_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`, tokenHash, time.Now().UTC()))
	if err != nil {
		return nil, err
	}
	// Once a day is enough for "last seen"; this runs on every request.
	if time.Since(u.LastSeenAt) > 24*time.Hour {
		_, _ = s.db.ExecContext(ctx, "UPDATE users SET last_seen_at = ? WHERE id = ?", time.Now().UTC(), u.ID)
	}
	return u, nil
}

// DeleteSession ends a session (no error if it does not exist).
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", tokenHash)
	return err
}

// PurgeExpiredSessions removes sessions past their expiry.
func (s *Store) PurgeExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", time.Now().UTC())
	return err
}

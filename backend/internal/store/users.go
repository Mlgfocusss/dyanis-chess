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
//
// AvatarURL is what the person shows: "" (the letter of the name), "preset:<id>" (one of the ready-made
// avatars) or a Google photo address. GoogleAvatarURL is the last photo Google sent, kept apart so that a
// later sign-in never replaces a choice. The profile fields are public and all optional ("" = not set);
// Country is an ISO 3166-1 alpha-2 code checked by the caller against internal/countries.
type User struct {
	ID              string
	Kind            string
	GoogleSub       string
	Name            string
	Email           string
	AvatarURL       string
	GoogleAvatarURL string
	FirstName       string
	LastName        string
	Location        string
	About           string
	Country         string
	Settings        []byte // JSON object or nil
	CreatedAt       time.Time
	LastSeenAt      time.Time
}

const userColumns = "id, kind, google_sub, name, email, avatar_url, google_avatar_url, first_name, last_name, location, about, country, settings, created_at, last_seen_at"

type rowScanner interface{ Scan(dest ...any) error }

func scanUser(r rowScanner) (*User, error) {
	var u User
	var sub, email, avatar, gavatar, first, last, loc, about, country sql.NullString
	var settings []byte
	if err := r.Scan(&u.ID, &u.Kind, &sub, &u.Name, &email, &avatar, &gavatar, &first, &last, &loc, &about, &country,
		&settings, &u.CreatedAt, &u.LastSeenAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.GoogleSub, u.Email, u.AvatarURL, u.Settings = sub.String, email.String, avatar.String, settings
	u.GoogleAvatarURL, u.FirstName, u.LastName, u.Location, u.About, u.Country =
		gavatar.String, first.String, last.String, loc.String, about.String, country.String
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
		(id, kind, google_sub, name, email, avatar_url, google_avatar_url, first_name, last_name, location, about, country,
		 settings, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Kind, nullIfEmpty(u.GoogleSub), u.Name, nullIfEmpty(u.Email), nullIfEmpty(u.AvatarURL),
		nullIfEmpty(u.GoogleAvatarURL), nullIfEmpty(u.FirstName), nullIfEmpty(u.LastName), nullIfEmpty(u.Location),
		nullIfEmpty(u.About), nullIfEmpty(u.Country), settings, now, now)
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
// stay), or refreshes the profile of an existing Google user. photo is the
// picture Google sent now; it always becomes google_avatar_url, but the shown
// avatar follows it only for a person who was showing the photo (or had chosen
// nothing yet): a ready-made avatar or the letter, once chosen, stays.
func (s *Store) LinkGoogle(ctx context.Context, id, sub, name, email, photo string) error {
	// MySQL assigns left to right, so avatar_url still sees the old google_avatar_url.
	_, err := s.db.ExecContext(ctx, `UPDATE users
		SET kind = ?, google_sub = ?, name = ?, email = ?,
			avatar_url = IF(avatar_url <=> google_avatar_url, ?, avatar_url), google_avatar_url = ?
		WHERE id = ?`,
		KindGoogle, sub, name, nullIfEmpty(email), nullIfEmpty(photo), nullIfEmpty(photo), id)
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
		if _, err := tx.ExecContext(ctx, "UPDATE variant_games SET "+col+" = ? WHERE "+col+" = ?", to, from); err != nil {
			return err
		}
	}
	for _, col := range []string{"third_id", "fourth_id"} { // the seats of board 2 in bughouse
		if _, err := tx.ExecContext(ctx, "UPDATE variant_games SET "+col+" = ? WHERE "+col+" = ?", to, from); err != nil {
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
	// Puzzle points are added up, the better series stays; the daily sums are added per day.
	if _, err := tx.ExecContext(ctx, `INSERT INTO puzzle_points (user_id, points, streak, best_streak, updated_at)
		SELECT ?, src.points, src.streak, src.best_streak, src.updated_at FROM (SELECT points, streak, best_streak, updated_at FROM puzzle_points WHERE user_id = ?) AS src
		ON DUPLICATE KEY UPDATE points = puzzle_points.points + src.points, streak = GREATEST(puzzle_points.streak, src.streak),
			best_streak = GREATEST(puzzle_points.best_streak, src.best_streak), updated_at = GREATEST(puzzle_points.updated_at, src.updated_at)`, to, from); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO puzzle_points_days (user_id, day, points)
		SELECT ?, src.day, src.points FROM (SELECT day, points FROM puzzle_points_days WHERE user_id = ?) AS src
		ON DUPLICATE KEY UPDATE points = puzzle_points_days.points + src.points`, to, from); err != nil {
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
	// Lesson progress is combined: a lesson done on either account stays done (with the fewer
	// mistakes), otherwise the further run counts. The rows of the deleted account go with it.
	if _, err := tx.ExecContext(ctx, `INSERT INTO lesson_progress (user_id, lesson_id, step_index, mistakes, completed_at, updated_at)
		SELECT ?, src.lesson_id, src.step_index, src.mistakes, src.completed_at, src.updated_at
		FROM (SELECT lesson_id, step_index, mistakes, completed_at, updated_at FROM lesson_progress WHERE user_id = ?) AS src
		ON DUPLICATE KEY UPDATE
			mistakes = CASE
				WHEN lesson_progress.completed_at IS NOT NULL AND src.completed_at IS NOT NULL THEN LEAST(lesson_progress.mistakes, src.mistakes)
				WHEN lesson_progress.completed_at IS NOT NULL THEN lesson_progress.mistakes
				WHEN src.completed_at IS NOT NULL THEN src.mistakes
				WHEN src.step_index > lesson_progress.step_index THEN src.mistakes
				ELSE lesson_progress.mistakes END,
			step_index = GREATEST(lesson_progress.step_index, src.step_index),
			completed_at = COALESCE(LEAST(lesson_progress.completed_at, src.completed_at), lesson_progress.completed_at, src.completed_at),
			updated_at = GREATEST(lesson_progress.updated_at, src.updated_at)`, to, from); err != nil {
		return err
	}
	// Correspondence games waiting for the guest's move now wait for the account.
	if _, err := tx.ExecContext(ctx, "UPDATE games SET turn_id = ? WHERE turn_id = ?", to, from); err != nil {
		return err
	}
	// Friends, requests and the talks move over; what would join the account to itself, or repeat a
	// pair it already has, is dropped with the deleted account.
	for _, q := range []string{
		"INSERT IGNORE INTO friends (user_id, friend_id, mute_challenges, created_at) SELECT ?, friend_id, mute_challenges, created_at FROM friends WHERE user_id = ? AND friend_id <> ?",
		"INSERT IGNORE INTO friends (user_id, friend_id, mute_challenges, created_at) SELECT user_id, ?, mute_challenges, created_at FROM friends WHERE friend_id = ? AND user_id <> ?",
		"INSERT IGNORE INTO friend_requests (from_id, to_id, created_at) SELECT ?, to_id, created_at FROM friend_requests WHERE from_id = ? AND to_id <> ?",
		"INSERT IGNORE INTO friend_requests (from_id, to_id, created_at) SELECT from_id, ?, created_at FROM friend_requests WHERE to_id = ? AND from_id <> ?",
		"UPDATE messages SET from_id = ? WHERE from_id = ? AND to_id <> ?",
		"UPDATE messages SET to_id = ? WHERE to_id = ? AND from_id <> ?",
	} {
		if _, err := tx.ExecContext(ctx, q, to, from, to); err != nil {
			return err
		}
	}
	// What the guest wrote on the forum stays and is the account's.
	for _, q := range []string{"UPDATE forum_topics SET author_id = ? WHERE author_id = ?", "UPDATE forum_posts SET author_id = ? WHERE author_id = ?"} {
		if _, err := tx.ExecContext(ctx, q, to, from); err != nil {
			return err
		}
	}
	// Achievements are combined: a badge earned on either account is the account's, with the earlier date.
	if _, err := tx.ExecContext(ctx, `INSERT INTO achievements (user_id, ach_key, unlocked_at, seen)
		SELECT ?, src.ach_key, src.unlocked_at, src.seen FROM (SELECT ach_key, unlocked_at, seen FROM achievements WHERE user_id = ?) AS src
		ON DUPLICATE KEY UPDATE unlocked_at = LEAST(achievements.unlocked_at, src.unlocked_at), seen = GREATEST(achievements.seen, src.seen)`, to, from); err != nil {
		return err
	}
	// Blocks move over like friends do (never a block of the account on itself); a friendship or a
	// request that the move has put next to a block is dropped. Reports the guest made or was
	// the subject of go to the account too (a duplicate of an open one stays unmoved and loses the guest).
	for _, q := range []string{
		"INSERT IGNORE INTO blocks (blocker_id, blocked_id, created_at) SELECT ?, blocked_id, created_at FROM blocks WHERE blocker_id = ? AND blocked_id <> ?",
		"INSERT IGNORE INTO blocks (blocker_id, blocked_id, created_at) SELECT blocker_id, ?, created_at FROM blocks WHERE blocked_id = ? AND blocker_id <> ?",
	} {
		if _, err := tx.ExecContext(ctx, q, to, from, to); err != nil {
			return err
		}
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"DELETE f FROM friends f JOIN blocks b ON (b.blocker_id = f.user_id AND b.blocked_id = f.friend_id) OR (b.blocker_id = f.friend_id AND b.blocked_id = f.user_id) WHERE f.user_id = ? OR f.friend_id = ?", []any{to, to}},
		{"DELETE f FROM friend_requests f JOIN blocks b ON (b.blocker_id = f.from_id AND b.blocked_id = f.to_id) OR (b.blocker_id = f.to_id AND b.blocked_id = f.from_id) WHERE f.from_id = ? OR f.to_id = ?", []any{to, to}},
		{"UPDATE IGNORE reports SET reporter_id = ? WHERE reporter_id = ?", []any{to, from}},
		{"UPDATE reports SET target_user_id = ? WHERE target_user_id = ?", []any{to, from}},
		{"UPDATE reports SET closed_by = ? WHERE closed_by = ?", []any{to, from}},
	} {
		if _, err := tx.ExecContext(ctx, q.sql, q.args...); err != nil {
			return err
		}
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
		u.google_avatar_url, u.first_name, u.last_name, u.location, u.about, u.country,
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

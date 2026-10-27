package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// The community tables (migration 0020): friends, friend requests, private messages, challenges and
// vacations. The rules (who may write to whom, how many) belong to internal/community; here are the
// reads and writes.

// Errors of the community writes.
var (
	ErrAlreadyFriends = errors.New("store: already friends")
	ErrRequestExists  = errors.New("store: friend request exists")
)

// FriendRow is one side of a friendship.
type FriendRow struct {
	FriendID       string
	MuteChallenges bool // this user does not receive challenges of the friend
	Since          time.Time
}

// Friends returns the friends of a user, the oldest friendship first.
func (s *Store) Friends(ctx context.Context, userID string) ([]FriendRow, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT friend_id, mute_challenges, created_at FROM friends WHERE user_id = ? ORDER BY created_at", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FriendRow
	for rows.Next() {
		var f FriendRow
		if err := rows.Scan(&f.FriendID, &f.MuteChallenges, &f.Since); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// Friend returns the friendship as userID sees it, ErrNotFound when they are not friends.
func (s *Store) Friend(ctx context.Context, userID, friendID string) (*FriendRow, error) {
	var f FriendRow
	err := s.db.QueryRowContext(ctx, "SELECT friend_id, mute_challenges, created_at FROM friends WHERE user_id = ? AND friend_id = ?", userID, friendID).
		Scan(&f.FriendID, &f.MuteChallenges, &f.Since)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// CountFriends returns how many friends a user has.
func (s *Store) CountFriends(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM friends WHERE user_id = ?", userID).Scan(&n)
	return n, err
}

// RequestRow is a pending request to be friends.
type RequestRow struct {
	UserID string // the other side
	At     time.Time
}

// IncomingRequests are the requests waiting for the user's answer, the newest first.
func (s *Store) IncomingRequests(ctx context.Context, userID string) ([]RequestRow, error) {
	return s.requests(ctx, "SELECT from_id, created_at FROM friend_requests WHERE to_id = ? ORDER BY created_at DESC", userID)
}

// OutgoingRequests are the requests the user has sent and nobody has answered.
func (s *Store) OutgoingRequests(ctx context.Context, userID string) ([]RequestRow, error) {
	return s.requests(ctx, "SELECT to_id, created_at FROM friend_requests WHERE from_id = ? ORDER BY created_at DESC", userID)
}

func (s *Store) requests(ctx context.Context, query, userID string) ([]RequestRow, error) {
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RequestRow
	for rows.Next() {
		var r RequestRow
		if err := rows.Scan(&r.UserID, &r.At); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RequestFriend records a request from one user to another. When the other side has already asked,
// the two become friends at once (accepted is true). ErrAlreadyFriends and ErrRequestExists say
// what is in the way.
func (s *Store) RequestFriend(ctx context.Context, from, to string, at time.Time) (accepted bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM friends WHERE user_id = ? AND friend_id = ?", from, to).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return false, ErrAlreadyFriends
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM friend_requests WHERE from_id = ? AND to_id = ?", from, to).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return false, ErrRequestExists
	}
	res, err := tx.ExecContext(ctx, "DELETE FROM friend_requests WHERE from_id = ? AND to_id = ?", to, from)
	if err != nil {
		return false, err
	}
	if k, _ := res.RowsAffected(); k > 0 { // they had asked already: that is a yes
		if err := makeFriends(ctx, tx, from, to, at); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO friend_requests (from_id, to_id, created_at) VALUES (?, ?, ?)", from, to, at.UTC()); err != nil {
		return false, err
	}
	return false, tx.Commit()
}

func makeFriends(ctx context.Context, tx *sql.Tx, a, b string, at time.Time) error {
	for _, p := range [][2]string{{a, b}, {b, a}} {
		if _, err := tx.ExecContext(ctx, "INSERT IGNORE INTO friends (user_id, friend_id, mute_challenges, created_at) VALUES (?, ?, 0, ?)", p[0], p[1], at.UTC()); err != nil {
			return err
		}
	}
	return nil
}

// AcceptFriend answers yes to the request that from sent to userID; ErrNotFound when there is none.
func (s *Store) AcceptFriend(ctx context.Context, userID, from string, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, "DELETE FROM friend_requests WHERE from_id = ? AND to_id = ?", from, userID)
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrNotFound
	}
	if err := makeFriends(ctx, tx, userID, from, at); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteRequest removes the request that from sent to to (the addressee declines it, or the sender
// takes it back); false when there was none.
func (s *Store) DeleteRequest(ctx context.Context, from, to string) (bool, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM friend_requests WHERE from_id = ? AND to_id = ?", from, to)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// RemoveFriend ends a friendship on both sides; false when they were not friends.
func (s *Store) RemoveFriend(ctx context.Context, a, b string) (bool, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM friends WHERE (user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?)", a, b, b, a)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// MuteFriend sets whether userID refuses challenges from the friend; ErrNotFound when not friends.
func (s *Store) MuteFriend(ctx context.Context, userID, friendID string, muted bool) error {
	if _, err := s.Friend(ctx, userID, friendID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "UPDATE friends SET mute_challenges = ? WHERE user_id = ? AND friend_id = ?", muted, userID, friendID)
	return err
}

// SearchUsers finds people whose name starts with q (case-insensitive), as seen by the user exclude:
// not that user, and not the people who have blocked them (a blocked person does not find the one
// who blocked them).
func (s *Store) SearchUsers(ctx context.Context, q, exclude string, limit int) ([]*PublicUser, error) {
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
	rows, err := s.db.QueryContext(ctx, "SELECT "+publicColumns+" FROM users WHERE name LIKE ? AND id <> ? AND id NOT IN (SELECT blocker_id FROM blocks WHERE blocked_id = ?) ORDER BY name, created_at LIMIT ?",
		esc+"%", exclude, exclude, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*PublicUser
	for rows.Next() {
		u, err := scanPublicUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- messages

// MessageRow is a private message.
type MessageRow struct {
	ID     uint64
	FromID string
	ToID   string
	Body   string
	At     time.Time
	Read   bool
}

// AddMessage stores a message and returns its id.
func (s *Store) AddMessage(ctx context.Context, from, to, body string, at time.Time) (uint64, error) {
	res, err := s.db.ExecContext(ctx, "INSERT INTO messages (from_id, to_id, body, created_at) VALUES (?, ?, ?, ?)", from, to, body, at.UTC())
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return uint64(id), err
}

func scanMessages(rows *sql.Rows) ([]MessageRow, error) {
	defer rows.Close()
	var out []MessageRow
	for rows.Next() {
		var m MessageRow
		var read sql.NullTime
		if err := rows.Scan(&m.ID, &m.FromID, &m.ToID, &m.Body, &m.At, &read); err != nil {
			return nil, err
		}
		m.Read = read.Valid
		out = append(out, m)
	}
	return out, rows.Err()
}

// Conversation returns the talk of two people, oldest first: the last limit messages, or when
// after is above 0 the messages newer than that id.
func (s *Store) Conversation(ctx context.Context, a, b string, after uint64, limit int) ([]MessageRow, error) {
	const pair = "((from_id = ? AND to_id = ?) OR (from_id = ? AND to_id = ?))"
	if after > 0 {
		rows, err := s.db.QueryContext(ctx, "SELECT id, from_id, to_id, body, created_at, read_at FROM messages WHERE "+pair+" AND id > ? ORDER BY id LIMIT ?",
			a, b, b, a, after, limit)
		if err != nil {
			return nil, err
		}
		return scanMessages(rows)
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id, from_id, to_id, body, created_at, read_at FROM (SELECT * FROM messages WHERE "+pair+" ORDER BY id DESC LIMIT ?) t ORDER BY id",
		a, b, b, a, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// MessageByID returns one message or ErrNotFound.
func (s *Store) MessageByID(ctx context.Context, id uint64) (*MessageRow, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, from_id, to_id, body, created_at, read_at FROM messages WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	list, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

// MarkRead marks what other has written to me as read.
func (s *Store) MarkRead(ctx context.Context, me, other string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, "UPDATE messages SET read_at = ? WHERE to_id = ? AND from_id = ? AND read_at IS NULL", at.UTC(), me, other)
	return err
}

// UnreadByFriend counts the unread messages of each sender.
func (s *Store) UnreadByFriend(ctx context.Context, me string) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT from_id, COUNT(*) FROM messages WHERE to_id = ? AND read_at IS NULL GROUP BY from_id", me)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// LastMessages returns the latest message of each talk the user has, keyed by the other person.
func (s *Store) LastMessages(ctx context.Context, me string) (map[string]MessageRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.id, m.from_id, m.to_id, m.body, m.created_at, m.read_at
		FROM messages m JOIN (
			SELECT IF(from_id = ?, to_id, from_id) AS other, MAX(id) AS mid FROM messages WHERE from_id = ? OR to_id = ? GROUP BY other
		) t ON m.id = t.mid`, me, me, me)
	if err != nil {
		return nil, err
	}
	list, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	out := make(map[string]MessageRow, len(list))
	for _, m := range list {
		other := m.FromID
		if other == me {
			other = m.ToID
		}
		out[other] = m
	}
	return out, nil
}

// CountUnread counts all the unread messages of a user.
func (s *Store) CountUnread(ctx context.Context, me string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM messages WHERE to_id = ? AND read_at IS NULL", me).Scan(&n)
	return n, err
}

// -------------------------------------------------------------- challenges

// Challenge statuses.
const (
	ChallengePending   = "pending"
	ChallengeAccepted  = "accepted"
	ChallengeDeclined  = "declined"
	ChallengeCancelled = "cancelled"
	ChallengeExpired   = "expired"
)

// ChallengeRow is an invitation to a game.
type ChallengeRow struct {
	ID      string
	FromID  string
	ToID    string
	Color   string // the sender's: white, black or random
	BaseMs  int64
	IncMs   int64
	Days    int
	Rated   bool
	Status  string
	GameID  string
	Created time.Time
	Expires time.Time
}

const challengeColumns = "id, from_id, to_id, color, base_ms, inc_ms, days, rated, status, game_id, created_at, expires_at"

func scanChallenge(r rowScanner) (*ChallengeRow, error) {
	var c ChallengeRow
	var game sql.NullString
	if err := r.Scan(&c.ID, &c.FromID, &c.ToID, &c.Color, &c.BaseMs, &c.IncMs, &c.Days, &c.Rated, &c.Status, &game, &c.Created, &c.Expires); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c.GameID = game.String
	return &c, nil
}

// CreateChallenge stores a new challenge.
func (s *Store) CreateChallenge(ctx context.Context, c *ChallengeRow) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO challenges (id, from_id, to_id, color, base_ms, inc_ms, days, rated, status, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.FromID, c.ToID, c.Color, c.BaseMs, c.IncMs, c.Days, c.Rated, ChallengePending, c.Created.UTC(), c.Expires.UTC())
	return err
}

// Challenge returns a challenge or ErrNotFound.
func (s *Store) Challenge(ctx context.Context, id string) (*ChallengeRow, error) {
	return scanChallenge(s.db.QueryRowContext(ctx, "SELECT "+challengeColumns+" FROM challenges WHERE id = ?", id))
}

// SetChallengeStatus moves a pending challenge to a new status (and its game) and says whether it did:
// false when somebody else answered it first.
func (s *Store) SetChallengeStatus(ctx context.Context, id, status, gameID string) (bool, error) {
	res, err := s.db.ExecContext(ctx, "UPDATE challenges SET status = ?, game_id = ? WHERE id = ? AND status = ?", status, nullIfEmpty(gameID), id, ChallengePending)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SetChallengeGame records the game made for an accepted challenge.
func (s *Store) SetChallengeGame(ctx context.Context, id, gameID string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE challenges SET game_id = ? WHERE id = ?", gameID, id)
	return err
}

// ChallengesFor returns the pending, still valid challenges sent to the user (incoming) or by them
// (outgoing; here the ones answered in the last answeredFor too, so that the sender learns the result).
func (s *Store) ChallengesFor(ctx context.Context, userID string, incoming bool, now time.Time, answeredFor time.Duration) ([]*ChallengeRow, error) {
	var q string
	var args []any
	if incoming {
		q = "SELECT " + challengeColumns + " FROM challenges WHERE to_id = ? AND status = 'pending' AND expires_at > ? ORDER BY created_at DESC LIMIT 50"
		args = []any{userID, now.UTC()}
	} else {
		q = "SELECT " + challengeColumns + ` FROM challenges WHERE from_id = ? AND ((status = 'pending' AND expires_at > ?) OR
			(status IN ('accepted', 'declined') AND created_at > ?)) ORDER BY created_at DESC LIMIT 50`
		args = []any{userID, now.UTC(), now.Add(-answeredFor).UTC()}
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ChallengeRow
	for rows.Next() {
		c, err := scanChallenge(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CountIncomingChallenges counts the challenges waiting for the user's answer.
func (s *Store) CountIncomingChallenges(ctx context.Context, userID string, now time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM challenges WHERE to_id = ? AND status = 'pending' AND expires_at > ?", userID, now.UTC()).Scan(&n)
	return n, err
}

// CountPendingChallenges counts the open challenges from one user, to one person when to is not "".
func (s *Store) CountPendingChallenges(ctx context.Context, from, to string, now time.Time) (int, error) {
	q, args := "SELECT COUNT(*) FROM challenges WHERE from_id = ? AND status = 'pending' AND expires_at > ?", []any{from, now.UTC()}
	if to != "" {
		q += " AND to_id = ?"
		args = append(args, to)
	}
	var n int
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&n)
	return n, err
}

// CleanChallenges marks the unanswered old challenges expired and forgets the answered ones after a week.
func (s *Store) CleanChallenges(ctx context.Context, now time.Time) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE challenges SET status = 'expired' WHERE status = 'pending' AND expires_at <= ?", now.UTC()); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM challenges WHERE status <> 'pending' AND created_at < ?", now.Add(-7*24*time.Hour).UTC())
	return err
}

// --------------------------------------------------------------- vacations

// VacationRow is the vacation of a user; zero times when there is none.
type VacationRow struct {
	Ends      time.Time
	LastEnded time.Time
}

// Vacation returns the vacation record of a user (zero values if they never had one).
func (s *Store) Vacation(ctx context.Context, userID string) (VacationRow, error) {
	var ends, last sql.NullTime
	err := s.db.QueryRowContext(ctx, "SELECT ends_at, last_ended FROM vacations WHERE user_id = ?", userID).Scan(&ends, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return VacationRow{}, nil
	}
	return VacationRow{Ends: ends.Time, LastEnded: last.Time}, err
}

// SetVacation stores the end of the vacation (zero ends it) and when the previous one ended.
func (s *Store) SetVacation(ctx context.Context, userID string, v VacationRow) error {
	var ends, last any
	if !v.Ends.IsZero() {
		ends = v.Ends.UTC()
	}
	if !v.LastEnded.IsZero() {
		last = v.LastEnded.UTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO vacations (user_id, ends_at, last_ended) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE ends_at = VALUES(ends_at), last_ended = VALUES(last_ended)`, userID, ends, last)
	return err
}

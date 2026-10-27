package store

import (
	"context"
	"time"
)

// Blocking a player (migration 0026). The rules (limits, what a block forbids) belong to
// internal/community; here are the reads and the one write that has to be a transaction.

// BlockRow is somebody the user has blocked.
type BlockRow struct {
	BlockedID string
	At        time.Time
}

// Block makes blockerID block blockedID. In one transaction it also ends what would let the two
// reach each other: the friendship (both rows), the friend requests either way, the open
// challenges either way; what the blocked person wrote before is marked read, so that it does not
// stay on the badge. It reports whether the block is new (false: it was there already).
func (s *Store) Block(ctx context.Context, blockerID, blockedID string, at time.Time) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, "INSERT IGNORE INTO blocks (blocker_id, blocked_id, created_at) VALUES (?, ?, ?)", blockerID, blockedID, at.UTC())
	if err != nil {
		return false, err
	}
	fresh := false
	if n, _ := res.RowsAffected(); n > 0 {
		fresh = true
	}
	for _, q := range []string{
		"DELETE FROM friends WHERE (user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?)",
		"DELETE FROM friend_requests WHERE (from_id = ? AND to_id = ?) OR (from_id = ? AND to_id = ?)",
		"UPDATE challenges SET status = 'cancelled' WHERE status = 'pending' AND ((from_id = ? AND to_id = ?) OR (from_id = ? AND to_id = ?))",
	} {
		if _, err := tx.ExecContext(ctx, q, blockerID, blockedID, blockedID, blockerID); err != nil {
			return false, err
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE messages SET read_at = ? WHERE to_id = ? AND from_id = ? AND read_at IS NULL", at.UTC(), blockerID, blockedID); err != nil {
		return false, err
	}
	return fresh, tx.Commit()
}

// Unblock removes a block; false when there was none.
func (s *Store) Unblock(ctx context.Context, blockerID, blockedID string) (bool, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM blocks WHERE blocker_id = ? AND blocked_id = ?", blockerID, blockedID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Blocks lists whom the user has blocked, the latest block first.
func (s *Store) Blocks(ctx context.Context, blockerID string) ([]BlockRow, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT blocked_id, created_at FROM blocks WHERE blocker_id = ? ORDER BY created_at DESC", blockerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BlockRow
	for rows.Next() {
		var b BlockRow
		if err := rows.Scan(&b.BlockedID, &b.At); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// CountBlocks counts whom the user has blocked.
func (s *Store) CountBlocks(ctx context.Context, blockerID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM blocks WHERE blocker_id = ?", blockerID).Scan(&n)
	return n, err
}

// BlockState is how a block stands between two people, from the point of view of the first.
type BlockState struct {
	ByMe   bool // I have blocked them
	ByThem bool // they have blocked me
}

// Any says whether there is a block either way.
func (b BlockState) Any() bool { return b.ByMe || b.ByThem }

// BlockBetween tells whether me has blocked other and whether other has blocked me.
func (s *Store) BlockBetween(ctx context.Context, me, other string) (BlockState, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT blocker_id FROM blocks WHERE (blocker_id = ? AND blocked_id = ?) OR (blocker_id = ? AND blocked_id = ?)", me, other, other, me)
	if err != nil {
		return BlockState{}, err
	}
	defer rows.Close()
	var st BlockState
	for rows.Next() {
		var by string
		if err := rows.Scan(&by); err != nil {
			return BlockState{}, err
		}
		if by == me {
			st.ByMe = true
		} else {
			st.ByThem = true
		}
	}
	return st, rows.Err()
}

// BlockedIDs are the people the user has blocked.
func (s *Store) BlockedIDs(ctx context.Context, blockerID string) (map[string]bool, error) {
	return s.idSet(ctx, "SELECT blocked_id FROM blocks WHERE blocker_id = ?", blockerID)
}

// BlockerIDs are the people who have blocked the user.
func (s *Store) BlockerIDs(ctx context.Context, blockedID string) (map[string]bool, error) {
	return s.idSet(ctx, "SELECT blocker_id FROM blocks WHERE blocked_id = ?", blockedID)
}

// BlockedWith are the people the user has a block with, whichever way: matchmaking never pairs them.
func (s *Store) BlockedWith(ctx context.Context, userID string) (map[string]bool, error) {
	return s.idSet(ctx, "SELECT blocked_id FROM blocks WHERE blocker_id = ? UNION SELECT blocker_id FROM blocks WHERE blocked_id = ?", userID, userID)
}

func (s *Store) idSet(ctx context.Context, query string, args ...any) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

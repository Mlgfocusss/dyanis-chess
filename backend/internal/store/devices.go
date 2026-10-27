package store

import (
	"context"
	"time"
)

// Devices and addresses of accounts (migration 0029). The rules about what counts as "the same
// person" belong to internal/sameperson; here are the reads and writes.

// Device kinds.
const (
	DeviceCookie = "d"
	DeviceIP     = "i"
)

// DeviceMark is one thing an account was seen with.
type DeviceMark struct {
	Kind, Hash string
}

// TouchDevices records that the user was seen with the marks now.
func (s *Store) TouchDevices(ctx context.Context, userID string, marks []DeviceMark, now time.Time) error {
	for _, m := range marks {
		_, err := s.db.ExecContext(ctx, `INSERT INTO user_devices (user_id, kind, hash, first_seen, last_seen) VALUES (?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE last_seen = VALUES(last_seen)`, userID, m.Kind, m.Hash, now.UTC(), now.UTC())
		if err != nil {
			return err
		}
	}
	return nil
}

// SharingUsers returns the other accounts that were seen with the same device cookie since
// deviceSince, or with the same address since ipSince.
func (s *Store) SharingUsers(ctx context.Context, userID string, deviceSince, ipSince time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT o.user_id FROM user_devices m
		JOIN user_devices o ON o.kind = m.kind AND o.hash = m.hash AND o.user_id <> m.user_id
		WHERE m.user_id = ? AND ((m.kind = 'd' AND m.last_seen >= ? AND o.last_seen >= ?) OR (m.kind = 'i' AND m.last_seen >= ? AND o.last_seen >= ?))`,
		userID, deviceSince.UTC(), deviceSince.UTC(), ipSince.UTC(), ipSince.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ShareDevice says whether the two accounts were seen with a common device cookie (since
// deviceSince) or a common address (since ipSince).
func (s *Store) ShareDevice(ctx context.Context, a, b string, deviceSince, ipSince time.Time) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_devices m
		JOIN user_devices o ON o.kind = m.kind AND o.hash = m.hash
		WHERE m.user_id = ? AND o.user_id = ? AND ((m.kind = 'd' AND m.last_seen >= ? AND o.last_seen >= ?) OR (m.kind = 'i' AND m.last_seen >= ? AND o.last_seen >= ?))`,
		a, b, deviceSince.UTC(), deviceSince.UTC(), ipSince.UTC(), ipSince.UTC()).Scan(&n)
	return n > 0, err
}

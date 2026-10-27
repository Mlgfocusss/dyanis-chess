package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Reports to the moderators (migration 0026). The rules (who may report what, the limits, what a
// moderator may do) belong to internal/forum; here are the reads and writes.

// Kinds of things that can be reported, and the reasons.
const (
	ReportMessage    = "message"
	ReportForumPost  = "forum_post"
	ReportForumTopic = "forum_topic"
	ReportUser       = "user"

	ReportOpen   = "open"
	ReportClosed = "closed"
)

// ErrReportExists is returned when the person already has an open report on the same thing.
var ErrReportExists = errors.New("store: report exists")

// ComplaintRow is a complaint.
type ComplaintRow struct {
	ID           uint64
	ReporterID   string // "" once the account is gone
	ReporterName string
	Kind         string
	TargetID     string // the id of the message, post, topic or user
	TargetUserID string // the author of what is reported (or the reported person); "" once gone
	TargetName   string
	TopicID      uint64 // for a forum post or topic: the topic, to link to
	Reason       string
	Note         string
	Snapshot     string // the text at the moment of the report
	Status       string
	Action       string // what the moderator did when closing
	Created      time.Time
	ClosedBy     string
	ClosedAt     time.Time
}

const reportColumns = "id, reporter_id, reporter_name, target_kind, target_id, target_user_id, target_name, topic_id, reason, note, snapshot, status, action, created_at, closed_by, closed_at"

func scanReport(r rowScanner) (*ComplaintRow, error) {
	var x ComplaintRow
	var reporter, targetUser, closedBy sql.NullString
	var topic sql.NullInt64
	var closedAt sql.NullTime
	if err := r.Scan(&x.ID, &reporter, &x.ReporterName, &x.Kind, &x.TargetID, &targetUser, &x.TargetName, &topic, &x.Reason, &x.Note,
		&x.Snapshot, &x.Status, &x.Action, &x.Created, &closedBy, &closedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	x.ReporterID, x.TargetUserID, x.ClosedBy = reporter.String, targetUser.String, closedBy.String
	x.TopicID, x.ClosedAt = uint64(topic.Int64), closedAt.Time
	return &x, nil
}

// CreateReport stores a complaint and returns its id; ErrReportExists when the reporter has one
// open on the same target already.
func (s *Store) CreateReport(ctx context.Context, r *ComplaintRow) (uint64, error) {
	var topic any
	if r.TopicID > 0 {
		topic = r.TopicID
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO reports (reporter_id, reporter_name, target_kind, target_id, target_user_id, target_name, topic_id,
		reason, note, snapshot, status, open_key, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'open', 1, ?)`,
		nullIfEmpty(r.ReporterID), r.ReporterName, r.Kind, r.TargetID, nullIfEmpty(r.TargetUserID), r.TargetName, topic,
		r.Reason, r.Note, r.Snapshot, r.Created.UTC())
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1062 {
		return 0, ErrReportExists
	}
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return uint64(id), err
}

// Reports lists the complaints with a status ("open": the oldest first; "closed": the latest first).
func (s *Store) Reports(ctx context.Context, status string, limit int) ([]*ComplaintRow, error) {
	order := "created_at, id"
	if status == ReportClosed {
		order = "closed_at DESC, id DESC"
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+reportColumns+" FROM reports WHERE status = ? ORDER BY "+order+" LIMIT ?", status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ComplaintRow
	for rows.Next() {
		x, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// Report returns a complaint or ErrNotFound.
func (s *Store) Report(ctx context.Context, id uint64) (*ComplaintRow, error) {
	return scanReport(s.db.QueryRowContext(ctx, "SELECT "+reportColumns+" FROM reports WHERE id = ?", id))
}

// CountOpenReports counts the complaints waiting for a moderator.
func (s *Store) CountOpenReports(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reports WHERE status = 'open'").Scan(&n)
	return n, err
}

// DeleteReport removes a complaint for good (the tests clean up with it; no page does).
func (s *Store) DeleteReport(ctx context.Context, id uint64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM reports WHERE id = ?", id)
	return err
}

// CloseReports closes every open complaint on one target (several people may have complained of the
// same message) with what the moderator did, and returns how many it closed.
func (s *Store) CloseReports(ctx context.Context, kind, targetID, by, action string, at time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE reports SET status = 'closed', open_key = NULL, action = ?, closed_by = ?, closed_at = ?
		WHERE target_kind = ? AND target_id = ? AND status = 'open'`, action, nullIfEmpty(by), at.UTC(), kind, targetID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

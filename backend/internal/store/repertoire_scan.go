package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// RepScan is the last search of a repertoire for holes.
type RepScan struct {
	RepertoireID int64
	ScannedAt    time.Time
	Moves        int   // moves of the tree the search saw
	MaxMoveID    int64 // and the greatest id among them
	Missing      int
	Answer       int
	Coverage     float64
	Partial      bool
	Source       string
	Result       string // JSON
}

// SaveRepScan stores a search of one of the person's repertoires (replacing the last one);
// ErrNotFound for somebody else's.
func (s *Store) SaveRepScan(ctx context.Context, userID string, sc RepScan) error {
	if _, err := s.Repertoire(ctx, userID, sc.RepertoireID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO rep_scans (repertoire_id, scanned_at, moves, max_move_id, missing, answer, coverage, partial, source, result)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE scanned_at = VALUES(scanned_at), moves = VALUES(moves), max_move_id = VALUES(max_move_id), missing = VALUES(missing),
			answer = VALUES(answer), coverage = VALUES(coverage), partial = VALUES(partial), source = VALUES(source), result = VALUES(result)`,
		sc.RepertoireID, sc.ScannedAt.UTC(), sc.Moves, sc.MaxMoveID, sc.Missing, sc.Answer, sc.Coverage, sc.Partial, sc.Source, sc.Result)
	return err
}

const repScanColumns = "repertoire_id, scanned_at, moves, max_move_id, missing, answer, coverage, partial, source"

func scanRepScan(r rowScanner, withResult bool) (RepScan, error) {
	var x RepScan
	dest := []any{&x.RepertoireID, &x.ScannedAt, &x.Moves, &x.MaxMoveID, &x.Missing, &x.Answer, &x.Coverage, &x.Partial, &x.Source}
	if withResult {
		dest = append(dest, &x.Result)
	}
	err := r.Scan(dest...)
	return x, err
}

// RepScan returns the last search of one of the person's repertoires with its JSON;
// ErrNotFound when there was none (or the repertoire is somebody else's).
func (s *Store) RepScan(ctx context.Context, userID string, repertoireID int64) (RepScan, error) {
	x, err := scanRepScan(s.db.QueryRowContext(ctx, `SELECT `+repScanColumns+`, result FROM rep_scans
		WHERE repertoire_id = ? AND repertoire_id IN (SELECT id FROM repertoires WHERE user_id = ?)`, repertoireID, userID), true)
	if errors.Is(err, sql.ErrNoRows) {
		return RepScan{}, ErrNotFound
	}
	return x, err
}

// RepOverview is what the start page of the openings shows about a repertoire beyond its counts,
// without asking Lichess: the lines that end with the opponent's move, the size of the tree, and
// the last search (nil when there was none; its JSON is left out).
type RepOverview struct {
	Open    int   // moves of the opponent without a reply of the person (holes of the first kind)
	MaxMove int64 // the greatest move id of the tree (0: empty)
	Scan    *RepScan
}

// RepOverviews returns the overview of every repertoire of the person, by repertoire id.
func (s *Store) RepOverviews(ctx context.Context, userID string) (map[int64]RepOverview, error) {
	out := map[int64]RepOverview{}
	rows, err := s.db.QueryContext(ctx, `SELECT m.repertoire_id, MAX(m.id),
			COALESCE(SUM(m.mine = 0 AND NOT EXISTS (SELECT 1 FROM rep_moves c WHERE c.repertoire_id = m.repertoire_id AND c.parent_id = m.id)), 0)
		FROM rep_moves m JOIN repertoires r ON r.id = m.repertoire_id WHERE r.user_id = ? GROUP BY m.repertoire_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var o RepOverview
		if err := rows.Scan(&id, &o.MaxMove, &o.Open); err != nil {
			return nil, err
		}
		out[id] = o
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	srows, err := s.db.QueryContext(ctx, `SELECT `+repScanColumns+` FROM rep_scans
		WHERE repertoire_id IN (SELECT id FROM repertoires WHERE user_id = ?)`, userID)
	if err != nil {
		return nil, err
	}
	defer srows.Close()
	for srows.Next() {
		x, err := scanRepScan(srows, false)
		if err != nil {
			return nil, err
		}
		o := out[x.RepertoireID]
		o.Scan = &x
		out[x.RepertoireID] = o
	}
	return out, srows.Err()
}

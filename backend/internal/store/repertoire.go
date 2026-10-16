package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Limits that keep one account from filling the database.
const (
	MaxRepertoires = 30
	MaxRepMoves    = 10000 // moves in one repertoire
)

// ErrLimit is returned when a limit above is reached.
var ErrLimit = errors.New("store: limit reached")

// Repertoire is a person's opening repertoire for one colour.
type Repertoire struct {
	ID        int64
	UserID    string
	Name      string
	Color     string // "white" or "black"
	CreatedAt time.Time
}

// RepertoireCounts is what a repertoire list shows about each one.
type RepertoireCounts struct {
	Moves   int // all moves of the tree
	Mine    int // the person's own moves (cards)
	New     int // never asked yet
	Due     int // asked before and due now
	Learned int // passed enough of the ladder (see LearnedBox)
}

// LearnedBox is the number of the ladder's steps after which a move counts as learned.
const LearnedBox = 5

// RepMove is one move of a repertoire's tree, with the memorising state of
// the move when it is the person's own.
type RepMove struct {
	ID           int64
	RepertoireID int64
	ParentID     int64 // 0 for a first move
	UCI, SAN     string
	FEN          string // after the move
	Ply          int
	Mine         bool
	Box          int
	DueAt        sql.NullTime
	Lapses       int
	Reviews      int
	ReviewedAt   sql.NullTime
}

const repMoveColumns = "id, repertoire_id, parent_id, uci, san, fen, ply, mine, box, due_at, lapses, reviews, reviewed_at"

func scanRepMove(r rowScanner) (RepMove, error) {
	var m RepMove
	err := r.Scan(&m.ID, &m.RepertoireID, &m.ParentID, &m.UCI, &m.SAN, &m.FEN, &m.Ply, &m.Mine, &m.Box, &m.DueAt, &m.Lapses, &m.Reviews, &m.ReviewedAt)
	return m, err
}

// CreateRepertoire adds an empty repertoire.
func (s *Store) CreateRepertoire(ctx context.Context, userID, name, color string) (*Repertoire, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM repertoires WHERE user_id = ?", userID).Scan(&n); err != nil {
		return nil, err
	}
	if n >= MaxRepertoires {
		return nil, ErrLimit
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, "INSERT INTO repertoires (user_id, name, color, created_at) VALUES (?, ?, ?, ?)", userID, name, color, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Repertoire{ID: id, UserID: userID, Name: name, Color: color, CreatedAt: now}, nil
}

// Repertoire returns one of the person's repertoires; ErrNotFound for somebody else's.
func (s *Store) Repertoire(ctx context.Context, userID string, id int64) (*Repertoire, error) {
	var r Repertoire
	err := s.db.QueryRowContext(ctx, "SELECT id, user_id, name, color, created_at FROM repertoires WHERE id = ? AND user_id = ?", id, userID).
		Scan(&r.ID, &r.UserID, &r.Name, &r.Color, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &r, err
}

// RepertoireWithCounts is a repertoire in a list.
type RepertoireWithCounts struct {
	Repertoire
	Counts RepertoireCounts
}

// ListRepertoires returns the person's repertoires, oldest first, with their counts as of now.
func (s *Store) ListRepertoires(ctx context.Context, userID string, now time.Time) ([]RepertoireWithCounts, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id, r.user_id, r.name, r.color, r.created_at,
			COUNT(m.id),
			COALESCE(SUM(m.mine), 0),
			COALESCE(SUM(m.mine AND m.due_at IS NULL), 0),
			COALESCE(SUM(m.mine AND m.due_at IS NOT NULL AND m.due_at <= ?), 0),
			COALESCE(SUM(m.mine AND m.box >= ?), 0)
		FROM repertoires r LEFT JOIN rep_moves m ON m.repertoire_id = r.id
		WHERE r.user_id = ? GROUP BY r.id ORDER BY r.id`, now.UTC(), LearnedBox, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RepertoireWithCounts
	for rows.Next() {
		var r RepertoireWithCounts
		if err := rows.Scan(&r.ID, &r.UserID, &r.Name, &r.Color, &r.CreatedAt,
			&r.Counts.Moves, &r.Counts.Mine, &r.Counts.New, &r.Counts.Due, &r.Counts.Learned); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RenameRepertoire changes the name.
func (s *Store) RenameRepertoire(ctx context.Context, userID string, id int64, name string) error {
	res, err := s.db.ExecContext(ctx, "UPDATE repertoires SET name = ? WHERE id = ? AND user_id = ?", name, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.Repertoire(ctx, userID, id); err != nil { // an unchanged name also affects 0 rows
			return err
		}
	}
	return nil
}

// DeleteRepertoire removes a repertoire with all its moves.
func (s *Store) DeleteRepertoire(ctx context.Context, userID string, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM repertoires WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RepMoves returns every move of a repertoire, in the order they were added
// (a move always comes after its parent).
func (s *Store) RepMoves(ctx context.Context, repertoireID int64) ([]RepMove, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+repMoveColumns+" FROM rep_moves WHERE repertoire_id = ? ORDER BY id", repertoireID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RepMove
	for rows.Next() {
		m, err := scanRepMove(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddRepMove adds a move to the tree; a move already there (same parent, same
// move) is returned as it is, with created false. Only RepertoireID, ParentID,
// UCI, SAN, FEN, Ply and Mine of m are used.
func (s *Store) AddRepMove(ctx context.Context, m RepMove) (RepMove, bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM rep_moves WHERE repertoire_id = ?", m.RepertoireID).Scan(&n); err != nil {
		return RepMove{}, false, err
	}
	existing, err := scanRepMove(s.db.QueryRowContext(ctx, "SELECT "+repMoveColumns+" FROM rep_moves WHERE repertoire_id = ? AND parent_id = ? AND uci = ?",
		m.RepertoireID, m.ParentID, m.UCI))
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RepMove{}, false, err
	}
	if n >= MaxRepMoves {
		return RepMove{}, false, ErrLimit
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO rep_moves (repertoire_id, parent_id, uci, san, fen, ply, mine, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, m.RepertoireID, m.ParentID, m.UCI, m.SAN, m.FEN, m.Ply, m.Mine, time.Now().UTC())
	if err != nil {
		return RepMove{}, false, err
	}
	id, _ := res.LastInsertId()
	m.ID = id
	return m, true, nil
}

// NewRepMove is a move to add in a batch. Its parent is a move already in the
// repertoire (ParentID, 0 for a first move) or an earlier move of the same batch
// (ParentIndex >= 0, its index in the batch; ParentID is then ignored).
type NewRepMove struct {
	ParentID    int64
	ParentIndex int
	UCI, SAN    string
	FEN         string
	Ply         int
	Mine        bool
}

// AddRepMovesBulk adds a batch of moves in one transaction: all of them or, when the
// repertoire would grow past MaxRepMoves (ErrLimit) or anything fails, none. The caller
// leaves out moves the tree already has.
func (s *Store) AddRepMovesBulk(ctx context.Context, repertoireID int64, moves []NewRepMove) error {
	if len(moves) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM rep_moves WHERE repertoire_id = ? FOR UPDATE", repertoireID).Scan(&n); err != nil {
		return err
	}
	if n+len(moves) > MaxRepMoves {
		return ErrLimit
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO rep_moves (repertoire_id, parent_id, uci, san, fen, ply, mine, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().UTC()
	ids := make([]int64, len(moves))
	for i, m := range moves {
		parent := m.ParentID
		if m.ParentIndex >= 0 {
			if m.ParentIndex >= i {
				return errors.New("store: a move of the batch comes before its parent")
			}
			parent = ids[m.ParentIndex]
		}
		res, err := stmt.ExecContext(ctx, repertoireID, parent, m.UCI, m.SAN, m.FEN, m.Ply, m.Mine, now)
		if err != nil {
			return err
		}
		if ids[i], err = res.LastInsertId(); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteRepMove removes a move and every line that continues it.
func (s *Store) DeleteRepMove(ctx context.Context, repertoireID, id int64) (int, error) {
	all, err := s.RepMoves(ctx, repertoireID)
	if err != nil {
		return 0, err
	}
	children := map[int64][]int64{}
	found := false
	for _, m := range all {
		children[m.ParentID] = append(children[m.ParentID], m.ID)
		found = found || m.ID == id
	}
	if !found {
		return 0, ErrNotFound
	}
	var doomed []int64
	stack := []int64{id}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		doomed = append(doomed, cur)
		stack = append(stack, children[cur]...)
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(doomed)), ",")
	args := make([]any, 0, len(doomed)+1)
	args = append(args, repertoireID)
	for _, d := range doomed {
		args = append(args, d)
	}
	_, err = s.db.ExecContext(ctx, "DELETE FROM rep_moves WHERE repertoire_id = ? AND id IN ("+marks+")", args...)
	return len(doomed), err
}

// Card is the memorising state of a move.
type Card struct {
	Box    int
	DueAt  time.Time
	Lapses int
}

// ReviewRepMove records one answer for a move of the person's own. next maps
// the card's state before to the state after; the card is locked meanwhile.
func (s *Store) ReviewRepMove(ctx context.Context, repertoireID, id int64, next func(box, lapses int) Card) (Card, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Card{}, err
	}
	defer tx.Rollback()
	var box, lapses int
	var mine bool
	err = tx.QueryRowContext(ctx, "SELECT box, lapses, mine FROM rep_moves WHERE id = ? AND repertoire_id = ? FOR UPDATE", id, repertoireID).Scan(&box, &lapses, &mine)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !mine) {
		return Card{}, ErrNotFound
	}
	if err != nil {
		return Card{}, err
	}
	c := next(box, lapses)
	_, err = tx.ExecContext(ctx, "UPDATE rep_moves SET box = ?, due_at = ?, lapses = ?, reviews = reviews + 1, reviewed_at = ? WHERE id = ?",
		c.Box, c.DueAt.UTC(), c.Lapses, time.Now().UTC(), id)
	if err != nil {
		return Card{}, err
	}
	return c, tx.Commit()
}

// RepMove returns one move of a repertoire; ErrNotFound if it is not there.
func (s *Store) RepMove(ctx context.Context, repertoireID, id int64) (RepMove, error) {
	m, err := scanRepMove(s.db.QueryRowContext(ctx, "SELECT "+repMoveColumns+" FROM rep_moves WHERE id = ? AND repertoire_id = ?", id, repertoireID))
	if errors.Is(err, sql.ErrNoRows) {
		return RepMove{}, ErrNotFound
	}
	return m, err
}

package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Puzzle is one row of the imported Lichess puzzle set.
type Puzzle struct {
	ID         string
	FEN        string // before the opponent's first move
	Moves      string // UCI moves, space separated; the first one is the opponent's
	Rating     int
	Popularity int
	Plays      int
	Themes     string // space separated
}

// ThemeCount is a puzzle theme and how many puzzles have it.
type ThemeCount struct {
	Theme string
	Count int
}

// PuzzleRating is a person's puzzle rating and record.
type PuzzleRating struct {
	Rating   int
	Attempts int
	Solved   int
}

// StartPuzzleRating is what a person who has never solved a puzzle has.
const StartPuzzleRating = 1000

const puzzleColumns = "p.id, p.fen, p.moves, p.rating, p.popularity, p.plays, p.themes"

func scanPuzzle(r rowScanner) (*Puzzle, error) {
	var p Puzzle
	if err := r.Scan(&p.ID, &p.FEN, &p.Moves, &p.Rating, &p.Popularity, &p.Plays, &p.Themes); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

// PuzzleByID returns one puzzle.
func (s *Store) PuzzleByID(ctx context.Context, id string) (*Puzzle, error) {
	return scanPuzzle(s.db.QueryRowContext(ctx, "SELECT "+puzzleColumns+" FROM puzzles p WHERE p.id = ?", id))
}

// CountPuzzles is the size of the imported set.
func (s *Store) CountPuzzles(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM puzzles").Scan(&n)
	return n, err
}

// PickPuzzle returns a random puzzle rated lo..hi, of the theme when one is
// named. With skipTried, puzzles the person has already attempted are left
// out. ErrNotFound means nothing fits.
func (s *Store) PickPuzzle(ctx context.Context, userID, theme string, lo, hi int, skipTried bool) (*Puzzle, error) {
	var q strings.Builder
	var args []any
	q.WriteString("SELECT " + puzzleColumns + " FROM puzzles p")
	if theme != "" {
		q.WriteString(" JOIN puzzle_themes t ON t.puzzle_id = p.id WHERE t.theme = ? AND t.rating BETWEEN ? AND ?")
		args = append(args, theme, lo, hi)
	} else {
		q.WriteString(" WHERE p.rating BETWEEN ? AND ?")
		args = append(args, lo, hi)
	}
	if skipTried && userID != "" {
		q.WriteString(" AND NOT EXISTS (SELECT 1 FROM puzzle_attempts a WHERE a.user_id = ? AND a.puzzle_id = p.id)")
		args = append(args, userID)
	}
	q.WriteString(" ORDER BY RAND() LIMIT 1")
	return scanPuzzle(s.db.QueryRowContext(ctx, q.String(), args...))
}

// PuzzleThemes lists every theme with the number of puzzles, most common first.
func (s *Store) PuzzleThemes(ctx context.Context) ([]ThemeCount, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT theme, COUNT(*) AS n FROM puzzle_themes GROUP BY theme ORDER BY n DESC, theme")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ThemeCount
	for rows.Next() {
		var t ThemeCount
		if err := rows.Scan(&t.Theme, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// PuzzleRatingOf returns the person's puzzle rating; a person with no
// attempts has StartPuzzleRating.
func (s *Store) PuzzleRatingOf(ctx context.Context, userID string) (PuzzleRating, error) {
	r := PuzzleRating{Rating: StartPuzzleRating}
	err := s.db.QueryRowContext(ctx, "SELECT rating, attempts, solved FROM puzzle_ratings WHERE user_id = ?", userID).
		Scan(&r.Rating, &r.Attempts, &r.Solved)
	if errors.Is(err, sql.ErrNoRows) {
		return PuzzleRating{Rating: StartPuzzleRating}, nil
	}
	return r, err
}

// PuzzleAttempt is the outcome of RecordPuzzleAttempt.
type PuzzleAttempt struct {
	Counted bool // false: the puzzle was tried before, so the rating is unchanged
	Before  int
	After   int
	Record  PuzzleRating // the person's rating and record after the attempt
}

// RecordPuzzleAttempt writes down the first try at a puzzle. update maps the
// person's current rating and attempt count to the new rating. A puzzle tried
// before changes nothing and reports Counted false.
func (s *Store) RecordPuzzleAttempt(ctx context.Context, userID, puzzleID string, solved bool, update func(rating, attempts int) int) (PuzzleAttempt, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PuzzleAttempt{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()

	cur := PuzzleRating{Rating: StartPuzzleRating}
	err = tx.QueryRowContext(ctx, "SELECT rating, attempts, solved FROM puzzle_ratings WHERE user_id = ? FOR UPDATE", userID).
		Scan(&cur.Rating, &cur.Attempts, &cur.Solved)
	exists := !errors.Is(err, sql.ErrNoRows)
	if err != nil && exists {
		return PuzzleAttempt{}, err
	}

	var tried int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM puzzle_attempts WHERE user_id = ? AND puzzle_id = ?", userID, puzzleID).Scan(&tried); err != nil {
		return PuzzleAttempt{}, err
	}
	if tried > 0 {
		return PuzzleAttempt{Before: cur.Rating, After: cur.Rating, Record: cur}, nil
	}

	next := PuzzleRating{Rating: update(cur.Rating, cur.Attempts), Attempts: cur.Attempts + 1, Solved: cur.Solved}
	if solved {
		next.Solved++
	}
	if exists {
		_, err = tx.ExecContext(ctx, "UPDATE puzzle_ratings SET rating = ?, attempts = ?, solved = ?, updated_at = ? WHERE user_id = ?",
			next.Rating, next.Attempts, next.Solved, now, userID)
	} else {
		_, err = tx.ExecContext(ctx, "INSERT INTO puzzle_ratings (user_id, rating, attempts, solved, updated_at) VALUES (?, ?, ?, ?, ?)",
			userID, next.Rating, next.Attempts, next.Solved, now)
	}
	if err != nil {
		return PuzzleAttempt{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO puzzle_attempts (user_id, puzzle_id, solved, rating_before, rating_after, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, userID, puzzleID, solved, cur.Rating, next.Rating, now); err != nil {
		return PuzzleAttempt{}, err
	}
	return PuzzleAttempt{Counted: true, Before: cur.Rating, After: next.Rating, Record: next}, tx.Commit()
}

// importBatch is how many puzzles go into one INSERT.
const importBatch = 500

// ImportPuzzles adds puzzles to the set; ones whose id is already there are
// left as they are. It returns how many were new.
func (s *Store) ImportPuzzles(ctx context.Context, ps []Puzzle) (int, error) {
	added := 0
	for start := 0; start < len(ps); start += importBatch {
		end := min(start+importBatch, len(ps))
		n, err := s.importChunk(ctx, ps[start:end])
		if err != nil {
			return added, err
		}
		added += n
	}
	return added, nil
}

func (s *Store) importChunk(ctx context.Context, ps []Puzzle) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var q strings.Builder
	var args []any
	q.WriteString("INSERT IGNORE INTO puzzles (id, fen, moves, rating, popularity, plays, themes) VALUES ")
	for i, p := range ps {
		if i > 0 {
			q.WriteByte(',')
		}
		q.WriteString("(?,?,?,?,?,?,?)")
		args = append(args, p.ID, p.FEN, p.Moves, p.Rating, p.Popularity, p.Plays, p.Themes)
	}
	res, err := tx.ExecContext(ctx, q.String(), args...)
	if err != nil {
		return 0, err
	}
	added, _ := res.RowsAffected()

	q.Reset()
	args = args[:0]
	for _, p := range ps {
		for _, th := range strings.Fields(p.Themes) {
			if q.Len() > 0 {
				q.WriteByte(',')
			}
			q.WriteString("(?,?,?)")
			args = append(args, th, p.Rating, p.ID)
		}
	}
	if len(args) > 0 {
		if _, err := tx.ExecContext(ctx, "INSERT IGNORE INTO puzzle_themes (theme, rating, puzzle_id) VALUES "+q.String(), args...); err != nil {
			return 0, err
		}
	}
	return int(added), tx.Commit()
}

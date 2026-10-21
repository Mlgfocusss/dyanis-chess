package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// LeaderRow is one rating considered for the leaderboard of a category.
type LeaderRow struct {
	UserID     string
	Rating     float64
	RD         float64
	Volatility float64
	Games      int
	UpdatedAt  time.Time
}

// LeaderCandidates returns the ratings of a category with at least minGames games and a stored
// deviation of at most maxRD, the best rating first (ties by id, so that places are stable). The
// stored deviation only grows with idle days, so a person left out here is out for good, but one
// kept still has to be checked against the grown deviation by the caller. At most limit rows.
//
// A non-empty country (an ISO 3166-1 alpha-2 code) keeps only the people who named that country; the
// join starts from idx_users_country, so the table of one country does not read all the ratings.
func (s *Store) LeaderCandidates(ctx context.Context, category string, minGames int, maxRD float64, limit int, country string) ([]LeaderRow, error) {
	query := `SELECT r.user_id, r.rating, r.rd, r.volatility, r.games, r.updated_at FROM ratings r`
	args := []any{}
	if country != "" {
		query += ` JOIN users u ON u.id = r.user_id AND u.country = ?`
		args = append(args, country)
	}
	query += ` WHERE r.category = ? AND r.games >= ? AND r.rd <= ? ORDER BY r.rating DESC, r.user_id LIMIT ?`
	args = append(args, category, minGames, maxRD, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LeaderRow
	for rows.Next() {
		var x LeaderRow
		if err := rows.Scan(&x.UserID, &x.Rating, &x.RD, &x.Volatility, &x.Games, &x.UpdatedAt); err != nil {
			return nil, err
		}
		x.UpdatedAt = x.UpdatedAt.UTC()
		out = append(out, x)
	}
	return out, rows.Err()
}

// PublicUser is what other people may see of an account: no e-mail, settings or session data.
type PublicUser struct {
	ID        string
	Kind      string
	Name      string
	AvatarURL string
	FirstName string
	LastName  string
	Location  string
	About     string
	Country   string
	CreatedAt time.Time
}

const publicColumns = "id, kind, name, avatar_url, first_name, last_name, location, about, country, created_at"

func scanPublicUser(r rowScanner) (*PublicUser, error) {
	var u PublicUser
	var avatar, first, last, loc, about, country sql.NullString
	if err := r.Scan(&u.ID, &u.Kind, &u.Name, &avatar, &first, &last, &loc, &about, &country, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.AvatarURL, u.FirstName, u.LastName, u.Location, u.About, u.Country =
		avatar.String, first.String, last.String, loc.String, about.String, country.String
	u.CreatedAt = u.CreatedAt.UTC()
	return &u, nil
}

// PublicUser returns the public part of an account or ErrNotFound.
func (s *Store) PublicUser(ctx context.Context, id string) (*PublicUser, error) {
	return scanPublicUser(s.db.QueryRowContext(ctx, "SELECT "+publicColumns+" FROM users WHERE id = ?", id))
}

// PublicUsers returns the public part of the accounts with these ids, by id; unknown ids are absent.
func (s *Store) PublicUsers(ctx context.Context, ids []string) (map[string]*PublicUser, error) {
	out := make(map[string]*PublicUser, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+publicColumns+" FROM users WHERE id IN (?"+
		strings.Repeat(", ?", len(ids)-1)+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		u, err := scanPublicUser(rows)
		if err != nil {
			return nil, err
		}
		out[u.ID] = u
	}
	return out, rows.Err()
}

// CountEndedGames returns how many finished online games (against people and bots) the person played.
func (s *Store) CountEndedGames(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM games WHERE (white_id = ? OR black_id = ?) AND status = 'ended'",
		userID, userID).Scan(&n)
	return n, err
}

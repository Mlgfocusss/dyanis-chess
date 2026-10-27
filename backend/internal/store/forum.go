package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// The forum tables (migration 0021). The rules (categories, lengths, who may edit) belong to internal/forum.

// ForumTopic is a thread.
type ForumTopic struct {
	ID         uint64
	Category   string
	AuthorID   string // "" once the account is gone
	AuthorName string
	Title      string
	Created    time.Time
	LastPost   time.Time
	Posts      int
	Pinned     bool
	Locked     bool
}

// ForumPost is a message in a thread.
type ForumPost struct {
	ID         uint64
	TopicID    uint64
	AuthorID   string
	AuthorName string
	Body       string
	Created    time.Time
	Edited     time.Time // zero when never edited
	Deleted    bool
}

const topicColumns = "id, category, author_id, author_name, title, created_at, last_post_at, posts, pinned, locked"

func scanTopic(r rowScanner) (*ForumTopic, error) {
	var t ForumTopic
	var author sql.NullString
	if err := r.Scan(&t.ID, &t.Category, &author, &t.AuthorName, &t.Title, &t.Created, &t.LastPost, &t.Posts, &t.Pinned, &t.Locked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	t.AuthorID = author.String
	return &t, nil
}

const postColumns = "id, topic_id, author_id, author_name, body, created_at, edited_at, deleted"

func scanPost(r rowScanner) (*ForumPost, error) {
	var p ForumPost
	var author sql.NullString
	var edited sql.NullTime
	if err := r.Scan(&p.ID, &p.TopicID, &author, &p.AuthorName, &p.Body, &p.Created, &edited, &p.Deleted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.AuthorID, p.Edited = author.String, edited.Time
	return &p, nil
}

// ForumCategoryStat is what the front page of the forum shows of a category.
type ForumCategoryStat struct {
	Category string
	Topics   int
	Posts    int
	LastPost time.Time
}

// ForumStats counts the topics and posts of every category that has any.
func (s *Store) ForumStats(ctx context.Context) (map[string]ForumCategoryStat, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT category, COUNT(*), SUM(posts), MAX(last_post_at) FROM forum_topics GROUP BY category")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]ForumCategoryStat{}
	for rows.Next() {
		var c ForumCategoryStat
		if err := rows.Scan(&c.Category, &c.Topics, &c.Posts, &c.LastPost); err != nil {
			return nil, err
		}
		out[c.Category] = c
	}
	return out, rows.Err()
}

// ForumTopics lists a category (all of them when category is ""): pinned first, then the latest talk.
func (s *Store) ForumTopics(ctx context.Context, category string, limit, offset int) ([]*ForumTopic, int, error) {
	where, args := "", []any{}
	if category != "" {
		where, args = " WHERE category = ?", []any{category}
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM forum_topics"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+topicColumns+" FROM forum_topics"+where+" ORDER BY pinned DESC, last_post_at DESC LIMIT ? OFFSET ?",
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*ForumTopic
	for rows.Next() {
		t, err := scanTopic(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

// ForumTopicByID returns a thread or ErrNotFound.
func (s *Store) ForumTopicByID(ctx context.Context, id uint64) (*ForumTopic, error) {
	return scanTopic(s.db.QueryRowContext(ctx, "SELECT "+topicColumns+" FROM forum_topics WHERE id = ?", id))
}

// ForumPosts returns a page of a thread, oldest first.
func (s *Store) ForumPosts(ctx context.Context, topicID uint64, limit, offset int) ([]*ForumPost, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+postColumns+" FROM forum_posts WHERE topic_id = ? ORDER BY id LIMIT ? OFFSET ?", topicID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ForumPost
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ForumPostByID returns a message or ErrNotFound.
func (s *Store) ForumPostByID(ctx context.Context, id uint64) (*ForumPost, error) {
	return scanPost(s.db.QueryRowContext(ctx, "SELECT "+postColumns+" FROM forum_posts WHERE id = ?", id))
}

// ForumCreateTopic stores a thread with its first message and returns the thread's id.
func (s *Store) ForumCreateTopic(ctx context.Context, t *ForumTopic, body string) (uint64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO forum_topics (category, author_id, author_name, title, created_at, last_post_at, posts)
		VALUES (?, ?, ?, ?, ?, ?, 1)`, t.Category, nullIfEmpty(t.AuthorID), t.AuthorName, t.Title, t.Created.UTC(), t.Created.UTC())
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO forum_posts (topic_id, author_id, author_name, body, created_at) VALUES (?, ?, ?, ?, ?)",
		id, nullIfEmpty(t.AuthorID), t.AuthorName, body, t.Created.UTC()); err != nil {
		return 0, err
	}
	return uint64(id), tx.Commit()
}

// ErrLocked is returned when a message is added to a closed thread.
var ErrLocked = errors.New("store: topic is locked")

// ForumAddPost adds a message to a thread (ErrNotFound for an unknown thread, ErrLocked for a closed
// one) and returns its id.
func (s *Store) ForumAddPost(ctx context.Context, p *ForumPost) (uint64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var locked bool
	if err := tx.QueryRowContext(ctx, "SELECT locked FROM forum_topics WHERE id = ? FOR UPDATE", p.TopicID).Scan(&locked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	if locked {
		return 0, ErrLocked
	}
	res, err := tx.ExecContext(ctx, "INSERT INTO forum_posts (topic_id, author_id, author_name, body, created_at) VALUES (?, ?, ?, ?, ?)",
		p.TopicID, nullIfEmpty(p.AuthorID), p.AuthorName, p.Body, p.Created.UTC())
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE forum_topics SET posts = posts + 1, last_post_at = ? WHERE id = ?", p.Created.UTC(), p.TopicID); err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return uint64(id), tx.Commit()
}

// ForumEditPost replaces the text of a message.
func (s *Store) ForumEditPost(ctx context.Context, id uint64, body string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, "UPDATE forum_posts SET body = ?, edited_at = ? WHERE id = ? AND deleted = 0", body, at.UTC(), id)
	return err
}

// ForumDeletePost removes a message's text (the row stays so that the thread keeps its shape).
func (s *Store) ForumDeletePost(ctx context.Context, id uint64) error {
	_, err := s.db.ExecContext(ctx, "UPDATE forum_posts SET deleted = 1, body = '' WHERE id = ?", id)
	return err
}

// ForumDeleteTopic removes a thread with all its messages.
func (s *Store) ForumDeleteTopic(ctx context.Context, id uint64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM forum_topics WHERE id = ?", id)
	return err
}

// ForumSetFlag sets "pinned" or "locked" on a thread.
func (s *Store) ForumSetFlag(ctx context.Context, id uint64, flag string, on bool) error {
	col := "locked"
	if flag == "pinned" {
		col = "pinned"
	}
	_, err := s.db.ExecContext(ctx, "UPDATE forum_topics SET "+col+" = ? WHERE id = ?", on, id)
	return err
}

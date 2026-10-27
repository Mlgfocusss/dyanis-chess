// Package forum is the forum of the site: a fixed set of categories, topics in them and messages in
// the topics. Anybody reads; a signed-in person (a guest too, with tighter limits) writes. The text is
// plain: the page shows it as text and never as markup.
//
// There is no moderation staff beyond the people named in FORUM_ADMINS (user ids, comma-separated),
// who may delete any message or topic, pin a topic and close it. Everybody else can edit their own
// message for half an hour and delete it.
package forum

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"dyanis-chess/backend/internal/store"
)

// Category is a section of the forum.
type Category struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Categories are the sections, in the order the front page lists them.
var Categories = []Category{
	{"general", "Общий разговор", "Всё о шахматах и о сайте, что не подошло в другие разделы."},
	{"games", "Партии и разбор", "Разберём вашу партию, обсудим интересные позиции и ошибки."},
	{"openings", "Дебюты", "Репертуар, теория, ловушки и вопросы по конструктору дебютов."},
	{"puzzles", "Задачи и тактика", "Трудные задачи, штурм, дуэли и приёмы."},
	{"endgames", "Эндшпиль и стратегия", "Типовые окончания, план игры, структуры."},
	{"site", "Сайт: помощь и предложения", "Вопросы, ошибки и идеи по самому сайту."},
	{"offtopic", "Не о шахматах", "Разговоры обо всём остальном, в рамках приличия."},
}

func categoryByID(id string) (Category, bool) {
	for _, c := range Categories {
		if c.ID == id {
			return c, true
		}
	}
	return Category{}, false
}

// Limits.
const (
	TopicsPerPage = 30
	PostsPerPage  = 20
	MinTitleRunes = 3
	MaxTitleRunes = 120
	MaxBodyRunes  = 5000
	EditWindow    = 30 * time.Minute
)

// Store is the part of *store.Store the service uses.
type Store interface {
	PublicUsers(ctx context.Context, ids []string) (map[string]*store.PublicUser, error)
	ForumStats(ctx context.Context) (map[string]store.ForumCategoryStat, error)
	ForumTopics(ctx context.Context, category string, limit, offset int) ([]*store.ForumTopic, int, error)
	ForumTopicByID(ctx context.Context, id uint64) (*store.ForumTopic, error)
	ForumPosts(ctx context.Context, topicID uint64, limit, offset int) ([]*store.ForumPost, error)
	ForumPostByID(ctx context.Context, id uint64) (*store.ForumPost, error)
	ForumCreateTopic(ctx context.Context, t *store.ForumTopic, body string) (uint64, error)
	ForumAddPost(ctx context.Context, p *store.ForumPost) (uint64, error)
	ForumEditPost(ctx context.Context, id uint64, body string, at time.Time) error
	ForumDeletePost(ctx context.Context, id uint64) error
	ForumDeleteTopic(ctx context.Context, id uint64) error
	ForumSetFlag(ctx context.Context, id uint64, flag string, on bool) error

	// Blocks: whose messages the reader has hidden.
	BlockedIDs(ctx context.Context, blockerID string) (map[string]bool, error)

	// Reports (reports.go).
	PublicUser(ctx context.Context, id string) (*store.PublicUser, error)
	MessageByID(ctx context.Context, id uint64) (*store.MessageRow, error)
	CreateReport(ctx context.Context, r *store.ComplaintRow) (uint64, error)
	Reports(ctx context.Context, status string, limit int) ([]*store.ComplaintRow, error)
	Report(ctx context.Context, id uint64) (*store.ComplaintRow, error)
	CountOpenReports(ctx context.Context) (int, error)
	CloseReports(ctx context.Context, kind, targetID, by, action string, at time.Time) (int, error)
}

// Config of a Service.
type Config struct {
	Store Store
	// User returns the signed-in user of a request, or nil.
	User func(r *http.Request) *store.User
	// Admins are the ids of the people who moderate.
	Admins []string
	Now    func() time.Time
}

// Service serves the forum API.
type Service struct {
	cfg    Config
	admins map[string]bool

	mu   sync.Mutex
	hits map[string][]time.Time
}

// New returns a Service.
func New(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	s := &Service{cfg: cfg, admins: map[string]bool{}, hits: map[string][]time.Time{}}
	for _, id := range cfg.Admins {
		s.admins[strings.TrimSpace(id)] = true
	}
	return s
}

// Handler returns the routes of the package.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/forum/categories", s.categories)
	mux.HandleFunc("GET /api/forum/topics", s.topics)
	mux.HandleFunc("POST /api/forum/topics", s.createTopic)
	mux.HandleFunc("GET /api/forum/topics/{id}", s.topic)
	mux.HandleFunc("DELETE /api/forum/topics/{id}", s.deleteTopic)
	mux.HandleFunc("POST /api/forum/topics/{id}/posts", s.reply)
	mux.HandleFunc("POST /api/forum/topics/{id}/pin", s.flag("pinned"))
	mux.HandleFunc("POST /api/forum/topics/{id}/lock", s.flag("locked"))
	mux.HandleFunc("PUT /api/forum/posts/{id}", s.editPost)
	mux.HandleFunc("DELETE /api/forum/posts/{id}", s.deletePost)
	mux.HandleFunc("POST /api/reports", s.report)
	mux.HandleFunc("GET /api/reports", s.reports)
	mux.HandleFunc("POST /api/reports/{id}/close", s.closeReport)
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func serverError(w http.ResponseWriter, what string, err error) {
	log.Printf("forum: %s: %v", what, err)
	writeError(w, http.StatusInternalServerError, "не удалось выполнить запрос")
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "ожидался JSON")
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32*1024)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "не удалось прочитать запрос")
		return false
	}
	return true
}

func (s *Service) user(r *http.Request) *store.User {
	if s.cfg.User == nil {
		return nil
	}
	return s.cfg.User(r)
}

// writer returns the signed-in user or writes the answer.
func (s *Service) writer(w http.ResponseWriter, r *http.Request) *store.User {
	u := s.user(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "чтобы писать на форуме, войдите или продолжите как гость")
	}
	return u
}

func (s *Service) allow(key string, n int, window time.Duration) bool {
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.hits[key][:0]
	for _, t := range s.hits[key] {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= n {
		s.hits[key] = kept
		return false
	}
	s.hits[key] = append(kept, now)
	return true
}

func (s *Service) isAdmin(u *store.User) bool { return u != nil && s.admins[u.ID] }

// cleanBody trims a text, drops control characters (a line break stays, with at most one empty line in
// a row) and reports how many runes it has.
func cleanBody(s string, max int) (string, int) {
	var b strings.Builder
	n, nl := 0, 0
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r == '\n' || r == '\r':
			if nl++; nl > 2 {
				continue
			}
			r = '\n'
		case r == '\t':
			r, nl = ' ', 0
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || r == 0x2028 || r == 0x2029:
			continue
		default:
			nl = 0
		}
		b.WriteRune(r)
		if n++; n >= max {
			break
		}
	}
	out := strings.TrimSpace(b.String())
	return out, len([]rune(out))
}

func cleanTitle(s string) (string, int) {
	t := strings.Join(strings.Fields(s), " ")
	var b strings.Builder
	for _, r := range t {
		if r >= 0x20 && r != 0x7f && !(r >= 0x80 && r < 0xa0) {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if rs := []rune(out); len(rs) > MaxTitleRunes {
		out = string(rs[:MaxTitleRunes])
	}
	return out, len([]rune(out))
}

// ------------------------------------------------------------------ reading

// Author is how the writer of a topic or message is shown.
type Author struct {
	ID        string `json:"id,omitempty"` // empty when the account is gone
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	Country   string `json:"country,omitempty"`
	Gone      bool   `json:"gone,omitempty"`
}

func authors(users map[string]*store.PublicUser, id, stored string) Author {
	if u, ok := users[id]; ok && id != "" {
		return Author{ID: u.ID, Name: u.Name, AvatarURL: u.AvatarURL, Country: u.Country}
	}
	return Author{Name: stored, Gone: true}
}

// CategoryJSON is a section with its numbers.
type CategoryJSON struct {
	Category
	Topics   int   `json:"topics"`
	Posts    int   `json:"posts"`
	LastPost int64 `json:"lastPost,omitempty"`
}

func (s *Service) categories(w http.ResponseWriter, r *http.Request) {
	stats, err := s.cfg.Store.ForumStats(r.Context())
	if err != nil {
		serverError(w, "stats", err)
		return
	}
	out := make([]CategoryJSON, 0, len(Categories))
	for _, c := range Categories {
		j := CategoryJSON{Category: c}
		if st, ok := stats[c.ID]; ok {
			j.Topics, j.Posts, j.LastPost = st.Topics, st.Posts, st.LastPost.UnixMilli()
		}
		out = append(out, j)
	}
	res := map[string]any{"categories": out}
	if me := s.user(r); s.isAdmin(me) { // only moderators are told, and how many complaints wait
		res["moderator"] = true
		if n, err := s.cfg.Store.CountOpenReports(r.Context()); err == nil {
			res["openReports"] = n
		}
	}
	writeJSON(w, http.StatusOK, res)
}

// TopicJSON is a topic in a list or at the head of a page.
type TopicJSON struct {
	ID       uint64 `json:"id"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Author   Author `json:"author"`
	Posts    int    `json:"posts"`
	Created  int64  `json:"created"`
	LastPost int64  `json:"lastPost"`
	Pinned   bool   `json:"pinned,omitempty"`
	Locked   bool   `json:"locked,omitempty"`
}

func pageParam(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func pagesOf(total, per int) int {
	if total <= 0 {
		return 1
	}
	return (total + per - 1) / per
}

func (s *Service) topics(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	if category != "" {
		if _, ok := categoryByID(category); !ok {
			writeError(w, http.StatusBadRequest, "неизвестный раздел")
			return
		}
	}
	page := pageParam(r)
	ctx := r.Context()
	rows, total, err := s.cfg.Store.ForumTopics(ctx, category, TopicsPerPage, (page-1)*TopicsPerPage)
	if err != nil {
		serverError(w, "topics", err)
		return
	}
	var ids []string
	for _, t := range rows {
		if t.AuthorID != "" {
			ids = append(ids, t.AuthorID)
		}
	}
	users, err := s.cfg.Store.PublicUsers(ctx, ids)
	if err != nil {
		serverError(w, "authors", err)
		return
	}
	out := make([]TopicJSON, 0, len(rows))
	for _, t := range rows {
		out = append(out, topicJSON(t, users))
	}
	writeJSON(w, http.StatusOK, map[string]any{"topics": out, "page": page, "pages": pagesOf(total, TopicsPerPage), "total": total})
}

func topicJSON(t *store.ForumTopic, users map[string]*store.PublicUser) TopicJSON {
	return TopicJSON{ID: t.ID, Category: t.Category, Title: t.Title, Author: authors(users, t.AuthorID, t.AuthorName), Posts: t.Posts,
		Created: t.Created.UnixMilli(), LastPost: t.LastPost.UnixMilli(), Pinned: t.Pinned, Locked: t.Locked}
}

// PostJSON is a message of a topic.
type PostJSON struct {
	ID        uint64 `json:"id"`
	N         int    `json:"n"` // the number in the topic, from 1
	Author    Author `json:"author"`
	Body      string `json:"body"`
	Created   int64  `json:"created"`
	Edited    int64  `json:"edited,omitempty"`
	Deleted   bool   `json:"deleted,omitempty"`
	CanEdit   bool   `json:"canEdit,omitempty"`
	CanDelete bool   `json:"canDelete,omitempty"`
	// BlockedByMe: the reader has blocked the author; the page hides the text until asked.
	BlockedByMe bool `json:"blockedByMe,omitempty"`
}

func topicID(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "тема не найдена")
		return 0, false
	}
	return id, true
}

func (s *Service) topic(w http.ResponseWriter, r *http.Request) {
	id, ok := topicID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	t, err := s.cfg.Store.ForumTopicByID(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "тема не найдена")
		return
	}
	if err != nil {
		serverError(w, "topic", err)
		return
	}
	page := pageParam(r)
	posts, err := s.cfg.Store.ForumPosts(ctx, id, PostsPerPage, (page-1)*PostsPerPage)
	if err != nil {
		serverError(w, "posts", err)
		return
	}
	ids := []string{}
	for _, p := range posts {
		if p.AuthorID != "" {
			ids = append(ids, p.AuthorID)
		}
	}
	if t.AuthorID != "" {
		ids = append(ids, t.AuthorID)
	}
	users, err := s.cfg.Store.PublicUsers(ctx, ids)
	if err != nil {
		serverError(w, "authors", err)
		return
	}
	me := s.user(r)
	admin, now := s.isAdmin(me), s.cfg.Now()
	out := make([]PostJSON, 0, len(posts))
	for i, p := range posts {
		j := PostJSON{ID: p.ID, N: (page-1)*PostsPerPage + i + 1, Author: authors(users, p.AuthorID, p.AuthorName), Body: p.Body,
			Created: p.Created.UnixMilli(), Deleted: p.Deleted}
		if !p.Edited.IsZero() {
			j.Edited = p.Edited.UnixMilli()
		}
		if me != nil && !p.Deleted {
			own := p.AuthorID == me.ID
			j.CanEdit = own && now.Sub(p.Created) < EditWindow
			j.CanDelete = own || admin
		}
		out = append(out, j)
	}
	// The reader's own blocks: the page hides those messages behind "Показать" (the text is still sent).
	if me != nil {
		blocked, err := s.cfg.Store.BlockedIDs(ctx, me.ID)
		if err != nil {
			serverError(w, "blocked", err)
			return
		}
		for i := range out {
			out[i].BlockedByMe = blocked[out[i].Author.ID] && out[i].Author.ID != ""
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"topic": topicJSON(t, users), "posts": out, "page": page, "pages": pagesOf(t.Posts, PostsPerPage),
		"canPost": me != nil && !t.Locked, "moderator": admin,
		"canDeleteTopic": me != nil && (admin || (t.AuthorID == me.ID && t.Posts == 1)),
	})
}

// ------------------------------------------------------------------ writing

// limits applies the tighter limits to guests; false (with the answer written) if over.
func (s *Service) limits(w http.ResponseWriter, u *store.User, topic bool) bool {
	guest := u.Kind == store.KindGuest
	switch {
	case topic && guest && !s.allow("topic:"+u.ID, 2, time.Hour):
		writeError(w, http.StatusTooManyRequests, "гость может открыть не больше двух тем в час; войдите через Google, чтобы писать больше")
	case topic && !s.allow("topic2:"+u.ID, 6, time.Hour):
		writeError(w, http.StatusTooManyRequests, "слишком много новых тем, подождите")
	case !topic && guest && !s.allow("post:"+u.ID, 4, time.Minute):
		writeError(w, http.StatusTooManyRequests, "не так быстро")
	case !topic && !s.allow("post2:"+u.ID, 10, time.Minute):
		writeError(w, http.StatusTooManyRequests, "не так быстро")
	default:
		return true
	}
	return false
}

func (s *Service) createTopic(w http.ResponseWriter, r *http.Request) {
	u := s.writer(w, r)
	if u == nil {
		return
	}
	var req struct {
		Category string `json:"category"`
		Title    string `json:"title"`
		Body     string `json:"body"`
	}
	if !decode(w, r, &req) {
		return
	}
	if _, ok := categoryByID(req.Category); !ok {
		writeError(w, http.StatusBadRequest, "выберите раздел")
		return
	}
	title, tn := cleanTitle(req.Title)
	if tn < MinTitleRunes {
		writeError(w, http.StatusBadRequest, "заголовок слишком короткий")
		return
	}
	body, bn := cleanBody(req.Body, MaxBodyRunes)
	if bn < 2 {
		writeError(w, http.StatusBadRequest, "напишите текст сообщения")
		return
	}
	if !s.limits(w, u, true) {
		return
	}
	t := &store.ForumTopic{Category: req.Category, AuthorID: u.ID, AuthorName: u.Name, Title: title, Created: s.cfg.Now()}
	id, err := s.cfg.Store.ForumCreateTopic(r.Context(), t, body)
	if err != nil {
		serverError(w, "create topic", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]uint64{"id": id})
}

func (s *Service) reply(w http.ResponseWriter, r *http.Request) {
	u := s.writer(w, r)
	if u == nil {
		return
	}
	id, ok := topicID(w, r)
	if !ok {
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &req) {
		return
	}
	body, bn := cleanBody(req.Body, MaxBodyRunes)
	if bn < 1 {
		writeError(w, http.StatusBadRequest, "напишите текст сообщения")
		return
	}
	if !s.limits(w, u, false) {
		return
	}
	pid, err := s.cfg.Store.ForumAddPost(r.Context(), &store.ForumPost{TopicID: id, AuthorID: u.ID, AuthorName: u.Name, Body: body, Created: s.cfg.Now()})
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "тема не найдена")
	case errors.Is(err, store.ErrLocked):
		writeError(w, http.StatusConflict, "тема закрыта")
	case err != nil:
		serverError(w, "reply", err)
	default:
		writeJSON(w, http.StatusCreated, map[string]uint64{"id": pid})
	}
}

func (s *Service) postOf(w http.ResponseWriter, r *http.Request) *store.ForumPost {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "сообщение не найдено")
		return nil
	}
	p, err := s.cfg.Store.ForumPostByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "сообщение не найдено")
		return nil
	}
	if err != nil {
		serverError(w, "post", err)
		return nil
	}
	return p
}

func (s *Service) editPost(w http.ResponseWriter, r *http.Request) {
	u := s.writer(w, r)
	if u == nil {
		return
	}
	p := s.postOf(w, r)
	if p == nil {
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &req) {
		return
	}
	now := s.cfg.Now()
	if p.AuthorID != u.ID || p.Deleted {
		writeError(w, http.StatusForbidden, "изменять можно только своё сообщение")
		return
	}
	if now.Sub(p.Created) >= EditWindow {
		writeError(w, http.StatusForbidden, "сообщение можно изменять только полчаса")
		return
	}
	body, bn := cleanBody(req.Body, MaxBodyRunes)
	if bn < 1 {
		writeError(w, http.StatusBadRequest, "напишите текст сообщения")
		return
	}
	if err := s.cfg.Store.ForumEditPost(r.Context(), p.ID, body, now); err != nil {
		serverError(w, "edit post", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"body": body})
}

func (s *Service) deletePost(w http.ResponseWriter, r *http.Request) {
	u := s.writer(w, r)
	if u == nil {
		return
	}
	p := s.postOf(w, r)
	if p == nil {
		return
	}
	if p.AuthorID != u.ID && !s.isAdmin(u) {
		writeError(w, http.StatusForbidden, "удалить можно только своё сообщение")
		return
	}
	if err := s.cfg.Store.ForumDeletePost(r.Context(), p.ID); err != nil {
		serverError(w, "delete post", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// deleteTopic removes a topic: a moderator any, the author their own while nobody has answered.
func (s *Service) deleteTopic(w http.ResponseWriter, r *http.Request) {
	u := s.writer(w, r)
	if u == nil {
		return
	}
	id, ok := topicID(w, r)
	if !ok {
		return
	}
	t, err := s.cfg.Store.ForumTopicByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "тема не найдена")
		return
	}
	if err != nil {
		serverError(w, "topic", err)
		return
	}
	if !s.isAdmin(u) && !(t.AuthorID == u.ID && t.Posts == 1) {
		writeError(w, http.StatusForbidden, "тему может удалить автор, пока в ней нет ответов, или модератор")
		return
	}
	if err := s.cfg.Store.ForumDeleteTopic(r.Context(), id); err != nil {
		serverError(w, "delete topic", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// flag handles pin and lock: a moderator sets {"on": true|false}.
func (s *Service) flag(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := s.writer(w, r)
		if u == nil {
			return
		}
		if !s.isAdmin(u) {
			writeError(w, http.StatusForbidden, "это может делать только модератор")
			return
		}
		id, ok := topicID(w, r)
		if !ok {
			return
		}
		var req struct {
			On bool `json:"on"`
		}
		if !decode(w, r, &req) {
			return
		}
		if err := s.cfg.Store.ForumSetFlag(r.Context(), id, name, req.On); err != nil {
			serverError(w, "flag", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"on": req.On})
	}
}

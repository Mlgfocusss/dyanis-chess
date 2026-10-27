package forum

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"dyanis-chess/backend/internal/store"
)

// The tests need a real MySQL: point DYANIS_TEST_DSN at one.
type env struct {
	t        *testing.T
	st       *store.Store
	h        http.Handler
	now      time.Time
	accounts *sync.Map
	admin    *store.User
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dsn := os.Getenv("DYANIS_TEST_DSN")
	if dsn == "" {
		t.Skip("DYANIS_TEST_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := store.Open(ctx, dsn, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	e := &env{t: t, st: st, now: time.Now().UTC().Truncate(time.Millisecond), accounts: &sync.Map{}}
	e.admin = e.user("Модератор", store.KindGoogle)
	svc := New(Config{
		Store:  st,
		Admins: []string{e.admin.ID},
		User: func(r *http.Request) *store.User {
			if v, ok := e.accounts.Load(r.Header.Get("X-Test-User")); ok {
				return v.(*store.User)
			}
			return nil
		},
		Now: func() time.Time { return e.now },
	})
	e.h = svc.Handler()
	return e
}

func (e *env) user(name, kind string) *store.User {
	e.t.Helper()
	u := &store.User{ID: fmt.Sprintf("fm%014d", time.Now().UnixNano()%1e14), Kind: kind, Name: name}
	if kind == store.KindGoogle {
		u.GoogleSub = "sub-" + u.ID
	}
	if err := e.st.CreateUser(context.Background(), u); err != nil {
		e.t.Fatal(err)
	}
	e.accounts.Store(u.ID, u)
	e.t.Cleanup(func() { _ = e.st.DeleteUser(context.Background(), u.ID) })
	time.Sleep(2 * time.Millisecond)
	return u
}

func (e *env) call(u *store.User, method, path string, body any, out any) int {
	e.t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if u != nil {
		req.Header.Set("X-Test-User", u.ID)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if out != nil && rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			e.t.Fatalf("%s %s: bad JSON %q: %v", method, path, rec.Body.String(), err)
		}
	}
	return rec.Code
}

type topicPage struct {
	Topic          TopicJSON  `json:"topic"`
	Posts          []PostJSON `json:"posts"`
	Pages          int        `json:"pages"`
	CanPost        bool       `json:"canPost"`
	Moderator      bool       `json:"moderator"`
	CanDeleteTopic bool       `json:"canDeleteTopic"`
}

func (e *env) newTopic(u *store.User, category, title, body string) uint64 {
	e.t.Helper()
	var made map[string]uint64
	if code := e.call(u, "POST", "/api/forum/topics", map[string]string{"category": category, "title": title, "body": body}, &made); code != 201 {
		e.t.Fatalf("new topic: %d", code)
	}
	id := made["id"]
	e.t.Cleanup(func() { _ = e.st.ForumDeleteTopic(context.Background(), id) }) // a topic outlives its author, so it is removed here
	return id
}

func TestCategoriesAreFixed(t *testing.T) {
	e := newEnv(t)
	var out struct{ Categories []CategoryJSON }
	if code := e.call(nil, "GET", "/api/forum/categories", nil, &out); code != 200 || len(out.Categories) != len(Categories) {
		t.Fatalf("categories: %d %+v", code, out)
	}
	seen := map[string]bool{}
	for _, c := range out.Categories {
		if c.ID == "" || c.Name == "" || seen[c.ID] {
			t.Errorf("bad category %+v", c)
		}
		seen[c.ID] = true
	}
}

func TestTopicsAndReplies(t *testing.T) {
	e := newEnv(t)
	a, b := e.user("Аня", store.KindGoogle), e.user("Борис", store.KindGuest)

	if code := e.call(nil, "POST", "/api/forum/topics", map[string]string{"category": "general", "title": "Привет", "body": "Всем привет"}, nil); code != 401 {
		t.Errorf("nobody signed in writes: %d", code)
	}
	for name, body := range map[string]map[string]string{
		"unknown category": {"category": "weather", "title": "Привет", "body": "текст текст"},
		"short title":      {"category": "general", "title": "Hi", "body": "текст текст"},
		"empty text":       {"category": "general", "title": "Привет", "body": "   "},
	} {
		if code := e.call(a, "POST", "/api/forum/topics", body, nil); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}

	id := e.newTopic(a, "openings", "  Сицилианская   защита\x00 ", "Какой вариант выбрать?\n\n\n\n<script>alert(1)</script>")
	var page topicPage
	if code := e.call(nil, "GET", fmt.Sprintf("/api/forum/topics/%d", id), nil, &page); code != 200 {
		t.Fatalf("topic: %d", code)
	}
	if page.Topic.Title != "Сицилианская защита" || page.Topic.Posts != 1 || len(page.Posts) != 1 || page.Posts[0].Author.ID != a.ID || page.CanPost {
		t.Fatalf("topic page: %+v", page)
	}
	if strings.Contains(page.Posts[0].Body, "\n\n\n") || !strings.Contains(page.Posts[0].Body, "<script>alert(1)</script>") {
		t.Errorf("empty lines are trimmed, the text stays as text: %q", page.Posts[0].Body)
	}

	if code := e.call(b, "POST", fmt.Sprintf("/api/forum/topics/%d/posts", id), map[string]string{"body": "Играйте Найдорфа"}, nil); code != 201 {
		t.Fatalf("reply: %d", code)
	}
	e.call(b, "GET", fmt.Sprintf("/api/forum/topics/%d", id), nil, &page)
	if page.Topic.Posts != 2 || len(page.Posts) != 2 || page.Posts[1].N != 2 || !page.CanPost || !page.Posts[1].CanEdit || page.Posts[0].CanEdit || page.Posts[0].CanDelete {
		t.Fatalf("after the reply: %+v", page)
	}

	var list struct {
		Topics []TopicJSON
		Total  int
	}
	e.call(nil, "GET", "/api/forum/topics?category=openings", nil, &list)
	found := false
	for _, tp := range list.Topics {
		found = found || tp.ID == id
	}
	if !found {
		t.Errorf("the topic is in its category: %+v", list)
	}
	e.call(nil, "GET", "/api/forum/topics?category=puzzles", nil, &list)
	for _, tp := range list.Topics {
		if tp.ID == id {
			t.Error("the topic is in the wrong category")
		}
	}
	if code := e.call(nil, "GET", "/api/forum/topics?category=nope", nil, nil); code != 400 {
		t.Errorf("unknown category in the list: %d", code)
	}
	if code := e.call(nil, "GET", "/api/forum/topics/999999999", nil, nil); code != 404 {
		t.Errorf("no such topic: %d", code)
	}
}

func TestEditAndDelete(t *testing.T) {
	e := newEnv(t)
	a, b := e.user("Аня", store.KindGoogle), e.user("Борис", store.KindGoogle)
	id := e.newTopic(a, "general", "Тема про всё", "Первое сообщение")
	var made map[string]uint64
	e.call(b, "POST", fmt.Sprintf("/api/forum/topics/%d/posts", id), map[string]string{"body": "Ответ"}, &made)
	post := fmt.Sprintf("/api/forum/posts/%d", made["id"])

	if code := e.call(a, "PUT", post, map[string]string{"body": "чужое"}, nil); code != 403 {
		t.Errorf("editing someone else's message: %d", code)
	}
	if code := e.call(a, "DELETE", post, nil, nil); code != 403 {
		t.Errorf("deleting someone else's message: %d", code)
	}
	var edited map[string]string
	if code := e.call(b, "PUT", post, map[string]string{"body": "Ответ, исправленный"}, &edited); code != 200 || edited["body"] != "Ответ, исправленный" {
		t.Fatalf("edit: %d %v", code, edited)
	}
	var page topicPage
	e.call(nil, "GET", fmt.Sprintf("/api/forum/topics/%d", id), nil, &page)
	if page.Posts[1].Edited == 0 || page.Posts[1].Body != "Ответ, исправленный" {
		t.Errorf("shown as edited: %+v", page.Posts[1])
	}

	e.now = e.now.Add(EditWindow + time.Minute)
	if code := e.call(b, "PUT", post, map[string]string{"body": "поздно"}, nil); code != 403 {
		t.Errorf("editing after the window: %d", code)
	}
	if code := e.call(b, "DELETE", post, nil, nil); code != 200 {
		t.Errorf("the author deletes: %d", code)
	}
	e.call(nil, "GET", fmt.Sprintf("/api/forum/topics/%d", id), nil, &page)
	if !page.Posts[1].Deleted || page.Posts[1].Body != "" || len(page.Posts) != 2 {
		t.Errorf("the row stays so that the numbers do: %+v", page.Posts[1])
	}

	// A topic: the author can remove it while nobody has answered; a moderator always.
	e.call(a, "GET", fmt.Sprintf("/api/forum/topics/%d", id), nil, &page)
	if page.CanDeleteTopic {
		t.Error("the author cannot delete a topic that has answers")
	}
	if code := e.call(a, "DELETE", fmt.Sprintf("/api/forum/topics/%d", id), nil, nil); code != 403 {
		t.Errorf("author deletes a topic with answers: %d", code)
	}
	if code := e.call(e.admin, "DELETE", fmt.Sprintf("/api/forum/topics/%d", id), nil, nil); code != 200 {
		t.Errorf("moderator deletes: %d", code)
	}
	if code := e.call(nil, "GET", fmt.Sprintf("/api/forum/topics/%d", id), nil, nil); code != 404 {
		t.Errorf("deleted topic: %d", code)
	}
	lonely := e.newTopic(a, "general", "Одинокая тема", "Никто не ответил")
	if code := e.call(a, "DELETE", fmt.Sprintf("/api/forum/topics/%d", lonely), nil, nil); code != 200 {
		t.Errorf("the author deletes an unanswered topic: %d", code)
	}
}

func TestModeratorPinAndLock(t *testing.T) {
	e := newEnv(t)
	a, b := e.user("Аня", store.KindGoogle), e.user("Борис", store.KindGoogle)
	first := e.newTopic(a, "site", "Первая тема", "Текст первой")
	e.now = e.now.Add(time.Minute)
	second := e.newTopic(a, "site", "Вторая тема", "Текст второй")

	if code := e.call(a, "POST", fmt.Sprintf("/api/forum/topics/%d/pin", first), map[string]bool{"on": true}, nil); code != 403 {
		t.Errorf("a user pins: %d", code)
	}
	if code := e.call(e.admin, "POST", fmt.Sprintf("/api/forum/topics/%d/pin", first), map[string]bool{"on": true}, nil); code != 200 {
		t.Fatalf("moderator pins: %d", code)
	}
	var list struct{ Topics []TopicJSON }
	e.call(nil, "GET", "/api/forum/topics?category=site", nil, &list)
	if len(list.Topics) < 2 || list.Topics[0].ID != first || !list.Topics[0].Pinned {
		t.Errorf("pinned goes first: %+v", list.Topics)
	}
	_ = second

	e.call(e.admin, "POST", fmt.Sprintf("/api/forum/topics/%d/lock", first), map[string]bool{"on": true}, nil)
	if code := e.call(b, "POST", fmt.Sprintf("/api/forum/topics/%d/posts", first), map[string]string{"body": "можно?"}, nil); code != 409 {
		t.Errorf("a reply to a locked topic: %d", code)
	}
	var page topicPage
	e.call(b, "GET", fmt.Sprintf("/api/forum/topics/%d", first), nil, &page)
	if page.CanPost || !page.Topic.Locked {
		t.Errorf("locked: %+v", page)
	}
}

func TestGuestsAreLimitedMore(t *testing.T) {
	e := newEnv(t)
	g := e.user("Гость 1", store.KindGuest)
	for i := 0; i < 2; i++ {
		e.newTopic(g, "offtopic", fmt.Sprintf("Тема гостя %d", i), "Просто текст")
	}
	if code := e.call(g, "POST", "/api/forum/topics", map[string]string{"category": "offtopic", "title": "Третья тема", "body": "ещё текст"}, nil); code != 429 {
		t.Errorf("a third topic of a guest in an hour: %d", code)
	}
	reg := e.user("Зарегистрированный", store.KindGoogle)
	for i := 0; i < 3; i++ {
		e.newTopic(reg, "offtopic", fmt.Sprintf("Тема %d", i), "Просто текст")
	}
}

func TestCleanBody(t *testing.T) {
	got, n := cleanBody("  а\x07б\r\n\r\n\r\n\r\nв\tг  ", 100)
	if got != "аб\n\nв г" || n != 7 {
		t.Errorf("clean: %q %d", got, n)
	}
	if got, _ = cleanBody(strings.Repeat("я", 50), 10); len([]rune(got)) != 10 {
		t.Errorf("cut: %q", got)
	}
	if got, n = cleanTitle("  много    пробелов\x00  "); got != "много пробелов" || n != 14 {
		t.Errorf("title: %q %d", got, n)
	}
}

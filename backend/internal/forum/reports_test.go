package forum

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"dyanis-chess/backend/internal/store"
)

// report files a complaint and returns the answer; the report is removed when the test ends.
func (e *env) report(u *store.User, kind, target, reason, note string) (int, uint64, string) {
	e.t.Helper()
	var out struct {
		ID    uint64 `json:"id"`
		Error string `json:"error"`
	}
	code := e.call(u, "POST", "/api/reports", map[string]string{"kind": kind, "targetId": target, "reason": reason, "note": note}, &out)
	if out.ID > 0 {
		id := out.ID
		e.t.Cleanup(func() { _ = e.st.DeleteReport(context.Background(), id) })
	}
	return code, out.ID, out.Error
}

func (e *env) queue(u *store.User, status string) map[uint64]ReportJSON {
	e.t.Helper()
	var out struct{ Items []ReportJSON }
	if code := e.call(u, "GET", "/api/reports?status="+status, nil, &out); code != 200 {
		e.t.Fatalf("queue: %d", code)
	}
	m := map[uint64]ReportJSON{}
	for _, r := range out.Items {
		m[r.ID] = r
	}
	return m
}

func TestReportsOnForumMessages(t *testing.T) {
	e := newEnv(t)
	author, other, third := e.user("Автор", store.KindGoogle), e.user("Жалобщик", store.KindGuest), e.user("Третий", store.KindGuest)
	topic := e.newTopic(author, "general", "Спорная тема", "Первое сообщение автора")
	var made map[string]uint64
	e.call(author, "POST", fmt.Sprintf("/api/forum/topics/%d/posts", topic), map[string]string{"body": "Купите дёшево!!!"}, &made)
	post := fmt.Sprint(made["id"])

	if code, _, _ := e.report(nil, "forum_post", post, "spam", ""); code != 401 {
		t.Errorf("not signed in: %d", code)
	}
	if code, _, _ := e.report(other, "wall", post, "spam", ""); code != 400 {
		t.Errorf("unknown kind: %d", code)
	}
	if code, _, _ := e.report(other, "forum_post", post, "boredom", ""); code != 400 {
		t.Errorf("unknown reason: %d", code)
	}
	if code, _, _ := e.report(other, "forum_post", "99999999", "spam", ""); code != 404 {
		t.Errorf("no such message: %d", code)
	}
	if code, _, msg := e.report(author, "forum_post", post, "spam", ""); code != 400 || !strings.Contains(msg, "на себя") {
		t.Errorf("a complaint on oneself: %d %s", code, msg)
	}
	if code, _, _ := e.report(author, "forum_topic", fmt.Sprint(topic), "spam", ""); code != 400 {
		t.Errorf("a complaint on one's own topic: %d", code)
	}

	long := strings.Repeat("ж", 400)
	code, id, msg := e.report(other, "forum_post", post, "spam", long)
	if code != 201 || id == 0 {
		t.Fatalf("report: %d %s", code, msg)
	}
	if code, _, _ := e.report(other, "forum_post", post, "abuse", ""); code != 409 {
		t.Errorf("a second open report on the same message by the same person: %d", code)
	}
	code, id2, _ := e.report(third, "forum_post", post, "abuse", "")
	if code != 201 {
		t.Fatalf("another person reports the same message: %d", code)
	}

	// The author deletes the message; the report keeps its text.
	if code := e.call(author, "DELETE", "/api/forum/posts/"+post, nil, nil); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	if code, _, _ := e.report(third, "forum_post", post, "spam", ""); code != 404 {
		t.Errorf("a deleted message cannot be reported any more: %d", code)
	}
	if code := e.call(other, "GET", "/api/reports", nil, nil); code != 403 {
		t.Errorf("a non-moderator reads the queue: %d", code)
	}
	if code := e.call(nil, "GET", "/api/reports", nil, nil); code != 401 {
		t.Errorf("nobody reads the queue: %d", code)
	}
	open := e.queue(e.admin, "open")
	r := open[id]
	if r.Snapshot != "Купите дёшево!!!" || r.Kind != "forum_post" || r.Reason != "spam" || r.Reporter.ID != other.ID || r.Target.ID != author.ID || r.Target.Name != "Автор" || r.TopicID != topic {
		t.Fatalf("the queue shows %+v", r)
	}
	if n := len([]rune(r.Note)); n != MaxReportNoteRunes {
		t.Errorf("the note is cut to %d, got %d", MaxReportNoteRunes, n)
	}
	if _, ok := open[id2]; !ok {
		t.Error("the second report is not in the queue")
	}

	// Closing needs a moderator; closing closes the other reports on the same message too.
	closeURL := func(id uint64) string { return fmt.Sprintf("/api/reports/%d/close", id) }
	if code := e.call(other, "POST", closeURL(id), map[string]string{"action": "none"}, nil); code != 403 {
		t.Errorf("a non-moderator closes: %d", code)
	}
	if code := e.call(e.admin, "POST", closeURL(id), map[string]string{"action": "ban"}, nil); code != 400 {
		t.Errorf("unknown action: %d", code)
	}
	var closed struct{ Closed int }
	if code := e.call(e.admin, "POST", closeURL(id), map[string]string{"action": "none"}, &closed); code != 200 || closed.Closed != 2 {
		t.Fatalf("close: %d %+v", code, closed)
	}
	if code := e.call(e.admin, "POST", closeURL(id), map[string]string{"action": "none"}, nil); code != 409 {
		t.Errorf("closing twice: %d", code)
	}
	if _, ok := e.queue(e.admin, "open")[id]; ok {
		t.Error("a closed report is still open")
	}
	if got := e.queue(e.admin, "closed")[id2]; got.Status != "closed" || got.Action != "none" {
		t.Errorf("closed list: %+v", got)
	}
	// Once the old one is closed the same person may complain again (of something else, here: the topic).
	if code, _, _ := e.report(other, "forum_topic", fmt.Sprint(topic), "abuse", ""); code != 201 {
		t.Errorf("a new report: %d", code)
	}
}

func TestModeratorActsOnReports(t *testing.T) {
	e := newEnv(t)
	author, other := e.user("Хулиган", store.KindGuest), e.user("Свидетель", store.KindGuest)
	topic := e.newTopic(author, "offtopic", "Плохая тема", "Начало плохой темы")
	var made map[string]uint64
	e.call(author, "POST", fmt.Sprintf("/api/forum/topics/%d/posts", topic), map[string]string{"body": "Оскорбление"}, &made)
	post := fmt.Sprint(made["id"])
	closeURL := func(id uint64) string { return fmt.Sprintf("/api/reports/%d/close", id) }

	// "lock" shuts the topic of the reported message, "delete" removes the message (its text stays in the report).
	_, id, _ := e.report(other, "forum_post", post, "abuse", "")
	if code := e.call(e.admin, "POST", closeURL(id), map[string]string{"action": "lock"}, nil); code != 200 {
		t.Fatalf("lock: %d", code)
	}
	var page topicPage
	e.call(nil, "GET", fmt.Sprintf("/api/forum/topics/%d", topic), nil, &page)
	if !page.Topic.Locked {
		t.Error("the topic is not locked")
	}
	third := e.user("Ещё свидетель", store.KindGuest)
	_, id3, _ := e.report(third, "forum_post", post, "abuse", "")
	if code := e.call(e.admin, "POST", closeURL(id3), map[string]string{"action": "delete"}, nil); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	e.call(nil, "GET", fmt.Sprintf("/api/forum/topics/%d", topic), nil, &page)
	if !page.Posts[1].Deleted || page.Posts[1].Body != "" {
		t.Errorf("the message is still there: %+v", page.Posts[1])
	}
	if got := e.queue(e.admin, "closed")[id3]; got.Snapshot != "Оскорбление" || got.Action != "delete" {
		t.Errorf("the evidence: %+v", got)
	}

	// A report on a player: snapshot is the name; "delete" and "lock" make no sense there.
	code, uid, _ := e.report(other, "user", author.ID, "cheating", "")
	if code != 201 {
		t.Fatalf("report on a user: %d", code)
	}
	if code, _, _ := e.report(other, "user", other.ID, "cheating", ""); code != 400 {
		t.Errorf("a complaint on oneself: %d", code)
	}
	if code, _, _ := e.report(other, "user", "nobody-here", "cheating", ""); code != 404 {
		t.Errorf("a complaint on a player who is not there: %d", code)
	}
	for _, a := range []string{"delete", "lock"} {
		if code := e.call(e.admin, "POST", closeURL(uid), map[string]string{"action": a}, nil); code != 400 {
			t.Errorf("%s on a player: %d", a, code)
		}
	}
	if code := e.call(e.admin, "POST", closeURL(uid), map[string]string{"action": "none"}, nil); code != 200 {
		t.Errorf("close: %d", code)
	}

	// A complaint on a topic copies the title and the first message; "delete" removes the topic.
	t2 := e.newTopic(author, "general", "Реклама казино", "Заходите на наш сайт")
	_, tid, _ := e.report(other, "forum_topic", fmt.Sprint(t2), "spam", "")
	if got := e.queue(e.admin, "open")[tid]; !strings.Contains(got.Snapshot, "Реклама казино") || !strings.Contains(got.Snapshot, "Заходите на наш сайт") {
		t.Errorf("topic snapshot: %+v", got)
	}
	if code := e.call(e.admin, "POST", closeURL(tid), map[string]string{"action": "delete"}, nil); code != 200 {
		t.Fatalf("delete topic: %d", code)
	}
	if code := e.call(nil, "GET", fmt.Sprintf("/api/forum/topics/%d", t2), nil, nil); code != 404 {
		t.Errorf("the topic is still there: %d", code)
	}

	// The moderator is told by the sections page, others are not.
	var cats map[string]any
	e.call(e.admin, "GET", "/api/forum/categories", nil, &cats)
	if cats["moderator"] != true {
		t.Errorf("moderator flag: %v", cats)
	}
	cats = nil
	e.call(other, "GET", "/api/forum/categories", nil, &cats)
	if _, ok := cats["moderator"]; ok {
		t.Errorf("the flag leaks to a player: %v", cats)
	}
}

func TestReportsOnPrivateMessages(t *testing.T) {
	e := newEnv(t)
	a, b, c := e.user("Пишущий", store.KindGuest), e.user("Читающий", store.KindGuest), e.user("Посторонний", store.KindGuest)
	ctx := context.Background()
	mid, err := e.st.AddMessage(ctx, a.ID, b.ID, "Ты ужасен, уходи", e.now)
	if err != nil {
		t.Fatal(err)
	}
	reply, _ := e.st.AddMessage(ctx, b.ID, a.ID, "Ответ", e.now)
	target := fmt.Sprint(mid)

	if code, _, _ := e.report(c, "message", target, "abuse", ""); code != 404 {
		t.Errorf("a stranger reports a message that is not theirs: %d", code)
	}
	if code, _, _ := e.report(a, "message", target, "abuse", ""); code != 404 {
		t.Errorf("the writer reports their own message: %d", code)
	}
	if code, _, _ := e.report(a, "message", fmt.Sprint(reply), "abuse", ""); code != 201 {
		t.Errorf("the reader of a message reports it: %d", code)
	}
	code, id, _ := e.report(b, "message", target, "abuse", "лично мне")
	if code != 201 {
		t.Fatalf("report: %d", code)
	}
	if code, _, _ := e.report(b, "message", target, "spam", ""); code != 409 {
		t.Errorf("twice: %d", code)
	}
	if err := e.st.DeleteUser(ctx, a.ID); err != nil { // the writer's account goes, the message with it
		t.Fatal(err)
	}
	got := e.queue(e.admin, "open")[id]
	if got.Snapshot != "Ты ужасен, уходи" || got.Target.Name != "Пишущий" || got.Target.ID != "" || got.Reporter.ID != b.ID {
		t.Errorf("the evidence survives the account: %+v", got)
	}
}

func TestReportsPerHour(t *testing.T) {
	e := newEnv(t)
	reporter := e.user("Слишком бдительный", store.KindGuest)
	var targets []*store.User
	for i := 0; i <= ReportsPerHour; i++ {
		targets = append(targets, e.user(fmt.Sprintf("Цель %d", i), store.KindGuest))
	}
	for i := 0; i < ReportsPerHour; i++ {
		if code, _, msg := e.report(reporter, "user", targets[i].ID, "spam", ""); code != 201 {
			t.Fatalf("report %d: %d %s", i, code, msg)
		}
	}
	if code, _, _ := e.report(reporter, "user", targets[ReportsPerHour].ID, "spam", ""); code != 429 {
		t.Errorf("the eleventh in an hour: %d", code)
	}
	e.now = e.now.Add(61 * time.Minute)
	if code, _, _ := e.report(reporter, "user", targets[ReportsPerHour].ID, "spam", ""); code != 201 {
		t.Errorf("an hour later: %d", code)
	}
}

func TestPostsOfBlockedAuthorsAreMarked(t *testing.T) {
	e := newEnv(t)
	reader, noisy, calm := e.user("Читатель", store.KindGoogle), e.user("Шумный", store.KindGuest), e.user("Тихий", store.KindGuest)
	topic := e.newTopic(noisy, "general", "Шумная тема", "Шум-шум")
	e.call(calm, "POST", fmt.Sprintf("/api/forum/topics/%d/posts", topic), map[string]string{"body": "Тишина"}, nil)
	if _, err := e.st.Block(context.Background(), reader.ID, noisy.ID, e.now); err != nil {
		t.Fatal(err)
	}
	var page topicPage
	e.call(reader, "GET", fmt.Sprintf("/api/forum/topics/%d", topic), nil, &page)
	if len(page.Posts) != 2 || !page.Posts[0].BlockedByMe || page.Posts[1].BlockedByMe || page.Posts[0].Body != "Шум-шум" {
		t.Errorf("reader's page: %+v", page.Posts)
	}
	var anon topicPage
	e.call(nil, "GET", fmt.Sprintf("/api/forum/topics/%d", topic), nil, &anon)
	if len(anon.Posts) != 2 || anon.Posts[0].BlockedByMe {
		t.Errorf("anonymous page: %+v", anon.Posts)
	}
}

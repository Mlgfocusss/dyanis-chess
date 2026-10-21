package lessons

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dyanis-chess/backend/internal/store"
)

// memStore keeps progress in memory with the rules of the real table.
type memStore struct {
	mu   sync.Mutex
	rows map[string]map[string]store.LessonProgress
}

func newMemStore() *memStore { return &memStore{rows: map[string]map[string]store.LessonProgress{}} }

func (m *memStore) LessonProgressOf(_ context.Context, userID string) (map[string]store.LessonProgress, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]store.LessonProgress{}
	for k, v := range m.rows[userID] {
		out[k] = v
	}
	return out, nil
}

func (m *memStore) SaveLessonProgress(_ context.Context, userID, lessonID string, step, mistakes int, done bool) (store.LessonProgress, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rows[userID] == nil {
		m.rows[userID] = map[string]store.LessonProgress{}
	}
	old, has := m.rows[userID][lessonID]
	now := time.Now().UTC()
	p := store.LessonProgress{LessonID: lessonID, Step: step, Mistakes: mistakes, UpdatedAt: now}
	if done {
		p.CompletedAt = &now
	}
	if has {
		p.Step = max(old.Step, step)
		if old.CompletedAt != nil {
			p.CompletedAt = old.CompletedAt
		}
	}
	m.rows[userID][lessonID] = p
	return p, nil
}

func newTestService(t *testing.T) (*Service, *memStore, http.Handler) {
	t.Helper()
	st := newMemStore()
	svc := New(Config{
		Store: st,
		UserID: func(r *http.Request) string {
			return r.Header.Get("X-Test-User")
		},
		Library: mustLoad(t),
	})
	return svc, st, svc.Handler()
}

func get(t *testing.T, h http.Handler, path, user string, out any) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if out != nil && rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("%s: %v\n%s", path, err, rec.Body.String())
		}
	}
	return rec
}

func post(t *testing.T, h http.Handler, path, user, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCatalog(t *testing.T) {
	_, _, h := newTestService(t)
	var c Catalog
	if rec := get(t, h, "/api/lessons", "", &c); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if len(c.Sections) != 5 || c.Total < 24 || c.Completed != 0 {
		t.Fatalf("catalog: %d sections, %d lessons, %d done", len(c.Sections), c.Total, c.Completed)
	}
	if c.Next != c.Sections[0].Lessons[0].ID {
		t.Errorf("the lesson to go on with is the first one, got %q", c.Next)
	}
	for _, s := range c.Sections {
		for _, l := range s.Lessons {
			if l.Steps < 5 || l.Exercises < 2 || l.Progress != nil || l.Title == "" || l.Icon == "" {
				t.Errorf("lesson %+v", l)
			}
		}
	}
}

func TestLessonCarriesTheTreeForTheBrowser(t *testing.T) {
	_, _, h := newTestService(t)
	var l LessonJSON
	rec := get(t, h, "/api/lessons/rook", "", &l)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, "\"solutions\"") || strings.Contains(body, "\"claim\"") {
		t.Error("the author fields (solutions, claim) must stay on the server")
	}
	if l.ID != "rook" || l.Section.ID != "basics" || l.Next == nil || l.Next.ID != "bishop" {
		t.Fatalf("lesson: %+v", l)
	}
	var sawDemo, sawTree bool
	for _, st := range l.Steps {
		switch st.Type {
		case TypeText:
			if len(st.Demo) > 0 {
				sawDemo = true
				for _, d := range st.Demo {
					if d.FEN == "" {
						t.Errorf("a demo move has no position: %+v", d)
					}
				}
			}
		case TypeExercise:
			sawTree = true
			if st.Tree == nil || len(st.Tree.Legal) == 0 || len(st.Tree.Moves) == 0 || st.Hint == "" || st.Success == "" {
				t.Errorf("exercise without a tree: %+v", st)
			}
		}
	}
	if !sawDemo || !sawTree {
		t.Errorf("demo %v tree %v", sawDemo, sawTree)
	}
	// the last lesson has no next one
	var last LessonJSON
	get(t, h, "/api/lessons/king-activity", "", &last)
	if last.Next != nil {
		t.Errorf("the last lesson points to %+v", last.Next)
	}
	if rec := get(t, h, "/api/lessons/nope", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown lesson: %d", rec.Code)
	}
}

func TestProgressNeedsAPersonAndSaves(t *testing.T) {
	svc, _, h := newTestService(t)
	if rec := post(t, h, "/api/lessons/rook/progress", "", `{"step":1}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("nobody: %d", rec.Code)
	}
	if rec := post(t, h, "/api/lessons/nope/progress", "u1", `{"step":1}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown lesson: %d", rec.Code)
	}
	total := svc.lib.StepCount("rook")
	for name, body := range map[string]string{
		"negative step":    `{"step":-1}`,
		"step too far":     `{"step":99}`,
		"negative mistake": `{"step":1,"mistakes":-3}`,
		"absurd mistakes":  `{"step":1,"mistakes":100000}`,
		"done too early":   `{"step":1,"done":true}`,
		"not json":         `[`,
	} {
		if rec := post(t, h, "/api/lessons/rook/progress", "u1", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}

	if rec := post(t, h, "/api/lessons/rook/progress", "u1", `{"step":2,"mistakes":1}`); rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
	}
	var l LessonJSON
	get(t, h, "/api/lessons/rook", "u1", &l)
	if l.Progress == nil || l.Progress.Step != 2 || l.Progress.Mistakes != 1 || l.Progress.Completed {
		t.Errorf("progress after two steps: %+v", l.Progress)
	}
	var other LessonJSON
	get(t, h, "/api/lessons/rook", "u2", &other)
	if other.Progress != nil {
		t.Errorf("another person sees progress: %+v", other.Progress)
	}

	// finishing: the last step completes it, with or without the flag
	rec := post(t, h, "/api/lessons/rook/progress", "u1", `{"step":`+itoa(total)+`,"mistakes":2,"done":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("finish: %d %s", rec.Code, rec.Body.String())
	}
	var c Catalog
	get(t, h, "/api/lessons", "u1", &c)
	if c.Completed != 1 || c.Sections[0].Lessons[0].Progress == nil || !c.Sections[0].Lessons[0].Progress.Completed {
		t.Errorf("catalog after finishing: completed %d, %+v", c.Completed, c.Sections[0].Lessons[0].Progress)
	}
	if c.Next != c.Sections[0].Lessons[1].ID {
		t.Errorf("next lesson after the first is done: %q", c.Next)
	}
	// a replay from the start does not undo the completion
	post(t, h, "/api/lessons/rook/progress", "u1", `{"step":1,"mistakes":0}`)
	get(t, h, "/api/lessons", "u1", &c)
	if c.Completed != 1 {
		t.Errorf("a replay undid the completion: %d", c.Completed)
	}
	// the last step alone completes a lesson
	post(t, h, "/api/lessons/bishop/progress", "u1", `{"step":`+itoa(svc.lib.StepCount("bishop"))+`}`)
	get(t, h, "/api/lessons", "u1", &c)
	if c.Completed != 2 {
		t.Errorf("completed %d", c.Completed)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

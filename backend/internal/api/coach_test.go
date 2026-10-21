package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"dyanis-chess/backend/internal/coach"
)

// router plays OpenRouter and keeps the last question it was asked.
type router struct {
	calls  int32
	prompt string
}

func (r *router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	atomic.AddInt32(&r.calls, 1)
	raw, _ := io.ReadAll(req.Body)
	var body struct {
		Messages []struct{ Content string } `json:"messages"`
	}
	_ = json.Unmarshal(raw, &body)
	if len(body.Messages) == 2 {
		r.prompt = body.Messages[1].Content
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "Ход g4 слабый: после него Qh4#."}}}})
}

func postCoach(h http.Handler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/coach", strings.NewReader(body)))
	return rec
}

func TestCoachExplainsAMoveOfAReview(t *testing.T) {
	or := &router{}
	srv := httptest.NewServer(or)
	defer srv.Close()
	s := New(Config{Coach: coach.New(coach.Config{APIKey: "k", BaseURL: srv.URL})})
	defer s.Close()
	h := s.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/coach", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"enabled":true`) {
		t.Errorf("status: %d %s", rec.Code, rec.Body)
	}

	id := create(t, h, jsonBody(foolsMate, `,"depth":5`))
	waitFor(t, h, id, StatusDone)

	rec = postCoach(h, `{"id":"`+id+`","ply":3}`)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var out coachResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Text != "Ход g4 слабый: после него Qh4#." || out.Cached || len(out.BestLine) == 0 || len(out.ReplyLine) == 0 || out.ReplyLine[0] != "Qh4#" || out.BestSAN == "" {
		t.Errorf("response: %+v", out)
	}
	// The model was given the engine's facts, not just the move.
	for _, want := range []string{"2. g4", "Qh4#", "Лучший ход по движку", "Позиция до хода"} {
		if !strings.Contains(or.prompt, want) {
			t.Errorf("the question lacks %q:\n%s", want, or.prompt)
		}
	}
	// Again: the saved note, no new question.
	rec = postCoach(h, `{"id":"`+id+`","ply":3}`)
	var again coachResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &again)
	if !again.Cached || atomic.LoadInt32(&or.calls) != 1 {
		t.Errorf("second ask: %+v, calls %d", again, or.calls)
	}

	for name, body := range map[string]string{
		"no such move":   `{"id":"` + id + `","ply":9}`,
		"ply zero":       `{"id":"` + id + `","ply":0}`,
		"no id":          `{"ply":1}`,
		"not json":       `nope`,
		"unknown review": `{"id":"zzzz","ply":1}`,
	} {
		code := postCoach(h, body).Code
		want := 400
		if name == "unknown review" {
			want = 404
		}
		if code != want {
			t.Errorf("%s: %d, want %d", name, code, want)
		}
	}
}

func TestCoachOff(t *testing.T) {
	s := New(Config{})
	defer s.Close()
	h := s.Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/coach", nil))
	if !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Errorf("status: %s", rec.Body)
	}
	if rec := postCoach(h, `{"id":"x","ply":1}`); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("off: %d", rec.Code)
	}
}

func TestCoachLimitAndFailureAreTold(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(402) }))
	defer bad.Close()
	s := New(Config{Coach: coach.New(coach.Config{APIKey: "k", BaseURL: bad.URL, PerHour: 1})})
	defer s.Close()
	h := s.Handler()
	id := create(t, h, jsonBody(foolsMate, `,"depth":3`))
	waitFor(t, h, id, StatusDone)
	if code := postCoach(h, `{"id":"`+id+`","ply":3}`).Code; code != http.StatusBadGateway {
		t.Errorf("model failure: %d", code)
	}
	// that failed try used the hour's only request: the next is refused before any call
	if code := postCoach(h, `{"id":"`+id+`","ply":3}`).Code; code != http.StatusTooManyRequests {
		t.Errorf("limit: %d", code)
	}
}

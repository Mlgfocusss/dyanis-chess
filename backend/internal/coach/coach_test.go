package coach

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"dyanis-chess/backend/internal/analysis"
	"dyanis-chess/backend/internal/pgn"
)

// The fool's mate: 2.g4 is a blunder that loses at once to 2...Qh4#.
func foolsMate(t *testing.T) *analysis.Report {
	t.Helper()
	games, err := pgn.ParseString("1. f3 e5 2. g4 Qh4#")
	if err != nil || len(games) != 1 {
		t.Fatalf("pgn: %v", err)
	}
	rep, err := analysis.Analyze(games[0], analysis.Options{Depth: 6, HashMB: 4, Workers: 2, NoBook: true}) // the book names 1.f3 e5 2.g4 itself ("Fool's Mate")
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestBuildFactsOfABlunder(t *testing.T) {
	rep := foolsMate(t)
	f, err := BuildFacts(rep, 3) // 2.g4
	if err != nil {
		t.Fatal(err)
	}
	if f.Played != "g4" || f.Mover != "white" || f.MoveNumber != 2 || f.Class != analysis.Blunder {
		t.Errorf("facts: %+v", f)
	}
	if f.BestSAN == "" || f.BestSAN == "g4" || len(f.BestLine) == 0 || f.BestLine[0] != f.BestSAN {
		t.Errorf("best: %q %v", f.BestSAN, f.BestLine)
	}
	if len(f.ReplyLine) == 0 || f.ReplyLine[0] != "Qh4#" {
		t.Errorf("the reply line should start with the mate: %v", f.ReplyLine)
	}
	if f.WinBefore < 30 || f.WinAfter > 5 {
		t.Errorf("chances before %.1f after %.1f", f.WinBefore, f.WinAfter)
	}
	if f.EvalAfter == "" || !strings.Contains(f.EvalAfter, "мат") {
		t.Errorf("eval after = %q", f.EvalAfter)
	}
	if !strings.Contains(f.Position, "Белые: Ke1 Qd1") || !strings.Contains(f.Position, "Ход белых") {
		t.Errorf("position: %s", f.Position)
	}

	// Black's mate is the best move, with no loss: seen from Black.
	m, err := BuildFacts(rep, 4)
	if err != nil {
		t.Fatal(err)
	}
	if m.Mover != "black" || m.Played != "Qh4#" || m.Terminal != "checkmate" || m.MoveNumber != 2 || m.WinAfter < 99 {
		t.Errorf("mate: %+v", m)
	}

	if _, err := BuildFacts(rep, 0); err == nil {
		t.Error("ply 0 has no move")
	}
	if _, err := BuildFacts(rep, 5); err == nil {
		t.Error("ply 5 does not exist")
	}
	if _, err := BuildFacts(nil, 1); err == nil {
		t.Error("no report")
	}
}

func TestPromptHoldsTheFactsAndTheLines(t *testing.T) {
	f := Facts{
		MoveNumber: 12, Mover: "black", Played: "Qxb2", Class: analysis.Blunder, Loss: 31.4,
		WinBefore: 52, WinAfter: 20, EvalBefore: "+0.1", EvalAfter: "-2.9",
		BestSAN: "O-O", BestLine: []string{"O-O", "Nd5", "exd5"},
		ReplyLine:      []string{"Rb1", "Qxa2", "Rxb7"},
		MaterialBefore: 0, MaterialBest: 0, MaterialReply: -3,
		Position: Describe("rnbqkb1r/pp2pppp/5n2/2pp4/2PP4/2N2N2/PP2PPPP/R1BQKB1R b KQkq - 0 4"),
	}
	p := Prompt(f)
	for _, want := range []string{
		"Ход 12, ходят чёрные", "12...Qxb2", "зевок", "до хода 52%, после хода 20%", "потерял 31%",
		"Лучший ход по движку: O-O", "12...O-O 13. Nd5 exd5", // the best line starts with Black's move
		"13. Rb1 Qxa2 14. Rxb7", // the reply starts with White's move of the next number
		"уменьшается: на лёгкую фигуру (коня или слона)",
		"Чёрные:", "Ход чёрных.",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("the prompt lacks %q:\n%s", want, p)
		}
	}
	// A good move gets no "best" section of its own.
	g := Facts{MoveNumber: 1, Mover: "white", Played: "e4", Class: analysis.Book, Book: true, Opening: "King's Pawn Game",
		BestLine: []string{"e4", "e5"}, ReplyLine: []string{"e5", "Nf3"}, EvalBefore: "+0.2", EvalAfter: "+0.3", WinBefore: 52, WinAfter: 53}
	gp := Prompt(g)
	if strings.Contains(gp, "Лучший ход по движку") || !strings.Contains(gp, "совпал с первым ходом лучшей линии") || !strings.Contains(gp, "King's Pawn Game") || !strings.Contains(gp, "1. e4") {
		t.Errorf("good move prompt:\n%s", gp)
	}
	if !strings.Contains(gp, "1...e5 2. Nf3") {
		t.Errorf("the reply to a white move starts with Black's:\n%s", gp)
	}
}

func TestNumberedAndPawns(t *testing.T) {
	if got := Numbered(5, true, []string{"Nf3", "Nc6", "Bb5"}); got != "5. Nf3 Nc6 6. Bb5" {
		t.Errorf("white first: %q", got)
	}
	if got := Numbered(5, false, []string{"Nc6", "Bb5", "a6"}); got != "5...Nc6 6. Bb5 a6" {
		t.Errorf("black first: %q", got)
	}
	for n, want := range map[int]string{1: "1 пешку", 2: "2 пешки", 3: "3 пешки", 5: "5 пешек", 9: "9 пешек", 11: "11 пешек", 21: "21 пешку"} {
		if got := pawns(n); got != want {
			t.Errorf("pawns(%d) = %q, want %q", n, got, want)
		}
	}
	if material("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1") != 0 || material("4k3/8/8/8/8/8/8/R3K3 w - - 0 1") != 5 || material("q3k3/8/8/8/8/8/8/4K3 w - - 0 1") != -9 {
		t.Error("material")
	}
}

func TestClean(t *testing.T) {
	got := Clean("\n## Итог\n**Ход** слабый: `Qxb2` теряет ферзя.\n\n\n- Нужно было O-O.\n")
	want := "Итог\nХод слабый: Qxb2 теряет ферзя.\n\nНужно было O-O."
	if got != want {
		t.Errorf("Clean = %q, want %q", got, want)
	}
}

// fakeRouter plays OpenRouter: it records what it was sent, the models it was asked for in order,
// and answers as told: status maps a model to the HTTP status it gets (200 when absent).
type fakeRouter struct {
	mu     sync.Mutex
	calls  int32
	last   map[string]any
	header http.Header
	models []string
	status map[string]int
	text   string
}

func (f *fakeRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt32(&f.calls, 1)
	raw, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.last = map[string]any{}
	_ = json.Unmarshal(raw, &f.last)
	f.header = r.Header.Clone()
	model, _ := f.last["model"].(string)
	f.models = append(f.models, model)
	st := http.StatusOK
	if c, ok := f.status[model]; ok {
		st = c
	}
	f.mu.Unlock()
	if r.URL.Path != "/api/v1/chat/completions" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(st)
	if st != http.StatusOK {
		_, _ = w.Write([]byte(`{"error":{"message":"nope"}}`))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": f.text}}}})
}

func (f *fakeRouter) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.models...)
}

func newService(t *testing.T, f *fakeRouter, cfg Config) *Service {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	cfg.APIKey = "test-key"
	cfg.BaseURL = srv.URL + "/api/v1"
	return New(cfg)
}

var someFacts = Facts{MoveNumber: 2, Mover: "white", Played: "g4", Class: analysis.Blunder, WinBefore: 50, WinAfter: 0,
	EvalBefore: "+0.2", EvalAfter: "поставлен мат", BestSAN: "d4", BestLine: []string{"d4"}, ReplyLine: []string{"Qh4#"}, Position: "x"}

func TestDefaultModelsAreFree(t *testing.T) {
	if len(DefaultModels) < 2 {
		t.Fatal("a list of one has nothing to fall back to")
	}
	for _, m := range DefaultModels {
		if !strings.HasSuffix(m, ":free") {
			t.Errorf("%s is not a free model: the coach must not spend money unless it is told to", m)
		}
	}
}

func TestExplainAsksTheModelOnceAndRemembers(t *testing.T) {
	f := &fakeRouter{text: "**Ход g4** открывает диагональ.\n\nНужно было играть d4."}
	s := newService(t, f, Config{Site: "http://site.test"})
	n, err := s.Explain(context.Background(), "u1", someFacts)
	if err != nil || n.Cached || n.Text != "Ход g4 открывает диагональ.\n\nНужно было играть d4." {
		t.Fatalf("%+v %v", n, err)
	}
	if f.header.Get("Authorization") != "Bearer test-key" || f.header.Get("HTTP-Referer") != "http://site.test" || f.header.Get("X-Title") == "" {
		t.Errorf("headers: %v", f.header)
	}
	if f.last["model"] != DefaultModels[0] {
		t.Errorf("model = %v, want the first of the list", f.last["model"])
	}
	if r, _ := f.last["reasoning"].(map[string]any); r["enabled"] != false {
		t.Errorf("reasoning is not turned off: %v", f.last["reasoning"])
	}
	msgs, _ := f.last["messages"].([]any)
	if len(msgs) != 2 || !strings.Contains(msgs[1].(map[string]any)["content"].(string), "Qh4#") {
		t.Errorf("messages: %v", f.last["messages"])
	}

	// The same facts: the saved text, no second call, and no use of the hourly count.
	again, err := s.Explain(context.Background(), "u1", someFacts)
	if err != nil || !again.Cached || again.Text != n.Text || atomic.LoadInt32(&f.calls) != 1 {
		t.Errorf("second: %+v %v calls %d", again, err, f.calls)
	}
	// Other facts are another question.
	other := someFacts
	other.Played = "h4"
	if _, err := s.Explain(context.Background(), "u1", other); err != nil || atomic.LoadInt32(&f.calls) != 2 {
		t.Errorf("other facts: %v calls %d", err, f.calls)
	}
}

func TestExplainLimitsEachPerson(t *testing.T) {
	f := &fakeRouter{text: "ok"}
	s := newService(t, f, Config{PerHour: 2})
	for i, p := range []string{"a", "b", "c"} {
		facts := someFacts
		facts.Played = p
		if _, err := s.Explain(context.Background(), "same", facts); (err != nil) != (i == 2) {
			t.Errorf("request %d: %v", i+1, err)
		} else if i == 2 && !errors.Is(err, ErrLimit) {
			t.Errorf("want ErrLimit, got %v", err)
		}
	}
	other := someFacts
	other.Played = "z"
	if _, err := s.Explain(context.Background(), "someone else", other); err != nil {
		t.Errorf("another person has their own count: %v", err)
	}
	// A saved explanation is free even for a person who has used the hour up.
	first := someFacts
	first.Played = "a"
	if n, err := s.Explain(context.Background(), "same", first); err != nil || !n.Cached {
		t.Errorf("saved note: %+v %v", n, err)
	}
}

func TestExplainGoesDownTheListOfModels(t *testing.T) {
	// the first model is rate-limited, the second answers
	f := &fakeRouter{text: "fine", status: map[string]int{"m1": 429}}
	s := newService(t, f, Config{Models: []string{"m1", "m2", "m3"}})
	n, err := s.Explain(context.Background(), "u", someFacts)
	if err != nil || n.Text != "fine" {
		t.Fatalf("%+v %v", n, err)
	}
	if got := f.asked(); strings.Join(got, ",") != "m1,m2" {
		t.Errorf("asked %v, want m1 then m2", got)
	}
	// m1 now rests: the next question starts with m2 and does not wait for m1 again
	other := someFacts
	other.Played = "h4"
	if _, err := s.Explain(context.Background(), "u", other); err != nil {
		t.Fatal(err)
	}
	if got := f.asked(); strings.Join(got, ",") != "m1,m2,m2" {
		t.Errorf("asked %v, want m1,m2,m2", got)
	}

	// every model refuses: an upstream error, and every model was tried once
	down := &fakeRouter{status: map[string]int{"a": 500, "b": 404, "c": 402}}
	d := newService(t, down, Config{Models: []string{"a", "b", "c"}})
	if _, err := d.Explain(context.Background(), "u", someFacts); !errors.Is(err, ErrUpstream) || strings.Join(down.asked(), ",") != "a,b,c" {
		t.Errorf("all down: %v asked %v", err, down.asked())
	}
	// ... and nothing was saved, so the next try asks again (resting models are tried when none is ready)
	if _, err := d.Explain(context.Background(), "u", someFacts); !errors.Is(err, ErrUpstream) || len(down.asked()) != 6 {
		t.Errorf("second try: %v asked %v", err, down.asked())
	}
}

func TestExplainFailures(t *testing.T) {
	// no key: off
	off := New(Config{})
	if off.Enabled() {
		t.Error("no key, yet enabled")
	}
	if _, err := off.Explain(context.Background(), "u", someFacts); !errors.Is(err, ErrDisabled) {
		t.Errorf("off: %v", err)
	}
	var nilSvc *Service
	if nilSvc.Enabled() {
		t.Error("a nil service is off")
	}

	// a refused key ends the walk at once: the other models would refuse it too
	bad := &fakeRouter{status: map[string]int{"a": 401}}
	b := newService(t, bad, Config{Models: []string{"a", "b"}})
	if _, err := b.Explain(context.Background(), "u", someFacts); !errors.Is(err, ErrUpstream) || len(bad.asked()) != 1 {
		t.Errorf("401: %v asked %v", err, bad.asked())
	}
	// an empty answer counts as a failure of that model
	empty := &fakeRouter{text: "  \n "}
	e := newService(t, empty, Config{Models: []string{"a", "b"}})
	if _, err := e.Explain(context.Background(), "u", someFacts); !errors.Is(err, ErrUpstream) || len(empty.asked()) != 2 {
		t.Errorf("empty: %v asked %v", err, empty.asked())
	}
}

type memStore struct {
	mu     sync.Mutex
	m      map[string]string
	models map[string]string
}

func (m *memStore) LoadCoachNote(_ context.Context, h string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.m[h]
	return t, ok, nil
}

func (m *memStore) SaveCoachNote(_ context.Context, h, model, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[h] = text
	m.models[h] = model
	return nil
}

func TestExplainUsesTheStore(t *testing.T) {
	st := &memStore{m: map[string]string{}, models: map[string]string{}}
	f := &fakeRouter{text: "from the model", status: map[string]int{"m1": 429}}
	s := newService(t, f, Config{Store: st, Models: []string{"m1", "m2"}})
	if _, err := s.Explain(context.Background(), "u", someFacts); err != nil || len(st.m) != 1 {
		t.Fatalf("not saved: %v %d", err, len(st.m))
	}
	for _, model := range st.models {
		if model != "m2" {
			t.Errorf("the note should name the model that wrote it (m2), got %q", model)
		}
	}
	// A new service (a restart) with other models finds the note in the store and does not ask:
	// the question, not the model, is what the note is kept under.
	f2 := &fakeRouter{text: "should not be asked"}
	s2 := newService(t, f2, Config{Store: st, Models: []string{"other"}})
	n, err := s2.Explain(context.Background(), "u", someFacts)
	if err != nil || !n.Cached || n.Text != "from the model" || f2.calls != 0 {
		t.Errorf("%+v %v calls %d", n, err, f2.calls)
	}
}

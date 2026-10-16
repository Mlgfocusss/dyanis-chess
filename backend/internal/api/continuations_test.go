package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func postContinuations(t *testing.T, s *Server, body string) (int, continuationsResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/api/continuations", strings.NewReader(body)))
	var out continuationsResponse
	if rec.Code == 200 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, out
}

func find(out continuationsResponse, uci string) *Continuation {
	for i := range out.Moves {
		if out.Moves[i].UCI == uci {
			return &out.Moves[i]
		}
	}
	return nil
}

func TestContinuationsFromTheStart(t *testing.T) {
	s := New(Config{})
	defer s.Close()
	code, out := postContinuations(t, s, `{"fen":"`+startFEN+`"}`)
	if code != 200 || len(out.Moves) == 0 {
		t.Fatalf("%d %+v", code, out)
	}
	// the reference even names the rare first moves, so the list has them all;
	// what tells them apart is how many lines of theory stand behind each move
	for _, want := range []string{"e2e4", "d2d4", "c2c4", "g1f3"} {
		if find(out, want) == nil {
			t.Errorf("%s is missing from %d moves", want, len(out.Moves))
		}
	}
	if e4 := find(out, "e2e4"); e4 == nil || e4.SAN != "e4" || e4.Opening == nil || e4.Opening.Name == "" || e4.Lines < 100 {
		t.Errorf("e4: %+v", e4)
	}
	top := map[string]bool{}
	for _, m := range out.Moves[:4] {
		top[m.UCI] = true
	}
	if !top["e2e4"] || !top["d2d4"] {
		t.Errorf("the best known moves come first: %+v", out.Moves[:4])
	}
	if h4 := find(out, "h2h4"); h4 != nil && h4.Lines >= find(out, "e2e4").Lines/10 {
		t.Errorf("h4 has %d lines against e4's %d", h4.Lines, find(out, "e2e4").Lines)
	}
	for i := 1; i < len(out.Moves); i++ {
		if out.Moves[i].Lines > out.Moves[i-1].Lines {
			t.Fatalf("not sorted by lines at %d", i)
		}
	}
}

func TestContinuationsDeeper(t *testing.T) {
	s := New(Config{})
	defer s.Close()
	// after 1.e4 e5 the book knows 2.Nf3 and 2.Bc4, not 2.a3
	_, out := postContinuations(t, s, `{"fen":"rnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq e6 0 2"}`)
	if !out.Book || find(out, "g1f3") == nil || find(out, "f1c4") == nil {
		t.Errorf("%+v", out)
	}
	if a3 := find(out, "a2a3"); a3 != nil && a3.Lines >= find(out, "g1f3").Lines/10 {
		t.Errorf("2.a3 has %d lines against Nf3's %d", a3.Lines, find(out, "g1f3").Lines)
	}
	// a position far from any theory has no suggestions
	_, out = postContinuations(t, s, `{"fen":"8/8/4k3/8/8/4K3/8/7R w - - 0 1"}`)
	if out.Book || len(out.Moves) != 0 {
		t.Errorf("an endgame: %+v", out)
	}
}

func TestContinuationsBadRequests(t *testing.T) {
	s := New(Config{})
	defer s.Close()
	if code, _ := postContinuations(t, s, `{"fen":"nonsense"}`); code != 400 {
		t.Errorf("bad fen: %d", code)
	}
	if code, _ := postContinuations(t, s, `not json`); code != 400 {
		t.Errorf("bad json: %d", code)
	}
}

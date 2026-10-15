package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"dyanis-chess/backend/internal/analysis"
	"dyanis-chess/engine"
)

const startFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"

func postLine(t *testing.T, s *Server, body string) (int, lineResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/api/line", strings.NewReader(body)))
	var out lineResponse
	if rec.Code == 200 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, out
}

func TestLinePlaysMovesInUCIAndSAN(t *testing.T) {
	s := New(Config{})
	defer s.Close()
	code, out := postLine(t, s, `{"fen":"`+startFEN+`","moves":["e2e4","e5","Nf3"]}`)
	if code != 200 || len(out.Steps) != 3 {
		t.Fatalf("%d %+v", code, out)
	}
	if out.Steps[0].SAN != "e4" || out.Steps[1].UCI != "e7e5" || out.Steps[2].SAN != "Nf3" {
		t.Errorf("steps: %+v", out.Steps)
	}
	if !strings.Contains(out.FEN, " b ") || out.Steps[2].FEN != out.FEN {
		t.Errorf("final fen %s", out.FEN)
	}
	if len(out.Legal) < 20 || out.Status != "" || out.Eval != nil {
		t.Errorf("legal %d, status %q, eval %v", len(out.Legal), out.Status, out.Eval)
	}
	// no moves: just the legal moves of the position
	if _, out := postLine(t, s, `{"fen":"`+startFEN+`"}`); len(out.Legal) != 20 || len(out.Steps) != 0 {
		t.Errorf("start position: %+v", out)
	}
}

func TestLineMateAndErrors(t *testing.T) {
	s := New(Config{})
	defer s.Close()
	_, out := postLine(t, s, `{"fen":"`+startFEN+`","moves":["f3","e5","g4","Qh4"]}`)
	if out.Status != "checkmate" || len(out.Legal) != 0 {
		t.Errorf("fool's mate: %+v", out)
	}
	for name, body := range map[string]string{
		"illegal move": `{"fen":"` + startFEN + `","moves":["e2e5"]}`,
		"bad fen":      `{"fen":"nonsense","moves":[]}`,
		"bad json":     `not json`,
		"too long":     `{"fen":"` + startFEN + `","moves":[` + strings.Repeat(`"Nf3",`, 41) + `"Nf3"]}`,
	} {
		if code, _ := postLine(t, s, body); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
}

func TestLineEval(t *testing.T) {
	var gotDepth int
	s := New(Config{
		EvalPosition: func(p *engine.Position, _ []uint64, o analysis.Options) analysis.PositionEval {
			gotDepth = o.Depth
			return analysis.PositionEval{FEN: p.FEN(), Score: 35, WinPct: 55, BestUCI: "g1f3"}
		},
		DefaultDepth: 9, MaxDepth: 12,
	})
	defer s.Close()
	_, out := postLine(t, s, `{"fen":"`+startFEN+`","moves":["e4"],"eval":true}`)
	if out.Eval == nil || out.Eval.Score != 35 || gotDepth != 9 {
		t.Errorf("eval %+v depth %d", out.Eval, gotDepth)
	}
	postLine(t, s, `{"fen":"`+startFEN+`","eval":true,"depth":50}`)
	if gotDepth != 12 {
		t.Errorf("depth is capped at MaxDepth: %d", gotDepth)
	}
}

func TestLineEvalWithTheRealEngine(t *testing.T) {
	s := New(Config{})
	defer s.Close()
	_, out := postLine(t, s, `{"fen":"`+startFEN+`","moves":["e4"],"eval":true,"depth":4}`)
	if out.Eval == nil || out.Eval.BestUCI == "" || out.Eval.FEN != out.FEN {
		t.Errorf("%+v", out.Eval)
	}
}

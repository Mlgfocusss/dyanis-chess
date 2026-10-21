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

func TestLineLines(t *testing.T) {
	var gotDepth, gotN int
	s := New(Config{
		EvalLines: func(p *engine.Position, _ []uint64, o analysis.Options, n int) ([]analysis.LineEval, error) {
			gotDepth, gotN = o.Depth, n
			return []analysis.LineEval{{Rank: 1, BestSAN: "e5", PV: []string{"e5", "Nf3"}}, {Rank: 2, BestSAN: "c5", PV: []string{"c5"}}}, nil
		},
		DefaultDepth: 18, MaxDepth: 20,
	})
	defer s.Close()
	_, out := postLine(t, s, `{"fen":"`+startFEN+`","moves":["e4"],"lines":3}`)
	if len(out.Lines) != 2 || out.Lines[0].BestSAN != "e5" || gotN != 3 || out.Eval != nil {
		t.Errorf("lines %+v n %d eval %v", out.Lines, gotN, out.Eval)
	}
	// without a depth the lines use the shorter default: each one is a search of its own
	if gotDepth != 14 {
		t.Errorf("default depth of lines = %d, want 14", gotDepth)
	}
	postLine(t, s, `{"fen":"`+startFEN+`","lines":99,"depth":30}`)
	if gotN != 3 || gotDepth != 20 {
		t.Errorf("lines capped at 3 and depth at MaxDepth: n %d depth %d", gotN, gotDepth)
	}
	// both at once
	_, both := postLine(t, s, `{"fen":"`+startFEN+`","eval":true,"lines":2,"depth":3}`)
	if both.Eval == nil || len(both.Lines) != 2 {
		t.Errorf("eval and lines together: %+v", both)
	}
}

func TestLineLinesWithTheRealEngine(t *testing.T) {
	s := New(Config{})
	defer s.Close()
	_, out := postLine(t, s, `{"fen":"`+startFEN+`","lines":3,"depth":5}`)
	if len(out.Lines) != 3 || out.Lines[0].BestUCI == out.Lines[1].BestUCI || out.Lines[2].Rank != 3 {
		t.Errorf("%+v", out.Lines)
	}
}

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dyanis-chess/backend/internal/variants"
)

// a game of Chess960 setup 518 with a castling by each side (the king takes its rook)
var castlingGame = []string{"e2e4", "e7e5", "g1f3", "b8c6", "f1c4", "g8f6", "e1h1", "f8e7", "d2d3", "e8h8"}

func variantCfg(src *VariantSource, err error) Config {
	return Config{
		DefaultDepth: 3,
		VariantGame: func(_ context.Context, id string) (*VariantSource, error) {
			if id != "g1" {
				return nil, ErrNoVariantGame
			}
			return src, err
		},
	}
}

func postVariant(h http.Handler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/analyses/variant", strings.NewReader(body)))
	return rec
}

func chessSource(status string, actions []string) *VariantSource {
	return &VariantSource{
		Variant: "chess960", Status: status, Result: "1-0", Actions: actions, White: "Анна", Black: "Борис",
		Options: variants.Options{Seed: 1, Params: map[string]any{"position": 518, "number": 518}},
	}
}

// A finished Chess960 game is reviewed through the same queue and store, found again by a
// second click and after a restart, and its report says it is Chess960 and has no theory.
func TestVariantReview(t *testing.T) {
	db := &memStore{}
	cfg := variantCfg(chessSource("ended", castlingGame), nil)
	cfg.Store = db
	s := New(cfg)
	h := s.Handler()
	rec := postVariant(h, `{"game":"g1"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body)
	}
	id := strings.Split(rec.Body.String(), `"`)[3]
	j := waitFor(t, h, id, StatusDone)
	if j.Game.Variant != "chess960" || j.Game.White != "Анна" || !strings.Contains(j.Game.Event, "518") {
		t.Errorf("game info %+v", j.Game)
	}
	if j.Report == nil || j.Report.Variant != "chess960" || len(j.Report.Moves) != len(castlingGame) || len(j.Report.Evals) != len(castlingGame)+1 {
		t.Fatalf("report %+v", j.Report)
	}
	if j.Report.Moves[6].SAN != "O-O" || j.Report.Moves[9].SAN != "O-O" || j.Report.BookPlies != 0 || j.Report.Opening != nil {
		t.Errorf("castling %q %q, book %d", j.Report.Moves[6].SAN, j.Report.Moves[7].SAN, j.Report.BookPlies)
	}
	// a second click, by anybody, finds the same review
	if again := postVariant(h, `{"game":"g1"}`); !strings.Contains(again.Body.String(), id) || again.Code != http.StatusAccepted {
		t.Errorf("second click: %d %s", again.Code, again.Body)
	}
	s.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := db.LoadAnalysis(context.Background(), id); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never stored")
		}
		time.Sleep(5 * time.Millisecond)
	}
	s2 := New(cfg)
	defer s2.Close()
	code, got := getJob(t, s2.Handler(), id)
	if code != 200 || got.Report == nil || got.Report.Variant != "chess960" || got.Game.Variant != "chess960" {
		t.Fatalf("after a restart: %d %+v", code, got)
	}
	if again := postVariant(s2.Handler(), `{"game":"g1"}`); !strings.Contains(again.Body.String(), id) {
		t.Errorf("click after a restart made another review: %s", again.Body)
	}
}

func TestVariantReviewRefusals(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		body string
		code int
	}{
		{"no hub", Config{}, `{"game":"g1"}`, 404},
		{"bad json", variantCfg(chessSource("ended", castlingGame), nil), `nonsense`, 400},
		{"no game id", variantCfg(chessSource("ended", castlingGame), nil), `{}`, 400},
		{"unknown game", variantCfg(chessSource("ended", castlingGame), nil), `{"game":"zzz"}`, 404},
		{"still playing", variantCfg(chessSource("playing", castlingGame), nil), `{"game":"g1"}`, 409},
		{"no moves", variantCfg(chessSource("ended", nil), nil), `{"game":"g1"}`, 400},
		{"other variant", variantCfg(&VariantSource{Variant: "atomic", Status: "ended", Actions: []string{"e2e4"}}, nil), `{"game":"g1"}`, 400},
		{"not a game of this variant", variantCfg(chessSource("ended", []string{"e2e5"}), nil), `{"game":"g1"}`, 400},
	}
	for _, c := range cases {
		s := New(c.cfg)
		rec := postVariant(s.Handler(), c.body)
		s.Close()
		if rec.Code != c.code {
			t.Errorf("%s: %d %s, want %d", c.name, rec.Code, rec.Body, c.code)
		}
	}
}

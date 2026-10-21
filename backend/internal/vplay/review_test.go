package vplay

import (
	"context"
	"errors"
	"testing"
	"time"

	"dyanis-chess/backend/internal/variants"
)

// ReviewGame gives the review the variant, the options with the seed (secret to clients) and the actions.
func TestReviewGame(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_800_000_000, 0)}
	h := New(Config{User: testUser, Now: clk.now})
	defer h.Close()
	v, opts, err := variants.Prepare("chess960", map[string]any{"position": 518}, 77)
	if err != nil {
		t.Fatal(err)
	}
	g, err := newGame("00000000000000a1", clk.now, v, opts, TimeControl{}, Player{ID: "anna", Name: "Anna"}, White)
	if err != nil {
		t.Fatal(err)
	}
	h.register(g)
	if _, err := h.ReviewGame(context.Background(), g.ID); err != nil {
		t.Fatal(err)
	}
	if err := g.Join(Player{ID: "boris", Name: "Boris"}); err != nil {
		t.Fatal(err)
	}
	if err := g.Act("anna", "e2e4"); err != nil {
		t.Fatal(err)
	}
	if err := g.Act("boris", "e7e5"); err != nil {
		t.Fatal(err)
	}
	rg, err := h.ReviewGame(context.Background(), g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rg.Variant != "chess960" || rg.Status != StatusPlaying || rg.White != "Anna" || rg.Black != "Boris" ||
		len(rg.Actions) != 2 || rg.Actions[0] != "e2e4" || rg.Options.Seed != 77 || rg.Options.Params["number"] != 518 {
		t.Errorf("%+v", rg)
	}
	if _, err := h.ReviewGame(context.Background(), "ffffffffffffffff"); !errors.Is(err, ErrNoGame) {
		t.Errorf("unknown game: %v", err)
	}
	if _, err := h.ReviewGame(context.Background(), "not-an-id"); !errors.Is(err, ErrNoGame) {
		t.Errorf("bad id: %v", err)
	}
}

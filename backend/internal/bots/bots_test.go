package bots

import (
	"context"
	"math/rand"
	"strings"
	"testing"
	"time"

	"dyanis-chess/engine"
)

func TestRegistry(t *testing.T) {
	seen := map[string]bool{}
	cats := map[string]bool{}
	for _, c := range Categories {
		cats[c.ID] = true
	}
	for _, b := range All() {
		if seen[b.ID] {
			t.Errorf("duplicate id %s", b.ID)
		}
		seen[b.ID] = true
		if !cats[b.Category] {
			t.Errorf("%s: unknown category %q", b.ID, b.Category)
		}
		if b.Name == "" || b.Desc == "" || b.Greeting == "" || b.OnWin == "" || b.OnLose == "" {
			t.Errorf("%s: missing text", b.ID)
		}
		if b.Elo < 100 || b.Elo > 3000 || b.Points <= 0 {
			t.Errorf("%s: Elo %d, points %d", b.ID, b.Elo, b.Points)
		}
		if b.Params.Depth == 0 && b.Params.MoveTime <= 0 {
			t.Errorf("%s: no depth and no time", b.ID)
		}
		if got, ok := Get(b.ID); !ok || got.Name != b.Name {
			t.Errorf("Get(%s) = %v, %v", b.ID, got, ok)
		}
	}
	if _, ok := Get("nobody"); ok {
		t.Error("an unknown bot was found")
	}
	if len(seen) < 8 || len(seen) > 14 {
		t.Errorf("%d bots; the list is meant to be short", len(seen))
	}

	// Inside a category the weakest comes first, and the categories keep their order.
	all := All()
	order := map[string]int{}
	for i, c := range Categories {
		order[c.ID] = i
	}
	for i := 1; i < len(all); i++ {
		a, b := all[i-1], all[i]
		if order[a.Category] > order[b.Category] || (a.Category == b.Category && a.Elo > b.Elo) {
			t.Errorf("%s (%s, %d) is listed before %s (%s, %d)", a.ID, a.Category, a.Elo, b.ID, b.Category, b.Elo)
		}
	}
	// A bot beaten earns more the stronger it is.
	byElo := append([]Bot(nil), all...)
	for i := range byElo {
		for j := i + 1; j < len(byElo); j++ {
			if byElo[j].Elo < byElo[i].Elo {
				byElo[i], byElo[j] = byElo[j], byElo[i]
			}
		}
	}
	for i := 1; i < len(byElo); i++ {
		if byElo[i].Points < byElo[i-1].Points {
			t.Errorf("%s (%d Elo) is worth %d points, less than %s (%d Elo) with %d",
				byElo[i].ID, byElo[i].Elo, byElo[i].Points, byElo[i-1].ID, byElo[i-1].Elo, byElo[i-1].Points)
		}
	}
}

func TestEveryBotPlaysALegalMove(t *testing.T) {
	positions := []string{
		engine.StartFEN,
		"r1bqkb1r/pppp1ppp/2n2n2/4p2Q/2B1P3/8/PPPP1PPP/RNB1K1NR w KQkq - 4 4", // mate in one is available
		"4k3/8/8/8/8/8/4P3/4K3 w - - 0 1",
	}
	rng := rand.New(rand.NewSource(1))
	for _, b := range All() {
		b := b
		t.Run(b.ID, func(t *testing.T) {
			for _, fen := range positions {
				pos, err := engine.ParseFEN(fen)
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				mv, err := b.Choose(ctx, pos, []uint64{pos.Hash()}, 0, 0, rng)
				cancel()
				if err != nil {
					t.Fatalf("%s: %v", fen, err)
				}
				legal := false
				for _, m := range pos.LegalMoves() {
					if m == mv {
						legal = true
					}
				}
				if !legal {
					t.Errorf("%s: %s is not legal in %s", b.ID, mv, fen)
				}
			}
		})
	}
}

// The bots with a real search see a mate in one.
func TestStrongerBotsTakeAMateInOne(t *testing.T) {
	pos, _ := engine.ParseFEN("r1bqkb1r/pppp1ppp/2n2n2/4p2Q/2B1P3/8/PPPP1PPP/RNB1K1NR w KQkq - 4 4")
	rng := rand.New(rand.NewSource(3))
	for _, id := range []string{"timur", "anna", "artem", "igor", "viktor", "sofia"} {
		b, _ := Get(id)
		hits := 0
		for i := 0; i < 10; i++ {
			mv, err := b.Choose(context.Background(), pos, nil, 0, 0, rng)
			if err != nil {
				t.Fatal(err)
			}
			if mv.String() == "h5f7" {
				hits++
			}
		}
		// Even a patient bot plays the mate nearly always: it is worth 5000 points of score.
		if hits < 8 {
			t.Errorf("%s found Qxf7# only %d times out of 10", id, hits)
		}
	}
}

func TestTheWeakestBlundersMoreThanTheStrong(t *testing.T) {
	// A queen is attacked by a pawn; count how often each bot leaves it there.
	pos, _ := engine.ParseFEN("rnb1kbnr/pppp1ppp/8/4p3/3q4/2P5/PP1PPPPP/RNBQKBNR w KQkq - 0 1") // c3xd4 wins the queen
	take := func(id string) int {
		b, _ := Get(id)
		rng := rand.New(rand.NewSource(5))
		n := 0
		for i := 0; i < 40; i++ {
			mv, _ := b.Choose(context.Background(), pos, nil, 0, 0, rng)
			if strings.HasSuffix(mv.String(), "d4") && strings.HasPrefix(mv.String(), "c3") {
				n++
			}
		}
		return n
	}
	weak, strong := take("misha"), take("viktor")
	if strong < 38 {
		t.Errorf("a strong bot took the free queen %d times out of 40", strong)
	}
	if weak >= strong {
		t.Errorf("the weakest bot took the queen %d times, the strong one %d: no difference in play", weak, strong)
	}
}

func TestOneLegalMoveIsPlayedAtOnce(t *testing.T) {
	pos, _ := engine.ParseFEN("k7/8/1K6/8/8/8/8/R7 b - - 0 1") // only Kb8
	b, _ := Get("max")
	start := time.Now()
	mv, err := b.Choose(context.Background(), pos, nil, 0, 0, rand.New(rand.NewSource(1)))
	if err != nil || mv.String() != "a8b8" {
		t.Fatalf("%v %v", mv, err)
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Errorf("a forced move took %v", time.Since(start))
	}
}

func TestChooseStopsWhenTheContextIsDone(t *testing.T) {
	// occupy every search slot, then ask for a move with a context that expires
	for i := 0; i < cap(semaphore); i++ {
		semaphore <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(semaphore); i++ {
			<-semaphore
		}
	}()
	pos := engine.NewPosition()
	b, _ := Get("misha")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := b.Choose(ctx, pos, nil, 0, 0, rand.New(rand.NewSource(1))); err == nil {
		t.Error("expected the wait for a free slot to give up")
	}
}

// The first bot has not been taught checkmate: a mate in one is just one more move to it.
func TestTheFirstBotDoesNotKnowMate(t *testing.T) {
	pos, _ := engine.ParseFEN("r1bqkb1r/pppp1ppp/2n2n2/4p2Q/2B1P3/8/PPPP1PPP/RNB1K1NR w KQkq - 4 4")
	b, _ := Get("misha")
	if !b.Params.NoMate {
		t.Fatal("misha should be blind to mate")
	}
	rng := rand.New(rand.NewSource(3))
	hits := 0
	for i := 0; i < 60; i++ {
		mv, err := b.Choose(context.Background(), pos, nil, 0, 0, rng)
		if err != nil {
			t.Fatal(err)
		}
		if mv.String() == "h5f7" {
			hits++
		}
	}
	if hits > 15 {
		t.Errorf("misha found Qxf7# %d times out of 60: it still sees the mate", hits)
	}
}

// The strongest bot plays the opening books while they know the position, so its first move comes at
// once instead of after its half-second search.
func TestMaxPlaysFromBooks(t *testing.T) {
	n, err := engine.LoadBooks("../../../engine/assets/gm2001.bin", "../../../engine/assets/komodo.bin", "../../../engine/assets/performance.bin")
	if n == 0 {
		t.Skipf("no opening books: %v", err)
	}
	t.Cleanup(func() { engine.LoadBooks() })
	max, ok := Get("max")
	if !ok || !max.Params.Book {
		t.Fatal("max has no book")
	}
	pos := engine.NewPosition()
	started := time.Now()
	mv, err := max.Choose(context.Background(), pos, []uint64{pos.Hash()}, 0, 0, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(started); d > 200*time.Millisecond {
		t.Errorf("book move took %v", d)
	}
	legal := false
	for _, m := range pos.LegalMoves() {
		legal = legal || m == mv
	}
	if !legal {
		t.Errorf("%v is not legal", mv)
	}
	if other, _ := Get("sofia"); other.Params.Book {
		t.Error("only the strongest bot uses the books")
	}
}

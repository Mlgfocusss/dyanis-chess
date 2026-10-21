package variants

import (
	"math/rand"
	"sort"
	"strings"
	"testing"
)

func newCrazyhouse(t *testing.T) *Game {
	t.Helper()
	st, err := pocketVariant(t, "crazyhouse").New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	return st.(*Game)
}

func pocketVariant(t *testing.T, id string) Variant {
	t.Helper()
	v, ok := Get(id)
	if !ok {
		t.Fatalf("variant %s is not registered", id)
	}
	return v
}

func pocketSetup(g *Game, fen string, pockets map[Side]map[Kind]int8) {
	g.Edit(func(p *Position) {
		*p = *mustFEN(fen)
		for s, m := range pockets {
			for k, n := range m {
				p.Pocket[s][k] = n
			}
		}
	})
}

func dropActions(actions []string) []string {
	var out []string
	for _, a := range actions {
		if strings.Contains(a, "@") {
			out = append(out, a)
		}
	}
	sort.Strings(out)
	return out
}

func TestCrazyhouseCaptureFillsPocket(t *testing.T) {
	g := newCrazyhouse(t)
	for _, a := range strings.Fields("e2e4 d7d5 e4d5") {
		if _, err := g.Apply(a); err != nil {
			t.Fatal(err)
		}
	}
	e := g.View(White).Extra.(PocketExtra)
	if e.White["P"] != 1 || len(e.Black) != 0 {
		t.Fatalf("pockets after exd5: %+v", e)
	}
	if got := g.Position().Pocket[White][Pawn]; got != 1 {
		t.Fatalf("pocket count %d", got)
	}
	if len(dropActions(g.Legal())) != 0 {
		t.Error("it is Black's turn and Black holds nothing")
	}
	g.Apply("g8f6")
	drops := dropActions(g.Legal())
	if len(drops) == 0 {
		t.Fatal("no drops for White, who holds a pawn")
	}
	for _, d := range drops {
		if !strings.HasPrefix(d, "P@") || strings.HasSuffix(d, "1") || strings.HasSuffix(d, "8") {
			t.Fatalf("a pawn drop on the first or last rank, or a drop of something not held: %s", d)
		}
	}
	if !contains(drops, "P@e2") || !contains(drops, "P@a3") || !contains(drops, "P@e4") || contains(drops, "P@d5") {
		t.Errorf("wrong set of drops: %v", drops)
	}
	note, err := g.Apply("P@e3")
	if err != nil || note != "P@e3" {
		t.Fatalf("drop: %q %v", note, err)
	}
	if g.Position().Pocket[White][Pawn] != 0 || g.Position().B[MakeSq(4, 2)] != (Piece{Kind: Pawn, Side: White}) {
		t.Error("the pawn did not leave the pocket for e3")
	}
	if v := g.View(NoSide); len(v.Last) != 1 || v.Last[0] != "e3" {
		t.Errorf("last of a drop: %v", v.Last)
	}
}

func TestCrazyhousePromotedPieceBecomesAPawn(t *testing.T) {
	g := newCrazyhouse(t)
	pocketSetup(g, "1r2k3/P7/8/8/8/8/8/4K3 w - - 0 1", nil)
	if _, err := g.Apply("a7a8q"); err != nil {
		t.Fatal(err)
	}
	if !g.Position().B[MakeSq(0, 7)].Promoted {
		t.Fatal("the new queen is not marked as promoted")
	}
	if e := g.View(White).Extra.(PocketExtra); len(e.Promoted) != 1 || e.Promoted[0] != "a8" {
		t.Errorf("promoted squares: %v", e.Promoted)
	}
	if _, err := g.Apply("b8a8"); err != nil { // the rook takes the queen
		t.Fatal(err)
	}
	black := g.Position().Pocket[Black]
	if black[Pawn] != 1 || black[Queen] != 0 {
		t.Errorf("a promoted queen must come back as a pawn, Black holds %v", black)
	}
	// a piece dropped from the pocket is not a promoted one
	pocketSetup(g, "4k3/8/8/8/8/8/8/4K3 w - - 0 1", map[Side]map[Kind]int8{White: {Queen: 1}})
	g.Apply("Q@d4")
	if g.Position().B[MakeSq(3, 3)].Promoted {
		t.Error("a dropped queen is not a promoted one")
	}
}

func TestCrazyhouseEnPassantFillsPocket(t *testing.T) {
	g := newCrazyhouse(t)
	pocketSetup(g, "4k3/3p4/8/4P3/8/8/8/4K3 b - - 0 1", nil)
	g.Apply("d7d5")
	if _, err := g.Apply("e5d6"); err != nil {
		t.Fatal(err)
	}
	if g.Position().Pocket[White][Pawn] != 1 {
		t.Error("en passant must give the pawn to the pocket")
	}
}

func TestCrazyhouseDropAnswersCheck(t *testing.T) {
	g := newCrazyhouse(t)
	pocketSetup(g, "4k3/8/8/8/8/8/8/r3K3 w - - 0 1", map[Side]map[Kind]int8{White: {Pawn: 1, Knight: 1}})
	got := strings.Join(dropActions(g.Legal()), " ")
	if got != "N@b1 N@c1 N@d1" {
		t.Errorf("drops that block the check: %s", got)
	}
}

func TestCrazyhouseMateByDrop(t *testing.T) {
	g := newCrazyhouse(t)
	pocketSetup(g, "7k/6pp/5N2/8/8/8/8/6K1 w - - 0 1", map[Side]map[Kind]int8{White: {Queen: 1}})
	if _, _, over := g.Over(); over {
		t.Fatal("the game is not over before the drop")
	}
	note, err := g.Apply("Q@g8")
	if err != nil {
		t.Fatal(err)
	}
	if note != "Q@g8#" {
		t.Errorf("notation %q", note)
	}
	res, reason, over := g.Over()
	if !over || res != ResultWhite || reason != ReasonCheckmate {
		t.Errorf("result %s %s %v", res, reason, over)
	}
	if len(g.View(Black).Legal) != 0 {
		t.Error("Black is still offered moves after mate")
	}
}

func TestCrazyhouseNoMateWhileADropSaves(t *testing.T) {
	g := newCrazyhouse(t)
	// back-rank mate, but Black holds a rook to drop between
	pocketSetup(g, "6k1/5ppp/8/8/8/8/8/R5K1 w - - 0 1", map[Side]map[Kind]int8{Black: {Rook: 1}})
	note, err := g.Apply("a1a8")
	if err != nil || note != "Ra8+" {
		t.Fatalf("%q %v", note, err)
	}
	if _, _, over := g.Over(); over {
		t.Fatal("a drop of the rook blocks the check, it is no mate")
	}
	if got := dropActions(g.Legal()); !contains(got, "R@b8") || !contains(got, "R@f8") {
		t.Errorf("Black should be able to block: %v", got)
	}
}

func TestCrazyhouseStalemateAndMaterial(t *testing.T) {
	g := newCrazyhouse(t)
	pocketSetup(g, "7k/5Q2/6K1/8/8/8/8/8 b - - 0 1", nil)
	if res, reason, over := g.Over(); !over || res != ResultDraw || reason != ReasonStalemate {
		t.Errorf("stalemate: %s %s %v", res, reason, over)
	}
	// stalemate is escaped by a pocket piece: Black holds a pawn, a drop is a move
	pocketSetup(g, "7k/5Q2/6K1/8/8/8/8/8 b - - 0 1", map[Side]map[Kind]int8{Black: {Pawn: 1}})
	if _, _, over := g.Over(); over {
		t.Error("with a pawn in hand Black is not stalemated")
	}
	// two bare kings are not a dead position: pockets bring material back
	pocketSetup(g, "4k3/8/8/8/8/8/8/4K3 w - - 0 1", nil)
	if _, _, over := g.Over(); over {
		t.Error("two kings are not a draw in crazyhouse")
	}
	if got := g.Timeout(White); got != ResultBlack {
		t.Errorf("a flag against a bare king still loses: %s", got)
	}
}

func TestCrazyhouseCastlingAndPocketInRepetition(t *testing.T) {
	g := newCrazyhouse(t)
	for _, a := range strings.Fields("e2e4 e7e5 g1f3 b8c6 f1c4 g8f6 e1g1") {
		if _, err := g.Apply(a); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
	}
	if g.Position().B[MakeSq(6, 0)].Kind != King || g.Position().B[MakeSq(5, 0)].Kind != Rook {
		t.Error("castling works as usual")
	}
	// the pockets are part of the position key
	a := positionKey(g.Position())
	p := g.Position().Clone()
	p.Pocket[White][Pawn] = 1
	if a == positionKey(p) {
		t.Error("the repetition key ignores the pockets")
	}
}

// Random games: no piece is made from nothing (32 on the board and in the pockets
// together, always), pockets never go negative, every legal action plays, a game
// restored by Replay is the same game.
func TestCrazyhouseRandomGames(t *testing.T) {
	games, plies := 60, 160
	if testing.Short() {
		games = 15
	}
	rng := rand.New(rand.NewSource(7))
	for gi := 0; gi < games; gi++ {
		g := newCrazyhouse(t)
		var played []string
		for ply := 0; ply < plies; ply++ {
			if _, _, over := g.Over(); over {
				break
			}
			legal := g.Legal()
			if len(legal) == 0 {
				t.Fatalf("game %d: not over and no legal action", gi)
			}
			a := legal[rng.Intn(len(legal))]
			if rng.Intn(3) == 0 { // prefer drops when there are any, to exercise them
				if d := dropActions(legal); len(d) > 0 {
					a = d[rng.Intn(len(d))]
				}
			}
			before := g.Position().Clone()
			if _, err := g.Apply(a); err != nil {
				t.Fatalf("game %d: %s: %v", gi, a, err)
			}
			p := g.Position()
			total := 0
			for _, pc := range p.B {
				if !pc.Empty() {
					total++
				}
			}
			for s := White; s <= Black; s++ {
				for k := range p.Pocket[s] {
					if p.Pocket[s][k] < 0 {
						t.Fatalf("game %d: negative pocket", gi)
					}
					total += int(p.Pocket[s][k])
				}
			}
			if total != 32 {
				t.Fatalf("game %d after %s (%s): %d pieces in all\nbefore %s", gi, a, p.FEN(), total, before.FEN())
			}
			played = append(played, a)
		}
		st, err := Replay("crazyhouse", Options{}, played)
		if err != nil {
			t.Fatalf("game %d: replay: %v", gi, err)
		}
		rg := st.(*Game)
		if rg.Position().FEN() != g.Position().FEN() || rg.Position().Pocket != g.Position().Pocket {
			t.Fatalf("game %d: replay differs", gi)
		}
	}
}

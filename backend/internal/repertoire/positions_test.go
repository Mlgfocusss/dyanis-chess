package repertoire

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"dyanis-chess/backend/internal/explorer"
	"dyanis-chess/backend/internal/store"
)

// The same examples are in web/src/openings/tree.test.js: the page and the server must agree on
// what one position is.
func TestPosKey(t *testing.T) {
	// the counters are not part of a position
	a := "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
	b := "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 12 40"
	if PosKey(a) != PosKey(b) || PosKey(a) != "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq -" {
		t.Errorf("counters: %q %q", PosKey(a), PosKey(b))
	}
	// 1.e4 c5 2.Nf3 and 1.Nf3 c5 2.e4: the FENs differ by the en passant square nobody can use
	x := fenAfter(t, "e2e4", "c7c5", "g1f3")
	y := fenAfter(t, "g1f3", "c7c5", "e2e4")
	if x == y {
		t.Fatal("the FENs were expected to differ")
	}
	if PosKey(x) != PosKey(y) {
		t.Errorf("a transposition has two keys: %q %q", PosKey(x), PosKey(y))
	}
	// an en passant square a pawn can really use stays
	ep := fenAfter(t, "e2e4", "a7a6", "e4e5", "d7d5")
	if got := PosKey(ep); !strings.HasSuffix(got, " d6") {
		t.Errorf("a possible en passant capture was dropped: %q", got)
	}
	noEp := fenAfter(t, "e2e4", "a7a6", "a2a3", "d7d5")
	if got := PosKey(noEp); !strings.HasSuffix(got, " -") {
		t.Errorf("an impossible en passant capture was kept: %q", got)
	}
	// Black pushes a pawn beside a white pawn on the fifth rank, and the other way round
	if got := PosKey(fenAfter(t, "a2a4", "h7h5", "a4a5", "b7b5")); !strings.HasSuffix(got, " b6") {
		t.Errorf("white takes en passant: %q", got)
	}
	if got := PosKey(fenAfter(t, "e2e4", "h7h6", "e4e5", "h6h5", "d2d4")); !strings.HasSuffix(got, " -") {
		t.Errorf("black cannot take: %q", got)
	}
	if got := PosKey(fenAfter(t, "e2e4", "d7d5", "e4e5", "f7f5")); !strings.HasSuffix(got, " f6") {
		t.Errorf("white takes on f6: %q", got)
	}
	if got := PosKey(fenAfter(t, "a2a3", "h7h5", "a3a4", "h5h4", "g2g4")); !strings.HasSuffix(got, " g3") {
		t.Errorf("black takes on g3: %q", got)
	}
	if PosKey("") != "" || PosKey("junk") != "junk" {
		t.Error("a broken FEN must not panic")
	}
}

// The two orders of 1.d4 Nf6 2.c4 e6: ids 1-5 are the first (3.Nc3 at the end), 6-9 the second.
func twoOrders(t testing.TB, extra ...[]string) []store.RepMove {
	lines := [][]string{
		{"d2d4", "g8f6", "c2c4", "e7e6", "b1c3"},
		{"c2c4", "e7e6", "d2d4", "g8f6"},
	}
	return buildTree(t, "white", append(lines, extra...)...)
}

func TestGraphClassesAndReplies(t *testing.T) {
	ms := twoOrders(t)
	g := NewGraph(ms)
	if !g.Same(4, 9) || g.Same(4, 8) || g.Same(0, 9) {
		t.Fatalf("classes: %v", g.class)
	}
	if fmt.Sprint(g.Class(9)) != "[4 9]" || g.Rep(g.Key(9)) != 4 || g.Rep(startKey) != 0 {
		t.Errorf("class %v rep %d", g.Class(9), g.Rep(g.Key(9)))
	}
	// the second order has no moves of its own after Nf6, and knows the reply of the first
	r := g.Replies(9)
	if len(r) != 1 || r[0].UCI != "b1c3" || r[0].ID != 5 {
		t.Errorf("replies %v", r)
	}
	// its own children come first
	ms = twoOrders(t, []string{"c2c4", "e7e6", "d2d4", "g8f6", "g1f3"})
	g = NewGraph(ms)
	r = g.Replies(4)
	if len(r) != 2 || r[0].UCI != "b1c3" || r[1].UCI != "g1f3" {
		t.Errorf("replies of the first order: %v", r)
	}
	if m, ok := g.Reply(9, "g1f3"); !ok || m.ParentID != 9 {
		t.Errorf("reply %v %v", m, ok)
	}
}

func TestGapsCountAPositionOnce(t *testing.T) {
	// 4 and 9 are the same position and the repository answers it (3.Nc3), so there is no hole
	if g := Gaps(twoOrders(t)); len(g) != 0 {
		t.Errorf("gaps %v", g)
	}
	// without the answer both orders end in one position: one hole, not two
	ms := buildTree(t, "white", []string{"d2d4", "g8f6", "c2c4", "e7e6"}, []string{"c2c4", "e7e6", "d2d4", "g8f6"})
	if g := Gaps(ms); fmt.Sprint(g) != "[4]" {
		t.Errorf("gaps %v", g)
	}
}

func TestCardsAreCountedOncePerPosition(t *testing.T) {
	// both orders end with 3.Nc3 (ids 5 and 10): the same question in the same position
	ms := twoOrders(t, []string{"c2c4", "e7e6", "d2d4", "g8f6", "b1c3"})
	if ms[9].UCI != "b1c3" || ms[9].ID != 10 {
		t.Fatalf("tree %v", ms)
	}
	g := NewGraph(ms)
	cards := g.Cards()
	// d4, c4 (after d4 Nf6), Nc3 [x2], c4 (first move), d4 (after c4 e6)
	if len(cards) != 5 {
		t.Fatalf("%d cards", len(cards))
	}
	var nc3 CardGroup
	for _, c := range cards {
		if c.Rep.UCI == "b1c3" {
			nc3 = c
		}
	}
	if fmt.Sprint(nc3.Members) != "[5 10]" || nc3.Rep.ID != 5 {
		t.Errorf("card %+v", nc3)
	}
	if e := g.Edges(); e != 9 {
		t.Errorf("edges %d of %d rows", e, len(ms))
	}
	// training asks it once; the most asked node stands for the card
	sel := Select(ms, t0, 10, 10)
	if fmt.Sprint(sel.New) != "[1 3 5 6 8]" || sel.NewTotal != 5 {
		t.Errorf("new %+v", sel)
	}
	ms[9].Reviews = 3
	ms[9].Box = 2
	ms[9].DueAt = sql.NullTime{Time: t0.Add(-time.Hour), Valid: true}
	sel = Select(ms, t0, 10, 10)
	// it is one card: asked at node 10, it is not new at node 5 any more
	if fmt.Sprint(sel.Due) != "[10]" || fmt.Sprint(sel.New) != "[1 3 6 8]" {
		t.Errorf("after one node was asked: %+v", sel)
	}
	c := countsOf(ms, t0)
	if c.Moves != 9 || c.Mine != 5 || c.Due != 1 || c.New != 4 {
		t.Errorf("counts %+v", c)
	}
}

func TestDrillKnowsMovesOfTheOtherOrder(t *testing.T) {
	// in the position after 4 / 9 the first order plays Nc3 and the second Nf3: neither is a mistake
	ms := twoOrders(t, []string{"c2c4", "e7e6", "d2d4", "g8f6", "g1f3"})
	d, ok := BuildDrill(ms, 5, false)
	if !ok || fmt.Sprint(d.Alt) != "[g1f3]" {
		t.Errorf("alt of Nc3: %v %v", d.Alt, ok)
	}
	d, ok = BuildDrill(ms, 10, false)
	if !ok || fmt.Sprint(d.Alt) != "[b1c3]" || len(d.Path) != 4 {
		t.Errorf("alt of Nf3: %v path %d", d.Alt, len(d.Path))
	}
}

func TestCompareTransposition(t *testing.T) {
	ms := buildTree(t, "white", []string{"d2d4", "g8f6", "c2c4", "e7e6", "b1c3"})
	game := func(moves ...string) Deviation { return Compare(ms, "white", moves) }

	// the other order of the same moves: not a deviation
	d := game("c2c4", "e7e6", "d2d4", "g8f6", "b1c3")
	if d.Status != StatusInBook || !d.Transposed || d.TransposedPly != 4 || len(d.Line) != 5 || d.Line[0] != "c4" {
		t.Errorf("transposed game: %+v", d)
	}
	if d := game("d2d4", "g8f6", "c2c4", "e7e6", "b1c3"); d.Transposed || d.Status != StatusInBook {
		t.Errorf("the tree's own order: %+v", d)
	}
	// ... and when it then leaves at its own move, the deviation is there
	d = game("c2c4", "e7e6", "d2d4", "g8f6", "g1f3")
	if d.Status != StatusYouDeviated || d.Ply != 5 || d.Node != 4 || len(d.Expected) != 1 || d.Expected[0].SAN != "Nc3" || !d.Transposed {
		t.Errorf("deviation after the transposition: %+v", d)
	}
	// a game that never reaches a position of the repository leaves it at the first move
	d = game("c2c4", "e7e6", "b1c3")
	if d.Status != StatusYouDeviated || d.Ply != 1 || d.Node != 0 || d.Transposed || d.Played.SAN != "c4" {
		t.Errorf("never in the repository: %+v", d)
	}
	// the opponent's order differs too: 1.d4 e6 2.c4 Nf6 is the same position as 1.d4 Nf6 2.c4 e6
	d = game("d2d4", "e7e6", "c2c4", "g8f6", "b1c3")
	if d.Status != StatusInBook || d.TransposedPly != 4 {
		t.Errorf("opponent's order: %+v", d)
	}
	// an illegal move after the repository is left stays a deviation; in the repository it is broken
	if d := game("d2d4", "g8f6", "d4d5", "e7e5", "a1a5"); d.Status != StatusYouDeviated || d.Ply != 3 {
		t.Errorf("deviation before a broken move: %+v", d)
	}
	if d := game("d2d4", "g8f6", "a1a5"); d.Status != StatusBroken {
		t.Errorf("broken: %+v", d)
	}
}

func TestScanCountsATranspositionOnce(t *testing.T) {
	// 1.d4 and then both orders of the opponent's Nf6 / e6 meeting at 2.c4: the repository answers
	// the common position with 3.Nc3 on one of the branches
	moves := buildTree(t, "white",
		[]string{"d2d4", "g8f6", "c2c4", "e7e6", "b1c3"},
		[]string{"d2d4", "e7e6", "c2c4", "g8f6"})
	f := newFake(map[string]explorer.Response{
		fenAfter(t, "d2d4"):                         answer(10000, mm{"g8f6", "Nf6", 6000}, mm{"e7e6", "e6", 3000}, mm{"d7d5", "d5", 1000}),
		fenAfter(t, "d2d4", "g8f6", "c2c4"):         answer(1000, mm{"e7e6", "e6", 500}, mm{"g7g6", "g6", 500}),
		fenAfter(t, "d2d4", "e7e6", "c2c4"):         answer(1000, mm{"g8f6", "Nf6", 500}, mm{"f8b4", "Bb4", 500}),
		fenAfter(t, "d2d4", "g8f6", "c2c4", "e7e6"): answer(0),
	})
	sc := scan(t, moves, "white", f, GapOptions{MinPct: 5})
	// holes: 1...d5 (10%), 2...g6 (6000 * 0.5 -> 30%), 2...Bb4 (3000 * 0.5 -> 15%); no "no answer" hole:
	// 2...Nf6 / 2...e6 of the second branch lead to the answered position
	if sc.Counts.Answer != 0 || sc.Counts.Missing != 3 {
		for _, g := range sc.Gaps {
			t.Logf("%+v", g)
		}
		t.Fatalf("counts %+v", sc.Counts)
	}
	if !near(sc.Coverage, 1-0.1-0.3-0.15) {
		t.Errorf("coverage %v", sc.Coverage)
	}
	// the common position is asked for once (the fake counts the requests it had to make)
	if f.calls != 4 {
		t.Errorf("%d requests, want 4: the start of the line is not a question, each position once", f.calls)
	}
}

func TestScanBackToTheStartDoesNotLoop(t *testing.T) {
	// 1.Nf3 Nf6 2.Ng1 Ng8: the position of the start again, with the whole tree below it
	moves := buildTree(t, "white", []string{"g1f3", "g8f6", "f3g1", "f6g8", "e2e4"})
	sc := scan(t, moves, "white", nil, GapOptions{})
	if sc.Coverage < 0 || sc.Coverage > 1 {
		t.Errorf("coverage %v", sc.Coverage)
	}
	if g := Gaps(moves); len(g) != 0 {
		t.Errorf("gaps %v", g)
	}
}

// ---------------------------------------------------------------- the API

func TestAddMoveLeadsToTheOtherOrder(t *testing.T) {
	a := newAPI(t)
	rep := a.mustCall("POST", "/api/repertoires", `{"name":"Ферзевый","color":"white"}`, 201)
	base := fmt.Sprintf("/api/repertoires/%d", id(rep["id"]))
	add := func(parent int64, uci string, want int) map[string]any {
		return a.mustCall("POST", base+"/moves", fmt.Sprintf(`{"parent":%d,"uci":%q}`, parent, uci), want)
	}
	mid := func(out map[string]any) int64 { return id(out["move"].(map[string]any)["id"]) }
	// 1.d4 Nf6 2.c4 e6 3.Nc3
	n1 := mid(add(0, "d2d4", 201))
	n2 := mid(add(n1, "g8f6", 201))
	n3 := mid(add(n2, "c2c4", 201))
	n4 := mid(add(n3, "e7e6", 201))
	n5 := mid(add(n4, "b1c3", 201))
	// 1...e6 2.c4 Nf6: the position of n4, where the repository has 3.Nc3
	m2 := mid(add(n1, "e7e6", 201))
	m3 := mid(add(m2, "c2c4", 201))
	m4 := mid(add(m3, "g8f6", 201))
	// 3.Nc3 here is the move the repository already has: nothing new is added
	out := add(m4, "b1c3", 200)
	if out["created"] != false || out["transposed"] != true || mid(out) != n5 {
		t.Errorf("transposed move: %v", out)
	}
	// a new move there is added under this node
	out = add(m4, "g1f3", 201)
	if out["transposed"] != false || id(out["move"].(map[string]any)["parent"]) != m4 {
		t.Errorf("new move: %v", out)
	}
	// nine rows: nothing was added twice
	got := a.mustCall("GET", base, "", 200)
	if rows := len(got["moves"].([]any)); rows != 9 {
		t.Errorf("rows %d", rows)
	}
	// the second order is not a hole: nothing of the opponent's is left unanswered
	if gaps := got["gaps"].([]any); len(gaps) != 0 {
		t.Errorf("gaps %v", gaps)
	}
}

func TestReviewMovesEveryNodeOfACard(t *testing.T) {
	a := newAPI(t)
	rep := a.mustCall("POST", "/api/repertoires", `{"name":"Ферзевый","color":"white"}`, 201)
	rid := id(rep["id"])
	base := fmt.Sprintf("/api/repertoires/%d", rid)
	ms := twoOrders(t, []string{"c2c4", "e7e6", "d2d4", "g8f6", "b1c3"})
	for i := range ms { // the store sets the ids itself: the tree is put there in the order of the lines
		ms[i].RepertoireID = rid
		if _, _, err := a.mem.AddRepMove(context.Background(), ms[i]); err != nil {
			t.Fatal(err)
		}
	}
	// the list counts Nc3 once
	lst := a.mustCall("GET", "/api/repertoires", "", 200)["repertoires"].([]any)
	c := lst[0].(map[string]any)["counts"].(map[string]any)
	if c["moves"] != 9.0 || c["mine"] != 5.0 || c["new"] != 5.0 {
		t.Errorf("list counts %v", c)
	}
	if g := lst[0].(map[string]any)["gaps"].(map[string]any); g["open"] != 0.0 {
		t.Errorf("open holes %v", g)
	}
	// the session has one question for the position
	drills := a.mustCall("GET", base+"/train", "", 200)["drills"].([]any)
	if len(drills) != 5 {
		t.Fatalf("%d drills", len(drills))
	}
	r := a.mustCall("POST", base+"/review", `{"move":10,"correct":true}`, 200)
	if r["box"] != 1.0 {
		t.Errorf("review %v", r)
	}
	// ... and both nodes moved together
	var first, second store.RepMove
	for _, m := range a.mem.moves {
		switch m.ID {
		case 5:
			first = m
		case 10:
			second = m
		}
	}
	if first.Box != 1 || second.Box != 1 || !first.DueAt.Valid || first.DueAt.Time != second.DueAt.Time {
		t.Errorf("states: %+v %+v", first, second)
	}
	// asking again goes up the ladder once, not twice
	r = a.mustCall("POST", base+"/review", `{"move":5,"correct":true}`, 200)
	if r["box"] != 2.0 {
		t.Errorf("second review %v", r)
	}
	drills = a.mustCall("GET", base+"/train", "", 200)["drills"].([]any)
	if len(drills) != 4 {
		t.Errorf("%d drills after one card was answered", len(drills))
	}
}

func TestImportMergesConvergingLines(t *testing.T) {
	a := newAPI(t)
	// two lines of the file meet in one position; the second goes on with the same 3.Nc3 and adds 3...Bb4
	text := `[Orientation "white"]

1. d4 Nf6 2. c4 e6 3. Nc3 *

[Orientation "white"]

1. c4 e6 2. d4 Nf6 3. Nc3 Bb4 *
`
	out := a.mustCall("POST", "/api/repertoires/import", importBody("Ферзевый", "white", text), 201)
	st := out["import"].(map[string]any)
	// 5 + the second line's c4 e6 d4 Nf6 (no 3.Nc3 again) + Bb4
	if st["added"] != 10.0 || st["transposed"] != 1.0 {
		t.Errorf("import %v", st)
	}
	base := fmt.Sprintf("/api/repertoires/%d", id(out["repertoire"].(map[string]any)["id"]))
	got := a.mustCall("GET", base, "", 200)
	var nc3, bb4 map[string]any
	for _, m := range got["moves"].([]any) {
		mv := m.(map[string]any)
		switch mv["uci"] {
		case "b1c3":
			if nc3 != nil {
				t.Errorf("3.Nc3 twice: %v and %v", nc3, mv)
			}
			nc3 = mv
		case "f8b4":
			bb4 = mv
		}
	}
	if nc3 == nil || bb4 == nil || bb4["parent"] != nc3["id"] {
		t.Fatalf("Nc3 %v Bb4 %v", nc3, bb4)
	}
	// importing the same file again adds nothing
	again := a.mustCall("POST", base+"/import", fmt.Sprintf(`{"pgn":%q}`, text), 200)["import"].(map[string]any)
	if again["added"] != 0.0 {
		t.Errorf("second import %v", again)
	}
}

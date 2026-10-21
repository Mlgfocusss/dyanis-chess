package analysis

import (
	"encoding/json"
	"math/rand"
	"testing"

	"dyanis-chess/backend/internal/variants"
)

func chess960Options(n int) variants.Options {
	return variants.Options{Seed: int64(n), Params: map[string]any{"position": n}}
}

// castleKind names the kind of castling an action is in the position, "" for another move.
func castleKind(pos *variants.Position, action string) string {
	from, ok1 := variants.ParseSq(action[0:2])
	to, ok2 := variants.ParseSq(action[2:4])
	if !ok1 || !ok2 {
		return ""
	}
	pc, target := pos.B[from], pos.B[to]
	if pc.Kind != variants.King || target.Kind != variants.Rook || target.Side != pc.Side {
		return ""
	}
	long := to.File() < from.File()
	kingTo, rookTo := 6, 5
	if long {
		kingTo, rookTo = 2, 3
	}
	switch {
	case kingTo == from.File():
		return "king-stays"
	case rookTo == from.File() && kingTo == to.File():
		return "swap"
	case long:
		return "long"
	default:
		return "short"
	}
}

// playCastling plays a random game of setup n of at most plies half-moves, taking
// a castling whenever one is offered. It returns the actions and the kinds of
// castling that were played.
func playCastling(t *testing.T, n int, seed int64, plies int) (actions []string, kinds map[string]int) {
	t.Helper()
	v, _ := variants.Get(VariantChess960)
	st, err := v.New(chess960Options(n))
	if err != nil {
		t.Fatal(err)
	}
	g := st.(*variants.Game)
	rng := rand.New(rand.NewSource(seed))
	kinds = map[string]int{}
	for len(actions) < plies {
		if _, _, over := st.Over(); over {
			break
		}
		legal := st.Legal()
		var castles []string
		for _, a := range legal {
			if castleKind(g.Position(), a) != "" {
				castles = append(castles, a)
			}
		}
		a := legal[rng.Intn(len(legal))]
		if len(castles) > 0 {
			a = castles[rng.Intn(len(castles))]
			kinds[castleKind(g.Position(), a)]++
		}
		if _, err := st.Apply(a); err != nil {
			t.Fatal(err)
		}
		actions = append(actions, a)
	}
	return actions, kinds
}

func checkVariantReport(t *testing.T, vg *VariantGame, rep *Report) {
	t.Helper()
	if rep.Variant != VariantChess960 {
		t.Errorf("Variant = %q", rep.Variant)
	}
	if len(rep.Moves) != len(vg.Moves) || len(rep.Evals) != len(vg.Moves)+1 {
		t.Fatalf("%d moves and %d evals for a game of %d moves", len(rep.Moves), len(rep.Evals), len(vg.Moves))
	}
	if rep.Opening != nil || rep.BookPlies != 0 {
		t.Errorf("an opening %v and %d book plies in a Chess960 game", rep.Opening, rep.BookPlies)
	}
	for i, m := range rep.Moves {
		if m.Book || m.Class == Book || m.Opening != nil {
			t.Errorf("move %d is marked as theory: %+v", i+1, m)
		}
		if m.SAN != vg.Moves[i].SAN || m.UCI != vg.Moves[i].Shown {
			t.Errorf("move %d: %s %s, game has %s %s", i+1, m.SAN, m.UCI, vg.Moves[i].SAN, vg.Moves[i].Shown)
		}
		if m.Accuracy < 0 || m.Accuracy > 100 || m.Loss < 0 {
			t.Errorf("move %d: accuracy %v loss %v", i+1, m.Accuracy, m.Loss)
		}
		if wantColor := []string{"white", "black"}[i%2]; m.Color != wantColor {
			t.Errorf("move %d is %s's, want %s", i+1, m.Color, wantColor)
		}
	}
	for c := 0; c < 2; c++ {
		if a := rep.Accuracy[c]; a < 0 || a > 100 {
			t.Errorf("accuracy of %d = %v", c, a)
		}
	}
}

// Castling of every kind is a move of the game: the review neither fails nor
// leaves it out, and its SAN is the one variants wrote ("O-O").
func TestAnalyzeChess960CastlingKinds(t *testing.T) {
	seen := map[string]int{}
	games := 0
	for n := 0; n < 40 && games < 14; n++ {
		actions, kinds := playCastling(t, n, int64(n)+1, 36)
		if len(kinds) == 0 {
			continue
		}
		fresh := false
		for k := range kinds {
			if seen[k] == 0 {
				fresh = true
			}
			seen[k] += kinds[k]
		}
		if !fresh && games >= 8 {
			continue
		}
		games++
		vg, err := NewVariantGame(VariantChess960, chess960Options(n), actions)
		if err != nil {
			t.Fatalf("setup %d: %v", n, err)
		}
		rep, err := AnalyzeVariant(vg, Options{Depth: 2, NoSpecial: n%2 == 0})
		if err != nil {
			t.Fatalf("setup %d: %v", n, err)
		}
		checkVariantReport(t, vg, rep)
		castled := 0
		for i, m := range vg.Moves {
			if !m.Castle {
				continue
			}
			castled++
			if san := rep.Moves[i].SAN; san != "O-O" && san != "O-O-O" && san != "O-O+" && san != "O-O-O+" && san != "O-O#" && san != "O-O-O#" {
				t.Errorf("setup %d: castling written %q", n, san)
			}
			if len(m.Shown) != 4 || m.Shown[2] != "gc"[btoi(m.UCI[2] < m.UCI[0])] || m.Shown[3] != m.UCI[1] {
				t.Errorf("castling %s is shown as %s", m.UCI, m.Shown)
			}
			if rep.Moves[i].BestUCI == m.UCI {
				t.Errorf("the engine recommended the castling %s", m.UCI)
			}
		}
		if castled == 0 {
			t.Errorf("setup %d: no castling in the game", n)
		}
	}
	for _, k := range []string{"king-stays", "swap", "short", "long"} {
		if seen[k] == 0 {
			t.Errorf("no castling of kind %q was played in the sample games (seen %v)", k, seen)
		}
	}
}

// A mate ends the review: the last position is terminal and the mating move is the best.
func TestAnalyzeChess960Mate(t *testing.T) {
	vg, err := NewVariantGame(VariantChess960, chess960Options(518), []string{"f2f3", "e7e5", "g2g4", "d8h4"})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := AnalyzeVariant(vg, Options{Depth: 4})
	if err != nil {
		t.Fatal(err)
	}
	checkVariantReport(t, vg, rep)
	last := rep.Evals[4]
	if last.Terminal != "checkmate" || last.WinPct != 0 {
		t.Errorf("last position: %+v", last)
	}
	if m := rep.Moves[3]; m.Class != Best || m.Loss != 0 {
		t.Errorf("the mating move: %+v", m)
	}
	if rep.Moves[2].Class == Best || rep.Moves[2].Loss < 20 {
		t.Errorf("g4 allowing mate in one should be a blunder: %+v", rep.Moves[2])
	}
	if rep.Counts[0][Book]+rep.Counts[1][Book] != 0 {
		t.Errorf("book moves counted: %v", rep.Counts)
	}
}

// The same game at the same depth gives the same report, and the report matches
// a PGN review's shape (JSON round trip keeps it).
func TestAnalyzeChess960Deterministic(t *testing.T) {
	actions, _ := playCastling(t, 330, 5, 30)
	vg, err := NewVariantGame(VariantChess960, chess960Options(330), actions)
	if err != nil {
		t.Fatal(err)
	}
	a, err := AnalyzeVariant(vg, Options{Depth: 3, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	b, err := AnalyzeVariant(vg, Options{Depth: 3, Workers: 3})
	if err != nil {
		t.Fatal(err)
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Errorf("two reviews of one game differ:\n%s\n%s", ja, jb)
	}
	var back Report
	if err := json.Unmarshal(ja, &back); err != nil || back.Variant != VariantChess960 || len(back.Moves) != len(vg.Moves) {
		t.Errorf("round trip: %v %+v", err, back.Variant)
	}
}

// The classical game is reviewed as before when it goes through the same road:
// every position of the 518 setup is the usual one.
func TestVariantGameStartPositions(t *testing.T) {
	vg, err := NewVariantGame(VariantChess960, chess960Options(518), []string{"e2e4"})
	if err != nil {
		t.Fatal(err)
	}
	want := "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w - - 0 1"
	if vg.FENs[0] != want {
		t.Errorf("start: %s", vg.FENs[0])
	}
	if vg.Moves[0].Legal != 20 {
		t.Errorf("legal moves at the start: %d", vg.Moves[0].Legal)
	}
	if _, err := NewVariantGame("atomic", variants.Options{}, nil); err == nil {
		t.Error("atomic games must be refused")
	}
	if _, err := NewVariantGame(VariantChess960, chess960Options(518), []string{"e2e5"}); err == nil {
		t.Error("an illegal move must be refused")
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

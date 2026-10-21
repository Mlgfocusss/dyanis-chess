package variants

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func newBlind(t *testing.T, peek bool) State {
	t.Helper()
	_, o, err := Prepare("blindfold", map[string]any{"peek": peek}, 1)
	if err != nil {
		t.Fatal(err)
	}
	st, err := Get0("blindfold", o)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestBlindfoldCheckOptions(t *testing.T) {
	_, o, err := Prepare("blindfold", nil, 1)
	if err != nil || o.Params["peek"] != false {
		t.Errorf("default: %v %v", o.Params, err)
	}
	_, o, err = Prepare("blindfold", map[string]any{"peek": true}, 1)
	if err != nil || o.Params["peek"] != true {
		t.Errorf("peek on: %v %v", o.Params, err)
	}
	for _, bad := range []map[string]any{{"peek": "yes"}, {"peek": 1}, {"show": true}} {
		if _, _, err := Prepare("blindfold", bad, 1); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

// While the game is on the view carries no piece, no last move and no check, to
// anyone; the player to move still gets the legal actions; the JSON has an
// empty object for the board, not null.
func TestBlindfoldHidesThePosition(t *testing.T) {
	st := newBlind(t, false)
	playAll(t, st, "e2e4", "f7f5", "d1h5") // check on e8
	for _, viewer := range []Side{White, Black, NoSide} {
		v := st.View(viewer)
		if len(v.Board) != 0 || v.Board == nil || len(v.Last) != 2 || v.Last[0] != "d1" || v.Last[1] != "h5" || len(v.Check) != 0 {
			t.Errorf("viewer %v sees board %v last %v check %v", viewer, v.Board, v.Last, v.Check)
		}
		raw, _ := json.Marshal(v)
		s := string(raw)
		if !strings.Contains(s, `"board":{}`) {
			t.Errorf("viewer %v: %s", viewer, s)
		}
		for _, code := range []string{"wP", "bK", "wQ", "bP", "e4", "h5"} {
			// "e4" and "h5" appear in the legal list of the mover as parts of long moves; the board and the extra must not hold them
			if strings.Contains(string(mustJSON(v.Board)), code) || strings.Contains(string(mustJSON(v.Extra)), code) {
				t.Errorf("viewer %v: %s leaks into the board or the extra", viewer, code)
			}
		}
		x := v.Extra.(blindExtra)
		if !x.Blind || x.Peek || x.Board != nil {
			t.Errorf("viewer %v: extra %+v", viewer, x)
		}
	}
	if v := st.View(Black); len(v.Legal) == 0 {
		t.Error("the player to move is not told the legal moves")
	}
	if v := st.View(White); len(v.Legal) != 0 {
		t.Error("the player who waits is told the legal moves")
	}
}

// The last move is in the view of the players and of a spectator while the game is on (castling: the
// king's squares; en passant and promotion: the pawn's), but the raw snapshot holds no piece code, no
// non-empty board and no check.
func TestBlindfoldLastMoveWithoutLeaks(t *testing.T) {
	pieceCode := regexp.MustCompile(`"[wb][PNBRQK]"`)
	check := func(st State, want ...string) {
		t.Helper()
		for _, viewer := range []Side{White, Black, NoSide} {
			v := st.View(viewer)
			if strings.Join(v.Last, "") != strings.Join(want, "") || len(v.Last) != 2 {
				t.Errorf("viewer %v: last %v, want %v", viewer, v.Last, want)
			}
			raw := string(mustJSON(v))
			if !strings.Contains(raw, `"board":{}`) || pieceCode.MatchString(raw) || strings.Contains(raw, `"check"`) {
				t.Errorf("viewer %v leaks: %s", viewer, raw)
			}
			if !strings.Contains(raw, `"last":["`+want[0]+`","`+want[1]+`"]`) {
				t.Errorf("viewer %v: no last in %s", viewer, raw)
			}
		}
	}
	st := newBlind(t, false)
	playAll(t, st, "e2e4")
	check(st, "e2", "e4")
	playAll(t, st, "e7e5", "g1f3", "b8c6", "f1c4", "g8f6")
	check(st, "g8", "f6")
	playAll(t, st, "e1g1") // castling: the king's squares
	check(st, "e1", "g1")

	// en passant: the pawn's squares
	st = newBlind(t, false)
	playAll(t, st, "e2e4", "a7a6", "e4e5", "d7d5", "e5d6")
	check(st, "e5", "d6")

	// promotion: the pawn's squares; with check on the board, still no check in the view
	st = newBlind(t, false)
	playAll(t, st, "a2a4", "b7b5", "a4b5", "a7a6", "b5a6", "c8b7", "a6a7", "b7c6", "a7b8q")
	check(st, "a7", "b8")
	if v := st.View(Black); len(v.Check) != 0 {
		t.Errorf("check shown: %v", v.Check)
	}

	// peeking allowed: the position stays in the extra only, the last move in the view
	st = newBlind(t, true)
	playAll(t, st, "e2e4")
	v := st.View(Black)
	if len(v.Board) != 0 || len(v.Last) != 2 {
		t.Errorf("peek: %+v", v)
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// With peeking allowed the position is in the extra (the browser decides when
// to show it), never in the board of the view.
func TestBlindfoldPeek(t *testing.T) {
	st := newBlind(t, true)
	playAll(t, st, "e2e4")
	for _, viewer := range []Side{White, Black, NoSide} {
		v := st.View(viewer)
		if len(v.Board) != 0 || len(v.Last) != 2 {
			t.Errorf("viewer %v: the view shows the position or lacks the last move", viewer)
		}
		x := v.Extra.(blindExtra)
		if !x.Blind || !x.Peek || len(x.Board) != 32 || x.Board["e4"] != "wP" {
			t.Errorf("viewer %v: extra %+v", viewer, x)
		}
	}
}

func TestBlindfoldOpensAtTheEnd(t *testing.T) {
	// mate on the board
	st := newBlind(t, false)
	playAll(t, st, "f2f3", "e7e5", "g2g4", "d8h4")
	res, reason, over := st.Over()
	if !over || res != ResultBlack || reason != ReasonCheckmate {
		t.Fatalf("fool's mate: %q %q %v", res, reason, over)
	}
	v := st.View(NoSide)
	if len(v.Board) != 32 {
		t.Errorf("after the mate the board has %d pieces", len(v.Board))
	}
	if v.Board["h4"] != "bQ" || len(v.Check) != 1 || v.Check[0] != "e1" || len(v.Last) == 0 {
		t.Errorf("the finished game is not opened: %+v", v)
	}
	if x := v.Extra.(blindExtra); x.Blind || x.Board != nil {
		t.Errorf("extra after the end: %+v", x)
	}

	// a resignation (the server tells the state)
	st = newBlind(t, true)
	playAll(t, st, "e2e4", "e7e5")
	st.(Finisher).Finish(ResultBlack, "resign")
	v = st.View(White)
	if len(v.Board) != 32 || v.Board["e5"] != "bP" || v.Legal != nil {
		t.Errorf("after resignation: %d pieces, legal %v", len(v.Board), v.Legal)
	}
	if x := v.Extra.(blindExtra); x.Blind {
		t.Error("still blind after resignation")
	}
}

func TestBlindfoldTypedMoves(t *testing.T) {
	type step struct{ in, san string }
	st := newBlind(t, false)
	for _, s := range []step{
		{"e4", "e4"}, {"e7e5", "e5"}, {"Nf3", "Nf3"}, {"nc6", "Nc6"}, {"Bc4", "Bc4"}, {"Bc5", "Bc5"},
		{"d3", "d3"}, {"Nf6", "Nf6"}, {"Nc3", "Nc3"}, {"d6", "d6"}, {"Bg5", "Bg5"}, {"h6", "h6"},
		{"Bxf6", "Bxf6"}, {"Qxf6", "Qxf6"}, {"Nd5", "Nd5"}, {"Qd8", "Qd8"},
		{"0-0", "O-O"}, {"o-o", "O-O"},
	} {
		note, err := st.Apply(s.in)
		if err != nil {
			t.Fatalf("%q: %v (legal %v)", s.in, err, st.Legal())
		}
		if note != s.san {
			t.Errorf("%q gives %q, want %q", s.in, note, s.san)
		}
	}
	p := st.(*blindState).Position()
	if p.B[MakeSq(6, 0)].Kind != King || p.B[MakeSq(5, 0)].Kind != Rook {
		t.Errorf("castling by typing: %s", p.Placement())
	}
}

func TestBlindfoldTypedForms(t *testing.T) {
	// the same move in the many ways a person writes it
	for _, in := range []string{"Nf3", "nf3", "NF3", "Ng1f3", "g1f3", "G1F3", "g1-f3", "Ng1-f3", " Nf3 ", "Nf3+", "N f3"} {
		st := newBlind(t, false)
		if note, err := st.Apply(in); err != nil || note != "Nf3" {
			t.Errorf("%q: %q %v", in, note, err)
		}
	}
	// a pawn capture: "exd5", "ed5", "e4d5"; a file b pawn is not a bishop
	st := newBlind(t, false)
	playAll(t, st, "e2e4", "d7d5")
	for _, in := range []string{"exd5", "ed5", "e4d5", "e4xd5"} {
		g, _ := Replay("blindfold", st.(*blindState).Options(), []string{"e2e4", "d7d5"})
		if note, err := g.Apply(in); err != nil || note != "exd5" {
			t.Errorf("%q: %q %v", in, note, err)
		}
	}
	st = newBlind(t, false)
	playAll(t, st, "b1c3", "a7a6", "e2e4", "b7b5", "f1b5")
	if note, err := st.Apply("axb5"); err != nil || note != "axb5" {
		t.Errorf("axb5: %q %v", note, err)
	}
	st = newBlind(t, false)
	playAll(t, st, "e2e4", "e7e5")
	if note, err := st.Apply("bc4"); err != nil || note != "Bc4" {
		t.Errorf("a lower-case b with no pawn to read it as falls back to the bishop: %q %v", note, err)
	}
}

func TestBlindfoldTypedPromotion(t *testing.T) {
	for _, in := range []string{"e8=Q", "e8Q", "e7e8q", "e8=q"} {
		g := newBlind(t, false).(*blindState)
		g.Game.Edit(func(p *Position) { *p = *mustFEN("7k/4P3/8/8/8/8/8/K7 w - - 0 1") })
		note, err := g.Apply(in)
		if err != nil || note != "e8=Q+" {
			t.Errorf("%q: %q %v", in, note, err)
		}
	}
	g := newBlind(t, false).(*blindState)
	g.Game.Edit(func(p *Position) { *p = *mustFEN("7k/4P3/8/8/8/8/8/K7 w - - 0 1") })
	_, err := g.Apply("e8")
	if err == nil || !strings.Contains(err.Error(), "превращения") {
		t.Errorf("a promotion without its piece: %v", err)
	}
	if note, err := g.Apply("e8=N"); err != nil || note != "e8=N" {
		t.Errorf("e8=N: %q %v", note, err)
	}
}

func TestBlindfoldPoliteErrors(t *testing.T) {
	st := newBlind(t, false)
	for in, want := range map[string]string{
		"":               "Введите ход",
		"hello":          "Не понял запись",
		"Nf9":            "Не понял запись",
		"Кf3":            "латиницей",
		"Nf6":            "сейчас нет",
		"e5":             "сейчас нет",
		"Qh5":            "сейчас нет",
		"O-O":            "Короткая рокировка сейчас невозможна",
		"O-O-O":          "Длинная рокировка сейчас невозможна",
		"e2e4e5e6e7e8e9": "Не понял запись",
	} {
		_, err := st.Apply(in)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want something with %q", in, err, want)
		}
	}
	// nothing was played
	if g := st.(*blindState); len(g.Legal()) != 20 || g.Turn() != White {
		t.Error("a refused text changed the game")
	}
	// ambiguity: two knights reach d2
	g := newBlind(t, false).(*blindState)
	// knights on b1 and f1 both reach d2
	g.Game.Edit(func(p *Position) { *p = *mustFEN("4k3/8/8/8/8/8/8/1N2KN2 w - - 0 1") })
	_, err := g.Apply("Nd2")
	if err == nil || !strings.Contains(err.Error(), "Уточните") || !strings.Contains(err.Error(), "Nbd2") || !strings.Contains(err.Error(), "Nfd2") {
		t.Errorf("ambiguous Nd2: %v", err)
	}
	if note, err := g.Apply("Nbd2"); err != nil || note != "Nbd2" {
		t.Errorf("Nbd2: %q %v", note, err)
	}
}

// A game with typed moves is restored by replaying the actions as they were sent.
func TestBlindfoldReplay(t *testing.T) {
	_, o, _ := Prepare("blindfold", map[string]any{"peek": true}, 3)
	actions := strings.Fields("e4 e5 Nf3 Nc6 Nc3 d7d5")
	st, err := Replay("blindfold", o, actions)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.(*blindState).Position().Placement(); got != "r1bqkbnr/ppp2ppp/2n5/3pp3/4P3/2N2N2/PPPP1PPP/R1BQKB1R" {
		t.Errorf("placement %s", got)
	}
	a, _ := Replay("blindfold", o, actions)
	if a.(*blindState).Position().FEN() != st.(*blindState).Position().FEN() {
		t.Error("two replays differ")
	}
	if _, err := Replay("blindfold", o, []string{"e4", "e4"}); err == nil {
		t.Error("an impossible typed move was replayed")
	}
}

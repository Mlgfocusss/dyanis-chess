package variants

import (
	"math/rand"
	"sort"
	"strings"
	"testing"

	"dyanis-chess/engine"
)

func sortedActions(a []string) []string {
	out := append([]string(nil), a...)
	sort.Strings(out)
	return out
}

func perft(t *testing.T, fen string, want ...uint64) {
	t.Helper()
	p, err := ParseFEN(fen)
	if err != nil {
		t.Fatal(err)
	}
	for d, w := range want {
		if got := Perft(ClassicRules{}, p, d+1); got != w {
			t.Errorf("perft(%d) of %q = %d, want %d", d+1, fen, got, w)
		}
	}
}

func TestPerftStartPosition(t *testing.T) {
	perft(t, StartFEN, 20, 400, 8902, 197281)
}

func TestPerftKnownPositions(t *testing.T) {
	// the standard test positions of the chess programming wiki
	perft(t, "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1", 48, 2039, 97862)
	perft(t, "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1", 14, 191, 2812, 43238)
	perft(t, "r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1", 6, 264, 9467)
	perft(t, "rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8", 44, 1486, 62379)
	perft(t, "r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10", 46, 2079, 89890)
}

func TestPerftChess960(t *testing.T) {
	// Chess960 positions from the same list; the castling is the Fischer one.
	perft(t, "bqnb1rkr/pp3ppp/3ppn2/2p5/5P2/P2P4/NPP1P1PP/BQ1BNRKR w HFhf - 2 9", 21, 528, 12189)
	perft(t, "2nnrbkr/p1qppppp/8/1ppb4/6PP/3PP3/PPP2P2/BQNNRBKR w HEhe - 1 9", 21, 807, 18002)
	perft(t, "b1q1rrkb/pppppppp/3nn3/8/P7/1PPP4/4PPPP/BQNNRKRB w GE - 1 9", 20, 479, 10471)
}

// A castling in Chess960 where the king does not move, or the rook is next to it.
func TestCastlingOfChess960(t *testing.T) {
	r := chess960Rules{}
	p, _ := ParseFEN("4k3/8/8/8/8/8/8/5RKR w HF - 0 1")
	got := map[string]bool{}
	for _, m := range p.LegalMoves(r) {
		got[Encode(r, m)] = true
	}
	// king g1 with rooks f1 and h1: O-O (king stays on g1, rook h1 goes to f1) is "g1h1"
	// but f1 stands there, so the king side is blocked; O-O-O needs f1 gone: no.
	if got["g1h1"] {
		t.Errorf("castling through the rook of the other side should be blocked: %v", got)
	}
	p, _ = ParseFEN("4k3/8/8/8/8/8/8/6KR w H - 0 1")
	got = map[string]bool{}
	for _, m := range p.LegalMoves(r) {
		got[Encode(r, m)] = true
	}
	if !got["g1h1"] {
		t.Errorf("the king stays on g1 and the rook crosses: want g1h1 among %v", got)
	}
	next := p.Apply(r, Move{From: MakeSq(6, 0), To: MakeSq(6, 0), Castle: true, Rook: MakeSq(7, 0)})
	if next.B[MakeSq(6, 0)] != (Piece{Kind: King, Side: White}) || next.B[MakeSq(5, 0)] != (Piece{Kind: Rook, Side: White}) || !next.B[MakeSq(7, 0)].Empty() {
		t.Errorf("after the castling: %s", next.Placement())
	}
}

// chess960Rules is the classical rules with the castling written as king-takes-rook.
type chess960Rules struct{ ClassicRules }

func (chess960Rules) CastleAsKingTakesRook() bool { return true }

func TestFENRoundTrip(t *testing.T) {
	for _, fen := range []string{
		StartFEN,
		"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1",
		"rnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq e6 0 2",
		"bqnb1rkr/pp3ppp/3ppn2/2p5/5P2/P2P4/NPP1P1PP/BQ1BNRKR w KQkq - 2 9", // X-FEN: the outermost rooks are K and Q
		"4k3/8/8/8/8/8/8/1R2KR1R w FQ - 0 1",                                // inner rooks need their files
	} {
		p, err := ParseFEN(fen)
		if err != nil {
			t.Fatal(err)
		}
		got := p.FEN()
		// the en passant square is written only when a pawn can take
		if fen == "rnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq e6 0 2" {
			fen = "rnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq - 0 2"
		}
		if got != fen {
			t.Errorf("FEN round trip:\n got %s\nwant %s", got, fen)
		}
	}
	for _, bad := range []string{"", "8/8/8/8/8/8/8 w - - 0 1", "9/8/8/8/8/8/8/8 w - - 0 1", "8/8/8/8/8/8/8/8 x - - 0 1", "8/8/8/8/8/8/8/7z w - - 0 1"} {
		if _, err := ParseFEN(bad); err == nil {
			t.Errorf("ParseFEN(%q) accepted", bad)
		}
	}
}

// The base against the engine: 200 random games of up to 80 plies; in every
// position the set of legal moves is the engine's, and so is the verdict.
func TestAgainstTheEngine(t *testing.T) {
	games := 200
	if testing.Short() {
		games = 30
	}
	rng := rand.New(rand.NewSource(20261021))
	positions := 0
	ended := map[string]int{}
	for gi := 0; gi < games; gi++ {
		ep := engine.NewPosition()
		hashes := []uint64{ep.Hash()}
		st, err := Get0("classic", Options{})
		if err != nil {
			t.Fatal(err)
		}
		g := st.(*Game)
		for ply := 0; ply < 80; ply++ {
			positions++
			var want []string
			emoves := ep.LegalMoves()
			for _, m := range emoves {
				want = append(want, m.String())
			}
			got := g.Legal()
			if res, reason, over := g.Over(); over {
				out := ep.Outcome(hashes)
				if !out.Over {
					t.Fatalf("game %d ply %d: we ended (%s %s), the engine did not: %s", gi, ply, res, reason, g.pos.FEN())
				}
				if (reason == ReasonCheckmate) != out.Checkmate {
					t.Fatalf("game %d ply %d: mate disagreement (%s vs %q): %s", gi, ply, reason, out.Reason, g.pos.FEN())
				}
				ended[reason]++
				break
			}
			if out := ep.Outcome(hashes); out.Over {
				t.Fatalf("game %d ply %d: the engine ended (%s), we did not: %s", gi, ply, out.Reason, g.pos.FEN())
			}
			gs, ws := sortedActions(got), sortedActions(want)
			if strings.Join(gs, " ") != strings.Join(ws, " ") {
				t.Fatalf("game %d ply %d: moves differ in %s\n ours   %v\n engine %v", gi, ply, ep.FEN(), gs, ws)
			}
			// the placement, side, castling rights and clocks agree too
			if a, b := strings.Join(strings.Fields(g.pos.FEN())[:3], " "), strings.Join(strings.Fields(ep.FEN())[:3], " "); a != b {
				t.Fatalf("game %d ply %d: positions differ:\n ours   %s\n engine %s", gi, ply, g.pos.FEN(), ep.FEN())
			}
			// the notation is the engine's SAN
			pick := rng.Intn(len(emoves))
			wantSAN := ep.SAN(emoves[pick])
			gotSAN, err := g.Apply(want[pick])
			if err != nil {
				t.Fatalf("game %d ply %d: %s refused: %v", gi, ply, want[pick], err)
			}
			if gotSAN != wantSAN {
				t.Fatalf("game %d ply %d: SAN %q, the engine writes %q in %s", gi, ply, gotSAN, wantSAN, ep.FEN())
			}
			ep.Play(emoves[pick])
			hashes = append(hashes, ep.Hash())
		}
	}
	t.Logf("%d games, %d positions compared; endings: %v", games, positions, ended)
}

// Get0 makes a game of a registered variant (a test helper).
func Get0(id string, o Options) (State, error) {
	v, ok := Get(id)
	if !ok {
		return nil, ErrIllegal
	}
	return v.New(o)
}

// The endings the random games rarely reach, from set-up positions.
func TestClassicEndings(t *testing.T) {
	cases := []struct {
		fen, result, reason string
	}{
		{"7k/5Q2/6K1/8/8/8/8/8 b - - 0 1", ResultDraw, ReasonStalemate},
		{"R6k/8/6K1/8/8/8/8/8 b - - 0 1", ResultWhite, ReasonCheckmate},
		{"8/8/8/4k3/8/8/8/4K3 w - - 0 1", ResultDraw, ReasonInsufficient},
		{"8/8/8/4k3/8/8/8/3NK3 w - - 0 1", ResultDraw, ReasonInsufficient},
		{"8/8/8/4k3/8/8/8/3NKN2 w - - 0 1", "", ""},
		{"8/8/8/4k3/8/8/4P3/4K3 w - - 100 80", ResultDraw, ReasonFiftyMoves},
		{"8/8/8/4k3/8/8/4P3/4K3 w - - 99 80", "", ""},
	}
	for _, c := range cases {
		g := &Game{rules: ClassicRules{}}
		p, err := ParseFEN(c.fen)
		if err != nil {
			t.Fatal(err)
		}
		g.pos, g.keys = p, []string{positionKey(p)}
		g.Judge()
		res, reason, _ := g.Over()
		if res != c.result || reason != c.reason {
			t.Errorf("%s: got %q %q, want %q %q", c.fen, res, reason, c.result, c.reason)
		}
	}
}

func TestThreefoldRepetition(t *testing.T) {
	st, _ := Get0("classic", Options{})
	for i, a := range strings.Fields("g1f3 g8f6 f3g1 f6g8 g1f3 g8f6 f3g1 f6g8") {
		if _, err := st.Apply(a); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
		_, reason, over := st.Over()
		if over != (i == 7) || (over && reason != ReasonRepetition) {
			t.Fatalf("after %d plies: over=%v reason=%s", i+1, over, reason)
		}
	}
	if _, err := st.Apply("e2e4"); err != ErrOver {
		t.Errorf("a move after the end: %v", err)
	}
}

func TestFlagAgainstALoneKnightIsADraw(t *testing.T) {
	g := &Game{rules: ClassicRules{}}
	p, _ := ParseFEN("8/8/8/4k3/8/8/8/3NK3 w - - 0 1")
	g.pos = p
	if got := g.Timeout(Black); got != ResultDraw {
		t.Errorf("White has a lone knight, Black's flag falls: %s", got)
	}
	p, _ = ParseFEN("8/8/8/4k3/8/8/4P3/3NK3 w - - 0 1")
	g.pos = p
	if got := g.Timeout(Black); got != ResultWhite {
		t.Errorf("White has a pawn, Black's flag falls: %s", got)
	}
}

func TestViewOfClassic(t *testing.T) {
	st, _ := Get0("classic", Options{})
	w, b, s := st.View(White), st.View(Black), st.View(NoSide)
	if len(w.Legal) != 20 || len(b.Legal) != 0 || len(s.Legal) != 0 {
		t.Errorf("legal: white %d, black %d, spectator %d", len(w.Legal), len(b.Legal), len(s.Legal))
	}
	if w.Board["e1"] != "wK" || w.Board["d8"] != "bQ" || len(w.Board) != 32 || w.Turn != White {
		t.Errorf("board: %v", w.Board)
	}
	st.Apply("e2e4")
	st.Apply("f7f6")
	st.Apply("d1h5")
	v := st.View(Black)
	if len(v.Check) != 1 || v.Check[0] != "e8" || len(v.Last) != 2 || v.Last[0] != "d1" || v.Last[1] != "h5" {
		t.Errorf("check %v, last %v", v.Check, v.Last)
	}
	if _, err := st.Apply("e8e7"); err == nil {
		t.Errorf("a king step into a line of check")
	}
}

func TestSANDisambiguationAndSuffix(t *testing.T) {
	g := &Game{rules: ClassicRules{}}
	p, _ := ParseFEN("4k3/8/8/8/8/8/8/R3K2R w KQ - 0 1")
	g.pos, g.keys = p, []string{positionKey(p)}
	for action, want := range map[string]string{"a1b1": "Rb1", "a1a8": "Ra8+", "e1g1": "O-O", "e1c1": "O-O-O"} {
		note, err := g.Apply(action)
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if note != want {
			t.Errorf("%s written %q, want %q", action, note, want)
		}
		g.pos, g.keys, g.over, g.legalOK = p, []string{positionKey(p)}, false, false
	}
	// two rooks that can reach d1: the file tells them apart
	p2, _ := ParseFEN("4k3/8/8/8/8/8/4K3/R6R w - - 0 1")
	g.pos, g.keys, g.legalOK = p2, []string{positionKey(p2)}, false
	note, _ := g.Apply("a1d1")
	if note != "Rad1" {
		t.Errorf("disambiguation: %q", note)
	}
}

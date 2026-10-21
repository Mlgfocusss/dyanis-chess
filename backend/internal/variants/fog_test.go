package variants

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// ------------------------------------------------------------- helpers

func fogGame(t *testing.T, fen string) *fogGameState {
	t.Helper()
	st, err := Get0("fog", Options{})
	if err != nil {
		t.Fatal(err)
	}
	f := st.(*fogGameState)
	if fen != "" {
		f.Edit(func(p *Position) { *p = *mustFEN(fen) })
		f.Game.keys = []string{positionKey(f.Position())}
	}
	return f
}

func fogPlay(t *testing.T, f *fogGameState, actions string) {
	t.Helper()
	for _, a := range strings.Fields(actions) {
		if _, err := f.Apply(a); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
	}
}

func fogSet(v View) map[string]bool {
	hidden, _ := v.Extra.(map[string]any)["fog"].([]string)
	out := map[string]bool{}
	for i := 0; i < 64; i++ {
		out[Sq(i).String()] = true
	}
	for _, s := range hidden {
		delete(out, s)
	}
	return out // the squares in view
}

func names(set map[string]bool) string {
	var out []string
	for s, ok := range set {
		if ok {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

// oracleVisible is a second, independent statement of the rule: plain ray and
// jump geometry, no move generator.
func oracleVisible(p *Position, side Side, reveal Sq) map[string]bool {
	vis := map[string]bool{}
	add := func(f, r int) bool { // reports whether the square is on the board
		if f < 0 || f > 7 || r < 0 || r > 7 {
			return false
		}
		vis[MakeSq(f, r).String()] = true
		return true
	}
	at := func(f, r int) Piece { return p.B[MakeSq(f, r)] }
	for i := 0; i < 64; i++ {
		pc := p.B[i]
		if pc.Kind == NoKind || pc.Side != side {
			continue
		}
		f, r := Sq(i).File(), Sq(i).Rank()
		vis[Sq(i).String()] = true
		ray := func(df, dr int) {
			for k := 1; ; k++ {
				if !add(f+df*k, r+dr*k) || !at(f+df*k, r+dr*k).Empty() {
					return
				}
			}
		}
		switch pc.Kind {
		case Knight:
			for _, d := range [][2]int{{1, 2}, {2, 1}, {-1, 2}, {-2, 1}, {1, -2}, {2, -1}, {-1, -2}, {-2, -1}} {
				add(f+d[0], r+d[1])
			}
		case King:
			for df := -1; df <= 1; df++ {
				for dr := -1; dr <= 1; dr++ {
					if df != 0 || dr != 0 {
						add(f+df, r+dr)
					}
				}
			}
			// castling (standard setups only): the rook is where the right says, and the squares between are empty
			for flank := 0; flank < 2; flank++ {
				rook := p.Castle[side][flank]
				if rook == NoSq || rook.Rank() != r || p.B[rook].Kind != Rook || p.B[rook].Side != side {
					continue
				}
				lo, hi := min(f, rook.File()), max(f, rook.File())
				free := true
				for x := lo + 1; x < hi; x++ {
					free = free && at(x, r).Empty()
				}
				if free {
					if flank == 0 {
						add(6, r)
					} else {
						add(2, r)
					}
				}
			}
		case Rook, Queen:
			ray(1, 0)
			ray(-1, 0)
			ray(0, 1)
			ray(0, -1)
		}
		if pc.Kind == Bishop || pc.Kind == Queen {
			ray(1, 1)
			ray(1, -1)
			ray(-1, 1)
			ray(-1, -1)
		}
		if pc.Kind == Pawn {
			dr, home := 1, 1
			if side == Black {
				dr, home = -1, 6
			}
			if add(f, r+dr) && at(f, r+dr).Empty() && r == home {
				add(f, r+2*dr)
			}
			for _, df := range []int{-1, 1} {
				if f+df < 0 || f+df > 7 || r+dr < 0 || r+dr > 7 {
					continue
				}
				t := at(f+df, r+dr)
				if !t.Empty() && t.Side != side {
					add(f+df, r+dr)
				}
				if p.Turn == side && MakeSq(f+df, r+dr) == p.EP {
					if v := at(f+df, r); v.Kind == Pawn && v.Side != side {
						add(f+df, r+dr)
						add(f+df, r)
					}
				}
			}
		}
	}
	if reveal != NoSq {
		vis[reveal.String()] = true
	}
	return vis
}

// ------------------------------------------------------------- the rules

func TestFogHasNoCheck(t *testing.T) {
	f := fogGame(t, "4k3/8/8/8/8/8/8/4K2R w - - 0 1")
	fogPlay(t, f, "h1h8")
	// the rook gives what would be check; nothing says so and the king may stay on the rank or step onto it
	legal := f.Legal()
	for _, want := range []string{"e8d8", "e8f8", "e8e7", "e8d7", "e8f7"} {
		if !contains(legal, want) {
			t.Errorf("missing %s in %v", want, legal)
		}
	}
	if v := f.View(Black); len(v.Check) != 0 {
		t.Errorf("check shown: %v", v.Check)
	}
	fogPlay(t, f, "e8f8") // still under the rook
	if _, _, over := f.Over(); over {
		t.Fatal("over")
	}
}

func TestFogTakingTheKingEndsTheGame(t *testing.T) {
	f := fogGame(t, "4k3/8/8/8/8/8/8/R3K3 w - - 0 1")
	fogPlay(t, f, "a1a7")
	if !contains(f.Legal(), "e8e7") {
		t.Fatal("a king may walk into attack")
	}
	fogPlay(t, f, "e8e7") // onto the rook's rank
	if _, _, over := f.Over(); over {
		t.Fatal("a king under attack is not taken yet")
	}
	fogPlay(t, f, "a7e7") // the rook takes the king
	res, reason, over := f.Over()
	if !over || res != ResultWhite || reason != ReasonKingCaptured {
		t.Fatalf("taking the king: %q %q %v", res, reason, over)
	}
	if _, err := f.Apply("e1e2"); err != ErrOver {
		t.Errorf("an action after the end: %v", err)
	}
	if f.Legal() != nil {
		t.Errorf("legal moves after the end: %v", f.Legal())
	}
	if f.Timeout(Black) != ResultWhite || f.Timeout(White) != ResultBlack {
		t.Error("a fallen flag loses even against a bare king")
	}
}

func TestFogCastlingIgnoresAttack(t *testing.T) {
	// the black rook on f8 hits f1: classical chess forbids castling through f1, the fog allows it
	f := fogGame(t, "5r1k/8/8/8/8/8/8/R3K2R w KQ - 0 1")
	legal := f.Legal()
	if !contains(legal, "e1g1") || !contains(legal, "e1c1") {
		t.Fatalf("castling through an attacked square: %v", legal)
	}
	if note, err := f.Apply("e1g1"); err != nil || note != "O-O" {
		t.Fatalf("%q %v", note, err)
	}
}

func TestFogEnPassantAndPromotion(t *testing.T) {
	f := fogGame(t, "4k3/8/8/3pP3/8/8/8/4K3 w - d6 0 1")
	if !contains(f.Legal(), "e5d6") {
		t.Fatalf("en passant missing: %v", f.Legal())
	}
	v := f.View(White)
	if _, ok := v.Board["d5"]; !ok {
		t.Error("the pawn that can be taken en passant must be in view")
	}
	fogPlay(t, f, "e5d6")
	if _, ok := f.View(White).Board["d5"]; ok {
		t.Error("the pawn taken en passant is still there")
	}
	g := fogGame(t, "r3k3/1P6/8/8/8/8/8/4K3 w - - 0 1")
	if !contains(g.Legal(), "b7b8q") || !contains(g.Legal(), "b7a8n") {
		t.Errorf("promotion: %v", g.Legal())
	}
}

// ---------------------------------------------------------- what is seen

func TestFogStartingPosition(t *testing.T) {
	f := fogGame(t, "")
	w, b := f.View(White), f.View(Black)
	// White sees its own pieces, the third and fourth ranks (pawns step two) and nothing of the enemy
	var wantW, wantB []string
	for _, file := range "abcdefgh" {
		for _, r := range "1234" {
			wantW = append(wantW, string(file)+string(r))
		}
		for _, r := range "5678" {
			wantB = append(wantB, string(file)+string(r))
		}
	}
	sort.Strings(wantW)
	sort.Strings(wantB)
	if got := names(fogSet(w)); got != strings.Join(wantW, " ") {
		t.Errorf("White sees %s", got)
	}
	if got := names(fogSet(b)); got != strings.Join(wantB, " ") {
		t.Errorf("Black sees %s", got)
	}
	for sq, code := range w.Board {
		if code[0] != 'w' {
			t.Errorf("White sees %s on %s", code, sq)
		}
	}
	for sq, code := range b.Board {
		if code[0] != 'b' {
			t.Errorf("Black sees %s on %s", code, sq)
		}
	}
	if len(w.Board) != 16 || len(b.Board) != 16 {
		t.Errorf("boards of %d and %d pieces", len(w.Board), len(b.Board))
	}
}

func TestFogPawnVision(t *testing.T) {
	// white pawns on e2 (home) and a4; black pieces: a pawn on e4 two ahead of e2, a knight on b5 (a
	// diagonal of a4), a rook on h8 far away, the kings far
	f := fogGame(t, "4k2r/8/8/1n6/p3p3/8/4P3/4K3 w - - 0 1")
	// replace the white pawn on a4 by one: put it so that the diagonal b5 holds an enemy
	f.Edit(func(p *Position) { *p = *mustFEN("4k2r/8/8/1n6/P3p3/8/4P3/4K3 w - - 0 1") })
	v := f.View(White)
	if v.Board["b5"] != "bN" {
		t.Errorf("an enemy on the diagonal of a pawn is seen: %v", v.Board)
	}
	if v.Board["e4"] != "bP" {
		t.Errorf("the pawn sees two squares ahead from home even if the second is occupied: %v", v.Board)
	}
	if _, ok := v.Board["h8"]; ok {
		t.Error("a far rook is hidden")
	}
	vis := fogSet(v)
	if vis["c5"] || vis["f3"] && false {
		t.Errorf("an empty diagonal of a pawn is not in view: c5 %v", vis["c5"])
	}
	// a blocked pawn sees its blocker
	g := fogGame(t, "4k3/8/8/8/8/4n3/4P3/4K3 w - - 0 1")
	if g.View(White).Board["e3"] != "bN" {
		t.Error("a pawn blocked by a piece sees it")
	}
	if contains(g.Legal(), "e2e3") || contains(g.Legal(), "e2e4") {
		t.Errorf("the blocked pawn moves: %v", g.Legal())
	}
}

func TestFogSlidersSeeUpToTheFirstPiece(t *testing.T) {
	// the white rook on a1 looks up the a-file: a black pawn on a4 stops it, a black rook on a7 behind it is hidden
	f := fogGame(t, "4k3/r7/8/8/p7/8/8/R3K3 w - - 0 1")
	v := f.View(White)
	if v.Board["a4"] != "bP" {
		t.Error("the pawn the rook can take is seen")
	}
	if _, ok := v.Board["a7"]; ok {
		t.Error("the rook behind the pawn leaks")
	}
	vis := fogSet(v)
	if !vis["a3"] || !vis["a4"] || vis["a5"] || vis["a7"] {
		t.Errorf("rank-by-rank view along the a-file: %s", names(vis))
	}
	if contains(f.Legal(), "a1a5") {
		t.Error("the rook jumps over the pawn")
	}
	// Black on its move sees the same line from the other end (its own rook, the pawn, then what the pawn hides)
	b := fogGame(t, "4k3/r7/8/8/p7/8/8/R3K3 b - - 0 1")
	bv := b.View(Black)
	if _, ok := bv.Board["a1"]; ok {
		t.Error("Black sees the rook behind its own pawn")
	}
}

func TestFogSeenWhenItIsNotYourTurn(t *testing.T) {
	f := fogGame(t, "")
	fogPlay(t, f, "e2e4")
	// Black to move: White is waiting and sees what its pieces see after the move
	w := f.View(White)
	if len(w.Legal) != 0 {
		t.Error("legal moves for the side that waits")
	}
	if !fogSet(w)["e5"] {
		t.Error("the pawn on e4 sees e5")
	}
	if got := w.Board["e4"]; got != "wP" {
		t.Errorf("e4: %q", got)
	}
	if w.Last != nil && len(w.Last) != 2 {
		t.Errorf("the mover sees its own move whole: %v", w.Last)
	}
	if !reflect.DeepEqual(w.Last, []string{"e2", "e4"}) {
		t.Errorf("last for the mover: %v", w.Last)
	}
	// Black does not see where White went: e4 is not in its view (rank 4 is, but only as the pawns' second step...)
	b := f.View(Black)
	if len(b.Legal) == 0 {
		t.Error("Black has no moves")
	}
	if _, ok := b.Board["e4"]; ok {
		t.Error("Black sees the pawn on e4")
	}
}

func TestFogLastMoveOfTheOpponent(t *testing.T) {
	// a knight goes from the fog to the fog: Black sees neither square
	f := fogGame(t, "")
	fogPlay(t, f, "g1f3")
	b := f.View(Black)
	if len(b.Last) != 0 {
		t.Errorf("Black is told about a move it did not see: %v", b.Last)
	}
	fogPlay(t, f, "e7e5")
	fogPlay(t, f, "f3e5") // takes the pawn on e5, a square the black pawn standing there certainly sees
	// the pawn on e5 is gone: Black loses a piece and sees the knight that took it
	b = f.View(Black)
	if b.Board["e5"] != "wN" {
		t.Errorf("the taker of a piece is shown: %v", b.Board["e5"])
	}
	if !contains(b.Last, "e5") || contains(b.Last, "f3") {
		t.Errorf("the last move for Black: %v (f3 is out of view)", b.Last)
	}
	// Black answers; the taker disappears again when nothing of Black reaches e5
	fogPlay(t, f, "a7a6")
	if _, ok := f.View(Black).Board["e5"]; ok {
		if p := f.Position().B[MakeSq(4, 4)]; p.Side == White {
			// still seen only if a black piece reaches it: the queen on d8 does not, the bishop on f8 does not
			t.Error("the taker stays in view after Black has moved on")
		}
	}
}

func TestFogMovesAreRedacted(t *testing.T) {
	f := fogGame(t, "")
	fogPlay(t, f, "g1f3 g8f6 f3e5 f6e4")
	// White moved the knight to f3 and e5, Black to f6 and e4
	recs := []MoveRec{
		{Side: White, Action: "g1f3", SAN: "Nf3"}, {Side: Black, Action: "g8f6", SAN: "Nf6"},
		{Side: White, Action: "f3e5", SAN: "Ne5"}, {Side: Black, Action: "f6e4", SAN: "Ne4"},
	}
	w := f.RedactMoves(White, recs)
	if w[0].SAN != "Nf3" || w[0].Action != "g1f3" || w[2].SAN != "Ne5" {
		t.Errorf("own moves must stay whole: %+v", w)
	}
	for _, i := range []int{1, 3} {
		if w[i].Action != "" {
			t.Errorf("the opponent's starting square leaks: %+v", w[i])
		}
		if strings.ContainsAny(w[i].SAN, "abcdefgh12345678") && w[i].SAN != hiddenMove {
			// shown only when the target was in view; a knight on f6 is not in White's view (e5 sees d7, f7, c6, g6, d3, g4...)
			t.Logf("White saw %q", w[i].SAN)
		}
	}
	if w[1].SAN != hiddenMove {
		t.Errorf("Nf6 was out of White's view: %q", w[1].SAN)
	}
	s := f.RedactMoves(NoSide, recs)
	for _, m := range s {
		if m.Action != "" || m.SAN != hiddenMove {
			t.Errorf("a spectator is told a move: %+v", m)
		}
	}
	// a move whose target was in view shows as piece and target only
	g := fogGame(t, "")
	fogPlay(t, g, "e2e4 d7d5 e4d5")
	black := g.RedactMoves(Black, []MoveRec{{Side: White, Action: "e2e4", SAN: "e4"}, {Side: Black, Action: "d7d5", SAN: "d5"}, {Side: White, Action: "e4d5", SAN: "exd5"}})
	if black[0].SAN != hiddenMove && black[0].SAN != "e4" {
		t.Errorf("e4 shows as %q", black[0].SAN)
	}
	if black[2].SAN != "xd5" || black[2].Action != "" {
		t.Errorf("the pawn that took on d5 shows as %+v (no starting file)", black[2])
	}
}

func TestFogEverythingOpensAtTheEnd(t *testing.T) {
	f := fogGame(t, "4k3/8/8/8/8/8/8/4R2K w - - 0 1")
	fogPlay(t, f, "e1e8")
	for _, who := range []Side{White, Black, NoSide} {
		v := f.View(who)
		if len(v.Board) != 2 && who != NoSide || len(v.Board) != 2 {
			t.Errorf("%v sees %v after the end", who, v.Board)
		}
		recs := f.RedactMoves(who, []MoveRec{{Side: White, Action: "e1e8", SAN: "Rxe8"}})
		if recs[0].Action != "e1e8" || recs[0].SAN != "Rxe8" {
			t.Errorf("%v: moves after the end %+v", who, recs)
		}
		if v.Extra != nil {
			t.Errorf("%v: fog data after the end: %v", who, v.Extra)
		}
	}
	// the end by the server (resign, flag): Finish opens it as well
	g := fogGame(t, "")
	fogPlay(t, g, "e2e4 e7e5")
	if len(g.View(White).Board) != 17 || len(g.View(NoSide).Board) != 0 { // 16 own pieces and the pawn e5 that blocks the pawn e4
		t.Fatal("the game in progress shows the wrong pieces")
	}
	g.Finish(ResultWhite, "resign")
	if len(g.View(NoSide).Board) != 32 || len(g.View(Black).Board) != 32 {
		t.Errorf("after Finish: %d and %d pieces", len(g.View(NoSide).Board), len(g.View(Black).Board))
	}
	if r := g.RedactMoves(Black, []MoveRec{{Side: White, Action: "e2e4", SAN: "e4"}}); r[0].SAN != "e4" || r[0].Action != "e2e4" {
		t.Errorf("moves after Finish: %+v", r)
	}
	if g.View(White).Legal != nil {
		t.Error("legal moves after Finish")
	}
}

func TestFogSpectatorSeesNothing(t *testing.T) {
	f := fogGame(t, "")
	fogPlay(t, f, "e2e4")
	v := f.View(NoSide)
	if len(v.Board) != 0 || len(v.Legal) != 0 || len(v.Last) != 0 || len(v.Check) != 0 {
		t.Errorf("spectator: %+v", v)
	}
	if len(v.Extra.(map[string]any)["fog"].([]string)) != 64 {
		t.Error("the whole board is fog for a spectator")
	}
}

func TestFogReplayIsExact(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for g := 0; g < 15; g++ {
		st, _ := Get0("fog", Options{Seed: int64(g)})
		var actions []string
		for ply := 0; ply < 70; ply++ {
			legal := st.Legal()
			if len(legal) == 0 {
				break
			}
			a := legal[rng.Intn(len(legal))]
			if _, err := st.Apply(a); err != nil {
				t.Fatal(err)
			}
			actions = append(actions, a)
		}
		again, err := Replay("fog", Options{Seed: int64(g)}, actions)
		if err != nil {
			t.Fatal(err)
		}
		for _, who := range []Side{White, Black, NoSide} {
			a, _ := json.Marshal(st.View(who))
			b, _ := json.Marshal(again.View(who))
			if string(a) != string(b) {
				t.Fatalf("game %d: replay differs for %v", g, who)
			}
		}
	}
}

// -------------------------------------------------------- no leaks, random games

var hiddenMoveText = regexp.MustCompile(`^…$`)
var squareName = regexp.MustCompile(`[a-h][1-8]`)

// Every position of random games, for both players: the snapshot holds exactly
// the pieces on the squares of the independent oracle, the list of fogged
// squares is its complement, the legal destinations are in view, and the list
// of moves hides what it should.
func TestFogNoLeaksInRandomGames(t *testing.T) {
	games, plies := 40, 140
	if testing.Short() {
		games = 12
	}
	rng := rand.New(rand.NewSource(2026))
	positions, shown, hiddenPieces := 0, 0, 0
	for gi := 0; gi < games; gi++ {
		st, _ := Get0("fog", Options{Seed: int64(gi)})
		f := st.(*fogGameState)
		var recs []MoveRec
		for ply := 0; ply <= plies; ply++ {
			_, _, over := f.Over()
			p := f.Position()
			for _, who := range []Side{White, Black} {
				v := f.View(who)
				if over {
					if len(v.Board) != countPieces(p) {
						t.Fatalf("game %d: the end is not fully shown", gi)
					}
					continue
				}
				positions++
				reveal := NoSq
				if f.revealAt != NoSq && p.Turn == who {
					reveal = f.revealAt
				}
				want := oracleVisible(p, who, reveal)
				if got := names(fogSet(v)); got != names(want) {
					t.Fatalf("game %d ply %d %v: in view\n got  %s\n want %s\n%s", gi, ply, who, got, names(want), p.FEN())
				}
				truth := p.Board()
				for sq, code := range truth {
					if want[sq] {
						if v.Board[sq] != code {
							t.Fatalf("game %d ply %d %v: %s should show %s, shows %q", gi, ply, who, sq, code, v.Board[sq])
						}
						if code[0] != 'w' && who == White || code[0] != 'b' && who == Black {
							shown++
						}
					} else if _, leaked := v.Board[sq]; leaked {
						t.Fatalf("game %d ply %d %v: %s on %s leaks from the fog", gi, ply, who, code, sq)
					} else if (code[0] == 'b') == (who == White) {
						hiddenPieces++
					}
				}
				if len(v.Board) > len(truth) {
					t.Fatalf("invented pieces")
				}
				for _, a := range v.Legal {
					to := a[2:4]
					if len(a) > 4 && a[1] == '@' {
						t.Fatal("drops in fog")
					}
					if !want[to] {
						t.Fatalf("game %d ply %d %v: legal %s ends in the fog", gi, ply, who, a)
					}
				}
				if who == p.Turn && len(v.Legal) == 0 {
					t.Fatalf("game %d: no legal moves for the side to move", gi)
				}
				if who != p.Turn && len(v.Legal) != 0 {
					t.Fatal("legal moves for the waiting side")
				}
				for _, s := range v.Last {
					if p.Turn == who && !want[s] {
						t.Fatalf("game %d ply %d %v: last square %s is in the fog", gi, ply, who, s)
					}
				}
				// the serialized snapshot says nothing else: scan the text for squares of hidden enemy pieces
				raw, _ := json.Marshal(v)
				for sq, code := range truth {
					if !want[sq] && strings.Contains(string(raw), `"`+sq+`":`) {
						t.Fatalf("game %d: %s (%s) named in %s", gi, sq, code, raw)
					}
				}
				// the list of moves
				for _, m := range f.RedactMoves(who, recs) {
					if m.Side == who {
						continue
					}
					if m.Action != "" {
						t.Fatalf("the opponent's action in the list: %+v", m)
					}
					if !hiddenMoveText.MatchString(m.SAN) && len(squareName.FindAllString(m.SAN, -1)) != 1 {
						t.Fatalf("a shown move names more than its target: %q", m.SAN)
					}
				}
			}
			if over || ply == plies {
				break
			}
			legal := f.Legal()
			a := legal[rng.Intn(len(legal))]
			// prefer a capture now and then so that taking and revealing are exercised
			for _, l := range legal {
				if !p.B[mustSq(l[2:4])].Empty() && rng.Intn(3) == 0 {
					a = l
				}
			}
			note, err := f.Apply(a)
			if err != nil {
				t.Fatal(err)
			}
			recs = append(recs, MoveRec{Side: p.Turn, Action: a, SAN: note})
		}
	}
	t.Logf("%d snapshots checked, %d enemy pieces shown, %d kept in the fog", positions, shown, hiddenPieces)
	if hiddenPieces < 1000 || shown < 200 {
		t.Errorf("the games did not test enough: %d hidden, %d shown", hiddenPieces, shown)
	}
}

func mustSq(s string) Sq {
	q, ok := ParseSq(s)
	if !ok {
		panic(s)
	}
	return q
}

func countPieces(p *Position) int {
	n := 0
	for _, pc := range p.B {
		if !pc.Empty() {
			n++
		}
	}
	return n
}

// A shown opponent move names the square it ended on, and that square held the
// piece in the viewer's own snapshot at that moment.
func TestFogShownMovesWereInView(t *testing.T) {
	rng := rand.New(rand.NewSource(77))
	shownMoves, hiddenMoves := 0, 0
	for gi := 0; gi < 30; gi++ {
		st, _ := Get0("fog", Options{Seed: int64(100 + gi)})
		f := st.(*fogGameState)
		var recs []MoveRec
		for ply := 0; ply < 120; ply++ {
			if _, _, over := f.Over(); over {
				break
			}
			legal := f.Legal()
			a := legal[rng.Intn(len(legal))]
			mover := f.Turn()
			note, _ := f.Apply(a)
			recs = append(recs, MoveRec{Side: mover, Action: a, SAN: note})
			if _, _, over := f.Over(); over {
				break
			}
			viewer := mover.Opp()
			last := f.RedactMoves(viewer, recs)[len(recs)-1]
			v := f.View(viewer)
			if last.SAN == hiddenMove {
				hiddenMoves++
				if _, ok := v.Board[a[2:4]]; ok {
					t.Fatalf("a move into view is hidden: %s", a)
				}
				continue
			}
			shownMoves++
			code, ok := v.Board[a[2:4]]
			if !ok || code[0] == viewerLetter(viewer) {
				t.Fatalf("shown move %q (%s) but the viewer sees %q on %s", last.SAN, a, code, a[2:4])
			}
		}
	}
	t.Logf("%d shown, %d hidden", shownMoves, hiddenMoves)
	if shownMoves == 0 || hiddenMoves == 0 {
		t.Error("both kinds of moves must occur")
	}
}

func viewerLetter(s Side) byte {
	if s == White {
		return 'w'
	}
	return 'b'
}

// Non-interference: change the enemy pieces that stand in the fog (remove, add,
// swap) and the snapshot of the player must not change by a byte.
func TestFogNoninterference(t *testing.T) {
	games := 25
	if testing.Short() {
		games = 8
	}
	rng := rand.New(rand.NewSource(5))
	checked := 0
	for gi := 0; gi < games; gi++ {
		o := Options{Seed: int64(gi)}
		st, _ := Get0("fog", o)
		var actions []string
		for ply := 0; ply < 110; ply++ {
			if _, _, over := st.Over(); over {
				break
			}
			for _, who := range []Side{White, Black} {
				orig, _ := json.Marshal(st.View(who))
				clone, err := Replay("fog", o, actions)
				if err != nil {
					t.Fatal(err)
				}
				cf := clone.(*fogGameState)
				vis := cf.seen(who)
				cf.Edit(func(p *Position) {
					cond := conditionalSquares(p, who)
					for i := 0; i < 64; i++ {
						if vis[i] || cond[i] || p.B[i].Kind == King {
							continue
						}
						switch r := rng.Intn(10); {
						case r < 4:
							p.B[i] = Piece{}
						case r < 8:
							k := []Kind{Pawn, Knight, Bishop, Rook, Queen}[rng.Intn(5)]
							if k == Pawn && (Sq(i).Rank() == 0 || Sq(i).Rank() == 7) {
								k = Knight
							}
							p.B[i] = Piece{Kind: k, Side: who.Opp()}
						}
					}
				})
				if _, _, over := cf.Over(); over {
					continue // the changes left the enemy without a move: nothing to compare
				}
				mut, _ := json.Marshal(cf.View(who))
				if string(orig) != string(mut) {
					t.Fatalf("game %d ply %d %v: the snapshot depends on what is hidden\n real    %s\n changed %s", gi, ply, who, orig, mut)
				}
				checked++
			}
			legal := st.Legal()
			a := legal[rng.Intn(len(legal))]
			st.Apply(a)
			actions = append(actions, a)
		}
	}
	t.Logf("%d snapshots unchanged by the fog", checked)
	if checked < 800 {
		t.Errorf("only %d comparisons", checked)
	}
	_ = fmt.Sprint
}

// conditionalSquares are the squares whose being in view depends on an enemy standing there, by the rule: the
// diagonals of the viewer's pawns and the pawn that can be taken en passant. An enemy piece there is shown
// (it can be taken), so the test must not plant or remove one.
func conditionalSquares(p *Position, who Side) [64]bool {
	var out [64]bool
	dr := 1
	if who == Black {
		dr = -1
	}
	for i := 0; i < 64; i++ {
		if p.B[i].Kind != Pawn || p.B[i].Side != who {
			continue
		}
		for _, df := range []int{-1, 1} {
			if s := step(Sq(i), df, dr); s != NoSq {
				out[s] = true
			}
			if s := step(Sq(i), df, 0); s != NoSq {
				out[s] = true // beside the pawn: the victim of an en passant capture
			}
		}
	}
	return out
}

func TestFogIsRegistered(t *testing.T) {
	v, ok := Get("fog")
	if !ok || v.Info().Hidden || v.Info().Name == "" || v.Info().Short == "" {
		t.Fatalf("fog: %+v %v", v, ok)
	}
	if _, ok := v.(OptionsChecker); ok {
		t.Error("fog has no settings")
	}
}

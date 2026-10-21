package variants

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
)

// Small variants that use each extension point of the base. They are also the
// examples to copy when writing a real variant.

// ------------------------------------------------- (c)(d) no check, take the king

// kingfall: no check, the king may be taken, and taking it wins; the king may
// walk into attack; a side whose king is gone has lost.
type kingfallRules struct {
	ClassicRules
	fen string
}

func (r kingfallRules) Start(Options) (*Position, error)  { return ParseFEN(r.fen) }
func (kingfallRules) Check(*Position, Side) bool          { return false }
func (kingfallRules) CastleSafe(*Position, Side, Sq) bool { return true }
func (r kingfallRules) Result(g *Game) (string, string, bool) {
	p := g.Position()
	for _, s := range [2]Side{White, Black} {
		if p.KingSq(s) == NoSq {
			return ResultFor(s.Opp()), ReasonKingCaptured, true
		}
	}
	if len(g.Moves()) == 0 { // no move: the side that cannot move wins (like the duck)
		return ResultFor(p.Turn), ReasonNoMoves, true
	}
	return "", "", false
}

func TestNoCheckAndTakingTheKing(t *testing.T) {
	g, err := NewGame(kingfallRules{fen: "4k3/8/8/8/8/8/8/4K2R w - - 0 1"}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Apply("h1h8"); err != nil {
		t.Fatal(err)
	}
	if v := g.View(Black); len(v.Check) != 0 || len(v.Legal) == 0 {
		t.Errorf("no check in this game, the view shows %v", v.Check)
	}
	// Black may leave its king under the rook, or step along the rank into its fire
	black := g.Legal()
	if !contains(black, "e8f8") || !contains(black, "e8d8") || !contains(black, "e8e7") {
		t.Errorf("a king may walk into attack: %v", black)
	}
	if _, err := g.Apply("e8f8"); err != nil {
		t.Fatal(err)
	}
	if res, _, over := g.Over(); over {
		t.Errorf("a king under the rook is not captured yet: %s", res)
	}
}

func TestTakingTheKingWins(t *testing.T) {
	g, _ := NewGame(kingfallRules{fen: "3rk3/8/8/8/8/8/8/R3K3 w - - 0 1"}, Options{})
	g.Apply("a1a8") // takes nothing: the rook goes to a8, attacking along the rank... d8 stands between
	g.Apply("d8d1") // the black rook comes down to the white king's file: it attacks e1 along the first rank
	g.Apply("a8a1") // back
	if _, _, over := g.Over(); over {
		t.Fatal("over")
	}
	g.Apply("d1e1") // the rook takes the white king
	res, reason, over := g.Over()
	if !over || res != ResultBlack || reason != ReasonKingCaptured {
		t.Errorf("king taken: %q %q %v", res, reason, over)
	}
	if _, err := g.Apply("a1a2"); err != ErrOver {
		t.Errorf("an action after the end: %v", err)
	}
	if g.Legal() != nil {
		t.Errorf("legal moves after the end: %v", g.Legal())
	}
}

// ------------------------------------------------------ (c) another way to win

// hill: the first king on the four central squares wins; the rest is classical.
type hillRules struct{ ClassicRules }

func (hillRules) Result(g *Game) (string, string, bool) {
	p := g.Position()
	for _, s := range [2]Side{White, Black} {
		k := p.KingSq(s)
		if k != NoSq && k.File() >= 3 && k.File() <= 4 && k.Rank() >= 3 && k.Rank() <= 4 {
			return ResultFor(s), "king-of-the-hill", true
		}
	}
	return ClassicResult(g)
}

func TestAnotherWayToWin(t *testing.T) {
	g, err := NewGame(hillRules{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range strings.Fields("e2e4 a7a6 e1e2 a6a5 e2e3 a5a4 e3d4") {
		if _, _, over := g.Over(); over {
			t.Fatalf("over before %s", a)
		}
		if _, err := g.Apply(a); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
	}
	res, reason, over := g.Over()
	if !over || res != ResultWhite || reason != "king-of-the-hill" {
		t.Errorf("king on d4: %q %q %v", res, reason, over)
	}
}

// ------------------------------------------- (b) captures with side effects

// explode: a capture removes the capturing piece and every non-pawn piece
// around the square, like the atomic variant (but the kings are not special).
type explodeRules struct {
	ClassicRules
	fen string
}

func (r explodeRules) Start(Options) (*Position, error) { return ParseFEN(r.fen) }
func (explodeRules) AfterMove(next *Position, info MoveInfo) {
	if info.Captured.Empty() {
		return
	}
	at := info.Move.To
	next.B[at] = Piece{}
	for df := -1; df <= 1; df++ {
		for dr := -1; dr <= 1; dr++ {
			if s := step(at, df, dr); s != NoSq && next.B[s].Kind != Pawn {
				next.B[s] = Piece{}
			}
		}
	}
	next.FixCastling()
}

func TestCaptureSideEffects(t *testing.T) {
	g, err := NewGame(explodeRules{fen: "rn2k3/pp6/8/8/8/8/8/R3K3 w - - 0 1"}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Apply("a1a7"); err != nil {
		t.Fatal(err)
	}
	b := g.View(NoSide).Board
	// the pawn on a7 is taken and the rook goes with it; the rook a8 and the knight b8 stand next to a7,
	// the pawn b7 does not explode
	for _, sq := range []string{"a7", "a1", "a8", "b8"} {
		if _, ok := b[sq]; ok {
			t.Errorf("%s should be empty after the explosion: %v", sq, b)
		}
	}
	if b["b7"] != "bP" || b["e8"] != "bK" || b["e1"] != "wK" {
		t.Errorf("the pawn b7 and the kings stay: %v", b)
	}
}

// ---------------------------------------- (a)(f) random setup and castling

// scharnagl makes a Chess960 back rank from a generator: bishops on opposite
// colours, the king between the rooks.
func scharnagl(r *rand.Rand) string {
	rank := make([]byte, 8)
	rank[2*r.Intn(4)+1] = 'b'
	rank[2*r.Intn(4)] = 'b'
	free := func() []int {
		var out []int
		for i, c := range rank {
			if c == 0 {
				out = append(out, i)
			}
		}
		return out
	}
	f := free()
	rank[f[r.Intn(len(f))]] = 'q'
	for n := 0; n < 2; n++ {
		f = free()
		rank[f[r.Intn(len(f))]] = 'n'
	}
	f = free()
	rank[f[0]], rank[f[1]], rank[f[2]] = 'r', 'k', 'r'
	return string(rank)
}

type fischerRules struct{ ClassicRules }

func (fischerRules) CastleAsKingTakesRook() bool { return true }
func (fischerRules) Start(o Options) (*Position, error) {
	black := scharnagl(o.Rand())
	white := strings.ToUpper(black)
	// Shredder-FEN names the rooks by their files
	var files []byte
	for i := 0; i < 8; i++ {
		if black[i] == 'r' {
			files = append(files, byte('a'+i))
		}
	}
	cast := strings.ToUpper(string(files)) + string(files)
	return ParseFEN(fmt.Sprintf("%s/pppppppp/8/8/8/8/PPPPPPPP/%s w %s - 0 1", black, white, cast))
}

type fischer struct{}

func (fischer) Info() Info                   { return Info{ID: "t-fischer", Name: "Тест 960", Hidden: true} }
func (fischer) New(o Options) (State, error) { return NewGame(fischerRules{}, o) }

func init() { Register(fischer{}) }

func TestSeededSetupIsReproducible(t *testing.T) {
	seen := map[string]bool{}
	for seed := int64(1); seed <= 60; seed++ {
		a, err := Get0("t-fischer", Options{Seed: seed})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := Get0("t-fischer", Options{Seed: seed})
		fa, fb := a.(*Game).Position().FEN(), b.(*Game).Position().FEN()
		if fa != fb {
			t.Fatalf("seed %d gives two positions: %s and %s", seed, fa, fb)
		}
		seen[fa] = true
		p := a.(*Game).Position()
		if k := p.KingSq(White); k == NoSq || p.Castle[White][0] == NoSq || p.Castle[White][1] == NoSq ||
			p.Castle[White][1].File() > k.File() || p.Castle[White][0].File() < k.File() {
			t.Fatalf("seed %d: the king is not between the rooks: %s", seed, fa)
		}
		if n := len(a.Legal()); n < 18 {
			t.Fatalf("seed %d: %d moves in %s", seed, n, fa)
		}
	}
	if len(seen) < 30 {
		t.Errorf("60 seeds gave only %d setups", len(seen))
	}
}

// A game of random moves, castlings included, is restored exactly by replaying
// its actions with its options.
func TestReplayRestoresAGame(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	castled := 0
	for seed := int64(1); seed <= 40; seed++ {
		o := Options{Seed: seed}
		st, _ := Get0("t-fischer", o)
		var actions []string
		for ply := 0; ply < 60; ply++ {
			legal := st.Legal()
			if len(legal) == 0 {
				break
			}
			// prefer castling when it is there, so that the path is exercised
			a := legal[rng.Intn(len(legal))]
			for _, l := range legal {
				p := st.(*Game).Position()
				from, _ := ParseSq(l[:2])
				to, _ := ParseSq(l[2:4])
				if len(l) == 4 && p.B[from].Kind == King && p.B[to].Kind == Rook && p.B[to].Side == p.Turn {
					a = l
				}
			}
			if _, err := st.Apply(a); err != nil {
				t.Fatal(err)
			}
			actions = append(actions, a)
		}
		again, err := Replay("t-fischer", o, actions)
		if err != nil {
			t.Fatal(err)
		}
		if st.(*Game).Position().FEN() != again.(*Game).Position().FEN() {
			t.Fatalf("seed %d: replay differs", seed)
		}
		if p := again.(*Game).Position(); p.Castle[White] != [2]Sq{NoSq, NoSq} || p.Castle[Black] != [2]Sq{NoSq, NoSq} {
			castled++
		}
	}
	if _, err := Replay("t-fischer", Options{Seed: 1}, []string{"a1a2"}); err == nil {
		t.Error("an impossible action was replayed")
	}
	if _, err := Replay("nope", Options{}, nil); err == nil {
		t.Error("unknown variant replayed")
	}
	t.Logf("%d of 40 games ended with castling rights still held", castled)
}

func TestFischerCastlingByTakingTheRook(t *testing.T) {
	// find a seed where the king and a rook are ready to castle after a few developing moves is
	// hard; instead set the position by hand
	g, _ := NewGame(fischerRules{}, Options{Seed: 3})
	g.Edit(func(p *Position) {
		*p = *mustFEN("4k3/8/8/8/8/8/8/1R2K1R1 w GB - 0 1")
	})
	legal := sortedActions(g.Legal())
	// king e1, rooks b1 and g1: kingside castling is "e1g1" (king takes rook g1), queenside "e1b1"
	if !contains(legal, "e1g1") || !contains(legal, "e1b1") {
		t.Fatalf("castlings missing: %v", legal)
	}
	if _, err := g.Apply("e1g1"); err != nil {
		t.Fatal(err)
	}
	p := g.Position()
	if p.B[MakeSq(6, 0)] != (Piece{Kind: King, Side: White}) || p.B[MakeSq(5, 0)] != (Piece{Kind: Rook, Side: White}) {
		t.Errorf("after e1g1: %s", p.Placement())
	}
	g.Edit(func(p *Position) { *p = *mustFEN("4k3/8/8/8/8/8/8/1R2K1R1 w GB - 0 1") })
	if note, err := g.Apply("e1b1"); err != nil || note != "O-O-O" {
		t.Errorf("queen side: %q %v", note, err)
	}
	if p := g.Position(); p.B[MakeSq(2, 0)].Kind != King || p.B[MakeSq(3, 0)].Kind != Rook {
		t.Errorf("after e1b1: %s", p.Placement())
	}
}

func mustFEN(fen string) *Position {
	p, err := ParseFEN(fen)
	if err != nil {
		panic(err)
	}
	return p
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ------------------------------------------------------- (e) drops from a pocket

// pocketRules: a captured piece goes into the taker's pocket and may be
// dropped on any empty square (pawns not on the first or last rank), as in
// crazyhouse. A promoted piece comes back as a pawn.
type pocketRules struct{ ClassicRules }

func (pocketRules) AfterMove(next *Position, info MoveInfo) {
	if info.Captured.Empty() {
		return
	}
	k := info.Captured.Kind
	if info.Captured.Promoted {
		k = Pawn
	}
	next.Pocket[info.Side][k]++
}

func (pocketRules) ExtraMoves(p *Position) []Move {
	var out []Move
	for k := Pawn; k <= Queen; k++ {
		if p.Pocket[p.Turn][k] == 0 {
			continue
		}
		for s := Sq(0); s < 64; s++ {
			if !p.B[s].Empty() || k == Pawn && (s.Rank() == 0 || s.Rank() == 7) {
				continue
			}
			out = append(out, Move{From: NoSq, To: s, Drop: k, Rook: NoSq})
		}
	}
	return out
}

func (pocketRules) Extra(g *Game, _ Side) any {
	count := func(s Side) map[string]int {
		m := map[string]int{}
		for k := Pawn; k <= Queen; k++ {
			if n := g.Position().Pocket[s][k]; n > 0 {
				m[string(rune(k.Letter()))] = int(n)
			}
		}
		return m
	}
	return map[string]any{"white": count(White), "black": count(Black)}
}

func TestDropsFromAPocket(t *testing.T) {
	g, err := NewGame(pocketRules{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range strings.Fields("e2e4 d7d5") {
		g.Apply(a)
	}
	if hasDrop(g.Legal()) {
		t.Fatal("a drop with an empty pocket")
	}
	note, err := g.Apply("e4d5") // White takes a pawn
	if err != nil || note != "exd5" {
		t.Fatalf("%q %v", note, err)
	}
	extra := g.View(White).Extra.(map[string]any)
	if w := extra["white"].(map[string]int); w["P"] != 1 {
		t.Errorf("pocket after the capture: %v", extra)
	}
	g.Apply("g8f6") // Black moves; White now holds a pawn in hand
	if !hasDrop(g.Legal()) {
		t.Fatal("no drops while holding a pawn, on White's turn")
	}
	legal := g.Legal()
	if contains(legal, "P@e1") || contains(legal, "P@a8") {
		t.Error("a pawn dropped on the first or last rank")
	}
	if !contains(legal, "P@e4") {
		t.Error("P@e4 should be a legal drop")
	}
	note, err = g.Apply("P@e4")
	if err != nil || note != "P@e4" {
		t.Fatalf("drop: %q %v", note, err)
	}
	if g.Position().B[MakeSq(4, 3)] != (Piece{Kind: Pawn, Side: White}) || g.Position().Pocket[White][Pawn] != 0 {
		t.Error("the pawn is not on e4, or still in the pocket")
	}
	if v := g.View(NoSide); len(v.Last) != 1 || v.Last[0] != "e4" {
		t.Errorf("last of a drop: %v", v.Last)
	}
}

func hasDrop(actions []string) bool {
	for _, a := range actions {
		if strings.Contains(a, "@") {
			return true
		}
	}
	return false
}

// A drop must answer a check: the check filter of the base applies to drops too.
func TestDropMustAnswerCheck(t *testing.T) {
	g, _ := NewGame(pocketRules{}, Options{})
	g.Edit(func(p *Position) {
		*p = *mustFEN("4k3/8/8/8/8/8/8/r3K3 w - - 0 1")
		p.Pocket[White][Pawn] = 1
		p.Pocket[White][Knight] = 1
	})
	// the rook on a1 checks along the first rank; only a drop between a1 and e1 or a king step helps,
	// but a pawn cannot stand on the first rank: the knight can
	var drops []string
	for _, a := range g.Legal() {
		if strings.Contains(a, "@") {
			drops = append(drops, a)
		}
	}
	sort.Strings(drops)
	if strings.Join(drops, " ") != "N@b1 N@c1 N@d1" {
		t.Errorf("drops that block the check: %v", drops)
	}
}

// ------------------------------------------------ (d) a turn of several actions

// twice: every turn is two moves of the same side. The State wraps a Game and
// gives the move back to the same side after the first.
type twice struct {
	*Game
	step int
}

func (s *twice) Apply(action string) (string, error) {
	note, err := s.Game.Apply(action)
	if err != nil {
		return "", err
	}
	s.step++
	if s.step%2 == 1 && !s.over() {
		s.Game.Edit(func(p *Position) { p.Turn = p.Turn.Opp() })
	}
	return note, nil
}

func (s *twice) over() bool { _, _, o := s.Game.Over(); return o }

func TestATurnOfSeveralActions(t *testing.T) {
	g, _ := NewGame(kingfallRules{fen: StartFEN}, Options{})
	st := &twice{Game: g}
	var turns []Side
	for _, a := range strings.Fields("e2e4 d2d4 e7e5 d7d5 e4d5") {
		if _, err := st.Apply(a); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
		turns = append(turns, st.Turn())
	}
	want := []Side{White, Black, Black, White, White}
	for i := range want {
		if turns[i] != want[i] {
			t.Errorf("after action %d the turn is %v, want %v", i+1, turns[i], want[i])
		}
	}
	if v := st.View(White); v.Turn != White || len(v.Legal) == 0 {
		t.Errorf("White must act again: %+v", v.Turn)
	}
}

// --------------------------------------------------------- (g) hidden information

// fog: a viewer sees their own pieces only; the other side's moves are hidden
// from the list as well, until the end.
type fogState struct{ *Game }

func (f fogState) View(viewer Side) View {
	v := f.Game.View(viewer)
	_, _, over := f.Game.Over()
	if over {
		return v
	}
	for sq, code := range v.Board {
		if pc, _ := ParseCode(code); pc.Side != viewer {
			delete(v.Board, sq)
		}
	}
	v.Last = nil
	return v
}

func (f fogState) RedactMoves(viewer Side, moves []MoveRec) []MoveRec {
	if _, _, over := f.Game.Over(); over {
		return moves
	}
	out := make([]MoveRec, len(moves))
	for i, m := range moves {
		if m.Side != viewer {
			m.Action, m.SAN = "", "?"
		}
		out[i] = m
	}
	return out
}

func TestHiddenInformation(t *testing.T) {
	g, _ := NewGame(ClassicRules{}, Options{})
	var st State = fogState{g}
	st.Apply("e2e4")
	w, b, s := st.View(White), st.View(Black), st.View(NoSide)
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
	if len(s.Board) != 0 || len(s.Legal) != 0 || len(b.Legal) == 0 {
		t.Errorf("spectator sees %d pieces; black has %d legal actions", len(s.Board), len(b.Legal))
	}
	if _, ok := w.Board["e4"]; !ok {
		t.Error("White must see its own pawn on e4")
	}
	rec := []MoveRec{{Side: White, Action: "e2e4", SAN: "e4"}, {Side: Black, Action: "e7e5", SAN: "e5"}}
	seen := st.(MoveRedactor).RedactMoves(White, rec)
	if seen[0].SAN != "e4" || seen[1].SAN != "?" || seen[1].Action != "" {
		t.Errorf("redacted: %+v", seen)
	}
}

// -------------------------------------------------------------- (a) other setup

type hordeLikeRules struct{ ClassicRules }

func (hordeLikeRules) Start(Options) (*Position, error) {
	return ParseFEN("rnbqkbnr/pppppppp/8/1PP2PP1/PPPPPPPP/PPPPPPPP/PPPPPPPP/PPPPPPPP w kq - 0 1")
}

// the horde: no white king, pawns on the first rank may step two squares
func (hordeLikeRules) PawnDoubleStep(p *Position, from Sq) (bool, bool) {
	home := 1
	if p.B[from].Side == Black {
		home = 6
	}
	return from.Rank() == home || from.Rank() == 0 && p.B[from].Side == White, from.Rank() == home
}

func TestAnotherStartAndPawnSteps(t *testing.T) {
	g, err := NewGame(hordeLikeRules{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	p := g.Position()
	if p.KingSq(White) != NoSq {
		t.Fatal("the horde has no king")
	}
	if g.rules.Check(p, White) {
		t.Fatal("a side without a king is never in check")
	}
	// a pawn on the first rank may go two squares; one on the second rank, too; neither is blocked by far pawns
	legal := g.Legal()
	if len(legal) == 0 {
		t.Fatal("no moves")
	}
	if _, err := g.Apply("b5b6"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Apply("a7a5"); err != nil {
		t.Fatal(err)
	}
	// a pawn on the first rank that has an empty square two ahead: make room on a2 and a3
	g.Edit(func(p *Position) {
		p.B[MakeSq(0, 1)], p.B[MakeSq(0, 2)] = Piece{}, Piece{}
		p.B[MakeSq(0, 3)] = Piece{}
	})
	legal = g.Legal()
	if !contains(legal, "a1a3") {
		t.Errorf("a pawn from the first rank may step two squares: %v", legal)
	}
	// but it does not leave an en passant square
	if _, err := g.Apply("a1a3"); err != nil {
		t.Fatal(err)
	}
	if g.Position().EP != NoSq {
		t.Errorf("en passant square %v after a double step from the first rank", g.Position().EP)
	}
}

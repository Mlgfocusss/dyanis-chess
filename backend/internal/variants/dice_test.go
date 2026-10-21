package variants

import (
	"math/rand"
	"strings"
	"testing"
)

func diceOf(t *testing.T, seed int64) *diceState {
	t.Helper()
	st, err := Get0("dice", Options{Seed: seed})
	if err != nil {
		t.Fatal(err)
	}
	return st.(*diceState)
}

// a game from a position of the test's own: diceRules with another start
type diceFenRules struct {
	diceRules
	fen string
}

func (r diceFenRules) Start(Options) (*Position, error) { return ParseFEN(r.fen) }

func diceFrom(t *testing.T, fen string, seed int64) *diceState {
	t.Helper()
	st, err := newDiceWith(diceFenRules{fen: fen}, Options{Seed: seed})
	if err != nil {
		t.Fatal(err)
	}
	return st.(*diceState)
}

// kindOfFrom is the kind of the piece that an action moves.
func kindOfFrom(d *diceState, action string) Kind {
	from, _ := ParseSq(action[:2])
	return d.pos.B[from].Kind
}

// kinds of the dice that can still be played
func (d *diceState) freeKinds() map[Kind]int {
	m := map[Kind]int{}
	for i, k := range d.dice {
		if d.st[i] == diceFree {
			m[k]++
		}
	}
	return m
}

func diceExtraOf(d *diceState, side Side) diceExtra { return d.View(side).Extra.(diceExtra) }

// The three dice are in the snapshot of both sides and of a spectator from the start of the turn; only
// the side to act is offered moves, and only moves of the kinds of its dice.
func TestDiceThreeDiceAreShownAtOnce(t *testing.T) {
	for seed := int64(1); seed <= 60; seed++ {
		d := diceOf(t, seed)
		for _, viewer := range []Side{White, Black, NoSide} {
			ex := diceExtraOf(d, viewer)
			if ex.Of != 3 || len(ex.Dice) != 3 || ex.Left != d.left() {
				t.Fatalf("seed %d: extra %+v", seed, ex)
			}
			for i, f := range ex.Dice {
				if f.Die != string(rune(d.dice[i].Letter())) || f.State == "used" {
					t.Fatalf("seed %d: face %d is %+v", seed, i, f)
				}
			}
		}
		legal := d.Legal()
		if len(legal) == 0 {
			t.Fatalf("seed %d: not over, yet no moves", seed)
		}
		free := d.freeKinds()
		for _, a := range legal {
			if free[kindOfFrom(d, a)] == 0 {
				t.Fatalf("seed %d: dice %v offer %s with a %v", seed, d.dice, a, kindOfFrom(d, a))
			}
		}
		// at the start of the game only pawns and knights can move: the other dice are skipped at once
		for _, f := range diceExtraOf(d, d.Turn()).Dice {
			playable := f.Die == "P" || f.Die == "N"
			if (f.State == "free") != playable || (f.State == "skip") == playable {
				t.Fatalf("seed %d: die %s is %s at the start", seed, f.Die, f.State)
			}
		}
		if d.Turn() == White && len(d.View(Black).Legal) != 0 || d.Turn() == Black && len(d.View(White).Legal) != 0 {
			t.Fatalf("seed %d: the side that does not act is offered moves", seed)
		}
		if len(d.View(NoSide).Legal) != 0 {
			t.Fatalf("seed %d: a spectator is offered moves", seed)
		}
		// a move of a kind that has no die is refused
		other := ""
		switch {
		case free[Pawn] == 0:
			other = "e2e4"
		case free[Knight] == 0:
			other = "g1f3"
		}
		if other == "" {
			continue
		}
		if _, err := d.Apply(other); err == nil {
			t.Fatalf("seed %d: %s accepted with the dice %v", seed, other, d.dice)
		}
	}
}

// The side picks the order: either of a pawn and a knight die can be played first.
func TestDiceAnyOrder(t *testing.T) {
	for _, first := range []string{"e2e4", "g1f3"} {
		found := false
		for seed := int64(1); seed <= 300 && !found; seed++ {
			d := diceOf(t, seed)
			free := d.freeKinds()
			if d.Turn() != White || free[Pawn] == 0 || free[Knight] == 0 {
				continue
			}
			found = true
			second := map[string]string{"e2e4": "g1f3", "g1f3": "e2e4"}[first]
			if _, err := d.Apply(first); err != nil {
				t.Fatalf("seed %d: %s first: %v", seed, first, err)
			}
			if d.Turn() != White && d.left() > 0 {
				t.Fatalf("seed %d: the turn went away with a die left", seed)
			}
			if d.Turn() == White {
				if ex := diceExtraOf(d, White); ex.Left < 1 || ex.Left > 2 {
					t.Fatalf("seed %d: %d dice left after one move", seed, ex.Left)
				}
				if !contains(d.Legal(), second) {
					t.Fatalf("seed %d: %s is not offered after %s: %v", seed, second, first, d.Legal())
				}
				if _, err := d.Apply(second); err != nil {
					t.Fatalf("seed %d: %s second: %v", seed, second, err)
				}
			}
		}
		if !found {
			t.Fatal("no seed with a pawn and a knight die for White")
		}
	}
}

// Two dice of one kind are one and the same die: the kind can move twice, not three times.
func TestDiceSameKindsAreInterchangeable(t *testing.T) {
	found := false
	for seed := int64(1); seed <= 600 && !found; seed++ {
		d := diceOf(t, seed)
		free := d.freeKinds()
		if d.Turn() != White || free[Pawn] != 2 {
			continue
		}
		found = true
		who, passes, left0 := d.Turn(), d.TurnsPassed(), d.left()
		if _, err := d.Apply("a2a3"); err != nil {
			t.Fatal(err)
		}
		if d.Turn() != who || diceExtraOf(d, who).Left != left0-1 {
			t.Fatalf("seed %d: after one pawn move the turn is %v, extra %+v", seed, d.Turn(), diceExtraOf(d, who))
		}
		if _, err := d.Apply("b2b3"); err != nil {
			t.Fatal(err)
		}
		// the third die is something else (or the same: a third pawn die was the third face)
		if d.Turn() == who && d.TurnsPassed() == passes {
			if d.freeKinds()[Pawn] != 0 {
				t.Fatal("a pawn die is left after two pawn moves")
			}
			if _, err := d.Apply("c2c3"); err == nil {
				t.Fatal("a third pawn move without a third pawn die")
			}
		}
	}
	if !found {
		t.Fatal("no seed with two pawn dice")
	}
}

// A die whose kind cannot move is skipped at once and for good: a later move that gives the kind a move does
// not bring it back.
func TestDiceSkipIsFinal(t *testing.T) {
	// the rook on a1 is shut in by its pawn and knight; moving the knight lets it out
	found := false
	for seed := int64(1); seed <= 600 && !found; seed++ {
		d := diceFrom(t, "4k3/8/8/8/8/8/P7/RN2K3 w - - 0 1", seed)
		hasRook, hasKnight := false, false
		for _, f := range diceExtraOf(d, White).Dice {
			if f.Die == "R" {
				hasRook = true
				if f.State != "skip" {
					t.Fatalf("seed %d: the shut-in rook die is %s at the start", seed, f.State)
				}
			}
			if f.Die == "N" {
				hasKnight = true
			}
		}
		if !hasRook || !hasKnight || d.Turn() != White {
			continue
		}
		found = true
		if _, err := d.Apply("b1c3"); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		if d.TurnsPassed() == 0 && contains(d.Legal(), "a1b1") {
			t.Fatalf("seed %d: the skipped rook can move again: %v", seed, d.Legal())
		}
		if d.Turn() == White && d.TurnsPassed() == 0 {
			for _, f := range diceExtraOf(d, White).Dice {
				if f.Die == "R" && f.State != "skip" {
					t.Fatalf("seed %d: the rook die is %s after the knight moved", seed, f.State)
				}
			}
		}
	}
	if !found {
		t.Fatal("no seed with a rook die and a knight die")
	}
}

// When no die left can be played, the rest of the turn is skipped and the notation says so.
func TestDiceTurnEndsWhenNothingLeftCanMove(t *testing.T) {
	skips := 0
	for seed := int64(1); seed <= 300; seed++ {
		// White has a king and a pawn only: any other die cannot move
		d := diceFrom(t, "4k3/8/8/8/8/8/P7/4K3 w - - 0 1", seed)
		for guard := 0; guard < 50 && !isOver(d); guard++ {
			actor := d.Turn()
			leftBefore := d.left()
			passes := d.TurnsPassed()
			note, err := d.Apply(d.Legal()[0])
			if err != nil {
				t.Fatal(err)
			}
			if isOver(d) {
				break
			}
			if d.TurnsPassed() == passes {
				// the turn goes on: a die is left and it can play
				if d.Turn() != actor || d.left() > leftBefore-1 || d.left() == 0 || len(d.Legal()) == 0 {
					t.Fatalf("seed %d: the turn went on with %d left (was %d), %d moves", seed, d.left(), leftBefore, len(d.Legal()))
				}
				continue
			}
			if leftBefore-1 > 0 {
				skips++
				if !strings.Contains(note, "пропуск (") {
					t.Fatalf("seed %d: dice were skipped but the note is %q", seed, note)
				}
			}
		}
	}
	if skips == 0 {
		t.Error("no rest-of-turn skip happened")
	}
}

func TestDiceSameSeedSameDice(t *testing.T) {
	play := func(seed int64) (string, []string) {
		d := diceOf(t, seed)
		rng := rand.New(rand.NewSource(99))
		var actions []string
		for i := 0; i < 90 && !isOver(d); i++ {
			l := d.Legal()
			a := l[rng.Intn(len(l))]
			if _, err := d.Apply(a); err != nil {
				t.Fatal(err)
			}
			actions = append(actions, a)
		}
		return d.pos.FEN(), actions
	}
	f1, a1 := play(5)
	f2, a2 := play(5)
	if f1 != f2 || strings.Join(a1, " ") != strings.Join(a2, " ") {
		t.Error("the same seed gave two games")
	}
	f3, _ := play(6)
	if f1 == f3 {
		t.Error("two seeds gave the same game")
	}
	// the restored game stands on the same dice, used the same way, at every point of the game
	o := Options{Seed: 5}
	orig := diceOf(t, 5)
	for i, a := range a1 {
		if _, err := orig.Apply(a); err != nil {
			t.Fatal(err)
		}
		if i%7 != 0 && i != len(a1)-1 {
			continue
		}
		again, err := Replay("dice", o, a1[:i+1])
		if err != nil {
			t.Fatal(err)
		}
		b := again.(*diceState)
		if b.dice != orig.dice || b.st != orig.st || b.Turn() != orig.Turn() || b.pos.FEN() != orig.pos.FEN() || b.passes != orig.passes {
			t.Fatalf("a game replayed to action %d differs in the dice or the step", i+1)
		}
		ja, jb := diceExtraOf(orig, orig.Turn()), diceExtraOf(b, b.Turn())
		if len(ja.Dice) != len(jb.Dice) || ja.Left != jb.Left {
			t.Fatalf("snapshots differ after a replay: %+v %+v", ja, jb)
		}
		for k := range ja.Dice {
			if ja.Dice[k] != jb.Dice[k] {
				t.Fatalf("snapshots differ after a replay: %+v %+v", ja, jb)
			}
		}
	}
}

func TestDiceCastlingIsAKingMove(t *testing.T) {
	// find a seed where a die is the king's and castling is possible
	for seed := int64(1); seed < 400; seed++ {
		d := diceFrom(t, "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1", seed)
		if d.freeKinds()[King] > 0 {
			l := d.Legal()
			if !contains(l, "e1g1") || !contains(l, "e1c1") {
				t.Fatalf("king die, castlings missing: %v", l)
			}
			for _, a := range l {
				if d.freeKinds()[kindOfFrom(d, a)] == 0 {
					t.Fatalf("%s is a move of a kind with no die (%v)", a, d.dice)
				}
			}
			if _, err := d.Apply("e1g1"); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("no seed with the king's die")
}

func TestDiceKingCaptureEndsTheTurnAtOnce(t *testing.T) {
	// White rook takes the king: the game ends mid-turn, with no more moves
	for seed := int64(1); seed < 400; seed++ {
		d := diceFrom(t, "4k3/8/8/8/8/8/8/R3K3 w - - 0 1", seed)
		if d.freeKinds()[Rook] == 0 {
			continue
		}
		if _, err := d.Apply("a1a8"); err != nil {
			t.Fatal(err)
		}
		if isOver(d) {
			t.Fatal("over after the rook reached a8")
		}
		// the rook on a8 attacks e8 along the rank, but check does not exist: the dice decide what moves
		for guard := 0; guard < 60 && !isOver(d); guard++ {
			l := d.Legal()
			a := l[0]
			for _, x := range l {
				if to, _ := ParseSq(x[2:4]); d.pos.B[to].Kind == King {
					a = x
				}
			}
			if _, err := d.Apply(a); err != nil {
				t.Fatal(err)
			}
		}
		res, reason, over := d.Over()
		if !over || reason != ReasonKingCaptured && reason != ReasonFiftyMoves {
			t.Fatalf("never ended: %q %q", res, reason)
		}
		if reason == ReasonKingCaptured {
			if d.Legal() != nil || d.View(White).Legal != nil {
				t.Error("moves offered after the king was taken")
			}
			if _, err := d.Apply("a1a2"); err != ErrOver {
				t.Errorf("a move after the end: %v", err)
			}
			return
		}
	}
}

func TestDiceAWholeTurnCanBeSkipped(t *testing.T) {
	// Kings and a lone pawn: White moves its king or pawn, and Black, with a king only, loses its whole turn
	// whenever its three dice are not the king's.
	skipped, inside := 0, 0
	for seed := int64(1); seed <= 300; seed++ {
		d := diceFrom(t, "8/4k3/8/8/8/8/P7/4K3 w - - 0 1", seed)
		for guard := 0; guard < 60 && !isOver(d); guard++ {
			actor := d.Turn()
			before := d.TurnsPassed()
			l := d.Legal()
			if len(l) == 0 {
				t.Fatal("not over, yet no moves")
			}
			note, err := d.Apply(l[0])
			if err != nil {
				t.Fatal(err)
			}
			if isOver(d) {
				break
			}
			gained := d.TurnsPassed() - before
			// the turn is the actor's again exactly when it passed an even number of times
			if (gained%2 == 0) != (d.Turn() == actor) {
				t.Fatalf("%d passes, yet the turn is %v (actor %v)", gained, d.Turn(), actor)
			}
			if gained >= 2 {
				skipped++
				if !strings.Contains(note, "пропуск хода") {
					t.Fatalf("a skipped turn is not in the note: %q", note)
				}
				if diceExtraOf(d, d.Turn()).Lost == nil {
					t.Fatal("Extra does not tell of the lost turn")
				}
			}
			if gained == 0 {
				inside++
			}
		}
	}
	if skipped == 0 {
		t.Error("no whole turn was skipped in 300 games")
	}
	if inside == 0 {
		t.Error("no mid-turn action seen")
	}
}

func TestDiceSkipsAreInTheNotation(t *testing.T) {
	found := 0
	for seed := int64(1); seed <= 200 && found < 3; seed++ {
		d := diceOf(t, seed)
		for guard := 0; guard < 30 && !isOver(d); guard++ {
			note, err := d.Apply(d.Legal()[0])
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(note, "пропуск") {
				found++
				if !strings.Contains(note, ": ") {
					t.Errorf("a note with a skip but no move: %q", note)
				}
			}
		}
	}
	if found == 0 {
		t.Error("no skip was ever written into a note")
	}
}

// a rules value that vetoes every move: a position in which a side has no move of any kind
type lockedRules struct{ diceRules }

func (lockedRules) Legal(*Position, Move, *Position) bool { return false }

func TestDiceStalemateIsADraw(t *testing.T) {
	st, err := newDiceWith(lockedRules{}, Options{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	res, reason, over := st.Over()
	if !over || res != ResultDraw || reason != ReasonStalemate {
		t.Errorf("a side with no move at all: %q %q %v", res, reason, over)
	}
	if st.Legal() != nil {
		t.Error("moves offered in a drawn game")
	}
}

func TestDiceFlagLoses(t *testing.T) {
	d := diceOf(t, 1)
	if got := d.Timeout(Black); got != ResultWhite {
		t.Errorf("black flag: %q", got)
	}
}

// All six kinds can move: each face of the dice comes up about a sixth of the time.
func TestDiceFacesAreFair(t *testing.T) {
	var count [8]int
	const n = 4000
	for seed := int64(1); seed <= n; seed++ {
		d := diceFrom(t, "4k3/8/8/3QR3/2B1N3/8/3P4/4K3 w - - 0 1", seed)
		if d.passes != 0 {
			t.Fatalf("seed %d: a turn was lost in a position where every kind moves", seed)
		}
		for _, k := range d.dice {
			count[k]++
		}
	}
	total := 3 * n
	for k := Pawn; k <= King; k++ {
		if count[k] < total/6-total/6/5 || count[k] > total/6+total/6/5 {
			t.Errorf("face %v came up %d times of %d", k, count[k], total)
		}
	}
}

// Random games: the Game's turn follows the dice's, at most three dice are in a turn, a turn that goes on has a
// die that can be played, and a game is rebuilt exactly from its actions.
func TestDiceRandomGamesAndReplay(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	ended := map[string]int{}
	for g := 0; g < 100; g++ {
		o := Options{Seed: int64(g)}
		d := diceOf(t, o.Seed)
		var actions []string
		for i := 0; i < 500 && !isOver(d); i++ {
			if d.pos.Turn != d.mover || d.left() < 1 || d.left() > diceMoves {
				t.Fatalf("game %d: Game turn %v, mover %v, %d dice left", g, d.pos.Turn, d.mover, d.left())
			}
			l := d.Legal()
			if len(l) == 0 {
				t.Fatalf("game %d: not over, yet no moves", g)
			}
			a := l[rng.Intn(len(l))]
			for _, x := range l {
				if to, _ := ParseSq(x[2:4]); d.pos.B[to].Kind == King {
					a = x
				}
			}
			if _, err := d.Apply(a); err != nil {
				t.Fatalf("game %d: %s: %v", g, a, err)
			}
			actions = append(actions, a)
		}
		_, reason, _ := d.Over()
		ended[reason]++
		again, err := Replay("dice", o, actions)
		if err != nil {
			t.Fatal(err)
		}
		b := again.(*diceState)
		if b.pos.FEN() != d.pos.FEN() || b.dice != d.dice || b.st != d.st || b.mover != d.mover || b.passes != d.passes {
			t.Fatalf("game %d: replay differs", g)
		}
	}
	t.Logf("random dice games ended: %v", ended)
	if ended[ReasonKingCaptured] == 0 {
		t.Error("no random game ended with a king taken")
	}
}

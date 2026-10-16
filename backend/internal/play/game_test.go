package play

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"dyanis-chess/backend/internal/pgn"
	"dyanis-chess/engine"
)

// fakeClock is a clock the test moves by hand.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

var (
	anna  = Player{ID: "anna", Name: "Анна"}
	boris = Player{ID: "boris", Name: "Борис"}
)

// started returns a running game, Anna white and Boris black.
func started(t *testing.T, tc TimeControl) (*Game, *fakeClock) {
	t.Helper()
	clk := &fakeClock{t: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)}
	g := newGame("0123456789abcdef", clk.now, tc, anna, White)
	t.Cleanup(g.Close)
	if err := g.Join(boris); err != nil {
		t.Fatal(err)
	}
	return g, clk
}

func play(t *testing.T, g *Game, moves ...string) {
	t.Helper()
	for i, m := range moves {
		who := "anna"
		if i%2 == 1 {
			who = "boris"
		}
		g.mu.Lock()
		turn := g.turn
		g.mu.Unlock()
		if turn == Black {
			who = "boris"
		} else {
			who = "anna"
		}
		if err := g.Move(who, m); err != nil {
			t.Fatalf("move %d %s: %v", i+1, m, err)
		}
	}
}

func state(g *Game) (status, result, term string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.status, g.result, g.termination
}

func TestWaitingThenJoin(t *testing.T) {
	clk := &fakeClock{t: time.Now()}
	g := newGame("0123456789abcdef", clk.now, TimeControl{}, anna, Black) // creator took Black
	defer g.Close()
	if st, _, _ := state(g); st != StatusWaiting {
		t.Fatalf("status %s", st)
	}
	if err := g.Join(anna); err == nil {
		t.Error("the creator joined their own game")
	}
	if err := g.Move("anna", "e2e4"); err == nil {
		t.Error("a move in a game that has not started")
	}
	if err := g.Join(boris); err != nil {
		t.Fatal(err)
	}
	if g.white.ID != "boris" || g.black.ID != "anna" {
		t.Errorf("seats: white %v black %v", g.white, g.black)
	}
	if err := g.Join(Player{ID: "carl", Name: "Карл"}); err == nil {
		t.Error("a third player joined")
	}
}

func TestMovesAndRules(t *testing.T) {
	g, _ := started(t, TimeControl{})
	if err := g.Move("boris", "e7e5"); err != errNotYours {
		t.Errorf("black moving first: %v", err)
	}
	if err := g.Move("carl", "e2e4"); err != errNotPlayer {
		t.Errorf("a stranger moving: %v", err)
	}
	if err := g.Move("anna", "e2e5"); err != errIllegal {
		t.Errorf("illegal move: %v", err)
	}
	if err := g.Move("anna", "nonsense"); err != errIllegal {
		t.Errorf("garbage move: %v", err)
	}
	play(t, g, "e2e4", "e7e5", "g1f3")
	g.mu.Lock()
	got := []string{g.moves[0].SAN, g.moves[1].SAN, g.moves[2].SAN}
	turn := g.turn
	g.mu.Unlock()
	if strings.Join(got, " ") != "e4 e5 Nf3" || turn != Black {
		t.Errorf("moves %v, turn %s", got, turn)
	}
}

func TestCheckmate(t *testing.T) {
	g, _ := started(t, TimeControl{})
	play(t, g, "f2f3", "e7e5", "g2g4", "d8h4")
	st, res, term := state(g)
	if st != StatusEnded || res != "0-1" || term != TermCheckmate {
		t.Errorf("fool's mate: %s %s %s", st, res, term)
	}
	if err := g.Move("anna", "a2a3"); err != errNotPlaying {
		t.Errorf("a move after the end: %v", err)
	}
}

func TestRepetitionDraw(t *testing.T) {
	g, _ := started(t, TimeControl{})
	play(t, g, "g1f3", "g8f6", "f3g1", "f6g8", "g1f3", "g8f6", "f3g1", "f6g8")
	st, res, term := state(g)
	if st != StatusEnded || res != "1/2-1/2" || term != TermRepetition {
		t.Errorf("threefold repetition: %s %s %s", st, res, term)
	}
}

func TestClockAndIncrement(t *testing.T) {
	g, clk := started(t, TimeControl{BaseMs: 60_000, IncMs: 2_000})
	// Until both sides have moved the clocks stand still.
	clk.advance(30 * time.Second)
	play(t, g, "e2e4")
	clk.advance(30 * time.Second)
	play(t, g, "e7e5")
	g.mu.Lock()
	w, b := g.ms[White], g.ms[Black]
	g.mu.Unlock()
	if w != 60_000 || b != 60_000 {
		t.Errorf("clocks moved before the game got going: %d %d", w, b)
	}
	// Now White thinks for 10 s: 60 - 10 + 2 = 52 s.
	clk.advance(10 * time.Second)
	play(t, g, "g1f3")
	g.mu.Lock()
	w = g.ms[White]
	g.mu.Unlock()
	if w != 52_000 {
		t.Errorf("white has %d ms, want 52000", w)
	}
	// The snapshot shows the running side's time going down.
	clk.advance(5 * time.Second)
	var m Message
	if err := json.Unmarshal(g.Snapshot("anna"), &m); err != nil {
		t.Fatal(err)
	}
	c := m.Game.Clock
	if c.Running != Black || c.BlackMs != 55_000 || c.WhiteMs != 52_000 || c.ServerNow != clk.t.UnixMilli() {
		t.Errorf("clock in snapshot: %+v", c)
	}
}

func TestFlagFalls(t *testing.T) {
	g, clk := started(t, TimeControl{BaseMs: 20_000})
	play(t, g, "e2e4", "e7e5")
	clk.advance(19 * time.Second)
	g.tick()
	if st, _, _ := state(g); st != StatusPlaying {
		t.Fatal("flagged with a second left")
	}
	clk.advance(2 * time.Second)
	g.tick()
	st, res, term := state(g)
	if st != StatusEnded || res != "0-1" || term != TermTimeout {
		t.Errorf("white out of time: %s %s %s", st, res, term)
	}
	var m Message
	json.Unmarshal(g.Snapshot("anna"), &m)
	if c := m.Game.Clock; c.WhiteMs != 0 || c.BlackMs != 20_000 || c.Running != "" {
		t.Errorf("final clocks: %+v", c)
	}
}

func TestMoveAfterFlagIsRefused(t *testing.T) {
	g, clk := started(t, TimeControl{BaseMs: 20_000})
	play(t, g, "e2e4", "e7e5")
	clk.advance(25 * time.Second) // the timer has not fired yet, but the time is up
	if err := g.Move("anna", "g1f3"); err == nil {
		t.Fatal("a move was accepted after the flag fell")
	}
	if _, res, term := state(g); res != "0-1" || term != TermTimeout {
		t.Errorf("result %s %s", res, term)
	}
}

func TestTimeoutAgainstBareKingIsADraw(t *testing.T) {
	g, clk := started(t, TimeControl{BaseMs: 20_000})
	play(t, g, "e2e4", "e7e5")
	// Black has only a king and a knight: nothing to mate with.
	pos, err := engine.ParseFEN("4k1n1/8/8/8/8/8/4P3/4K3 w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	g.pos = pos
	g.mu.Unlock()
	clk.advance(25 * time.Second) // white to move ran out; black cannot mate
	g.tick()
	if _, res, term := state(g); res != "1/2-1/2" || term != TermTimeout {
		t.Errorf("flag against a lone knight: %s %s", res, term)
	}
}

func TestCanMate(t *testing.T) {
	cases := []struct {
		fen  string
		side Side
		want bool
	}{
		{"4k3/8/8/8/8/8/8/4K3 w - - 0 1", White, false},  // bare kings
		{"4k3/8/8/8/8/8/8/3NK3 w - - 0 1", White, false}, // king and knight
		{"4k3/8/8/8/8/8/8/3BK3 w - - 0 1", White, false},
		{"4k3/8/8/8/8/8/8/2NNK3 w - - 0 1", White, true}, // two minors
		{"4k3/8/8/8/8/8/8/2BNK3 w - - 0 1", White, true},
		{"4k3/8/8/8/8/8/4P3/4K3 w - - 0 1", White, true}, // a pawn can promote
		{"4k3/8/8/8/8/8/8/3RK3 w - - 0 1", White, true},
		{"4k1n1/8/8/8/8/8/8/4K3 w - - 0 1", Black, false},
		{"r3k3/8/8/8/8/8/8/4K3 w - - 0 1", Black, true},
		{"4k1q1/8/8/8/8/8/8/4K3 w - - 0 1", Black, true},
		{"4k1q1/8/8/8/8/8/8/4K3 w - - 0 1", White, false}, // the other side's pieces do not count
	}
	for _, c := range cases {
		pos, err := engine.ParseFEN(c.fen)
		if err != nil {
			t.Fatalf("%s: %v", c.fen, err)
		}
		if got := (&Game{pos: pos}).canMate(c.side); got != c.want {
			t.Errorf("canMate(%s) in %s = %v, want %v", c.side, c.fen, got, c.want)
		}
	}
}

func TestNoFirstMoveAbortsTheGame(t *testing.T) {
	g, clk := started(t, TimeControl{})
	clk.advance(30 * time.Second)
	g.tick()
	if st, _, _ := state(g); st != StatusPlaying {
		t.Fatal("aborted too early")
	}
	clk.advance(40 * time.Second)
	g.tick()
	st, res, term := state(g)
	if st != StatusAborted || res != "" || term != TermNoFirstMove {
		t.Errorf("nobody moved: %s %q %s", st, res, term)
	}

	// The same for Black's first move.
	g2, clk2 := started(t, TimeControl{})
	play(t, g2, "e2e4")
	clk2.advance(61 * time.Second)
	g2.tick()
	if st, _, term := state(g2); st != StatusAborted || term != TermNoFirstMove {
		t.Errorf("black never moved: %s %s", st, term)
	}
}

func TestDrawOffers(t *testing.T) {
	g, _ := started(t, TimeControl{})
	if err := g.OfferDraw("anna"); err == nil {
		t.Error("a draw offered before any moves")
	}
	play(t, g, "e2e4", "e7e5")
	if err := g.OfferDraw("anna"); err != nil {
		t.Fatal(err)
	}
	if g.drawOffer != White {
		t.Fatal("offer not recorded")
	}
	// Black declines; White offers again; a move clears the offer.
	if err := g.DeclineDraw("boris"); err != nil || g.drawOffer != "" {
		t.Errorf("decline: %v %q", err, g.drawOffer)
	}
	g.OfferDraw("anna")
	play(t, g, "g1f3")
	if g.drawOffer != "" {
		t.Error("a move did not clear the draw offer")
	}
	// Offer and accept.
	if err := g.OfferDraw("boris"); err != nil {
		t.Fatal(err)
	}
	if err := g.OfferDraw("anna"); err != nil { // accepting = offering back
		t.Fatal(err)
	}
	if st, res, term := state(g); st != StatusEnded || res != "1/2-1/2" || term != TermAgreement {
		t.Errorf("agreed draw: %s %s %s", st, res, term)
	}
	if err := g.OfferDraw("carl"); err != errNotPlayer {
		t.Errorf("stranger: %v", err)
	}
}

func TestResignAndAbort(t *testing.T) {
	g, _ := started(t, TimeControl{})
	if err := g.Abort("carl"); err != errNotPlayer {
		t.Errorf("stranger aborting: %v", err)
	}
	if err := g.Abort("boris"); err != nil { // before the first moves either may abort
		t.Fatal(err)
	}
	if st, res, term := state(g); st != StatusAborted || res != "" || term != TermAborted {
		t.Errorf("abort: %s %q %s", st, res, term)
	}

	g2, _ := started(t, TimeControl{})
	play(t, g2, "e2e4", "e7e5")
	if err := g2.Abort("anna"); err == nil {
		t.Error("aborted a game that had begun")
	}
	if err := g2.Resign("anna"); err != nil {
		t.Fatal(err)
	}
	if st, res, term := state(g2); st != StatusEnded || res != "0-1" || term != TermResign {
		t.Errorf("resign: %s %s %s", st, res, term)
	}
	if err := g2.Resign("boris"); err != errNotPlaying {
		t.Errorf("resigning twice: %v", err)
	}

	// An invitation can be withdrawn by its creator only.
	clk := &fakeClock{t: time.Now()}
	w := newGame("0123456789abcdef", clk.now, TimeControl{}, anna, White)
	defer w.Close()
	if err := w.Abort("anna"); err != nil {
		t.Errorf("creator withdrawing: %v", err)
	}
}

func TestRematchSwapsColours(t *testing.T) {
	g, _ := started(t, TimeControl{BaseMs: 60_000, IncMs: 1_000})
	if _, err := g.Rematch("anna", "ffffffffffffffff"); err == nil {
		t.Error("rematch of a game still going")
	}
	play(t, g, "f2f3", "e7e5", "g2g4", "d8h4")
	if n, err := g.Rematch("anna", "ffffffffffffffff"); n != nil || err != nil {
		t.Fatalf("first wish: %v %v", n, err)
	}
	if g.rematchOffer != White {
		t.Fatal("offer not recorded")
	}
	// asking twice takes the offer back
	g.Rematch("anna", "ffffffffffffffff")
	if g.rematchOffer != "" {
		t.Error("offer not withdrawn")
	}
	g.Rematch("anna", "ffffffffffffffff")
	n, err := g.Rematch("boris", "eeeeeeeeeeeeeeee")
	if err != nil || n == nil {
		t.Fatalf("second wish: %v %v", n, err)
	}
	defer n.Close()
	if n.ID != "eeeeeeeeeeeeeeee" || n.white.ID != "boris" || n.black.ID != "anna" || n.status != StatusPlaying || n.tc != g.tc {
		t.Errorf("new game: %+v white %v black %v", n.status, n.white, n.black)
	}
	if g.rematchID != n.ID {
		t.Error("old game does not point at the new one")
	}
	if again, _ := g.Rematch("boris", "dddddddddddddddd"); again != nil {
		t.Error("a second rematch was created")
	}
}

func TestSnapshotLegalMovesOnlyForTheMover(t *testing.T) {
	g, _ := started(t, TimeControl{})
	var white, black, spectator Message
	json.Unmarshal(g.Snapshot("anna"), &white)
	json.Unmarshal(g.Snapshot("boris"), &black)
	json.Unmarshal(g.Snapshot(""), &spectator)
	if white.You != White || len(white.Game.Legal) != 20 {
		t.Errorf("white sees you=%s legal=%d", white.You, len(white.Game.Legal))
	}
	if black.You != Black || len(black.Game.Legal) != 0 {
		t.Errorf("black sees you=%s legal=%d", black.You, len(black.Game.Legal))
	}
	if spectator.You != "" || len(spectator.Game.Legal) != 0 {
		t.Errorf("spectator sees you=%q legal=%d", spectator.You, len(spectator.Game.Legal))
	}
	if white.Game.FirstMoveBy == 0 {
		t.Error("no first-move deadline in the snapshot")
	}
}

func TestPresenceAndBroadcast(t *testing.T) {
	g, _ := started(t, TimeControl{})
	a, b, s := newConn("anna"), newConn("boris"), newConn("")
	g.addConn(a)
	g.addConn(b)
	g.addConn(s)
	drain := func(c *conn) (last Message) {
		for {
			select {
			case raw := <-c.send:
				json.Unmarshal(raw, &last)
			default:
				return
			}
		}
	}
	got := drain(a)
	if !got.Game.Presence.White || !got.Game.Presence.Black || got.Game.Presence.Watching != 1 {
		t.Errorf("presence: %+v", got.Game.Presence)
	}
	drain(b)
	drain(s)
	play(t, g, "e2e4")
	if m := drain(s); m.Game == nil || len(m.Game.Moves) != 1 {
		t.Errorf("the spectator was not told about the move: %+v", m)
	}
	g.removeConn(b)
	if m := drain(a); m.Game.Presence.Black {
		t.Error("black still shown as present")
	}

	// A client that never reads is dropped instead of blocking the game.
	slow := newConn("boris")
	g.addConn(slow)
	for i := 0; i < 40; i++ {
		g.mu.Lock()
		g.broadcastLocked()
		g.mu.Unlock()
	}
	g.mu.Lock()
	_, still := g.conns[slow]
	g.mu.Unlock()
	if still {
		t.Error("a stuck client was kept")
	}
}

func TestStateRoundTrip(t *testing.T) {
	g, clk := started(t, TimeControl{BaseMs: 120_000, IncMs: 3_000})
	play(t, g, "e2e4", "c7c5", "g1f3")
	clk.advance(7 * time.Second)
	g.mu.Lock()
	g.drawOffer = White
	raw := g.stateLocked()
	fen, ms := g.pos.FEN(), g.ms[White]
	g.mu.Unlock()

	r, err := restore(g.ID, clk.now, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.pos.FEN() != fen || len(r.moves) != 3 || r.turn != Black || r.status != StatusPlaying ||
		r.ms[White] != ms || r.drawOffer != White || r.white.ID != "anna" || r.black.ID != "boris" || r.tc != g.tc {
		t.Errorf("restored game differs: %+v", r)
	}
	if len(r.hashes) != 4 {
		t.Errorf("hash history %d, want 4 (repetition would be blind)", len(r.hashes))
	}
	// The clock keeps running from where the saved turn began.
	clk.advance(3 * time.Second)
	var m Message
	json.Unmarshal(r.Snapshot("anna"), &m)
	if m.Game.Clock.BlackMs != 120_000-10_000 {
		t.Errorf("black has %d ms after restore", m.Game.Clock.BlackMs)
	}
	// And a game with a doctored move list is refused.
	bad := strings.Replace(string(raw), `"uci":"c7c5"`, `"uci":"c7c4"`, 1)
	if _, err := restore(g.ID, clk.now, []byte(bad)); err == nil {
		t.Error("an illegal saved move was accepted")
	}
}

func TestPGNIsReadableByTheReviewParser(t *testing.T) {
	g, _ := started(t, TimeControl{BaseMs: 300_000, IncMs: 2_000})
	play(t, g, "f2f3", "e7e5", "g2g4", "d8h4")
	text := g.PGN()
	games, err := pgn.ParseString(text)
	if err != nil || len(games) != 1 {
		t.Fatalf("parse: %v (%d games)\n%s", err, len(games), text)
	}
	gm := games[0]
	if len(gm.Moves) != 4 || gm.Result != "0-1" || gm.Tag("White") != "Анна" || gm.Tag("Black") != "Борис" {
		t.Errorf("parsed %d moves, result %s, %s - %s\n%s", len(gm.Moves), gm.Result, gm.Tag("White"), gm.Tag("Black"), text)
	}
	if !strings.Contains(text, `[TimeControl "300+2"]`) || !strings.Contains(text, `[Termination "checkmate"]`) {
		t.Errorf("tags missing:\n%s", text)
	}
}

func TestTimeControlValidate(t *testing.T) {
	ok := []TimeControl{{}, {BaseMs: 60_000}, {BaseMs: 180_000, IncMs: 2_000}, {BaseMs: 10_000}, {BaseMs: 3 * 3600_000, IncMs: 60_000}}
	bad := []TimeControl{{IncMs: 1000}, {BaseMs: 5_000}, {BaseMs: 4 * 3600_000}, {BaseMs: 60_000, IncMs: 61_000}, {BaseMs: 60_000, IncMs: -1}}
	for _, tc := range ok {
		if err := tc.Validate(); err != nil {
			t.Errorf("%+v: %v", tc, err)
		}
	}
	for _, tc := range bad {
		if tc.Validate() == nil {
			t.Errorf("%+v accepted", tc)
		}
	}
}

func TestClocksFreezeWhenTheGameEnds(t *testing.T) {
	g, clk := started(t, TimeControl{BaseMs: 60_000})
	play(t, g, "e2e4", "e7e5")
	clk.advance(12 * time.Second)
	if err := g.Resign("anna"); err != nil {
		t.Fatal(err)
	}
	clk.advance(30 * time.Second) // time passing after the end changes nothing
	var m Message
	json.Unmarshal(g.Snapshot("boris"), &m)
	if c := m.Game.Clock; c.WhiteMs != 48_000 || c.BlackMs != 60_000 || c.Running != "" {
		t.Errorf("clocks after resignation: %+v", c)
	}
}

// The browser lets the person choose the piece for a promotion, so every one of the
// four moves has to be found, and a promotion without a piece is no move.
func TestFindMoveAllPromotions(t *testing.T) {
	pos, err := engine.ParseFEN("8/4P1k1/8/8/8/8/8/4K3 w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"e7e8q", "e7e8r", "e7e8b", "e7e8n"} {
		m, ok := findMove(pos, u)
		if !ok {
			t.Errorf("%s not found", u)
			continue
		}
		after := pos.Copy()
		after.Play(m)
		if !strings.Contains(after.FEN(), map[string]string{"q": "Q", "r": "R", "b": "B", "n": "N"}[u[4:]]) {
			t.Errorf("%s: %s", u, after.FEN())
		}
	}
	if _, ok := findMove(pos, "e7e8"); ok {
		t.Error("a promotion without a piece was accepted")
	}
}

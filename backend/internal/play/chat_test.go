package play

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"dyanis-chess/backend/internal/bots"
)

func TestOpeningIsNamedAsTheGameGoes(t *testing.T) {
	g, _ := started(t, TimeControl{})
	snap := func() *View {
		var m Message
		json.Unmarshal(g.Snapshot("anna"), &m)
		return m.Game
	}
	if v := snap(); v.Opening != nil || v.Book {
		t.Errorf("a game that has not begun has an opening: %+v", v.Opening)
	}
	play(t, g, "e2e4")
	if v := snap(); v.Opening == nil || !strings.Contains(v.Opening.Name, "King's Pawn") || !v.Book {
		t.Errorf("after 1.e4: %+v book=%v", v.Opening, v.Book)
	}
	play(t, g, "e7e5", "g1f3", "b8c6", "f1b5")
	v := snap()
	if v.Opening == nil || !strings.Contains(v.Opening.Name, "Ruy Lopez") || !strings.HasPrefix(v.Opening.ECO, "C") || !v.Book {
		t.Errorf("after 3.Bb5: %+v book=%v", v.Opening, v.Book)
	}
	// Leaving theory keeps the last name but says it is not theory any more.
	play(t, g, "a7a6", "b5a4", "g8f6", "a2a3", "h7h6", "h2h3")
	v = snap()
	if v.Opening == nil || !strings.Contains(v.Opening.Name, "Ruy Lopez") {
		t.Errorf("the name was lost: %+v", v.Opening)
	}
	if v.Book {
		t.Error("a position far from theory is still called theory")
	}
	// It survives a save and a restore.
	g.mu.Lock()
	raw := g.stateLocked()
	g.mu.Unlock()
	r, err := restore(g.ID, g.now, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.opening == nil || r.opening.Name != v.Opening.Name || r.inBook != v.Book {
		t.Errorf("restored opening %+v book=%v", r.opening, r.inBook)
	}
}

func chatLines(g *Game) []ChatMsg {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]ChatMsg(nil), g.chat...)
}

func TestChatRules(t *testing.T) {
	g, clk := started(t, TimeControl{})
	if err := g.Chat("carl", "привет"); err == nil {
		t.Error("a spectator wrote in the chat")
	}
	if err := g.Chat("anna", "  привет,   как дела?  "); err != nil {
		t.Fatal(err)
	}
	if l := chatLines(g); len(l) != 1 || l[0].Text != "привет, как дела?" || l[0].Side != White || l[0].At == 0 {
		t.Fatalf("line: %+v", l)
	}
	// too fast
	if err := g.Chat("anna", "ещё"); err == nil {
		t.Error("two lines within the gap")
	}
	if err := g.Chat("boris", "здравствуй"); err != nil { // the other person has their own gap
		t.Error(err)
	}
	clk.advance(time.Second)
	// empty lines and control characters
	if err := g.Chat("anna", "   \n\t "); err != nil || len(chatLines(g)) != 2 {
		t.Errorf("an empty line: %v, %d lines", err, len(chatLines(g)))
	}
	if err := g.Chat("anna", "a\x00b\nc‮d"); err != nil {
		t.Fatal(err)
	}
	if l := chatLines(g); strings.ContainsAny(l[2].Text, "\x00\n ") || l[2].Text != "a b c d" && !strings.HasPrefix(l[2].Text, "a b c") {
		t.Errorf("control characters kept: %q", l[2].Text)
	}
	// length limit, counted in characters
	clk.advance(time.Second)
	g.Chat("anna", strings.Repeat("я", 500))
	if l := chatLines(g); len([]rune(l[len(l)-1].Text)) != maxChatRunes {
		t.Errorf("long line is %d characters", len([]rune(l[len(l)-1].Text)))
	}
	// only the last hundred are kept
	for i := 0; i < 130; i++ {
		clk.advance(time.Second)
		g.Chat("boris", "x")
	}
	if n := len(chatLines(g)); n != maxChatKept {
		t.Errorf("%d lines kept, want %d", n, maxChatKept)
	}
	// the chat is in the snapshot and in the saved state
	var m Message
	json.Unmarshal(g.Snapshot("boris"), &m)
	if len(m.Game.Chat) != maxChatKept {
		t.Errorf("snapshot has %d lines", len(m.Game.Chat))
	}
	g.mu.Lock()
	raw := g.stateLocked()
	g.mu.Unlock()
	r, err := restore(g.ID, g.now, raw)
	if err != nil || len(r.chat) != maxChatKept {
		t.Errorf("restored chat: %v (%d lines)", err, len(r.chat))
	}
	r.Close()
}

func TestChatNotBeforeOrAgainstABot(t *testing.T) {
	clk := &fakeClock{t: time.Now()}
	w := newGame("0123456789abcdef", clk.now, TimeControl{}, anna, White)
	defer w.Close()
	if err := w.Chat("anna", "hi"); err == nil {
		t.Error("chat in a waiting room")
	}
	b, _ := bots.Get("misha")
	g := blankGame("fedcba9876543210", clk.now, TimeControl{})
	bp := botPlayer(b)
	g.white, g.black = &anna, &bp
	g.start()
	defer g.Close()
	if err := g.Chat("anna", "привет"); err == nil {
		t.Error("chat with a bot")
	}
}

func TestChatOverTheSocket(t *testing.T) {
	_, srv := newTestHub(t, nil)
	id := createGame(t, srv, "anna", `{"color":"white"}`)
	call(t, srv, "boris", "POST", "/api/games/"+id+"/join", "")
	anna, boris, watcher := dial(t, srv, "anna", id), dial(t, srv, "boris", id), dial(t, srv, "", id)
	anna.until("state", func(m Message) bool { return m.Type == "state" })

	b, _ := json.Marshal(map[string]string{"type": "chat", "text": "Удачи!"})
	anna.sendRaw(b)
	for name, s := range map[string]*socket{"anna": anna, "boris": boris, "watcher": watcher} {
		m := s.until(name+" sees the line", func(m Message) bool { return m.Game != nil && len(m.Game.Chat) == 1 })
		if m.Game.Chat[0].Text != "Удачи!" || m.Game.Chat[0].Side != White {
			t.Errorf("%s: %+v", name, m.Game.Chat)
		}
	}
	// a spectator's line is refused with an error, and the chat stays as it was
	b, _ = json.Marshal(map[string]string{"type": "chat", "text": "я тоже"})
	watcher.sendRaw(b)
	watcher.until("error", func(m Message) bool { return m.Type == "error" })
	if _, raw := call(t, srv, "", "GET", "/api/games/"+id, ""); strings.Contains(string(raw), "я тоже") {
		t.Error("a spectator's line got into the chat")
	}
}

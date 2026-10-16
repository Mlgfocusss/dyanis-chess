package play

import (
	"context"
	"encoding/json"
	mrand "math/rand"
	"net/http"
	"strings"
	"testing"
	"time"

	"dyanis-chess/backend/internal/bots"
	"dyanis-chess/engine"
)

func TestOutcomeFor(t *testing.T) {
	cases := []struct {
		result string
		side   Side
		want   string
	}{
		{"1-0", White, "win"}, {"1-0", Black, "loss"}, {"0-1", Black, "win"}, {"0-1", White, "loss"},
		{"1/2-1/2", White, "draw"}, {"1/2-1/2", Black, "draw"},
	}
	for _, c := range cases {
		if got := outcomeFor(c.result, c.side); got != c.want {
			t.Errorf("outcomeFor(%s, %s) = %s, want %s", c.result, c.side, got, c.want)
		}
	}
	if !botAcceptsDraw(-300) || !botAcceptsDraw(-120) || botAcceptsDraw(-50) || botAcceptsDraw(200) {
		t.Error("a bot should take a draw only when it is clearly worse")
	}
}

func TestPlayingABot(t *testing.T) {
	db := &memStore{}
	h, srv := newTestHub(t, db)
	code, b := call(t, srv, "anna", "POST", "/api/games", `{"bot":"misha","color":"white"}`)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, b)
	}
	var created struct{ ID string }
	json.Unmarshal(b, &created)
	id := created.ID

	anna := dial(t, srv, "anna", id)
	st := anna.until("running game", func(m Message) bool { return m.Type == "state" })
	g := st.Game
	if g.Status != StatusPlaying || g.White.Name != "Anna" || g.Black.Bot != "misha" || g.Black.Name != "Миша" || g.Black.Elo == 0 {
		t.Fatalf("game: %+v black=%+v", g, g.Black)
	}
	if len(g.Legal) != 20 {
		t.Errorf("white has %d legal moves", len(g.Legal))
	}

	anna.send("move", "e2e4")
	reply := anna.untilMoves(2) // the bot answers by itself
	if reply.Game.Turn != White || len(reply.Game.Legal) == 0 {
		t.Errorf("after the bot's move: turn %s, legal %d", reply.Game.Turn, len(reply.Game.Legal))
	}
	// A bot's move is a legal one: the position continues from it.
	if reply.Game.Moves[1].UCI == "" || reply.Game.Moves[1].SAN == "" {
		t.Errorf("bot move: %+v", reply.Game.Moves[1])
	}

	anna.send("resign", "")
	anna.until("end", func(m Message) bool { return m.Game != nil && m.Game.Status == StatusEnded })
	deadline := time.Now().Add(3 * time.Second)
	for {
		db.mu.Lock()
		got := append([]string(nil), db.recorded...)
		db.mu.Unlock()
		if len(got) == 1 {
			if got[0] != "anna misha loss" {
				t.Errorf("recorded %v", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("result never recorded: %v", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = h

	// The row of the bot game has no user for the bot's seat and is in the person's list.
	row, err := db.LoadGame(context.Background(), id)
	if err != nil || row.WhiteID != "anna" || row.BlackID != "" {
		t.Errorf("stored row: %+v (%v)", row, err)
	}
	code, b = call(t, srv, "anna", "GET", "/api/games", "")
	var list struct{ Items []Listed }
	json.Unmarshal(b, &list)
	if code != 200 || len(list.Items) != 1 || list.Items[0].Black == nil || list.Items[0].Black.Bot != "misha" {
		t.Errorf("list: %d %s", code, b)
	}
}

func TestBotMovesFirstWhenItIsWhite(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	code, b := call(t, srv, "anna", "POST", "/api/games", `{"bot":"liza","color":"black"}`)
	if code != http.StatusCreated {
		t.Fatalf("%d %s", code, b)
	}
	var created struct{ ID string }
	json.Unmarshal(b, &created)
	anna := dial(t, srv, "anna", created.ID)
	st := anna.untilMoves(1)
	if st.You != Black || st.Game.White.Bot != "liza" || st.Game.Turn != Black || len(st.Game.Legal) == 0 {
		t.Errorf("after the bot's first move: you=%s turn=%s legal=%d", st.You, st.Game.Turn, len(st.Game.Legal))
	}
}

func TestBotAnswersADrawOfferAndAgreesToARematch(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	code, b := call(t, srv, "anna", "POST", "/api/games", `{"bot":"misha","color":"white"}`)
	if code != http.StatusCreated {
		t.Fatal(code, string(b))
	}
	var created struct{ ID string }
	json.Unmarshal(b, &created)
	anna := dial(t, srv, "anna", created.ID)
	anna.until("state", func(m Message) bool { return m.Type == "state" })
	anna.send("move", "e2e4")
	anna.untilMoves(2)

	// In an equal position the bot declines: the offer disappears, the game goes on.
	anna.send("draw", "")
	anna.until("offer", func(m Message) bool { return m.Game != nil && m.Game.DrawOffer == White })
	end := anna.until("answer", func(m Message) bool { return m.Game != nil && m.Game.DrawOffer == "" })
	if end.Game.Status != StatusPlaying {
		t.Errorf("the bot ended the game: %s", end.Game.Status)
	}

	// A rematch wish is granted at once, with the colours swapped.
	anna.send("resign", "")
	anna.until("end", func(m Message) bool { return m.Game != nil && m.Game.Status == StatusEnded })
	anna.send("rematch", "")
	m := anna.until("rematch id", func(m Message) bool { return m.Game != nil && m.Game.Rematch != nil && m.Game.Rematch.ID != "" })
	code, b = call(t, srv, "anna", "GET", "/api/games/"+m.Game.Rematch.ID, "")
	var n Message
	json.Unmarshal(b, &n)
	if code != 200 || n.Game.Status != StatusPlaying || n.You != Black || n.Game.White.Bot != "misha" {
		t.Fatalf("rematch game: %d %s", code, b)
	}
	// The bot is White there, so it has already moved (or is about to).
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, b = call(t, srv, "anna", "GET", "/api/games/"+m.Game.Rematch.ID, "")
		json.Unmarshal(b, &n)
		if len(n.Game.Moves) >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the bot did not open the rematch")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestResultsAreRecordedOnceAndPointsComeFromTheFirstWin(t *testing.T) {
	db := &memStore{}
	h, _ := newTestHub(t, db)
	b, _ := bots.Get("timur")
	mk := func(result string, humanWhite bool) *Game {
		g := blankGame(newID(), time.Now, TimeControl{})
		human := Player{ID: "anna", Name: "Анна"}
		bp := botPlayer(b)
		if humanWhite {
			g.white, g.black = &human, &bp
		} else {
			g.white, g.black = &bp, &human
		}
		g.after = h.after
		g.start()
		g.mu.Lock()
		g.finish(result, TermResign)
		g.changed()
		g.changed() // a second change of an ended game must not record again
		g.mu.Unlock()
		return g
	}
	mk("1-0", true)  // win as White
	mk("0-1", false) // win as Black
	mk("1/2-1/2", true)
	mk("1-0", false) // loss as Black
	deadline := time.Now().Add(3 * time.Second)
	for {
		db.mu.Lock()
		n := len(db.recorded)
		db.mu.Unlock()
		if n >= 4 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("recorded %d results", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond) // a double record would show up now
	db.mu.Lock()
	defer db.mu.Unlock()
	if len(db.recorded) != 4 {
		t.Errorf("recorded %d results, want 4: %v", len(db.recorded), db.recorded)
	}
	r := db.bots["anna/timur"]
	if r == nil || r.Wins != 2 || r.Draws != 1 || r.Losses != 1 || r.Points != b.Points {
		t.Errorf("progress: %+v (points should be %d, earned once)", r, b.Points)
	}
}

func TestBotList(t *testing.T) {
	db := &memStore{}
	_, srv := newTestHub(t, db)
	db.RecordBotResult(context.Background(), "anna", "misha", "win", 10)

	code, b := call(t, srv, "", "GET", "/api/bots", "")
	var anon struct {
		Categories []struct{ ID, Name string }
		Bots       []botJSON
		Progress   map[string]progressJSON
		Points     int
	}
	json.Unmarshal(b, &anon)
	if code != 200 || len(anon.Bots) != len(bots.All()) || len(anon.Categories) != len(bots.Categories) || len(anon.Progress) != 0 {
		t.Fatalf("anonymous list: %d %s", code, b)
	}
	for _, bt := range anon.Bots {
		if bt.Name == "" || bt.Desc == "" || bt.Elo <= 0 || bt.Points <= 0 || bt.Greeting == "" {
			t.Errorf("incomplete bot: %+v", bt)
		}
	}

	code, b = call(t, srv, "anna", "GET", "/api/bots", "")
	var mine struct {
		Progress map[string]progressJSON
		Points   int
	}
	json.Unmarshal(b, &mine)
	if code != 200 || !mine.Progress["misha"].Beaten || mine.Points != 10 || mine.Progress["liza"].Beaten {
		t.Errorf("progress: %d %s", code, b)
	}
}

func TestBotGameValidation(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	if code, _ := call(t, srv, "anna", "POST", "/api/games", `{"bot":"nobody"}`); code != 400 {
		t.Errorf("unknown bot: %d", code)
	}
	if code, _ := call(t, srv, "", "POST", "/api/games", `{"bot":"misha"}`); code != 401 {
		t.Errorf("anonymous: %d", code)
	}
	if code, _ := call(t, srv, "anna", "POST", "/api/games", `{"bot":"misha","timeMs":1000}`); code != 400 {
		t.Errorf("bad clock: %d", code)
	}
	// a bot game with a clock: the bot's clock runs while it thinks
	code, b := call(t, srv, "anna", "POST", "/api/games", `{"bot":"misha","color":"white","timeMs":180000,"incMs":2000}`)
	if code != 201 {
		t.Fatalf("%d %s", code, b)
	}
	if !strings.Contains(string(b), `"id"`) {
		t.Errorf("no id: %s", b)
	}
	// too many unfinished games against bots
	for i := 0; i < maxBotGames; i++ {
		call(t, srv, "boris", "POST", "/api/games", `{"bot":"misha","color":"white"}`)
	}
	if code, _ := call(t, srv, "boris", "POST", "/api/games", `{"bot":"misha","color":"white"}`); code != http.StatusTooManyRequests {
		t.Errorf("flood of bot games: %d", code)
	}
}

// A person beats a bot: the whole path from the first move to the points. The
// "person" here is the engine, driven through the same WebSocket a browser
// uses; the bot is a deliberately hopeless one that plays at random.
func TestBeatingABotEarnsPointsOnce(t *testing.T) {
	// the "person" is the engine and plays much faster than anyone could type
	oldBurst, oldRate := msgBurst, msgRate
	msgBurst, msgRate = 1000, 1000
	t.Cleanup(func() { msgBurst, msgRate = oldBurst, oldRate }) // runs after the server's own cleanup
	db := &memStore{}
	pushover := &bots.Bot{ID: "pushover", Name: "Тест", Elo: 100, Points: 25, Params: bots.Params{Depth: 1, Temp: 1e9}}
	h := New(Config{Store: db, User: testUser,
		BotThink: func(*bots.Bot, *mrand.Rand) time.Duration { return 0 },
		Bot: func(id string) (*bots.Bot, bool) {
			if id == "pushover" {
				return pushover, true
			}
			return bots.Get(id)
		}})
	srv := newServerFor(t, h)

	play := func() Message {
		code, b := call(t, srv, "anna", "POST", "/api/games", `{"bot":"pushover","color":"white"}`)
		if code != http.StatusCreated {
			t.Fatalf("%d %s", code, b)
		}
		var created struct{ ID string }
		json.Unmarshal(b, &created)
		anna := dial(t, srv, "anna", created.ID)
		strong, _ := bots.Get("igor")
		rng := mrand.New(mrand.NewSource(11))
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) {
			m := anna.until("my turn or the end", func(m Message) bool {
				return m.Type == "state" && (m.Game.Status == StatusEnded || len(m.Game.Legal) > 0)
			})
			if m.Game.Status == StatusEnded {
				return m
			}
			pos, err := engine.ParseFEN(m.Game.FEN)
			if err != nil {
				t.Fatal(err)
			}
			mv, err := strong.Choose(context.Background(), pos, nil, 0, 0, rng)
			if err != nil {
				t.Fatal(err)
			}
			anna.send("move", mv.String())
			after := anna.until("move accepted", func(n Message) bool { return n.Type == "state" && len(n.Game.Moves) > len(m.Game.Moves) })
			if after.Game.Status == StatusEnded { // the move itself ended the game
				return after
			}
		}
		t.Fatal("the game did not finish")
		return Message{}
	}

	end := play()
	if end.Game.Result != "1-0" {
		t.Fatalf("the engine did not beat a random mover: %s %s", end.Game.Result, end.Game.Termination)
	}
	waitRecorded := func(n int) {
		deadline := time.Now().Add(3 * time.Second)
		for {
			db.mu.Lock()
			got := len(db.recorded)
			db.mu.Unlock()
			if got >= n {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("recorded %d results, want %d", got, n)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitRecorded(1)
	db.mu.Lock()
	r := *db.bots["anna/pushover"]
	db.mu.Unlock()
	if r.Wins != 1 || r.Points != 25 {
		t.Errorf("after the first win: %+v", r)
	}
	play()
	waitRecorded(2)
	db.mu.Lock()
	r = *db.bots["anna/pushover"]
	db.mu.Unlock()
	if r.Wins != 2 || r.Points != 25 {
		t.Errorf("a second win must not pay again: %+v", r)
	}
}

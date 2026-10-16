package play

import (
	"context"
	"log"
	"math/rand"
	"net/http"
	"time"

	"dyanis-chess/backend/internal/bots"
	"dyanis-chess/backend/internal/store"
)

// Games against the computer are ordinary games (clocks, resignation, review,
// the list in the profile, reconnecting): the only difference is that one seat
// is played by a bot that the hub makes move. After every change of such a
// game, Hub.after looks at what the bot has to do: move, answer a draw offer
// or a wish for a rematch, and, once the game is over, tell the progress of
// the person that they beat it (or not).

const botIDPrefix = "bot:"

// botPlayer is the seat of a bot.
func botPlayer(b *bots.Bot) Player {
	return Player{ID: botIDPrefix + b.ID, Name: b.Name, Bot: b.ID, Elo: b.Elo}
}

// botThink is how long a bot waits before moving (the search time counts in
// it), so that the reply does not come the instant the person moves.
func botThink(b *bots.Bot, rng *rand.Rand) time.Duration {
	if b.Params.Depth == 0 {
		return 0 // the strongest searches for a while anyway
	}
	return 350*time.Millisecond + time.Duration(rng.Intn(900))*time.Millisecond
}

// maxBotGames is how many unfinished games against bots one person may have.
const maxBotGames = 8

// after is installed in every game; it runs with the game's lock held after
// every change and must not block.
func (h *Hub) after(g *Game) {
	h.afterRating(g)
	bs, bot := g.botSeat()
	if bot == nil {
		return
	}
	b, ok := h.cfg.bot(bot.Bot)
	if !ok {
		return
	}
	humanSide, human := g.humanOf()
	switch g.status {
	case StatusPlaying:
		if g.turn == bs && !g.botBusy {
			g.botBusy = true
			go h.botMove(g, b, bs)
		}
		if g.drawOffer == humanSide && !g.botReplying {
			g.botReplying = true
			go h.botAnswerDraw(g, b, bs)
		}
	case StatusEnded:
		if !g.recorded && human != nil {
			g.recorded = true
			go h.recordBotResult(human.ID, b, g.result, humanSide)
		}
		if g.rematchOffer == humanSide && g.rematchID == "" && !g.botReplying {
			g.botReplying = true
			go h.botRematch(g, bot.ID)
		}
	}
}

// botMove works out and plays the bot's move.
func (h *Hub) botMove(g *Game, b *bots.Bot, side Side) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("play: bot %s panicked: %v", b.ID, r)
		}
		g.mu.Lock()
		g.botBusy = false
		g.mu.Unlock()
	}()
	g.mu.Lock()
	if g.status != StatusPlaying || g.turn != side {
		g.mu.Unlock()
		return
	}
	now := g.now()
	pos := g.pos.Copy()
	hist := append([]uint64(nil), g.hashes...)
	var remaining, inc time.Duration
	if g.clockRunning() || (g.timed() && len(g.moves) < 2) {
		remaining = time.Duration(g.remaining(side, now)) * time.Millisecond
		inc = time.Duration(g.tc.IncMs) * time.Millisecond
	}
	g.mu.Unlock()

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mv, err := b.Choose(ctx, pos, hist, remaining, inc, rng)
	if err != nil {
		log.Printf("play: bot %s: %v", b.ID, err)
		legal := pos.LegalMoves()
		if len(legal) == 0 {
			return
		}
		mv = legal[rng.Intn(len(legal))] // never leave the game hanging
	}
	pause := h.cfg.botThink(b, rng) - time.Since(started)
	if remaining > 0 { // a short clock leaves no time for acting
		pause = min(pause, remaining/20)
	}
	if pause > 0 {
		time.Sleep(pause)
	}
	// The game may have ended (resignation, flag) while the bot thought: Move says so.
	_ = g.Move(botIDPrefix+b.ID, mv.String())
}

// botAcceptsDraw: a bot takes a draw when it is clearly worse, otherwise it plays on.
func botAcceptsDraw(botScore int) bool { return botScore <= -120 }

// botAnswerDraw answers the person's offer of a draw.
func (h *Hub) botAnswerDraw(g *Game, b *bots.Bot, side Side) {
	defer func() {
		g.mu.Lock()
		g.botReplying = false
		g.mu.Unlock()
	}()
	g.mu.Lock()
	pos := g.pos.Copy()
	hist := append([]uint64(nil), g.hashes...)
	turn := g.turn
	g.mu.Unlock()

	score, err := b.Score(pos, hist) // for the side to move
	if err != nil {
		score = 0
	}
	if turn != side {
		score = -score
	}
	time.Sleep(600 * time.Millisecond) // a moment of thought
	id := botIDPrefix + b.ID
	if botAcceptsDraw(score) {
		_ = g.AcceptDraw(id)
	} else {
		_ = g.DeclineDraw(id)
	}
}

// botRematch: a bot always agrees to play again.
func (h *Hub) botRematch(g *Game, botUserID string) {
	defer func() {
		g.mu.Lock()
		g.botReplying = false
		g.mu.Unlock()
	}()
	time.Sleep(400 * time.Millisecond)
	h.rematch(g, botUserID)
}

// outcomeFor translates a result for the person playing the bot.
func outcomeFor(result string, humanSide Side) string {
	switch {
	case result == "1/2-1/2":
		return "draw"
	case result == "1-0" && humanSide == White, result == "0-1" && humanSide == Black:
		return "win"
	default:
		return "loss"
	}
}

// recordBotResult adds a finished game to the person's progress against the bot.
func (h *Hub) recordBotResult(userID string, b *bots.Bot, result string, humanSide Side) {
	if h.cfg.Store == nil || userID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := h.cfg.Store.RecordBotResult(ctx, userID, b.ID, outcomeFor(result, humanSide), b.Points); err != nil {
		log.Printf("play: record result against %s: %v", b.ID, err)
	}
}

// -------------------------------------------------------------- the list

type botJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Elo      int    `json:"elo"`
	Points   int    `json:"points"`
	Desc     string `json:"desc"`
	Greeting string `json:"greeting"`
	OnWin    string `json:"onWin"`
	OnLose   string `json:"onLose"`
}

type progressJSON struct {
	Wins   int  `json:"wins"`
	Draws  int  `json:"draws"`
	Losses int  `json:"losses"`
	Beaten bool `json:"beaten"`
	Points int  `json:"points"` // earned from this bot
}

// listBots answers GET /api/bots: the bots, and the signed-in person's progress against them.
func (h *Hub) listBots(w http.ResponseWriter, r *http.Request) {
	type cat struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	cats := make([]cat, len(bots.Categories))
	for i, c := range bots.Categories {
		cats[i] = cat{c.ID, c.Name}
	}
	var list []botJSON
	for _, b := range bots.All() {
		list = append(list, botJSON{b.ID, b.Name, b.Category, b.Elo, b.Points, b.Desc, b.Greeting, b.OnWin, b.OnLose})
	}
	progress := map[string]progressJSON{}
	total := 0
	if u := h.user(r); u != nil && h.cfg.Store != nil {
		rows, err := h.cfg.Store.BotProgress(r.Context(), u.ID)
		if err != nil {
			log.Printf("play: bot progress: %v", err)
			writeError(w, http.StatusInternalServerError, "не удалось прочитать прогресс")
			return
		}
		for _, p := range rows {
			progress[p.BotID] = progressJSON{Wins: p.Wins, Draws: p.Draws, Losses: p.Losses, Beaten: p.Wins > 0, Points: p.Points}
			total += p.Points
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": cats, "bots": list, "progress": progress, "points": total})
}

// startBotGame creates a game of the person against a bot, already running.
func (h *Hub) startBotGame(w http.ResponseWriter, u *store.User, b *bots.Bot, side Side, tc TimeControl) {
	h.mu.Lock()
	running := 0
	for _, g := range h.games {
		g.mu.Lock()
		if g.status == StatusPlaying && g.sideOf(u.ID) != "" {
			if bs, _ := g.botSeat(); bs != "" {
				running++
			}
		}
		g.mu.Unlock()
	}
	full := len(h.games) >= maxGames
	h.mu.Unlock()
	if full {
		writeError(w, http.StatusServiceUnavailable, "сервер перегружен, попробуйте позже")
		return
	}
	if running >= maxBotGames {
		writeError(w, http.StatusTooManyRequests, "у вас слишком много незаконченных партий с ботами: закончите или отмените лишние")
		return
	}
	g := newGame(newID(), h.cfg.Now, tc, playerOf(u), side)
	bp := botPlayer(b)
	if side == White {
		g.black = &bp
	} else {
		g.white = &bp
	}
	h.register(g)
	g.mu.Lock()
	g.start()
	g.changed() // saves it, and makes the bot move if it plays White
	g.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]string{"id": g.ID})
}

// Package bots holds the computer opponents of "Играть с ботами": who they
// are (name, short story, rating, the points a first win earns) and how each
// one chooses a move.
//
// A bot is the engine made deliberately weaker in a controlled way. The
// strongest (and the second strongest) play with a full search of limited
// time. Every other bot looks at each legal move, searches the position
// after it to a limited depth, and then picks among the moves with a random
// preference for the better ones: the shallower the search and the higher the
// "temperature", the more often it overlooks a tactic and plays a worse move,
// as people do; the weakest also plays a random move now and then.
//
// Rating labels are estimates. The bots were played against each other
// (backend/cmd/botladder), the strength differences measured from those
// games, and the ladder put on a scale from 400 (the weakest, by convention)
// to 2900 (the strongest: the engine at 500 ms a move, which measured about
// 2890 on Stockfish's UCI_Elo scale, see README). The labels are therefore
// the engine's own scale and not FIDE or chess.com ratings; between
// neighbours they promise an order and a rough spacing, not a win rate.
package bots

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"runtime"
	"time"

	"dyanis-chess/engine"
)

// Params is how a bot plays.
type Params struct {
	// Depth is the search depth (plies) used to judge each candidate move; 0
	// means a full timed search of the position (MoveTime) and nothing else.
	Depth int
	// Temp (centipawns) is the softness of the choice among candidate moves:
	// the chance of a move falls with exp(-loss/Temp). 0 picks the best.
	Temp float64
	// Random is the chance of a plain random legal move instead.
	Random float64
	// MoveTime is the search time of a full search (Depth 0).
	MoveTime time.Duration
}

// Bot is one opponent.
type Bot struct {
	ID       string
	Name     string
	Category string // see Categories
	Elo      int
	Points   int // earned once, by the first win
	Desc     string
	Greeting string // said at the start of a game
	OnWin    string // said when the bot has won
	OnLose   string // said when the bot has lost
	Params   Params
}

// Categories in the order they are shown.
var Categories = []struct{ ID, Name string }{
	{"beginners", "Новички"},
	{"club", "Любители"},
	{"strong", "Сильные"},
	{"athletes", "Спортсмены"},
}

// mateCP is how a forced mate counts among centipawn scores.
const mateCP = 5000

// semaphore bounds the searches running at once, so that a crowd of games
// cannot starve the server (every search is one thread).
var semaphore = make(chan struct{}, max(1, runtime.NumCPU()/2))

// Choose picks the bot's move in pos. history lists the hashes of the
// positions so far, including the current one. remaining (0 = no clock) and
// inc keep the thinking within the bot's clock.
func (b *Bot) Choose(ctx context.Context, pos *engine.Position, history []uint64, remaining, inc time.Duration, rng *rand.Rand) (engine.Move, error) {
	select {
	case semaphore <- struct{}{}:
		defer func() { <-semaphore }()
	case <-ctx.Done():
		return engine.Move{}, ctx.Err()
	}
	moves := pos.LegalMoves()
	if len(moves) == 0 {
		return engine.Move{}, errors.New("bots: no legal moves")
	}
	if len(moves) == 1 {
		return moves[0], nil
	}
	p := b.Params
	if p.Depth == 0 {
		t := p.MoveTime
		if remaining > 0 { // never spend more than a share of the clock
			if share := remaining/25 + inc/2; share < t {
				t = max(share, 20*time.Millisecond)
			}
		}
		res, err := engine.Search(pos, history, engine.Limits{MoveTime: t, MaxDepth: 64, HashMB: hashFor(t)})
		if err != nil {
			return engine.Move{}, err
		}
		return res.Best, nil
	}
	if p.Random > 0 && rng.Float64() < p.Random {
		return moves[rng.Intn(len(moves))], nil
	}
	return b.choose(pos, history, moves, rng)
}

// choose scores every legal move and picks among them softly.
func (b *Bot) choose(pos *engine.Position, history []uint64, moves []engine.Move, rng *rand.Rand) (engine.Move, error) {
	p := b.Params
	scores := make([]float64, len(moves))
	best := math.Inf(-1)
	for i, m := range moves {
		child := pos.Copy()
		child.Play(m)
		h := append(history[:len(history):len(history)], child.Hash())
		var s float64
		if out := child.Outcome(h); out.Over {
			if out.Checkmate {
				s = mateCP
			}
		} else {
			res, err := engine.Search(child, h, engine.Limits{Depth: p.Depth, HashMB: 1})
			if err != nil {
				return engine.Move{}, err
			}
			s = -float64(res.Score)
			s = math.Max(-mateCP, math.Min(mateCP, s))
			s += openingPrior(pos, m)
		}
		scores[i] = s
		best = math.Max(best, s)
	}
	if p.Temp <= 0 {
		for i, s := range scores {
			if s == best {
				return moves[i], nil
			}
		}
	}
	// Softmax of the loss against the best move.
	weights := make([]float64, len(moves))
	var sum float64
	for i, s := range scores {
		weights[i] = math.Exp(-(best - s) / p.Temp)
		sum += weights[i]
	}
	r := rng.Float64() * sum
	for i, w := range weights {
		if r -= w; r <= 0 {
			return moves[i], nil
		}
	}
	return moves[len(moves)-1], nil
}

// Score is the bot's own opinion of a position (centipawns, for the side to
// move), used to decide on draw offers.
func (b *Bot) Score(pos *engine.Position, history []uint64) (int, error) {
	select {
	case semaphore <- struct{}{}:
		defer func() { <-semaphore }()
	case <-time.After(3 * time.Second):
		return 0, errors.New("bots: busy")
	}
	res, err := engine.Search(pos, history, engine.Limits{Depth: 8, HashMB: 8})
	if err != nil {
		return 0, err
	}
	return res.Score, nil
}

// hashFor sizes the search tables to the time: a short search cannot fill a big
// table, and clearing one would cost more than the search.
func hashFor(t time.Duration) int {
	if t < 150*time.Millisecond {
		return 8
	}
	return 32
}

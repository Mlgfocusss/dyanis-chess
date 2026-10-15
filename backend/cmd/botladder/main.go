// Command botladder measures how strong the bots of internal/bots are
// compared with each other, by playing them against their neighbours, and
// prints Elo labels tied to the engine's own strength.
//
//	botladder -evalfile engine/net-kingf-m4.bin -games 40
//
// Each pair plays the same number of games from random short openings, both
// colours of each opening. The Elo difference of a pair comes from its score;
// the differences are added up from the strongest bot, which is fixed at
// -top (the engine's measured strength, see README), downwards. The result is
// a ladder on the engine's scale, not a rating from real people.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"dyanis-chess/backend/internal/bots"
	"dyanis-chess/engine"
)

func main() {
	evalFile := flag.String("evalfile", "", "NNUE weights file")
	games := flag.Int("games", 40, "games per pair (half with each colour)")
	top := flag.Int("top", 2900, "Elo of the strongest bot")
	workers := flag.Int("workers", 6, "games played at once")
	seed := flag.Int64("seed", 1, "random seed")
	only := flag.String("only", "", "comma separated ids to measure (default: the whole ladder)")
	custom := flag.String("custom", "", "measure these settings instead of the bots, weakest first: depth:temp:random[:ms],... (depth 0 = full search of ms milliseconds)")
	flag.Parse()
	if *evalFile != "" {
		if err := engine.LoadNetwork(*evalFile); err != nil {
			log.Fatal(err)
		}
	}

	ladder := bots.All()
	if *custom != "" {
		ladder = nil
		for i, spec := range strings.Split(*custom, ",") {
			f := strings.Split(spec, ":")
			var p bots.Params
			p.Depth, _ = strconv.Atoi(f[0])
			if len(f) > 1 {
				p.Temp, _ = strconv.ParseFloat(f[1], 64)
			}
			if len(f) > 2 {
				p.Random, _ = strconv.ParseFloat(f[2], 64)
			}
			if len(f) > 3 {
				ms, _ := strconv.Atoi(f[3])
				p.MoveTime = time.Duration(ms) * time.Millisecond
			}
			ladder = append(ladder, bots.Bot{ID: fmt.Sprintf("c%d", i), Name: spec, Elo: i, Params: p})
		}
	}
	// order by the Elo currently claimed: the ladder is measured along it
	for i := range ladder {
		for j := i + 1; j < len(ladder); j++ {
			if ladder[j].Elo < ladder[i].Elo {
				ladder[i], ladder[j] = ladder[j], ladder[i]
			}
		}
	}
	if *only != "" {
		keep := map[string]bool{}
		for _, id := range splitComma(*only) {
			keep[id] = true
		}
		var f []bots.Bot
		for _, b := range ladder {
			if keep[b.ID] {
				f = append(f, b)
			}
		}
		ladder = f
	}

	type pair struct {
		score   float64
		w, d, l int
	}
	results := make([]pair, len(ladder)-1) // bot i+1 against bot i
	start := time.Now()
	for i := 0; i+1 < len(ladder); i++ {
		w, d, l := play(&ladder[i+1], &ladder[i], *games, *workers, *seed+int64(i))
		results[i] = pair{score: (float64(w) + float64(d)/2) / float64(w+d+l), w: w, d: d, l: l}
		fmt.Fprintf(os.Stderr, "%-18s vs %-18s  +%d =%d -%d  (%.0fs)\n", ladder[i+1].Name, ladder[i].Name, w, d, l, time.Since(start).Seconds())
	}

	fmt.Printf("\n%-18s %6s %6s   %s\n", "bot", "claimed", "measured", "against the next weaker")
	elo := float64(*top)
	rows := make([]float64, len(ladder))
	rows[len(ladder)-1] = elo
	for i := len(ladder) - 2; i >= 0; i-- {
		diff := eloDiff(results[i].score)
		elo -= diff
		rows[i] = elo
	}
	for i := len(ladder) - 1; i >= 0; i-- {
		note := ""
		if i > 0 {
			r := results[i-1]
			note = fmt.Sprintf("+%d =%d -%d  (%+.0f Elo)", r.w, r.d, r.l, eloDiff(r.score))
		}
		fmt.Printf("%-18s %6d %6.0f   %s\n", ladder[i].Name, ladder[i].Elo, rows[i], note)
	}
}

func splitComma(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(r)
		}
	}
	return append(out, cur)
}

// eloDiff converts a score fraction into an Elo difference, clamped so that a
// shut-out does not give infinity.
func eloDiff(score float64) float64 {
	score = math.Max(0.02, math.Min(0.98, score))
	return -400 * math.Log10(1/score-1)
}

// play plays n games of a against b (a is the stronger candidate) and returns
// a's wins, draws and losses.
func play(a, b *bots.Bot, n, workers int, seed int64) (w, d, l int) {
	type job struct {
		fen      string
		aIsWhite bool
		seed     int64
	}
	rng := rand.New(rand.NewSource(seed))
	var jobs []job
	for i := 0; i < n/2; i++ {
		fen := randomOpening(rng)
		s := rng.Int63()
		jobs = append(jobs, job{fen, true, s}, job{fen, false, s + 1})
	}
	ch := make(chan job)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for k := 0; k < workers; k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				r := game(a, b, j.fen, j.aIsWhite, j.seed)
				mu.Lock()
				switch {
				case r > 0:
					w++
				case r < 0:
					l++
				default:
					d++
				}
				mu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		ch <- j
	}
	close(ch)
	wg.Wait()
	return
}

// randomOpening plays a few random moves from the start, so that games differ.
func randomOpening(rng *rand.Rand) string {
	for {
		p := engine.NewPosition()
		ok := true
		for i := 0; i < 4; i++ {
			ms := p.LegalMoves()
			if len(ms) == 0 {
				ok = false
				break
			}
			p.Play(ms[rng.Intn(len(ms))])
		}
		if ok && !p.Outcome(nil).Over {
			return p.FEN()
		}
	}
}

// game returns +1 if a wins, -1 if b wins, 0 for a draw.
func game(a, b *bots.Bot, fen string, aIsWhite bool, seed int64) int {
	pos, err := engine.ParseFEN(fen)
	if err != nil {
		log.Fatal(err)
	}
	rng := rand.New(rand.NewSource(seed))
	hist := []uint64{pos.Hash()}
	for ply := 0; ply < 300; ply++ {
		if out := pos.Outcome(hist); out.Over {
			if !out.Checkmate {
				return 0
			}
			// the side to move is mated
			mated := pos.SideToMove()
			if (mated == engine.White) == aIsWhite {
				return -1
			}
			return 1
		}
		mover := a
		if (pos.SideToMove() == engine.White) != aIsWhite {
			mover = b
		}
		m, err := mover.Choose(context.Background(), pos, hist, 0, 0, rng)
		if err != nil {
			log.Fatal(err)
		}
		pos.Play(m)
		hist = append(hist, pos.Hash())
	}
	return 0
}

// Command gendata generates NNUE training data by self-play: every
// position the engine searched is written as a 32-byte record
// (internal/trainingdata, layout in NNUE.md) with the search score and
// the game's final result.
//
//	go build -o gendata.exe ./cmd/gendata
//	.\gendata.exe -out data/nnue/pilot -games 1000 -depth 9
//	.\gendata.exe -dump data/nnue/pilot/shard-00000.bin -n 20
//
// Output goes into shards: shard-NNNNN.bin (records only) plus
// shard-NNNNN.json (what is in it). A game never spans shards, so a
// validation set made of whole shards is split by game. The .json is
// written last and marks the shard complete; a .bin without it is
// removed on the next start. Interrupting and rerunning with the same
// flags resumes: openings come from -seed, and games already in a
// complete shard are skipped. progress.json is rewritten after every
// shard.
package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"dyanis-chess/engine/internal/board"
	"dyanis-chess/engine/internal/eval"
	"dyanis-chess/engine/internal/movegen"
	"dyanis-chess/engine/internal/nnue"
	"dyanis-chess/engine/internal/openings"
	"dyanis-chess/engine/internal/search"
	"dyanis-chess/engine/internal/trainingdata"
)

// mateBound mirrors search's unexported mateThreshold: a search score at
// least this far from zero is a forced mate.
const mateBound = search.MateScore - 1000

// adjMateScore stands in for a mate score in the adjudication streak.
const adjMateScore = 100000

// runConfig is everything that decides which games get played. It is
// saved as run.json on the first start, and a resumed run must match it.
type runConfig struct {
	Format        int    `json:"format"`
	Games         int    `json:"games"`
	Depth         int    `json:"depth"`
	Seed          int64  `json:"seed"`
	Book          string `json:"book"`
	OpenPlies     int    `json:"openplies"`
	RandPlies     int    `json:"randplies"`
	OpenEval      int    `json:"openeval"`
	OpenScore     int    `json:"openscore"`
	MaxPlies      int    `json:"maxplies"`
	AdjWin        int    `json:"adjwin"`
	AdjWinMoves   int    `json:"adjwinmoves"`
	HashMB        int    `json:"hash_mb"`
	GamesPerShard int    `json:"games_per_shard"`
	EvalFile      string `json:"evalfile,omitempty"`
}

// shardInfo is the .json sidecar of one shard.
type shardInfo struct {
	Format    int            `json:"format"`
	Shard     int            `json:"shard"`
	Games     int            `json:"games"`    // played; rejected openings not included
	Rejected  int            `json:"rejected"` // openings dropped by -openscore
	Positions int            `json:"positions"`
	Openings  []int          `json:"openings"` // indices into the run's opening list, played and rejected
	Results   map[string]int `json:"results"`  // white/draw/black
	Reasons   map[string]int `json:"reasons"`
	Plies     int            `json:"plies"` // all plies played, book moves excluded
	SearchMS  int64          `json:"search_ms"`
	Nodes     int64          `json:"nodes"`
	Created   string         `json:"created"`
}

type progress struct {
	GamesDone    int     `json:"games_done"` // openings processed, rejected included
	GamesTotal   int     `json:"games_total"`
	Rejected     int     `json:"rejected"`
	Positions    int     `json:"positions"`
	Shards       int     `json:"shards"`
	Bytes        int64   `json:"bytes"`
	GamesPerHour float64 `json:"games_per_hour_this_session"`
	ETAHours     float64 `json:"eta_hours"`
	AvgPlies     float64 `json:"avg_plies"`
	AvgMoveMS    float64 `json:"avg_move_ms"`
	AvgMoveNodes float64 `json:"avg_move_nodes"`
	ResultsWDL   [3]int  `json:"results_white_draw_black"`
	Updated      string  `json:"updated"`
	Finished     bool    `json:"finished"`
}

type gameRecord struct {
	opening  int
	rejected bool   // the first search was outside ±OpenScore; nothing recorded
	recs     []byte // len(recs)/RecordSize positions, result already set
	result   uint8
	reason   string
	plies    int
	searchMS int64
	nodes    int64
}

func main() {
	out := flag.String("out", "", "output directory (e.g. data/nnue/pilot)")
	games := flag.Int("games", 1000, "games in the whole run")
	depth := flag.Int("depth", 9, "search depth per move (iterative deepening up to it)")
	conc := flag.Int("concurrency", runtime.NumCPU(), "games played in parallel, one search thread each")
	seed := flag.Int64("seed", 1, "seed for the openings")
	bookPath := flag.String("book", "assets/gm2001.bin", "Polyglot book for the opening walk")
	openPlies := flag.Int("openplies", 8, "plies walked through the book")
	randPlies := flag.Int("randplies", 4, "random legal plies after the book walk")
	openEval := flag.Int("openeval", 100, "max |static eval| of a start position, centipawns")
	openScore := flag.Int("openscore", 200, "drop a start position whose first search scores beyond ± this, centipawns (0 = off); catches pieces left hanging by -randplies, which the static eval cannot see")
	maxPlies := flag.Int("maxplies", 400, "draw after this many plies")
	adjWin := flag.Int("adjwin", 1000, "adjudicate a win when the search scores at least this for one side for -adjwinmoves moves of each side (0 = off)")
	adjWinMoves := flag.Int("adjwinmoves", 3, "moves per side the -adjwin score must hold")
	hashMB := flag.Int("hash", 16, "hash table per game, MB")
	perShard := flag.Int("gamespershard", 50, "games per shard")
	dump := flag.String("dump", "", "print the records of this shard as FEN | score | result | flags and exit")
	dumpN := flag.Int("n", 0, "with -dump: records to print (0 = all)")
	evalFile := flag.String("evalfile", "", "NNUE weights file the search plays with (empty = hand-written eval); build with GOEXPERIMENT=simd, see NNUE.md")
	evals := flag.String("evals", "", "write the hand-written eval of every record of these shards (comma-separated .bin paths) next to each as .eval.i16 and exit")
	flag.Parse()

	if *evals != "" {
		for _, path := range strings.Split(*evals, ",") {
			if err := writeEvals(strings.TrimSpace(path)); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		return
	}

	if *dump != "" {
		if err := dumpShard(*dump, *dumpN); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *out == "" {
		fmt.Fprintln(os.Stderr, "usage: gendata -out DIR [-games 1000] [-depth 9] [-concurrency N] | gendata -dump shard.bin [-n 20]")
		os.Exit(2)
	}

	if *evalFile != "" {
		net, err := nnue.Load(*evalFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		search.SetNetwork(net)
	}

	cfg := runConfig{
		Format: trainingdata.FormatVersion, Games: *games, Depth: *depth, Seed: *seed,
		Book: filepath.ToSlash(*bookPath), OpenPlies: *openPlies, RandPlies: *randPlies, OpenEval: *openEval, OpenScore: *openScore,
		MaxPlies: *maxPlies, AdjWin: *adjWin, AdjWinMoves: *adjWinMoves, HashMB: *hashMB, GamesPerShard: *perShard,
		EvalFile: filepath.ToSlash(*evalFile),
	}
	if err := run(*out, cfg, max(*conc, 1)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(dir string, cfg runConfig, conc int) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := checkRunConfig(dir, cfg); err != nil {
		return err
	}
	done, prior, nextShard, err := scanShards(dir)
	if err != nil {
		return err
	}

	fens, err := openings.FromBookRandom(cfg.Book, cfg.Games, cfg.OpenPlies, cfg.RandPlies, cfg.OpenEval, cfg.Seed)
	if err != nil {
		return fmt.Errorf("openings: %w", err)
	}
	if len(fens) < cfg.Games {
		fmt.Fprintf(os.Stderr, "warning: only %d distinct openings for %d games; playing %d\n", len(fens), cfg.Games, len(fens))
	}
	var todo []int
	for i := range fens {
		if !done[i] {
			todo = append(todo, i)
		}
	}
	fmt.Printf("gendata: %s  depth=%d  games=%d (done %d, to play %d)  concurrency=%d  shards so far=%d\n",
		dir, cfg.Depth, len(fens), len(fens)-len(todo), len(todo), conc, nextShard)

	jobs := make(chan int)
	results := make(chan gameRecord, conc)
	go func() {
		defer close(jobs)
		for _, i := range todo {
			jobs <- i
		}
	}()
	var wg sync.WaitGroup
	for w := 0; w < conc; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				g, err := playGame(fens[i], cfg)
				if err != nil {
					fmt.Fprintf(os.Stderr, "game %d (%s) dropped: %v\n", i, fens[i], err)
					continue
				}
				g.opening = i
				results <- g
			}
		}()
	}
	go func() { wg.Wait(); close(results) }()

	start := time.Now()
	sessionGames := 0
	total := prior
	total.GamesTotal = len(fens)
	var pending []gameRecord
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		info, size, err := writeShard(dir, nextShard, pending)
		if err != nil {
			return err
		}
		nextShard++
		sessionGames += len(pending)
		pending = pending[:0]
		total.add(info, size)
		total.update(sessionGames, time.Since(start))
		fmt.Printf("shard %05d: %d games (+%d rejected), %d positions | total %d/%d openings, %d rejected, %d positions, %.1f MB, %.0f openings/h, ETA %.1f h\n",
			info.Shard, info.Games, info.Rejected, info.Positions, total.GamesDone, total.GamesTotal, total.Rejected,
			total.Positions, float64(total.Bytes)/1e6, total.GamesPerHour, total.ETAHours)
		return writeJSON(filepath.Join(dir, "progress.json"), total.progress)
	}
	for g := range results {
		pending = append(pending, g)
		if len(pending) >= cfg.GamesPerShard {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := flush(); err != nil {
		return err
	}
	total.Finished = true
	total.update(sessionGames, time.Since(start))
	fmt.Printf("finished: %d games played, %d openings rejected, %d positions, %.1f MB, avg %.1f plies/game, %.1f ms and %.0f nodes per move, W/D/B %v\n",
		total.GamesDone-total.Rejected, total.Rejected, total.Positions, float64(total.Bytes)/1e6,
		total.AvgPlies, total.AvgMoveMS, total.AvgMoveNodes, total.ResultsWDL)
	return writeJSON(filepath.Join(dir, "progress.json"), total.progress)
}

// playGame plays one self-play game from fen and returns its records.
func playGame(fen string, cfg runConfig) (gameRecord, error) {
	pos, err := board.FromFEN(fen)
	if err != nil {
		return gameRecord{}, err
	}
	tt := search.NewTranspositionTableMB(cfg.HashMB)
	history := []uint64{pos.Hash()}
	var g gameRecord
	var scores []int // white's point of view, one per searched ply

	for {
		if st := movegen.GameStatusWithHistory(pos, history); st.IsGameOver() {
			g.result, g.reason = trainingdata.Draw, st.String()
			if st == movegen.Checkmate {
				g.result = trainingdata.WhiteWin
				if pos.SideToMove == board.White {
					g.result = trainingdata.BlackWin
				}
			}
			break
		}
		if g.plies >= cfg.MaxPlies {
			g.result, g.reason = trainingdata.Draw, "max plies"
			break
		}

		var info search.SearchInfo
		t0 := time.Now()
		tt.SetGameHistory(history)
		m, err := search.BestMoveTimedInfoTT(pos, cfg.Depth, 24*time.Hour, tt.NewSearch(), func(i search.SearchInfo) { info = i })
		g.searchMS += time.Since(t0).Milliseconds()
		if err != nil {
			return gameRecord{}, err
		}
		g.nodes += info.Nodes

		score := info.Score
		if pos.SideToMove == board.Black {
			score = -score
		}
		mate := score >= mateBound || score <= -mateBound
		if g.plies == 0 && cfg.OpenScore > 0 && (mate || score > cfg.OpenScore || score < -cfg.OpenScore) {
			return gameRecord{rejected: true, reason: "unbalanced opening"}, nil
		}
		var rec [trainingdata.RecordSize]byte
		l := trainingdata.Label{
			Score:     score,
			Mate:      mate,
			InCheck:   movegen.InCheck(pos),
			NoisyBest: m.IsCapture() || m.Flag == board.Promotion,
		}
		if err := trainingdata.Encode(rec[:], pos, l); err != nil {
			return gameRecord{}, err
		}
		g.recs = append(g.recs, rec[:]...)

		adj := score
		if mate {
			adj = adjMateScore
			if score < 0 {
				adj = -adjMateScore
			}
		}
		scores = append(scores, adj)

		pos.MakeMove(m)
		history = append(history, pos.Hash())
		g.plies++

		if r, ok := adjudicate(scores, cfg); ok {
			g.result, g.reason = r, "adjudicated win"
			break
		}
	}
	for off := 0; off < len(g.recs); off += trainingdata.RecordSize {
		trainingdata.SetResult(g.recs[off:], g.result)
	}
	return g, nil
}

// adjudicate awards a win once the last 2*AdjWinMoves searches (both
// sides) all scored at least AdjWin for the same side.
func adjudicate(scores []int, cfg runConfig) (uint8, bool) {
	n := 2 * cfg.AdjWinMoves
	if cfg.AdjWin <= 0 || n <= 0 || len(scores) < n {
		return 0, false
	}
	white, black := true, true
	for _, s := range scores[len(scores)-n:] {
		white = white && s >= cfg.AdjWin
		black = black && s <= -cfg.AdjWin
	}
	switch {
	case white:
		return trainingdata.WhiteWin, true
	case black:
		return trainingdata.BlackWin, true
	}
	return 0, false
}

// ------------------------------------------------------------------ shards

func shardName(dir string, n int, ext string) string {
	return filepath.Join(dir, fmt.Sprintf("shard-%05d%s", n, ext))
}

// writeShard writes the records of games as shard n: the .bin first
// (via a temp file and rename), then the .json that marks it complete.
func writeShard(dir string, n int, games []gameRecord) (shardInfo, int64, error) {
	info := shardInfo{
		Format: trainingdata.FormatVersion, Shard: n,
		Results: map[string]int{}, Reasons: map[string]int{}, Created: time.Now().Format(time.RFC3339),
	}
	var data []byte
	for _, g := range games {
		if g.rejected {
			info.Rejected++
			info.Openings = append(info.Openings, g.opening)
			info.Reasons[g.reason]++
			continue
		}
		info.Games++
		data = append(data, g.recs...)
		info.Positions += len(g.recs) / trainingdata.RecordSize
		info.Openings = append(info.Openings, g.opening)
		info.Results[[]string{"black", "draw", "white"}[g.result]]++
		info.Reasons[g.reason]++
		info.Plies += g.plies
		info.SearchMS += g.searchMS
		info.Nodes += g.nodes
	}
	sort.Ints(info.Openings)
	bin := shardName(dir, n, ".bin")
	if err := os.WriteFile(bin+".tmp", data, 0o644); err != nil {
		return info, 0, err
	}
	if err := os.Rename(bin+".tmp", bin); err != nil {
		return info, 0, err
	}
	return info, int64(len(data)), writeJSON(shardName(dir, n, ".json"), info)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path+".tmp", append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

// checkRunConfig saves cfg as run.json, or, when resuming, insists that
// it matches the saved one: different flags would mean different
// openings and a mixed dataset.
func checkRunConfig(dir string, cfg runConfig) error {
	path := filepath.Join(dir, "run.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return writeJSON(path, cfg)
	}
	if err != nil {
		return err
	}
	var saved runConfig
	if err := json.Unmarshal(data, &saved); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if !reflect.DeepEqual(saved, cfg) {
		return fmt.Errorf("%s was made with different settings:\n  saved %+v\n  now   %+v\nuse the same flags to resume, or a new -out", path, saved, cfg)
	}
	return nil
}

// totals accumulates shard statistics into the progress report.
type totals struct {
	progress
	plies    int
	searchMS int64
	nodes    int64
}

func (t *totals) add(info shardInfo, size int64) {
	t.GamesDone += info.Games + info.Rejected
	t.Rejected += info.Rejected
	t.Positions += info.Positions
	t.Shards++
	t.Bytes += size
	t.plies += info.Plies
	t.searchMS += info.SearchMS
	t.nodes += info.Nodes
	t.ResultsWDL[0] += info.Results["white"]
	t.ResultsWDL[1] += info.Results["draw"]
	t.ResultsWDL[2] += info.Results["black"]
}

func (t *totals) update(sessionGames int, elapsed time.Duration) {
	if played := t.GamesDone - t.Rejected; played > 0 {
		t.AvgPlies = float64(t.plies) / float64(played)
	}
	if t.plies > 0 {
		t.AvgMoveMS = float64(t.searchMS) / float64(t.plies)
		t.AvgMoveNodes = float64(t.nodes) / float64(t.plies)
	}
	if h := elapsed.Hours(); h > 0 && sessionGames > 0 {
		t.GamesPerHour = float64(sessionGames) / h
		t.ETAHours = float64(t.GamesTotal-t.GamesDone) / t.GamesPerHour
	}
	t.Updated = time.Now().Format(time.RFC3339)
}

// scanShards finds the complete shards in dir (a .json next to its .bin),
// removes incomplete leftovers, and returns the openings already played,
// the totals so far and the next free shard number.
func scanShards(dir string) (map[int]bool, totals, int, error) {
	done := map[int]bool{}
	var t totals
	next := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, t, 0, err
	}
	for _, e := range entries {
		name := e.Name()
		path := filepath.Join(dir, name)
		switch {
		case strings.HasSuffix(name, ".tmp"):
			if err := os.Remove(path); err != nil {
				return nil, t, 0, err
			}
		case strings.HasPrefix(name, "shard-") && strings.HasSuffix(name, ".bin"):
			var n int
			if _, err := fmt.Sscanf(name, "shard-%05d.bin", &n); err != nil {
				return nil, t, 0, fmt.Errorf("unexpected file %s", path)
			}
			data, err := os.ReadFile(shardName(dir, n, ".json"))
			if errors.Is(err, os.ErrNotExist) {
				fmt.Printf("removing incomplete shard %s\n", path)
				if err := os.Remove(path); err != nil {
					return nil, t, 0, err
				}
				continue
			}
			if err != nil {
				return nil, t, 0, err
			}
			var info shardInfo
			if err := json.Unmarshal(data, &info); err != nil {
				return nil, t, 0, fmt.Errorf("shard %d: %w", n, err)
			}
			st, err := os.Stat(path)
			if err != nil {
				return nil, t, 0, err
			}
			if st.Size() != int64(info.Positions)*trainingdata.RecordSize {
				return nil, t, 0, fmt.Errorf("%s: %d bytes, but its .json says %d positions", path, st.Size(), info.Positions)
			}
			for _, i := range info.Openings {
				done[i] = true
			}
			t.add(info, st.Size())
			next = max(next, n+1)
		}
	}
	t.update(0, 0)
	return done, t, next, nil
}

// --------------------------------------------------------------- evals

// writeEvals stores eval.Evaluate of every record of the shard at path as
// little-endian int16, White's point of view, in path minus ".bin" plus
// ".eval.i16" — one value per record, same order. This is the hand-written
// eval's answer on exactly the positions a network is validated on, so the
// two can be compared by the same loss.
func writeEvals(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data)%trainingdata.RecordSize != 0 {
		return fmt.Errorf("%s: %d bytes is not a whole number of records", path, len(data))
	}
	n := len(data) / trainingdata.RecordSize
	out := make([]byte, 2*n)
	cache := eval.NewPawnCache()
	for i := 0; i < n; i++ {
		b, _, err := trainingdata.Decode(data[i*trainingdata.RecordSize:])
		if err != nil {
			return fmt.Errorf("%s: record %d: %w", path, i, err)
		}
		e := eval.Evaluate(b, cache)
		if b.SideToMove == board.Black {
			e = -e
		}
		binary.LittleEndian.PutUint16(out[2*i:], uint16(int16(min(max(e, -trainingdata.MaxScore), trainingdata.MaxScore))))
	}
	dst := strings.TrimSuffix(path, ".bin") + ".eval.i16"
	if err := os.WriteFile(dst, out, 0o644); err != nil {
		return err
	}
	fmt.Printf("%s: %d evals\n", dst, n)
	return nil
}

// ---------------------------------------------------------------- dump

func dumpShard(path string, n int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data)%trainingdata.RecordSize != 0 {
		return fmt.Errorf("%s: %d bytes is not a whole number of records", path, len(data))
	}
	count := len(data) / trainingdata.RecordSize
	if n > 0 && n < count {
		count = n
	}
	for i := 0; i < count; i++ {
		fen, l, err := trainingdata.DecodeFEN(data[i*trainingdata.RecordSize:])
		if err != nil {
			return fmt.Errorf("record %d: %w", i, err)
		}
		var flags []string
		if l.InCheck {
			flags = append(flags, "check")
		}
		if l.NoisyBest {
			flags = append(flags, "noisy")
		}
		if l.Mate {
			flags = append(flags, "mate")
		}
		fmt.Printf("%s | %d | %s | %s\n", fen, l.Score, []string{"0-1", "1/2", "1-0"}[l.Result], strings.Join(flags, ","))
	}
	return nil
}

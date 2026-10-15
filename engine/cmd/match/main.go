// Command match plays a series of games between two UCI engine binaries
// (typically two builds of cmd/cli: a saved baseline and a candidate) and
// reports W/D/L, the Elo difference with a 95% confidence interval and,
// optionally, an SPRT verdict. It is the "does this change actually make
// the engine stronger" thermometer that node counts and the tactical
// suite can't be.
//
//	go build -o baseline.exe ./cmd/cli      # before the change
//	# ... make the change ...
//	go build -o candidate.exe ./cmd/cli
//	go build -o match.exe ./cmd/match
//	.\match.exe -a candidate.exe -b baseline.exe -movetime=100 -pairs=100
//
// Games are played in pairs from the same opening with colors swapped,
// which cancels most of the opening's bias. Openings are either read from
// -openings (one FEN per line) or generated from -seed as short random
// non-capturing lines filtered to be roughly balanced, so the same seed
// gives the same openings for every run.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"dyanis-chess/engine/internal/board"
	"dyanis-chess/engine/internal/movegen"
	"dyanis-chess/engine/internal/openings"
)

// ---------------------------------------------------------------- engine

type engine struct {
	path  string
	cmd   *exec.Cmd
	in    io.WriteCloser
	lines chan string // closed when the process's stdout ends
}

func startEngine(path string, args []string) (*engine, error) {
	cmd := exec.Command(path, args...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	e := &engine{path: path, cmd: cmd, in: in, lines: make(chan string, 256)}
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			e.lines <- sc.Text()
		}
		close(e.lines)
	}()
	e.send("uci")
	if _, err := e.waitFor("uciok", 10*time.Second); err != nil {
		e.kill()
		return nil, fmt.Errorf("%s: no uciok: %w", path, err)
	}
	return e, nil
}

func (e *engine) send(s string) { fmt.Fprintln(e.in, s) }

func (e *engine) kill() {
	e.in.Close()
	if e.cmd.Process != nil {
		e.cmd.Process.Kill()
	}
	e.cmd.Wait()
}

func (e *engine) quit() {
	e.send("quit")
	e.in.Close()
	done := make(chan struct{})
	go func() { e.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		e.cmd.Process.Kill()
		<-done
	}
}

// waitFor returns the first line starting with prefix, discarding
// everything before it (info lines, etc.).
func (e *engine) waitFor(prefix string, timeout time.Duration) (string, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case line, ok := <-e.lines:
			if !ok {
				return "", fmt.Errorf("engine exited")
			}
			if strings.HasPrefix(line, prefix) {
				return line, nil
			}
		case <-timer.C:
			return "", fmt.Errorf("timeout waiting for %q", prefix)
		}
	}
}

// mateAdjScore is what a "score mate N" line counts as for adjudication.
const mateAdjScore = 100000

// waitBestmove is waitFor("bestmove") that also remembers the last
// "score cp X" / "score mate N" it saw on the way, from the mover's point
// of view. ok is false when the engine printed no score (e.g. a book move).
func (e *engine) waitBestmove(timeout time.Duration) (line string, score int, ok bool, err error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case l, open := <-e.lines:
			if !open {
				return "", 0, false, fmt.Errorf("engine exited")
			}
			if strings.HasPrefix(l, "bestmove") {
				return l, score, ok, nil
			}
			if strings.HasPrefix(l, "info") {
				f := strings.Fields(l)
				for i := 0; i+2 < len(f); i++ {
					if f[i] != "score" {
						continue
					}
					if v, perr := strconv.Atoi(f[i+2]); perr == nil {
						switch f[i+1] {
						case "cp":
							score, ok = v, true
						case "mate":
							if v >= 0 {
								score, ok = mateAdjScore, true
							} else {
								score, ok = -mateAdjScore, true
							}
						}
					}
					break
				}
			}
		case <-timer.C:
			return "", 0, false, fmt.Errorf("timeout waiting for \"bestmove\"")
		}
	}
}

func (e *engine) ready() error {
	e.send("isready")
	_, err := e.waitFor("readyok", 10*time.Second)
	return err
}

// ------------------------------------------------------------------ game

type result int

const (
	whiteWins result = iota
	draw
	blackWins
)

type gameOutcome struct {
	res    result
	reason string
	moves  []string
}

// adjudication ends a game early from the engines' own scores. A win is
// awarded when the last 2*winMoves plies (both engines' searches) all
// reported at least winCP for the same side; a draw when, from drawPly on,
// the last 2*drawMoves plies all stayed within ±drawCP. A ply with no score
// breaks the streak. winCP/drawCP of 0 disable the respective rule.
type adjudication struct {
	winCP, winMoves   int
	drawCP, drawMoves int
	drawPly           int
}

// check looks at scores (white's point of view, one per ply so far, with
// known[i] false for plies that printed no score).
func (a adjudication) check(scores []int, known []bool) (result, string, bool) {
	n := len(scores)
	streak := func(plies int, pred func(int) bool) bool {
		if plies <= 0 || n < plies {
			return false
		}
		for i := n - plies; i < n; i++ {
			if !known[i] || !pred(scores[i]) {
				return false
			}
		}
		return true
	}
	if a.winCP > 0 {
		if streak(2*a.winMoves, func(s int) bool { return s >= a.winCP }) {
			return whiteWins, "adjudicated win", true
		}
		if streak(2*a.winMoves, func(s int) bool { return s <= -a.winCP }) {
			return blackWins, "adjudicated win", true
		}
	}
	if a.drawCP > 0 && n >= a.drawPly &&
		streak(2*a.drawMoves, func(s int) bool { return s <= a.drawCP && s >= -a.drawCP }) {
		return draw, "adjudicated draw", true
	}
	return draw, "", false
}

// playGame plays one game from startFEN; white/black are the two engine
// processes. An engine that crashes, times out or plays an illegal move
// loses on the spot.
func playGame(white, black *engine, startFEN string, movetime, maxPlies int, adj adjudication) gameOutcome {
	pos, err := board.FromFEN(startFEN)
	if err != nil {
		return gameOutcome{res: draw, reason: "bad opening FEN: " + err.Error()}
	}
	history := []uint64{pos.Hash()}
	var moves []string
	var scores []int // white's point of view, one per ply played
	var known []bool
	hard := time.Duration(movetime)*10*time.Millisecond + 5*time.Second

	for _, e := range []*engine{white, black} {
		e.send("ucinewgame")
		if err := e.ready(); err != nil {
			return gameOutcome{res: draw, reason: "engine not ready: " + err.Error()}
		}
	}

	for ply := 0; ; ply++ {
		if st := movegen.GameStatusWithHistory(pos, history); st.IsGameOver() {
			if st == movegen.Checkmate {
				if pos.SideToMove == board.White {
					return gameOutcome{blackWins, "checkmate", moves}
				}
				return gameOutcome{whiteWins, "checkmate", moves}
			}
			return gameOutcome{draw, st.String(), moves}
		}
		if ply >= maxPlies {
			return gameOutcome{draw, "max plies", moves}
		}

		mover, other := white, black
		loserIfFail := blackWins
		if pos.SideToMove == board.Black {
			mover, other = black, white
			loserIfFail = whiteWins
		}
		_ = other

		posCmd := "position fen " + startFEN
		if len(moves) > 0 {
			posCmd += " moves " + strings.Join(moves, " ")
		}
		mover.send(posCmd)
		mover.send("go movetime " + strconv.Itoa(movetime))
		line, score, scored, err := mover.waitBestmove(hard)
		if err != nil {
			return gameOutcome{loserIfFail, "engine failure: " + err.Error(), moves}
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return gameOutcome{loserIfFail, "empty bestmove", moves}
		}

		var mv board.Move
		found := false
		for _, m := range movegen.GenerateLegalMoves(pos) {
			if m.String() == fields[1] {
				mv, found = m, true
				break
			}
		}
		if !found {
			return gameOutcome{loserIfFail, "illegal move " + fields[1], moves}
		}
		pos.MakeMove(mv)
		history = append(history, pos.Hash())
		moves = append(moves, fields[1])

		// Scores are the mover's; keep them from white's point of view.
		if mover != white {
			score = -score
		}
		scores = append(scores, score)
		known = append(known, scored)
		if res, reason, done := adj.check(scores, known); done {
			return gameOutcome{res, reason, moves}
		}
	}
}

// ----------------------------------------------------------------- stats

type tally struct{ w, d, l int }

func (t tally) n() int { return t.w + t.d + t.l }

func (t tally) mean() float64 {
	if t.n() == 0 {
		return 0.5
	}
	return (float64(t.w) + 0.5*float64(t.d)) / float64(t.n())
}

// variance of the per-game score.
func (t tally) variance() float64 {
	if t.n() == 0 {
		return 0
	}
	m := t.mean()
	n := float64(t.n())
	return (float64(t.w)*(1-m)*(1-m) + float64(t.d)*(0.5-m)*(0.5-m) + float64(t.l)*m*m) / n
}

func eloFromScore(s float64) float64 {
	if s <= 0 {
		return math.Inf(-1)
	}
	if s >= 1 {
		return math.Inf(1)
	}
	return -400 * math.Log10(1/s-1)
}

func scoreFromElo(elo float64) float64 { return 1 / (1 + math.Pow(10, -elo/400)) }

// eloReportFrom returns the Elo estimate and the half-width of its 95% CI
// for n samples with the given mean score and per-sample variance.
func eloReportFrom(m, v float64, n int) (elo, ci float64) {
	if n == 0 {
		return 0, math.Inf(1)
	}
	sd := math.Sqrt(v / float64(n))
	lo, hi := eloFromScore(m-1.96*sd), eloFromScore(m+1.96*sd)
	return eloFromScore(m), (hi - lo) / 2
}

// llrFrom is the normal-approximation SPRT log-likelihood ratio for
// H1: elo=elo1 versus H0: elo=elo0 on n samples with the given mean
// score and per-sample variance.
func llrFrom(m, v float64, n int, elo0, elo1 float64) float64 {
	if n < 2 || v == 0 {
		return 0
	}
	s0, s1 := scoreFromElo(elo0), scoreFromElo(elo1)
	return (s1 - s0) * (2*m - s0 - s1) * float64(n) / (2 * v)
}

func (t tally) eloReport() (elo, ci float64) { return eloReportFrom(t.mean(), t.variance(), t.n()) }

// penta counts finished pairs by pair score: index k means the two games
// of the pair gave A k/2 points in total (0, 0.5, 1, 1.5 or 2). Games of
// one pair share an opening and have colors swapped, so their results are
// correlated and the opening's bias largely cancels inside the pair; the
// variance of the per-pair score (not of the per-game score) is what the
// confidence interval and the SPRT must use, and it is smaller.
type penta [5]int

func (p penta) n() int { return p[0] + p[1] + p[2] + p[3] + p[4] }

// mean is A's average per-game score, from the pair counts.
func (p penta) mean() float64 {
	if p.n() == 0 {
		return 0.5
	}
	s := 0.0
	for k, c := range p {
		s += float64(c) * float64(k) / 4
	}
	return s / float64(p.n())
}

// variance of the per-pair score (each pair scored as its two games' mean).
func (p penta) variance() float64 {
	if p.n() == 0 {
		return 0
	}
	m := p.mean()
	s := 0.0
	for k, c := range p {
		d := float64(k)/4 - m
		s += float64(c) * d * d
	}
	return s / float64(p.n())
}

func (p penta) eloReport() (elo, ci float64) { return eloReportFrom(p.mean(), p.variance(), p.n()) }

func (p penta) llr(elo0, elo1 float64) float64 {
	return llrFrom(p.mean(), p.variance(), p.n(), elo0, elo1)
}

// carryState is what -carry persists between runs, so several series (each
// with its own -seed) add up to one result instead of being summed by hand.
type carryState struct {
	Penta penta   `json:"penta"`
	W     int     `json:"w"`
	D     int     `json:"d"`
	L     int     `json:"l"`
	Seeds []int64 `json:"seeds"`
}

func loadCarry(path string) (carryState, error) {
	var c carryState
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(data, &c)
}

func saveCarry(path string, c carryState) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ------------------------------------------------------------------ main

func main() {
	pathA := flag.String("a", "", "path to engine A (the candidate) — a cmd/cli build")
	pathB := flag.String("b", "", "path to engine B (the baseline) — a cmd/cli build")
	movetime := flag.Int("movetime", 100, "time per move in milliseconds")
	pairs := flag.Int("pairs", 50, "number of opening pairs (each pair = 2 games with colors swapped)")
	conc := flag.Int("concurrency", 0, "games played in parallel (0 = NumCPU/2, at least 1); each engine uses -threads threads, keep concurrency*2*threads <= physical cores")
	threads := flag.Int("threads", 1, "search threads per engine")
	maxPlies := flag.Int("maxplies", 400, "adjudicate a draw after this many plies")
	adjWin := flag.Int("adjwin", 1000, "adjudicate a win when both engines score at least this many centipawns for the same side for -adjwinmoves moves each (0 = off)")
	adjWinMoves := flag.Int("adjwinmoves", 3, "moves per side the -adjwin score must hold")
	adjDraw := flag.Int("adjdraw", 0, "adjudicate a draw when both engines stay within ±this many centipawns for -adjdrawmoves moves each, once -adjdrawply plies are played (0 = off)")
	adjDrawMoves := flag.Int("adjdrawmoves", 5, "moves per side the -adjdraw score must hold")
	adjDrawPly := flag.Int("adjdrawply", 80, "no draw adjudication before this many plies")
	optsA := flag.String("aopts", "", "UCI options for engine A as Name=value,Name=value (e.g. RfpMargin=80,LmrBase=90)")
	optsB := flag.String("bopts", "", "UCI options for engine B, same format as -aopts")
	argsA := flag.String("aargs", "cli", "command-line arguments for engine A: \"cli\" = the cmd/cli UCI flags (-uci -book= -depth=64 -threads=N), anything else is split on spaces and passed as is (empty = none, e.g. for Stockfish)")
	argsB := flag.String("bargs", "cli", "command-line arguments for engine B, same format as -aargs")
	openingsPath := flag.String("openings", "", "file with one FEN/EPD per line; if empty, random balanced openings are generated")
	openBook := flag.String("openbook", "", "Polyglot .bin to draw openings from (a weighted random walk of -openplies moves through the book) instead of random moves; ignored with -openings")
	seed := flag.Int64("seed", 1, "seed for generated openings")
	openPlies := flag.Int("openplies", 8, "plies in a generated opening (random moves, or a walk through -openbook)")
	openEval := flag.Int("openeval", 80, "max |static eval| in centipawns for a generated opening")
	elo0 := flag.Float64("elo0", 0, "SPRT H0 Elo (enables SPRT together with -elo1)")
	elo1 := flag.Float64("elo1", 0, "SPRT H1 Elo; 0 with -elo0=0 disables SPRT")
	outPath := flag.String("out", "", "append one line per game (result, reason, moves) to this file")
	carryPath := flag.String("carry", "", "file accumulating results across runs: loaded at start, SPRT/Elo cover all runs together, saved at the end (use a different -seed for every run)")
	flag.Parse()
	adj := adjudication{
		winCP: *adjWin, winMoves: *adjWinMoves,
		drawCP: *adjDraw, drawMoves: *adjDrawMoves, drawPly: *adjDrawPly,
	}

	if *pathA == "" || *pathB == "" {
		fmt.Fprintln(os.Stderr, "usage: match -a candidate.exe -b baseline.exe [-movetime=100] [-pairs=50] [-concurrency=N] [-elo0=0 -elo1=5]")
		os.Exit(2)
	}
	for _, p := range []*string{pathA, pathB} {
		abs, err := filepath.Abs(*p)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		*p = abs
	}
	if *conc <= 0 {
		*conc = runtime.NumCPU() / 2
		if *conc < 1 {
			*conc = 1
		}
	}

	var prior carryState
	if *carryPath != "" {
		var err error
		prior, err = loadCarry(*carryPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "carry:", err)
			os.Exit(1)
		}
		if *openingsPath == "" {
			for _, s := range prior.Seeds {
				if s == *seed {
					fmt.Fprintf(os.Stderr, "warning: -seed=%d was already used in this -carry file; the same openings are replayed, results will be correlated\n", s)
				}
			}
		}
	}

	var starts []string
	if *openingsPath != "" {
		var err error
		starts, err = openings.Read(*openingsPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "openings:", err)
			os.Exit(1)
		}
	} else if *openBook != "" {
		var err error
		starts, err = openings.FromBook(*openBook, *pairs, *openPlies, *openEval, *seed)
		if err != nil {
			fmt.Fprintln(os.Stderr, "openbook:", err)
			os.Exit(1)
		}
	} else {
		starts = openings.Random(*pairs, *openPlies, *openEval, *seed)
	}
	if len(starts) == 0 {
		fmt.Fprintln(os.Stderr, "no openings available")
		os.Exit(1)
	}
	if len(starts) < *pairs {
		fmt.Fprintf(os.Stderr, "warning: only %d distinct openings for %d pairs; openings repeat, pairs are not independent (try more -openplies or a bigger book)\n", len(starts), *pairs)
	}

	var outFile *os.File
	if *outPath != "" {
		var err error
		outFile, err = os.OpenFile(*outPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			fmt.Fprintln(os.Stderr, "out:", err)
			os.Exit(1)
		}
		defer outFile.Close()
	}

	sprtOn := *elo0 != 0 || *elo1 != 0
	const alpha, beta = 0.05, 0.05
	lower, upper := math.Log(beta/(1-alpha)), math.Log((1-beta)/alpha)

	fmt.Printf("A=%s  B=%s  movetime=%dms  threads=%d  pairs=%d  concurrency=%d  openings=%d\n",
		*pathA, *pathB, *movetime, *threads, *pairs, *conc, len(starts))
	if sprtOn {
		fmt.Printf("SPRT: H0 elo=%.1f vs H1 elo=%.1f, alpha=beta=0.05, bounds [%.2f, %.2f]\n", *elo0, *elo1, lower, upper)
	}

	type job struct {
		pair   int
		fen    string
		aWhite bool
	}
	jobs := make(chan job)
	go func() {
		defer close(jobs)
		for p := 0; p < *pairs; p++ {
			fen := starts[p%len(starts)]
			jobs <- job{p, fen, true}
			jobs <- job{p, fen, false}
		}
	}()

	var (
		mu       sync.Mutex
		total    = tally{prior.W, prior.D, prior.L} // from A's point of view
		pairStat = prior.Penta                      // finished pairs, all runs
		pending  = map[int]float64{}                // pair -> A's score in its first finished game
		aWhiteT  tally
		aBlackT  tally
		stopped  bool
		verdict  string
		games    int
	)
	if prior.Penta.n() > 0 {
		elo, ci := pairStat.eloReport()
		fmt.Printf("carry: %d pairs from earlier runs, Elo %+.1f ± %.1f\n", pairStat.n(), elo, ci)
	}
	cliArgs := []string{"-uci", "-book=", "-depth=64", "-threads=" + strconv.Itoa(*threads)}
	argsFor := func(spec string) []string {
		if spec == "cli" {
			return cliArgs
		}
		return strings.Fields(spec)
	}
	argvA, argvB := argsFor(*argsA), argsFor(*argsB)

	var wg sync.WaitGroup
	for w := 0; w < *conc; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := startEngine(*pathA, argvA)
			if err != nil {
				fmt.Fprintln(os.Stderr, "engine A:", err)
				os.Exit(1)
			}
			defer a.quit()
			a.setOptions(*optsA)
			b, err := startEngine(*pathB, argvB)
			if err != nil {
				fmt.Fprintln(os.Stderr, "engine B:", err)
				os.Exit(1)
			}
			defer b.quit()
			b.setOptions(*optsB)

			for j := range jobs {
				mu.Lock()
				s := stopped
				mu.Unlock()
				if s {
					continue // drain
				}

				white, black := a, b
				if !j.aWhite {
					white, black = b, a
				}
				out := playGame(white, black, j.fen, *movetime, *maxPlies, adj)

				// Result from A's point of view.
				aScore := 0.5
				switch {
				case out.res == whiteWins && j.aWhite, out.res == blackWins && !j.aWhite:
					aScore = 1
				case out.res == draw:
					aScore = 0.5
				default:
					aScore = 0
				}

				mu.Lock()
				games++
				add := func(t *tally) {
					switch aScore {
					case 1:
						t.w++
					case 0.5:
						t.d++
					default:
						t.l++
					}
				}
				add(&total)
				if j.aWhite {
					add(&aWhiteT)
				} else {
					add(&aBlackT)
				}
				// A pair counts once both of its games are in; the first
				// finisher waits here for its partner.
				if first, ok := pending[j.pair]; ok {
					pairStat[int(2*(first+aScore))]++
					delete(pending, j.pair)
				} else {
					pending[j.pair] = aScore
				}
				elo, ci := pairStat.eloReport()
				line := fmt.Sprintf("game %3d  pair %3d  A=%-5s  %-3s (%s, %d plies)   total +%d =%d -%d   Elo %+.1f ± %.1f",
					games, j.pair, map[bool]string{true: "white", false: "black"}[j.aWhite],
					map[float64]string{1: "1-0", 0.5: "1/2", 0: "0-1"}[aScore], out.reason, len(out.moves),
					total.w, total.d, total.l, elo, ci)
				if sprtOn {
					l := pairStat.llr(*elo0, *elo1)
					line += fmt.Sprintf("   LLR %.2f", l)
					if l >= upper {
						stopped, verdict = true, "H1 accepted: A is stronger (Elo >= elo1 likely)"
					} else if l <= lower {
						stopped, verdict = true, "H0 accepted: no gain of elo1 demonstrated"
					}
				}
				fmt.Println(line)
				if outFile != nil {
					fmt.Fprintf(outFile, "%s|A_white=%v|score=%.1f|%s|%s|%s\n", time.Now().Format(time.RFC3339), j.aWhite, aScore, out.reason, j.fen, strings.Join(out.moves, " "))
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if *carryPath != "" {
		state := carryState{Penta: pairStat, W: total.w, D: total.d, L: total.l, Seeds: prior.Seeds}
		if *openingsPath == "" {
			state.Seeds = append(state.Seeds, *seed)
		}
		if err := saveCarry(*carryPath, state); err != nil {
			fmt.Fprintln(os.Stderr, "carry: save failed:", err)
		}
	}

	elo, ci := pairStat.eloReport()
	gameElo, gameCI := total.eloReport()
	fmt.Println()
	fmt.Printf("Games:  %d   A: +%d =%d -%d   score %.1f%%\n", total.n(), total.w, total.d, total.l, 100*total.mean())
	fmt.Printf("  as white: +%d =%d -%d     as black: +%d =%d -%d   (this run only)\n", aWhiteT.w, aWhiteT.d, aWhiteT.l, aBlackT.w, aBlackT.d, aBlackT.l)
	fmt.Printf("Pairs:  %d   0:%d  0.5:%d  1:%d  1.5:%d  2:%d\n", pairStat.n(), pairStat[0], pairStat[1], pairStat[2], pairStat[3], pairStat[4])
	fmt.Printf("Elo (A - B): %+.1f ± %.1f (95%%, pairs)   [per-game estimate: %+.1f ± %.1f]\n", elo, ci, gameElo, gameCI)
	if sprtOn {
		if verdict == "" {
			verdict = "inconclusive (ran out of pairs before a bound was crossed)"
		}
		fmt.Printf("SPRT: LLR %.2f in [%.2f, %.2f] — %s\n", pairStat.llr(*elo0, *elo1), lower, upper, verdict)
	}
}

// setOptions sends "Name=value,Name=value" as UCI setoption commands.
func (e *engine) setOptions(opts string) {
	for _, kv := range strings.Split(opts, ",") {
		if name, value, ok := strings.Cut(strings.TrimSpace(kv), "="); ok && name != "" {
			e.send("setoption name " + name + " value " + value)
		}
	}
}

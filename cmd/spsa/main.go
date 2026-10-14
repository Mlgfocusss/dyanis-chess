// Command spsa tunes the engine's search parameters (search.Tunables) by
// SPSA: each iteration perturbs every parameter at once by ±c, plays a short
// match between the "plus" and "minus" settings (cmd/match, same engine
// binary on both sides, different -aopts/-bopts) and moves the parameters
// towards whichever side scored better.
//
//	go build -o spsa.exe ./cmd/spsa
//	./spsa.exe -cli=baseline.exe -match=match.exe -iters=1000 -state=spsa.json
//
// State (parameters, iteration, running average) is written after every
// iteration; rerun the same command to resume. Stop it any time with
// Ctrl-C or by killing the process. The result is a SUGGESTION: always
// confirm it with a normal cmd/match run against the unmodified baseline.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"dyanis-chess/internal/search"
)

type state struct {
	Iter  int                `json:"iter"`
	Theta map[string]float64 `json:"theta"`
	// Sum/N is the running sum of theta over the second half of the run,
	// the usual low-noise estimate of the final value.
	Sum map[string]float64 `json:"sum"`
	N   int                `json:"n"`
}

var gamesRe = regexp.MustCompile(`Games:\s+(\d+)\s+A: \+(\d+) =(\d+) -(\d+)`)

func main() {
	cli := flag.String("cli", "baseline.exe", "engine binary played on both sides (cmd/cli build)")
	matchExe := flag.String("match", "match.exe", "cmd/match binary")
	iters := flag.Int("iters", 1000, "total iterations")
	pairs := flag.Int("pairs", 16, "opening pairs per iteration (2 games each)")
	movetime := flag.Int("movetime", 100, "ms per move")
	conc := flag.Int("concurrency", 16, "parallel games")
	book := flag.String("openbook", "assets/gm2001.bin", "Polyglot book the openings are drawn from")
	statePath := flag.String("state", "spsa.json", "state file (resumed if it exists)")
	lr := flag.Float64("lr", 1.0, "learning rate (step is lr*score*c per iteration, decays over the run)")
	cFrac := flag.Float64("c", 0.08, "perturbation size as a fraction of the default (at least 1)")
	only := flag.String("params", "", "comma-separated parameter names to tune (default: all)")
	evalFile := flag.String("evalfile", "", "NNUE weights file loaded by both sides (UCI option EvalFile); default: hand-written eval")
	flag.Parse()
	for _, p := range []*string{cli, matchExe, book} {
		if abs, err := filepath.Abs(*p); err == nil {
			*p = abs
		}
	}

	var params []search.Tunable
	want := map[string]bool{}
	for _, n := range strings.Split(*only, ",") {
		if n = strings.TrimSpace(n); n != "" {
			want[n] = true
		}
	}
	for _, t := range search.Tunables() {
		if len(want) == 0 || want[t.Name] {
			params = append(params, t)
		}
	}
	if len(params) == 0 {
		fmt.Fprintln(os.Stderr, "no parameters selected")
		os.Exit(1)
	}

	st := state{Theta: map[string]float64{}, Sum: map[string]float64{}}
	if data, err := os.ReadFile(*statePath); err == nil {
		if err := json.Unmarshal(data, &st); err != nil {
			fmt.Fprintln(os.Stderr, "bad state file:", err)
			os.Exit(1)
		}
		fmt.Printf("resuming at iteration %d\n", st.Iter)
	}
	for _, p := range params {
		if _, ok := st.Theta[p.Name]; !ok {
			st.Theta[p.Name] = float64(p.Default)
		}
	}

	clamp := func(p search.Tunable, v float64) int {
		r := int(math.Round(v))
		if r < p.Min {
			r = p.Min
		}
		if r > p.Max {
			r = p.Max
		}
		return r
	}
	opts := func(vals map[string]int) string {
		var parts []string
		for _, p := range params {
			parts = append(parts, p.Name+"="+strconv.Itoa(vals[p.Name]))
		}
		if *evalFile != "" {
			parts = append([]string{"EvalFile=" + *evalFile}, parts...)
		}
		return strings.Join(parts, ",")
	}

	decayA := float64(*iters) / 10
	failures := 0
	for st.Iter < *iters {
		k := st.Iter
		rng := rand.New(rand.NewSource(int64(k) + 1))
		ck := 1 / math.Pow(float64(k+1), 0.101)
		lrk := *lr / math.Pow(1+float64(k)/decayA, 0.602)

		plus, minus := map[string]int{}, map[string]int{}
		delta := map[string]float64{}
		cs := map[string]float64{}
		for _, p := range params {
			c := math.Max(1, *cFrac*float64(p.Default))
			d := float64(rng.Intn(2)*2 - 1)
			delta[p.Name], cs[p.Name] = d, c
			plus[p.Name] = clamp(p, st.Theta[p.Name]+c*ck*d)
			minus[p.Name] = clamp(p, st.Theta[p.Name]-c*ck*d)
		}

		start := time.Now()
		cmd := exec.Command(*matchExe,
			"-a", *cli, "-b", *cli,
			"-aopts", opts(plus), "-bopts", opts(minus),
			"-movetime="+strconv.Itoa(*movetime),
			"-pairs="+strconv.Itoa(*pairs),
			"-concurrency="+strconv.Itoa(*conc),
			"-openbook="+*book,
			"-seed="+strconv.Itoa(k+1),
		)
		out, err := cmd.Output()
		m := gamesRe.FindSubmatch(out)
		if err != nil || m == nil {
			fmt.Fprintf(os.Stderr, "iteration %d: match failed (%v), retrying in 5s\n", k, err)
			if failures++; failures >= 5 {
				fmt.Fprintln(os.Stderr, "5 failures in a row, giving up")
				os.Exit(1)
			}
			time.Sleep(5 * time.Second)
			continue
		}
		failures = 0
		n, _ := strconv.Atoi(string(m[1]))
		w, _ := strconv.Atoi(string(m[2]))
		l, _ := strconv.Atoi(string(m[4]))
		r := float64(w-l) / float64(n) // plus side's score in [-1, 1]

		for _, p := range params {
			// Move towards the side that won; steps are scaled by c so every
			// parameter moves in proportion to its own size.
			st.Theta[p.Name] += lrk * r * delta[p.Name] * cs[p.Name]
			lo, hi := float64(p.Min), float64(p.Max)
			st.Theta[p.Name] = math.Min(hi, math.Max(lo, st.Theta[p.Name]))
		}
		st.Iter++
		if st.Iter > *iters/2 {
			for _, p := range params {
				st.Sum[p.Name] += st.Theta[p.Name]
			}
			st.N++
		}
		save(*statePath, &st)

		fmt.Printf("iter %4d  r=%+.2f  %4.1fs\n", st.Iter, r, time.Since(start).Seconds())
		if st.Iter%10 == 0 || st.Iter == *iters {
			report(params, &st)
		}
	}
	fmt.Println("done")
	report(params, &st)
}

func save(path string, st *state) {
	data, _ := json.MarshalIndent(st, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err == nil {
		os.Rename(tmp, path)
	}
}

// report prints current and (if available) second-half-average values as
// ready-to-use -aopts text.
func report(params []search.Tunable, st *state) {
	names := make([]string, 0, len(params))
	for _, p := range params {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	var cur, avg []string
	for _, n := range names {
		cur = append(cur, fmt.Sprintf("%s=%d", n, int(math.Round(st.Theta[n]))))
		if st.N > 0 {
			avg = append(avg, fmt.Sprintf("%s=%d", n, int(math.Round(st.Sum[n]/float64(st.N)))))
		}
	}
	fmt.Println("  current:", strings.Join(cur, ","))
	if len(avg) > 0 {
		fmt.Println("  average:", strings.Join(avg, ","))
	}
}

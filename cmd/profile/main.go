// Throwaway profiling harness — not part of the engine. By default
// runs BestMoveTimedMT(threads=1) — plain single-threaded iterative
// deepening — from the starting position under Go's CPU profiler for
// a fixed wall-clock -budget (default 5s), reporting how deep it got.
//
// Usage, from the module root:
//
//	go run ./cmd/profile
//	go tool pprof -top cpu.prof
//
// Add -threads=N to measure Lazy SMP instead (plan.md item 3) at the
// SAME -budget — this is the right comparison for a scaling question
// like "does adding threads help", where "fewer nodes for the same
// depth" (the metric every earlier pruning step in plan.md was judged
// by) isn't the point at all; getting DEEPER in the same wall-clock
// time is. -threads=1 and -threads=N MUST use the same -budget for
// the comparison to mean anything — both flags apply uniformly here
// regardless of thread count, there's no separate fixed-depth mode to
// accidentally compare against a time-limited one:
//
//	go run ./cmd/profile -threads=1 -budget=5s
//	go run ./cmd/profile -threads=4 -budget=5s
//
// Note the wall-clock time actually taken can run noticeably over
// -budget, more so with more threads: the deadline is only checked
// BETWEEN completed depths, never during one, and a multi-threaded
// run waits for the SLOWEST thread to finish its current depth — see
// runIterativeDeepening's and BestMoveTimedMT's own doc comments in
// internal/search/search.go.
//
// Add -fen="..." to profile a different position instead of the
// starting one — useful for anything that only fires in sharp
// tactical positions or in endgames rather than a quiet, balanced
// opening tree:
//
//	go run ./cmd/profile -fen="r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1" -budget=15s
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/pprof"
	"time"

	"dyanis-chess/internal/board"
	"dyanis-chess/internal/nnue"
	"dyanis-chess/internal/search"
)

func main() {
	threads := flag.Int("threads", 1, "search threads (Lazy SMP, plan.md item 3)")
	budget := flag.Duration("budget", 5*time.Second, "wall-clock time budget")
	fen := flag.String("fen", board.StartFEN, "FEN of the position to search instead of the starting one")
	out := flag.String("out", "cpu.prof", "CPU profile output path")
	evalFile := flag.String("evalfile", "", "NNUE weights to evaluate with instead of the hand-written eval (see NNUE.md)")
	flag.Parse()

	if *evalFile != "" {
		n, err := nnue.Load(*evalFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "evalfile:", err)
			os.Exit(1)
		}
		search.SetNetwork(n)
	}

	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "creating profile output:", err)
		os.Exit(1)
	}
	defer f.Close()

	if err := pprof.StartCPUProfile(f); err != nil {
		fmt.Fprintln(os.Stderr, "starting CPU profile:", err)
		os.Exit(1)
	}
	defer pprof.StopCPUProfile()

	b, err := board.FromFEN(*fen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid -fen:", err)
		os.Exit(1)
	}
	var lastInfo search.SearchInfo
	var lastAt time.Duration
	start := time.Now()

	// maxDepth here is just a ceiling this run never expects to
	// actually hit within a few seconds of budget — *budget is what
	// actually stops the search.
	move, searchErr := search.BestMoveTimedMT(b, 64, *budget, *threads, func(info search.SearchInfo) {
		lastInfo = info
		lastAt = time.Since(start)
		fmt.Printf("  depth %2d: nodes %10d  score %d\n", info.Depth, info.Nodes, info.Score)
	})
	elapsed := time.Since(start)
	if searchErr != nil {
		fmt.Fprintln(os.Stderr, "search error:", searchErr)
		os.Exit(1)
	}

	fmt.Printf("BestMoveTimedMT(threads=%d, budget=%s) from %q: %s (%s)\n", *threads, *budget, *fen, move.String(), elapsed)
	fmt.Printf("depth reached: %d  nodes: %d  score: %d\n", lastInfo.Depth, lastInfo.Nodes, lastInfo.Score)
	if lastAt > 0 {
		fmt.Printf("nodes per second (up to the last completed depth): %.0f\n", float64(lastInfo.Nodes)/lastAt.Seconds())
	}
	fmt.Printf("profile written to %s — now run: go tool pprof -top %s\n", *out, *out)
}

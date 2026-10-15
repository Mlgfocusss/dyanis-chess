// Command analyze reviews chess games from a PGN file with the engine
// and prints, for every move, its evaluation and quality class, plus
// the accuracy of each player. It is the command-line precursor of the
// web "game review" page (see WEB-PLAN.md): the same backend/internal/analysis
// package will serve both.
//
//	analyze -pgn game.pgn -evalfile net-kingf-m4.bin -movetime 300
//	analyze -pgn game.pgn -depth 12 -json > report.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"dyanis-chess/backend/internal/analysis"
	"dyanis-chess/backend/internal/pgn"
	"dyanis-chess/engine"
)

func main() {
	pgnPath := flag.String("pgn", "", "PGN file to analyze, or - for stdin")
	gameNo := flag.Int("game", 1, "which game of the file to analyze (1-based); 0 = all")
	moveTime := flag.Int("movetime", 300, "milliseconds of search per position")
	depth := flag.Int("depth", 0, "if > 0, search every position to this fixed depth instead of by time (reproducible)")
	workers := flag.Int("workers", 0, "positions searched in parallel; 0 = number of CPUs")
	hash := flag.Int("hash", 16, "transposition table size in MB per search")
	evalFile := flag.String("evalfile", "", "NNUE weights file; empty = hand-written eval")
	winK := flag.Float64("wink", analysis.DefaultWinK, "scale K of the win-probability curve 1/(1+exp(-cp/K))")
	asJSON := flag.Bool("json", false, "print the report as JSON instead of text")
	flag.Parse()

	if *pgnPath == "" {
		fmt.Fprintln(os.Stderr, "usage: analyze -pgn file.pgn [-evalfile net.bin] [-movetime ms | -depth N] [-json]")
		os.Exit(2)
	}
	if *evalFile != "" {
		if err := engine.LoadNetwork(*evalFile); err != nil {
			fatalf("evalfile: %v", err)
		}
	}

	var r io.Reader = os.Stdin
	if *pgnPath != "-" {
		f, err := os.Open(*pgnPath)
		if err != nil {
			fatalf("%v", err)
		}
		defer f.Close()
		r = f
	}
	games, err := pgn.Parse(r)
	if err != nil {
		fatalf("%v", err)
	}
	if len(games) == 0 {
		fatalf("no games in %s", *pgnPath)
	}
	if *gameNo > len(games) {
		fatalf("file has only %d game(s)", len(games))
	}
	sel := games
	if *gameNo > 0 {
		sel = games[*gameNo-1 : *gameNo]
	}

	var reports []*analysis.Report
	for gi, g := range sel {
		opt := analysis.Options{
			MoveTime: time.Duration(*moveTime) * time.Millisecond,
			Depth:    *depth,
			Workers:  *workers,
			HashMB:   *hash,
			WinK:     *winK,
		}
		if !*asJSON {
			opt.Progress = func(done, total int) {
				fmt.Fprintf(os.Stderr, "\r  positions %d/%d", done, total)
			}
		}
		t0 := time.Now()
		rep, err := analysis.Analyze(g, opt)
		if !*asJSON {
			fmt.Fprint(os.Stderr, "\r                          \r")
		}
		if err != nil {
			fatalf("game %d: %v", gi+1, err)
		}
		if *asJSON {
			reports = append(reports, rep)
			continue
		}
		printReport(os.Stdout, g, rep, time.Since(t0))
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		var out any = reports
		if len(reports) == 1 {
			out = reports[0]
		}
		if err := enc.Encode(out); err != nil {
			fatalf("%v", err)
		}
	}
}

func printReport(w io.Writer, g *pgn.Game, rep *analysis.Report, took time.Duration) {
	fmt.Fprintf(w, "%s — %s   %s   (%s)\n", orQ(g.Tag("White")), orQ(g.Tag("Black")), g.Result, g.Tag("Date"))
	fmt.Fprintf(w, "analysed %d positions in %.1fs\n", len(rep.Evals), took.Seconds())
	if rep.Opening != nil {
		fmt.Fprintf(w, "opening: %s %s (theory through ply %d)\n", rep.Opening.ECO, rep.Opening.Name, rep.BookPlies)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%4s  %-11s %-8s %8s  %-10s %6s  %s\n", "ply", "move", "eval", "win%", "class", "loss", "better was")
	for i, m := range rep.Moves {
		num := fmt.Sprintf("%d.", i/2+1)
		if m.Color == "black" {
			num += ".."
		}
		after := rep.Evals[i+1]
		note := ""
		if m.BestSAN != "" && m.Class != analysis.Excellent && m.Class != analysis.Good {
			note = m.BestSAN
		}
		if m.Opening != nil {
			note = m.Opening.Name
		}
		fmt.Fprintf(w, "%4d  %-11s %-8s %7.1f%%  %-10s %6.1f  %s\n",
			m.Ply, num+m.SAN, fmtScore(after), after.WinPct, m.Class, m.Loss, note)
	}
	fmt.Fprintln(w)
	for c, name := range []string{"White", "Black"} {
		fmt.Fprintf(w, "%-5s accuracy %5.1f   book %d  best %d  excellent %d  good %d  inaccuracies %d  mistakes %d  blunders %d\n",
			name, rep.Accuracy[c],
			rep.Counts[c][analysis.Book], rep.Counts[c][analysis.Best], rep.Counts[c][analysis.Excellent], rep.Counts[c][analysis.Good],
			rep.Counts[c][analysis.Inaccuracy], rep.Counts[c][analysis.Mistake], rep.Counts[c][analysis.Blunder])
	}
	if len(rep.Key) > 0 {
		fmt.Fprint(w, "key moments (ply):")
		for _, p := range rep.Key {
			m := rep.Moves[p-1]
			fmt.Fprintf(w, "  %d %s (%s, -%.0f%%)", p, m.SAN, m.Class, m.Loss)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, strings.Repeat("-", 60))
}

// fmtScore shows an evaluation from White's point of view: "+0.35",
// "-1.20", "M3" (White mates in 3), "-M2" (Black mates in 2).
func fmtScore(e analysis.PositionEval) string {
	if e.Terminal != "" {
		return e.Terminal
	}
	if e.Mate != 0 {
		if e.Mate > 0 {
			return fmt.Sprintf("M%d", e.Mate)
		}
		return fmt.Sprintf("-M%d", -e.Mate)
	}
	return fmt.Sprintf("%+.2f", float64(e.Score)/100)
}

func orQ(s string) string {
	if s == "" {
		return "?"
	}
	return s
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "analyze: "+format+"\n", a...)
	os.Exit(1)
}

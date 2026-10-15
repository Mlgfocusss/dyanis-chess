// cmd/tune implements a Texel-tuning measurement and coordinate-
// descent harness for internal/eval's weights.
//
// Scope has grown, and shrunk once, across five stages (see plan.md,
// "тюнинг эвала"): material+PST first (325 parameters: 5 material
// values + 5 PST tables of 64 cells), then mobility/pawn-structure/
// king-safety (15 more: 4 mobility weights, 2 pawn-structure
// penalties + 6 usable passed-pawn-bonus ranks, 3 king-safety
// weights), then King PST (128 more: kingMidgamePST + kingEndgamePST,
// 64 cells each), then bishop pair (1 more) — but King PST came back
// OUT after that: two separate runs, starting from the same original
// hand-authored tables, converged to the same extreme, incoherent
// pattern (single cells swinging by 100+ centipawns with no spatial
// coherence) — sparse per-square support for the king (it only
// occupies a handful of squares in any real midgame) is exactly the
// kind of thing unregularized coordinate descent overfits, and two
// runs reproducing the same overfit shape settled that it wasn't a
// one-off fluke. So King PST is fully out of TuneParams now, not
// merely "tuned but not pasted" — 341 total parameters (325 + 15 + 1).
// Mop-up is the one other, deliberate holdout; see TuneParams' own
// doc comment in tunable.go for why.
//
// Four modes:
//
//	-mode=findk   calibrates K, the sigmoid scaling constant, so that
//	              sigmoid(K*score/400) best predicts game results at
//	              the CURRENT eval weights. This is calibrating the
//	              error metric itself, not tuning the engine — the
//	              same way you'd zero a scale before weighing
//	              anything on it.
//	-mode=error   reports the MSE between sigmoid(K*score/400) and
//	              the actual game result across the whole dataset, at
//	              a given K, with the eval exactly as it is today.
//	              This is the number every future weight change gets
//	              compared against — the eval-strength analogue of
//	              cmd/profile's node counts for search speed.
//	-mode=verify  checks a performance optimization this file makes
//	              on top of the two above: the score is split into
//	              five weight-dependent Compute* calls (material+PST,
//	              mobility, pawn structure, king safety, bishop pair)
//	              plus eval.NonMaterialScore, cached once per position
//	              at load time (king-PST-term + mop-up — the parts
//	              that AREN'T tunable). Confirms that split is
//	              actually safe by comparing it against a full,
//	              honest recompute at DELIBERATELY SHIFTED weights
//	              across every group, not just at today's defaults
//	              (where cache and honest recompute would trivially
//	              agree even if the split were wrong).
//	-mode=tune    the actual coordinate descent: perturbs all 341
//	              parameters one at a time, in a fixed order
//	              (material, then PST tables pawn->knight->bishop->
//	              rook->queen, then mobility, then pawn structure,
//	              then king safety, then bishop pair), keeping
//	              whichever of +step/-step lowers TRAIN-set MSE
//	              (held-out MSE is measured every epoch too, purely
//	              to watch for overfitting — it never influences
//	              which direction a parameter
//	              moves). step starts at -step and halves whenever a
//	              full epoch improves nothing, down to a floor of 1
//	              (not lower — see tune.go's own top comment for why
//	              sub-1 steps are pointless on int-valued weights);
//	              the run ends when an epoch AT step=1 also improves
//	              nothing. Every epoch's progress is checkpointed to
//	              -checkpoint, and -resume picks a broken-off run
//	              back up from there. This mode NEVER writes to the
//	              production eval files itself — it prints the final
//	              weights as Go literals for a human to paste in by
//	              hand (see tune.go's printLiterals), so that change
//	              stays its own visible, reviewable step, same as
//	              everywhere else in this project.
//
// Usage:
//
//	go run ./cmd/tune -mode=findk
//	go run ./cmd/tune -mode=error -k=1.504328
//	go run ./cmd/tune -mode=verify
//	go run ./cmd/tune -mode=tune -k=1.504328
//	go run ./cmd/tune -mode=tune -k=1.504328 -resume
//	go run ./cmd/tune -epd=some/other/set.epd -mode=findk
//
// NOTE for whoever builds this: internal/board and internal/eval are
// imported below under the module path used throughout this repo's
// own test files (dyanis-chess/...). If
// go.mod's actual module line differs, these two import paths are the
// only thing that needs adjusting — nothing else in this file assumes
// anything about the module's name.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"sync"

	"dyanis-chess/engine/internal/board"
	"dyanis-chess/engine/internal/eval"
)

// sample is one quiet-labeled.epd line: a position plus the game's
// eventual result, White-relative (1.0 White won, 0.0 Black won, 0.5
// draw) — the same convention the c9 opcode itself uses
// ("1-0"/"0-1"/"1/2-1/2").
//
// restScore is computed once, at load time, and cached: it doesn't
// depend on the weights a coordinate-descent tuner probes (it's
// eval.NonMaterialScore — king-PST-term + mop-up; king PST was
// briefly tuned too, then removed from scope entirely after two runs
// reproduced the same overfit pattern on it — see NonMaterialScore's
// own doc comment in tunable.go for the full story. Mobility/pawn-
// structure/king-safety and bishop pair moved out of restScore in
// turn as each joined the tuned scope, and stayed out — see
// whiteScore below and this file's "-mode=verify", which checks the
// split empirically rather than assuming it from reading the eval
// package's source).
type sample struct {
	b         *board.Board
	result    float64
	restScore int
}

// loadEPD reads quiet-labeled.epd's line format:
//
//	<placement> <stm> <castling> <ep> c9 "<result>";
//
// board.FromFEN only requires the first 4 whitespace-separated
// fields (halfmove clock/fullmove number default to 0/1 when absent
// — see fen.go), so the EPD's 4-field position prefix parses as-is
// once the trailing "c9 ...;" is cut off. That cut, and turning the
// result string into a White-relative float, are this function's
// only real job.
//
// Lines that fail to parse (bad FEN, missing/unrecognized c9 value)
// are counted and logged, never silently dropped — a silently-dropped
// slice of the dataset would bias the baseline error without anyone
// noticing, which is exactly the kind of quiet failure the project's
// own discipline (plan.md) says to avoid.
func loadEPD(path string) (samples []sample, badLines int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		idx := strings.Index(line, "c9 ")
		if idx == -1 {
			badLines++
			log.Printf("line %d: no c9 opcode, skipping: %q", lineNo, line)
			continue
		}
		posPart := strings.TrimSpace(line[:idx])
		resultPart := strings.TrimSpace(line[idx+len("c9 "):])
		resultPart = strings.Trim(resultPart, `";`)

		var result float64
		switch resultPart {
		case "1-0":
			result = 1.0
		case "0-1":
			result = 0.0
		case "1/2-1/2":
			result = 0.5
		default:
			badLines++
			log.Printf("line %d: unrecognized result %q, skipping", lineNo, resultPart)
			continue
		}

		b, err := board.FromFEN(posPart)
		if err != nil {
			badLines++
			log.Printf("line %d: FEN parse failed (%v), skipping: %q", lineNo, err, posPart)
			continue
		}

		samples = append(samples, sample{
			b:         b,
			result:    result,
			restScore: eval.NonMaterialScore(b),
		})
	}
	if err := sc.Err(); err != nil {
		return samples, badLines, err
	}
	return samples, badLines, nil
}

// splitTrainHeldout partitions samples 80/20, deterministically: every
// 5th position (index%5==4) goes to held-out, the rest to train. No
// RNG on purpose — reproducible across runs/machines/Go versions
// without needing to pin a seed, and quiet-labeled.epd's own line
// order isn't sorted by anything a fixed stride could accidentally
// correlate with (game outcome, opening, etc.), so this is as good as
// a shuffled split here without the reproducibility risk one brings.
func splitTrainHeldout(samples []sample) (train, heldout []sample) {
	for i, s := range samples {
		if i%5 == 4 {
			heldout = append(heldout, s)
		} else {
			train = append(train, s)
		}
	}
	return train, heldout
}

// whiteScore is the hot-loop score a coordinate-descent tuner will
// call thousands of times per probed parameter: the five
// weight-dependent pieces (ComputeMaterialPST, ComputeMobilityScore,
// ComputePawnStructureScore, ComputeKingSafetyScore,
// ComputeBishopPairScore) plus s.restScore (king-PST-term + mop-up —
// see NonMaterialScore's own doc comment in tunable.go), cached once
// at load time — never a full eval.Evaluate-style recompute. Already White-relative
// by construction (every Compute*/NonMaterialScore function is
// White-minus-Black, unflipped for side to move), matching
// quiet-labeled's own "1-0" == White won convention — no sign flip to
// apply here.
func whiteScore(s sample, params eval.TuneParams) float64 {
	score := eval.ComputeMaterialPST(s.b, params.MaterialPST)
	score += eval.ComputeMobilityScore(s.b, params.Mobility)

	whitePawns := s.b.Pieces(board.Pawn, board.White)
	blackPawns := s.b.Pieces(board.Pawn, board.Black)
	score += eval.ComputePawnStructureScore(whitePawns, blackPawns, params.Pawns)

	score += eval.ComputeKingSafetyScore(s.b, params.KingSafety)
	score += eval.ComputeBishopPairScore(s.b, params.BishopPair)
	score += s.restScore
	return float64(score)
}

// whiteScoreFull is the honest, non-cached counterpart: a complete
// eval.EvaluateWithTuneParams recompute, flipped back to White's
// perspective. Used ONLY by -mode=verify below — nowhere in the hot
// loop — specifically so the cached path above can be checked against
// it at shifted weights across EVERY group, not just at params ==
// eval.DefaultTuneParams (which the cache trivially agrees with
// regardless of whether the split is actually safe).
func whiteScoreFull(b *board.Board, params eval.TuneParams) float64 {
	s := eval.EvaluateWithTuneParams(b, params)
	if b.SideToMove == board.Black {
		s = -s
	}
	return float64(s)
}

// sigmoid is Texel tuning's standard error model: the classic
// logistic curve, base 10 and /400 scaling — the conventional way
// centipawns relate to win probability (e.g. a ~400cp edge ~= 10x win
// odds). K is the one free parameter; see findK.
func sigmoid(k, score float64) float64 {
	return 1.0 / (1.0 + math.Pow(10, -k*score/400.0))
}

// meanSquaredError is the tuning objective: how well
// sigmoid(k*whiteScore(b)) predicts the actual game result, averaged
// over every sample. Lower is better. This is the one number every
// future material+PST change gets compared against.
//
// threads<=1 (the default everywhere in this file) takes the plain
// single-threaded path — bit-for-bit identical to every MSE this tool
// has ever printed, including the recorded baseline (0.064826 at
// K=1.504328). threads>1 splits the samples into that many
// contiguous chunks, sums each chunk's squared error in its own
// goroutine, and adds the partial sums together in FIXED index order
// (chunk 0, then 1, then 2, ...) — never in whichever order the
// goroutines happen to finish. That fixed order is what makes a given
// threads count reproducible run to run; it does NOT make the result
// bit-identical to threads=1, because float64 addition isn't
// associative — summing the same numbers in a different grouping
// gives a microscopically different result (differs many decimal
// places out, far below anything that could flip an accept/reject
// decision in coordinateDescent, but not literally the same float).
func meanSquaredError(samples []sample, k float64, params eval.TuneParams, threads int) float64 {
	if threads <= 1 || len(samples) < threads {
		return sumSquaredError(samples, k, params) / float64(len(samples))
	}

	chunk := (len(samples) + threads - 1) / threads
	partials := make([]float64, threads)
	var wg sync.WaitGroup
	for t := 0; t < threads; t++ {
		start := t * chunk
		if start >= len(samples) {
			break
		}
		end := start + chunk
		if end > len(samples) {
			end = len(samples)
		}
		wg.Add(1)
		go func(t, start, end int) {
			defer wg.Done()
			partials[t] = sumSquaredError(samples[start:end], k, params)
		}(t, start, end)
	}
	wg.Wait()

	sum := 0.0
	for _, p := range partials { // fixed order: index 0..threads-1
		sum += p
	}
	return sum / float64(len(samples))
}

// sumSquaredError is meanSquaredError's inner loop, factored out so
// both the single-threaded path and each per-chunk goroutine call the
// exact same code — no separate "fast path" logic that could drift
// out of sync with the real one.
func sumSquaredError(samples []sample, k float64, params eval.TuneParams) float64 {
	sum := 0.0
	for _, s := range samples {
		pred := sigmoid(k, whiteScore(s, params))
		diff := s.result - pred
		sum += diff * diff
	}
	return sum
}

// findK does a plain coarse-to-fine grid search for the K that
// minimizes meanSquaredError — not a derivative-based method. This
// runs exactly once, ever, per (dataset, eval-weights) pair, so a
// slower but obviously-correct search beats shaving runtime here.
// Each pass narrows the window around the best point the previous,
// coarser pass found.
func findK(samples []sample, params eval.TuneParams, threads int) float64 {
	lo, hi := 0.0, 2.0
	best := lo
	for pass := 0; pass < 8; pass++ {
		bestErr := math.MaxFloat64
		step := (hi - lo) / 20
		for k := lo; k <= hi; k += step {
			e := meanSquaredError(samples, k, params, threads)
			if e < bestErr {
				bestErr = e
				best = k
			}
		}
		lo, hi = best-step, best+step
		if lo < 0 {
			lo = 0
		}
	}
	return best
}

// shiftedParams returns eval.DefaultTuneParams with several weights
// moved substantially across EVERY remaining group — material values,
// a handful of PST cells, every mobility weight, both pawn-structure
// penalties plus two passed-bonus ranks, all three king-safety
// weights, and bishop pair — so this exercises every field
// EvaluateWithTuneParams' five Compute* calls actually read. Values
// are deliberately large and asymmetric (not round, not proportional
// to each other) specifically so a hidden dependency elsewhere in
// restScore would have to produce an equally odd, coincidental match
// to slip through undetected.
func shiftedParams() eval.TuneParams {
	p := eval.DefaultTuneParams

	p.MaterialPST.PawnValue += 53
	p.MaterialPST.KnightValue -= 137
	p.MaterialPST.BishopValue += 211
	p.MaterialPST.RookValue -= 89
	p.MaterialPST.QueenValue += 173
	p.MaterialPST.PawnPST[27] += 61    // d4-ish square, midboard
	p.MaterialPST.KnightPST[18] -= 97  // c3-ish
	p.MaterialPST.BishopPST[42] += 149 // c6-ish
	p.MaterialPST.RookPST[6] -= 71     // g1-ish
	p.MaterialPST.QueenPST[35] += 83   // d5-ish

	p.Mobility.Knight += 7
	p.Mobility.Bishop -= 5
	p.Mobility.Rook += 3
	p.Mobility.Queen -= 2

	p.Pawns.DoubledPenalty += 9
	p.Pawns.IsolatedPenalty -= 6
	p.Pawns.PassedBonus[3] += 17
	p.Pawns.PassedBonus[5] -= 23

	p.KingSafety.ShieldPawnBonus += 11
	p.KingSafety.OpenFilePenalty -= 8
	p.KingSafety.SemiOpenFilePenalty += 6

	p.BishopPair += 17

	return p
}

// verifyCacheSplit is -mode=verify's actual check: for each sample,
// compare whiteScore (the cheap cached path: the five Compute*
// functions at shifted weights + restScore cached at load time under
// DefaultTuneParams) against whiteScoreFull (a complete, honest
// EvaluateWithTuneParams recompute at the SAME shifted weights). If
// restScore genuinely doesn't depend on any of the tuned weights,
// these must match exactly — integer arithmetic, no
// tolerance/epsilon needed or wanted here; any mismatch at all means
// the split is unsafe and coordinate descent must not be built on top
// of it. Checks only a handful of positions (n), not the full 725k —
// this is a one-time structural check of the split itself, not a
// statistical measurement like meanSquaredError, so a small, chosen-
// for-diversity sample is enough: a genuine hidden dependency in the
// eval code would show up on essentially any position, not need
// hundreds of thousands of tries to surface.
func verifyCacheSplit(samples []sample, n int) (mismatches int) {
	shifted := shiftedParams()
	if n > len(samples) {
		n = len(samples)
	}
	for i := 0; i < n; i++ {
		s := samples[i]
		cached := whiteScore(s, shifted)
		full := whiteScoreFull(s.b, shifted)
		status := "OK"
		if cached != full {
			mismatches++
			status = "MISMATCH"
		}
		fmt.Printf("  [%s] pos %d: cached=%.0f full=%.0f fen=%s\n", status, i, cached, full, s.b.ToFEN())
	}
	return mismatches
}

func main() {
	epdPath := flag.String("epd", "cmd/tune/testdata/quiet-labeled.epd", "path to a quiet-labeled.epd-format file")
	mode := flag.String("mode", "error", "findk | error | verify | tune")
	k := flag.Float64("k", 0, "sigmoid scaling constant (required for -mode=error and -mode=tune; ignored otherwise)")
	step := flag.Int("step", 8, "-mode=tune: starting step size in centipawns, annealed by half each time a full epoch improves nothing, down to a floor of 1")
	checkpointPath := flag.String("checkpoint", "cmd/tune/tune_checkpoint.json", "-mode=tune: where to write progress after every epoch")
	resume := flag.Bool("resume", false, "-mode=tune: resume from -checkpoint instead of starting from eval.DefaultTuneParams")
	threads := flag.Int("threads", 1, "goroutines to split each MSE computation across. Default 1 reproduces every number this tool has ever printed bit-for-bit (float64 addition isn't associative, so >1 gives a very slightly different sum — see meanSquaredError's own doc comment). Safe to raise for -mode=tune, where it matters most.")
	flag.Parse()

	samples, bad, err := loadEPD(*epdPath)
	if err != nil {
		log.Fatalf("loading %s: %v", *epdPath, err)
	}
	fmt.Printf("loaded %d positions (%d bad lines skipped) from %s\n", len(samples), bad, *epdPath)
	if len(samples) == 0 {
		log.Fatal("no usable positions, nothing to do")
	}

	// DefaultTuneParams == today's production weights across material+
	// PST, mobility, pawn structure, king safety, and bishop pair
	// (already Texel-tuned from previous runs — see DefaultTuneParams'
	// own doc comment in tunable.go). King PST is deliberately not
	// part of this struct at all anymore — tried, its result rejected
	// twice as overfit, removed from scope rather than left in as
	// "tuned but distrusted". Not varied here — this is what
	// -mode=findk/-mode=error/-mode=verify measure AGAINST; -mode=tune
	// (below) is the only mode that actually moves these.
	params := eval.DefaultTuneParams

	switch *mode {
	case "findk":
		bestK := findK(samples, params, *threads)
		fmt.Printf("best K = %.6f (MSE = %.6f)\n", bestK, meanSquaredError(samples, bestK, params, *threads))
	case "error":
		if *k == 0 {
			fmt.Fprintln(os.Stderr, "-mode=error needs -k=<value from -mode=findk>")
			os.Exit(2)
		}
		fmt.Printf("MSE at K=%.6f = %.6f\n", *k, meanSquaredError(samples, *k, params, *threads))
	case "verify":
		fmt.Println("checking cached restScore against a full recompute at SHIFTED weights (not defaults):")
		mismatches := verifyCacheSplit(samples, 10)
		if mismatches == 0 {
			fmt.Println("all positions matched exactly — the cache split is safe.")
		} else {
			fmt.Printf("%d mismatch(es) — DO NOT proceed to coordinate descent until this is resolved.\n", mismatches)
		}
	case "tune":
		if *k == 0 {
			fmt.Fprintln(os.Stderr, "-mode=tune needs -k=<value from -mode=findk>")
			os.Exit(2)
		}

		train, heldout := splitTrainHeldout(samples)
		fmt.Printf("split: %d train, %d held-out (every 5th position)\n", len(train), len(heldout))

		startParams := eval.DefaultTuneParams
		startStep := *step
		startEpoch := 0
		if *resume {
			cp, ok, err := loadCheckpoint(*checkpointPath)
			if err != nil {
				log.Fatalf("loading checkpoint %s: %v", *checkpointPath, err)
			}
			if ok {
				startParams = cp.Params
				startStep = cp.Step
				startEpoch = cp.Epoch
				fmt.Printf("resuming from checkpoint: epoch %d, step %d, train MSE %.6f\n", cp.Epoch, cp.Step, cp.TrainMSE)
			} else {
				fmt.Println("-resume given but no checkpoint file found yet, starting fresh")
			}
		}

		final := coordinateDescent(train, heldout, *k, startParams, startStep, startEpoch, *checkpointPath, *threads)
		printLiterals(final)
	default:
		fmt.Fprintf(os.Stderr, "unknown -mode=%q, want findk, error, verify, or tune\n", *mode)
		os.Exit(2)
	}
}

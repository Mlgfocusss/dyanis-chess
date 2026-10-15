// This file is cmd/tune's step 4: the actual coordinate-descent
// tuning loop, split into its own file the same way search.go was
// split into tt.go/negamax.go/ordering.go/quiescence.go/search.go —
// one theme per file, no behavior split across the split itself.
//
// Step annealing floors at 1, not lower: every field across
// eval.TuneParams' groups (MaterialPSTParams, MobilityParams,
// PawnStructureParams, KingSafetyParams, and the bare BishopPair int)
// is int (centipawns), same as production's own constants/tables. A
// probe below step=1 rounds back to the SAME integer for almost every
// parameter — it only does anything for whichever parameter happens
// to sit exactly on a rounding boundary at that moment, which isn't a
// systematic search, just noise. If held-out MSE is still visibly
// dropping in the last few epochs at step=1 when a run ends, that's
// the signal to consider moving these to float64 (rounding only at
// printLiterals time) as its own, separately discussed step — not
// something to build pre-emptively here without evidence it's needed.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"dyanis-chess/engine/internal/eval"
)

// paramSlot is one tunable scalar — one of the five material
// constants, one cell of one PST table, one of the 15 mobility/
// pawn-structure/king-safety weights, or the bishop pair bonus —
// addressed by a raw *int into a live eval.TuneParams. name is for
// logging/checkpoints only, never parsed back into anything.
type paramSlot struct {
	name string
	ptr  *int
}

// paramSlots returns all 341 tunable slots (5 material + 5*64 PST +
// 4 mobility + 2 pawn-structure penalties + 6 passed-bonus ranks + 3
// king-safety + 1 bishop pair = 325 + 15 + 1) in a fixed, deliberate
// order — material first, then PST tables one full table at a time
// (pawn -> knight -> bishop -> rook -> queen), then mobility, then
// pawn structure, then king safety, then bishop pair last, never
// interleaved — so progress output and checkpoints read as "currently
// on bishop pair", not a shuffle between groups. King PST is
// deliberately not among these slots at all — see NonMaterialScore's
// doc comment in tunable.go for why it was tried and removed.
//
// Pawns.PassedBonus[0] and [7] are deliberately skipped — see
// PawnStructureParams' own doc comment in tunable.go: a pawn can
// never stand on its own back rank or the promotion rank, so those
// two slots are never read by ComputePawnStructureScore at all;
// probing them would just waste coordinate-descent passes.
func paramSlots(p *eval.TuneParams) []paramSlot {
	slots := []paramSlot{
		{"MaterialPST.PawnValue", &p.MaterialPST.PawnValue},
		{"MaterialPST.KnightValue", &p.MaterialPST.KnightValue},
		{"MaterialPST.BishopValue", &p.MaterialPST.BishopValue},
		{"MaterialPST.RookValue", &p.MaterialPST.RookValue},
		{"MaterialPST.QueenValue", &p.MaterialPST.QueenValue},
	}
	addPST := func(prefix string, table *[64]int) {
		for i := 0; i < 64; i++ {
			slots = append(slots, paramSlot{fmt.Sprintf("%s[%d]", prefix, i), &table[i]})
		}
	}
	addPST("MaterialPST.PawnPST", &p.MaterialPST.PawnPST)
	addPST("MaterialPST.KnightPST", &p.MaterialPST.KnightPST)
	addPST("MaterialPST.BishopPST", &p.MaterialPST.BishopPST)
	addPST("MaterialPST.RookPST", &p.MaterialPST.RookPST)
	addPST("MaterialPST.QueenPST", &p.MaterialPST.QueenPST)

	slots = append(slots,
		paramSlot{"Mobility.Knight", &p.Mobility.Knight},
		paramSlot{"Mobility.Bishop", &p.Mobility.Bishop},
		paramSlot{"Mobility.Rook", &p.Mobility.Rook},
		paramSlot{"Mobility.Queen", &p.Mobility.Queen},

		paramSlot{"Pawns.DoubledPenalty", &p.Pawns.DoubledPenalty},
		paramSlot{"Pawns.IsolatedPenalty", &p.Pawns.IsolatedPenalty},
	)
	for rank := 1; rank <= 6; rank++ { // 0 and 7: never read, see doc comment above
		slots = append(slots, paramSlot{fmt.Sprintf("Pawns.PassedBonus[%d]", rank), &p.Pawns.PassedBonus[rank]})
	}

	slots = append(slots,
		paramSlot{"KingSafety.ShieldPawnBonus", &p.KingSafety.ShieldPawnBonus},
		paramSlot{"KingSafety.OpenFilePenalty", &p.KingSafety.OpenFilePenalty},
		paramSlot{"KingSafety.SemiOpenFilePenalty", &p.KingSafety.SemiOpenFilePenalty},
	)

	slots = append(slots, paramSlot{"BishopPair", &p.BishopPair})

	return slots
}

// checkpoint is written to disk after every epoch — enough to both
// resume a broken-off run (-resume, see main.go) and to recover the
// best weights found so far even if the run is killed and never
// reaches convergence at all.
type checkpoint struct {
	Epoch      int             `json:"epoch"`
	Step       int             `json:"step"`
	TrainMSE   float64         `json:"train_mse"`
	HeldoutMSE float64         `json:"heldout_mse"`
	Params     eval.TuneParams `json:"params"`
}

func saveCheckpoint(path string, c checkpoint) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(c)
}

// loadCheckpoint reads back a checkpoint saveCheckpoint wrote.
// ok=false (with err==nil) means the file doesn't exist yet — the
// normal case the first time -resume is passed on a fresh run.
func loadCheckpoint(path string) (c checkpoint, ok bool, err error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return checkpoint{}, false, nil
	}
	if err != nil {
		return checkpoint{}, false, err
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&c); err != nil {
		return checkpoint{}, false, err
	}
	return c, true, nil
}

// coordinateDescent is the tuning loop. Every epoch, in paramSlots'
// fixed order: try current+step and current-step against the TRAIN
// set only, keep whichever (if either) lowers train MSE, otherwise
// leave the slot untouched. Held-out MSE is measured once per epoch
// for monitoring only — it never influences which direction a slot
// moves, which is what makes it a meaningful overfitting check rather
// than a second objective quietly being optimized against.
//
// step halves after any epoch that improves nothing, down to a floor
// of 1 (see this file's own top comment) — at the floor, a
// no-improvement epoch ends the run instead of halving further.
// startEpoch/startStep/startParams let -resume continue a checkpoint
// instead of beginning at epoch 0 / eval.DefaultTuneParams.
func coordinateDescent(train, heldout []sample, k float64, startParams eval.TuneParams, startStep, startEpoch int, checkpointPath string, threads int) eval.TuneParams {
	params := startParams
	slots := paramSlots(&params)
	step := startStep
	currentMSE := meanSquaredError(train, k, params, threads)

	for epoch := startEpoch + 1; ; epoch++ {
		improvedAny := false
		for _, slot := range slots {
			original := *slot.ptr

			*slot.ptr = original + step
			upMSE := meanSquaredError(train, k, params, threads)

			*slot.ptr = original - step
			downMSE := meanSquaredError(train, k, params, threads)

			switch {
			case upMSE < currentMSE && upMSE <= downMSE:
				*slot.ptr = original + step
				currentMSE = upMSE
				improvedAny = true
			case downMSE < currentMSE:
				*slot.ptr = original - step
				currentMSE = downMSE
				improvedAny = true
			default:
				*slot.ptr = original
			}
		}

		heldoutMSE := meanSquaredError(heldout, k, params, threads)
		fmt.Printf("epoch %d (step=%d): train MSE=%.6f  held-out MSE=%.6f\n", epoch, step, currentMSE, heldoutMSE)

		if err := saveCheckpoint(checkpointPath, checkpoint{
			Epoch: epoch, Step: step, TrainMSE: currentMSE, HeldoutMSE: heldoutMSE, Params: params,
		}); err != nil {
			// Not fatal: losing one epoch's checkpoint write isn't
			// worth aborting hours of tuning over. Next epoch's write
			// will very likely just succeed (transient disk/AV/etc.
			// issue) and overwrite it anyway.
			fmt.Fprintf(os.Stderr, "warning: checkpoint write failed: %v\n", err)
		}

		if !improvedAny {
			if step <= 1 {
				fmt.Println("no improvement at step=1 — converged.")
				break
			}
			next := step / 2
			fmt.Printf("no improvement at step=%d, annealing to step=%d\n", step, next)
			step = next
		}
	}
	return params
}

// printLiterals formats the final params as Go source in the same
// layout the production files already use — meant to be copy-pasted
// by hand into eval.go/pst.go/mobility.go/pawns.go/king_safety.go.
// This tool never writes those files itself; see this file's own
// top-of-file comment and main.go's -mode=tune doc for why that's
// deliberate.
func printLiterals(p eval.TuneParams) {
	fmt.Println()
	fmt.Println("--- paste into eval.go's const block ---")
	fmt.Printf("PawnValue   = %d\n", p.MaterialPST.PawnValue)
	fmt.Printf("KnightValue = %d\n", p.MaterialPST.KnightValue)
	fmt.Printf("BishopValue = %d\n", p.MaterialPST.BishopValue)
	fmt.Printf("RookValue   = %d\n", p.MaterialPST.RookValue)
	fmt.Printf("QueenValue  = %d\n", p.MaterialPST.QueenValue)

	printTable := func(varName string, t [64]int) {
		fmt.Printf("\n--- paste into pst.go: var %s = [64]int{ ... } ---\n", varName)
		for rank := 0; rank < 8; rank++ {
			fmt.Print("\t")
			for file := 0; file < 8; file++ {
				fmt.Printf("%d, ", t[rank*8+file])
			}
			fmt.Printf("// rank %d\n", rank+1)
		}
	}
	printTable("pawnPST", p.MaterialPST.PawnPST)
	printTable("knightPST", p.MaterialPST.KnightPST)
	printTable("bishopPST", p.MaterialPST.BishopPST)
	printTable("rookPST", p.MaterialPST.RookPST)
	printTable("queenPST", p.MaterialPST.QueenPST)

	fmt.Println("\n--- paste into mobility.go's const block ---")
	fmt.Printf("knightMobilityWeight = %d\n", p.Mobility.Knight)
	fmt.Printf("bishopMobilityWeight = %d\n", p.Mobility.Bishop)
	fmt.Printf("rookMobilityWeight   = %d\n", p.Mobility.Rook)
	fmt.Printf("queenMobilityWeight  = %d\n", p.Mobility.Queen)

	fmt.Println("\n--- paste into pawns.go's const block ---")
	fmt.Printf("doubledPawnPenalty  = %d\n", p.Pawns.DoubledPenalty)
	fmt.Printf("isolatedPawnPenalty = %d\n", p.Pawns.IsolatedPenalty)
	fmt.Println("\n--- paste into pawns.go: var passedPawnBonus = [8]int{ ... } ---")
	fmt.Print("\t")
	for i := 0; i < 8; i++ {
		fmt.Printf("%d, ", p.Pawns.PassedBonus[i])
	}
	fmt.Println("// index 0 and 7 unused — never read, see PawnStructureParams' doc comment")

	fmt.Println("\n--- paste into king_safety.go's const block ---")
	fmt.Printf("shieldPawnBonus         = %d\n", p.KingSafety.ShieldPawnBonus)
	fmt.Printf("openFileKingPenalty     = %d\n", p.KingSafety.OpenFilePenalty)
	fmt.Printf("semiOpenFileKingPenalty = %d\n", p.KingSafety.SemiOpenFilePenalty)

	fmt.Println("\n--- paste into bishops.go's const block ---")
	fmt.Printf("bishopPairBonus = %d\n", p.BishopPair)
}

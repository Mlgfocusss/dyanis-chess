// Package openings builds start positions for engine-vs-engine games:
// weighted random walks through a Polyglot book, random capture-free
// lines, or FEN/EPD files. Shared by cmd/match and cmd/gendata, so both
// draw from the same logic; the same seed gives the same FENs.
package openings

import (
	"bufio"
	"math/rand"
	"os"
	"strconv"
	"strings"

	"dyanis-chess/internal/board"
	"dyanis-chess/internal/book"
	"dyanis-chess/internal/eval"
	"dyanis-chess/internal/movegen"
)

// Accept reports whether pos is a usable start position: the game is not
// over, the side to move is not in check, and the static eval is within
// ±maxEval centipawns.
func Accept(pos *board.Board, maxEval int) bool {
	if movegen.GameStatus(pos).IsGameOver() || movegen.InCheck(pos) {
		return false
	}
	e := eval.Evaluate(pos, nil)
	return e <= maxEval && e >= -maxEval
}

// FromBook generates n openings by walking `plies` moves through the
// Polyglot book at path, picking each move at random in proportion to its
// weight, so the positions are real opening theory rather than random
// shuffling. A walk that leaves the book before `plies` moves is
// discarded, as is one that ends in an unbalanced position (see Accept)
// or repeats an earlier FEN. The same seed gives the same openings.
func FromBook(path string, n, plies, maxEval int, seed int64) ([]string, error) {
	return FromBookRandom(path, n, plies, 0, maxEval, seed)
}

// FromBookRandom is FromBook followed by randPlies uniformly random legal
// moves (captures included) after the book walk, for more varied start
// positions than the book alone can give. randPlies == 0 is exactly
// FromBook: no extra random numbers are drawn.
func FromBookRandom(path string, n, plies, randPlies, maxEval int, seed int64) ([]string, error) {
	bk, err := book.Load(path)
	if err != nil {
		return nil, err
	}
	rng := rand.New(rand.NewSource(seed))
	out := make([]string, 0, n)
	seen := map[string]bool{}
	for attempts := 0; len(out) < n && attempts < n*1000; attempts++ {
		pos := board.NewInitialBoard()
		ok := true
		for i := 0; i < plies && ok; i++ {
			entries := bk.LookupBoard(pos)
			if len(entries) == 0 {
				ok = false
				break
			}
			total := 0
			for _, e := range entries {
				total += int(e.Weight)
			}
			pick := entries[rng.Intn(len(entries))] // uniform if every weight is 0
			if total > 0 {
				r := rng.Intn(total)
				for _, e := range entries {
					if r < int(e.Weight) {
						pick = e
						break
					}
					r -= int(e.Weight)
				}
			}
			m, found := book.DecodeMove(pick, movegen.GenerateLegalMoves(pos))
			if !found {
				ok = false
				break
			}
			pos.MakeMove(m)
		}
		for i := 0; i < randPlies && ok; i++ {
			legal := movegen.GenerateLegalMoves(pos)
			if len(legal) == 0 {
				ok = false
				break
			}
			pos.MakeMove(legal[rng.Intn(len(legal))])
		}
		if !ok || !Accept(pos, maxEval) {
			continue
		}
		fen := pos.ToFEN()
		if seen[fen] {
			continue
		}
		seen[fen] = true
		out = append(out, fen)
	}
	return out, nil
}

// Random generates n balanced, capture-free openings: `plies` random
// legal non-capturing moves from the start position, kept only if the
// static eval is within ±maxEval centipawns.
func Random(n, plies, maxEval int, seed int64) []string {
	rng := rand.New(rand.NewSource(seed))
	out := make([]string, 0, n)
	seen := map[string]bool{}
	for attempts := 0; len(out) < n && attempts < n*1000; attempts++ {
		pos := board.NewInitialBoard()
		ok := true
		for i := 0; i < plies; i++ {
			var quiet []board.Move
			for _, m := range movegen.GenerateLegalMoves(pos) {
				if pos.PieceAt(m.To).IsNone() {
					quiet = append(quiet, m)
				}
			}
			if len(quiet) == 0 {
				ok = false
				break
			}
			pos.MakeMove(quiet[rng.Intn(len(quiet))])
		}
		if !ok || !Accept(pos, maxEval) {
			continue
		}
		fen := pos.ToFEN()
		if seen[fen] {
			continue
		}
		seen[fen] = true
		out = append(out, fen)
	}
	return out
}

// Read loads one FEN or EPD per line from path, skipping blank lines and
// '#' comments.
func Read(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// EPD lines carry only 4 FEN fields (and trailing opcodes); FEN
		// wants 6. Keep the first 4 fields and pad the clocks.
		fields := strings.Fields(line)
		if len(fields) >= 6 {
			if _, err := strconv.Atoi(fields[4]); err == nil {
				out = append(out, strings.Join(fields[:6], " "))
				continue
			}
		}
		if len(fields) >= 4 {
			out = append(out, strings.Join(fields[:4], " ")+" 0 1")
		}
	}
	return out, sc.Err()
}

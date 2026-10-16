// Package eco names chess openings. The reference data is the lichess
// "chess-openings" collection (https://github.com/lichess-org/chess-openings,
// CC0 / public domain): one TSV file per ECO letter, each line an ECO code,
// a name and the line that reaches the opening.
//
// Lines are indexed by the Zobrist hash of the position they end in, not by
// the move string, so a transposition (1.Nf3 d5 2.d4 reaching the same
// position as 1.d4 d5 2.Nf3) finds the same name. The hash is Polyglot's,
// which counts the en-passant square only when a capture is actually
// possible: the same rule FIDE uses for repetitions.
package eco

import (
	"bufio"
	"embed"
	"fmt"
	"strings"
	"sync"

	"dyanis-chess/backend/internal/pgn"
)

//go:embed data/*.tsv
var dataFS embed.FS

// Opening is one named line of the reference.
type Opening struct {
	ECO  string `json:"eco"`  // "B12"
	Name string `json:"name"` // "Caro-Kann Defense: Advance Variation"
	// Plies is the length of the reference line, in half-moves.
	Plies int `json:"plies"`
}

// Book maps positions to opening names.
type Book struct {
	byHash map[uint64]Opening
	// theory holds every position on any reference line, named or not:
	// a line named at ply 9 passes through unnamed positions at plies 7 and 8.
	theory map[uint64]struct{}
	// through counts the reference lines that pass through each position: a
	// rough measure of how much theory there is behind it (the main first moves
	// have hundreds of lines, a rare one a handful).
	through map[uint64]int
}

// Lines is the number of reference lines that pass through the position
// (0 when it is not in the book).
func (b *Book) Lines(hash uint64) int {
	if b == nil {
		return 0
	}
	return b.through[hash]
}

// Lookup returns the opening whose reference line ends in the position
// with this hash. When several lines end in the same position, the
// shortest wins (ties: the one listed first), which is the most direct
// route to the position.
func (b *Book) Lookup(hash uint64) (Opening, bool) {
	if b == nil {
		return Opening{}, false
	}
	o, ok := b.byHash[hash]
	return o, ok
}

// InBook reports whether the position lies on any reference line, whether or
// not it has a name of its own. A move that reaches such a position is
// opening theory.
func (b *Book) InBook(hash uint64) bool {
	if b == nil {
		return false
	}
	_, ok := b.theory[hash]
	return ok
}

// Len is the number of distinct named positions in the book.
func (b *Book) Len() int {
	if b == nil {
		return 0
	}
	return len(b.byHash)
}

var (
	defaultOnce sync.Once
	defaultBook *Book
)

// Default returns the embedded lichess book, built on first use. It
// panics if the embedded data is malformed: that is a build defect,
// caught by this package's tests, not a runtime condition.
func Default() *Book {
	defaultOnce.Do(func() {
		b := &Book{byHash: make(map[uint64]Opening, 4000), theory: make(map[uint64]struct{}, 12000), through: make(map[uint64]int, 12000)}
		for _, letter := range []string{"a", "b", "c", "d", "e"} {
			data, err := dataFS.ReadFile("data/" + letter + ".tsv")
			if err != nil {
				panic(err)
			}
			if err := b.load(string(data)); err != nil {
				panic(fmt.Sprintf("eco: data/%s.tsv: %v", letter, err))
			}
		}
		defaultBook = b
	})
	return defaultBook
}

// Parse builds a book from TSV text in the lichess format
// ("eco<TAB>name<TAB>pgn", first line a header).
func Parse(tsv string) (*Book, error) {
	b := &Book{byHash: make(map[uint64]Opening), theory: make(map[uint64]struct{}), through: make(map[uint64]int)}
	if err := b.load(tsv); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *Book) load(tsv string) error {
	sc := bufio.NewScanner(strings.NewReader(tsv))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimRight(sc.Text(), "\r")
		if n == 1 && strings.HasPrefix(line, "eco\t") {
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			return fmt.Errorf("line %d: want 3 tab-separated fields, got %d", n, len(f))
		}
		// The reference line has no result marker; "*" ends the movetext.
		g, err := pgn.ParseOne(f[2] + " *")
		if err != nil {
			return fmt.Errorf("line %d (%s): %v", n, f[1], err)
		}
		if len(g.Moves) == 0 {
			return fmt.Errorf("line %d (%s): no moves", n, f[1])
		}
		pos, err := g.Position()
		if err != nil {
			return fmt.Errorf("line %d: %v", n, err)
		}
		seen := map[uint64]bool{}
		for _, m := range g.Moves {
			pos.Play(m.Move)
			h := pos.Hash()
			b.theory[h] = struct{}{}
			if !seen[h] { // a line that repeats a position counts once
				seen[h] = true
				b.through[h]++
			}
		}
		o := Opening{ECO: f[0], Name: f[1], Plies: len(g.Moves)}
		h := pos.Hash()
		if old, dup := b.byHash[h]; dup && old.Plies <= o.Plies {
			continue
		}
		b.byHash[h] = o
	}
	return sc.Err()
}

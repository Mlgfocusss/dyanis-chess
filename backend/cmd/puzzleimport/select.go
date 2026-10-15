package main

import (
	"container/heap"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"dyanis-chess/backend/internal/puzzles"
	"dyanis-chess/backend/internal/store"
)

// criteria says which puzzles of the Lichess file are worth keeping.
type criteria struct {
	maxDeviation  int // rating deviation above this: the rating is not settled
	minPopularity int // Lichess "popularity", -100..100: how many players liked it
	minPlays      int
	bucket        int // rating width of one bucket
	perBucket     int // most puzzles kept from one bucket
}

// bucketOf is the index of the bucket a rating falls in.
func (c criteria) bucketOf(rating int) int { return rating / c.bucket }

// worse orders puzzles for dropping: lower popularity first, then fewer plays.
func worse(a, b store.Puzzle) bool {
	if a.Popularity != b.Popularity {
		return a.Popularity < b.Popularity
	}
	return a.Plays < b.Plays
}

// pool keeps the best n puzzles it was shown: a min-heap, so the worst of
// the kept ones is at the top and is the one replaced.
type pool struct {
	n  int
	ps []store.Puzzle
}

func (p *pool) Len() int           { return len(p.ps) }
func (p *pool) Less(i, j int) bool { return worse(p.ps[i], p.ps[j]) }
func (p *pool) Swap(i, j int)      { p.ps[i], p.ps[j] = p.ps[j], p.ps[i] }
func (p *pool) Push(x any)         { p.ps = append(p.ps, x.(store.Puzzle)) }
func (p *pool) Pop() any {
	last := p.ps[len(p.ps)-1]
	p.ps = p.ps[:len(p.ps)-1]
	return last
}

func (p *pool) offer(x store.Puzzle) {
	if len(p.ps) < p.n {
		heap.Push(p, x)
		return
	}
	if worse(p.ps[0], x) {
		p.ps[0] = x
		heap.Fix(p, 0)
	}
}

// stats counts what happened to the rows of the file.
type stats struct {
	rows, malformed, filtered, badLine int
}

// pick reads the Lichess puzzle CSV (header row first) and returns the best
// puzzles of each rating bucket, in rating order. Lines that are not legal
// are dropped, so that every puzzle in the database can be played.
func pick(r io.Reader, c criteria) ([]store.Puzzle, stats, error) {
	var st stats
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.ReuseRecord = false
	head, err := cr.Read()
	if err != nil {
		return nil, st, fmt.Errorf("reading the header: %w", err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[h] = i
	}
	for _, name := range []string{"PuzzleId", "FEN", "Moves", "Rating", "RatingDeviation", "Popularity", "NbPlays", "Themes"} {
		if _, ok := col[name]; !ok {
			return nil, st, fmt.Errorf("the file has no %q column (is it the Lichess puzzle CSV?)", name)
		}
	}

	pools := map[int]*pool{}
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			st.malformed++
			continue
		}
		st.rows++
		num := func(name string) (int, bool) {
			i := col[name]
			if i >= len(rec) {
				return 0, false
			}
			n, err := strconv.Atoi(rec[i])
			return n, err == nil
		}
		rating, ok1 := num("Rating")
		dev, ok2 := num("RatingDeviation")
		pop, ok3 := num("Popularity")
		plays, ok4 := num("NbPlays")
		if !(ok1 && ok2 && ok3 && ok4) || len(rec) <= col["Themes"] {
			st.malformed++
			continue
		}
		if dev > c.maxDeviation || pop < c.minPopularity || plays < c.minPlays {
			st.filtered++
			continue
		}
		b := c.bucketOf(rating)
		if pools[b] == nil {
			pools[b] = &pool{n: c.perBucket}
		}
		pools[b].offer(store.Puzzle{
			ID: rec[col["PuzzleId"]], FEN: rec[col["FEN"]], Moves: rec[col["Moves"]],
			Rating: rating, Popularity: pop, Plays: plays, Themes: strings.TrimSpace(rec[col["Themes"]]),
		})
	}

	var out []store.Puzzle
	for _, p := range pools {
		for _, x := range p.ps {
			if _, err := puzzles.BuildLine(x.FEN, x.Moves); err != nil {
				st.badLine++
				continue
			}
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Rating != out[j].Rating {
			return out[i].Rating < out[j].Rating
		}
		return out[i].ID < out[j].ID
	})
	return out, st, nil
}

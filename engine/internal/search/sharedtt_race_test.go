package search

import (
	"sync"
	"testing"
)

// TestSharedTTConcurrentAccess hammers a single sharedTT's probe and
// store from many goroutines at once. This is NOT a correctness test
// of the replacement policy — concurrent stores can legitimately race
// each other for the same slot, and whichever wins is fine, so
// nothing here asserts on the table's final contents. It exists
// purely so `go test -race ./internal/search` has something to
// actually catch: a data race needs at least two concurrent writers
// touching the same memory, which every other test in this package
// (still single-goroutine, since Lazy SMP itself isn't wired in yet —
// see plan.md item C) can never produce, regardless of whether the
// underlying code is race-free. Run it specifically with -race:
//
//	go test -race -run TestSharedTTConcurrentAccess ./internal/search
//
// A plain `go test` run (no -race) exercises the same code but proves
// nothing about thread-safety — the race detector is what actually
// does the checking here.
func TestSharedTTConcurrentAccess(t *testing.T) {
	const goroutines = 16
	const opsPerGoroutine = 20000
	// Deliberately few DISTINCT hashes relative to opsPerGoroutine —
	// a wide-open hash range would rarely have two goroutines land on
	// the same slot at the same instant, which is exactly what -race
	// needs many chances at in order to catch anything. Concentrating
	// on a small set of hashes forces frequent same-slot collisions.
	const distinctHashes = 8

	s := newSharedTT(1 << 6) // small table too: same reasoning, more collisions

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				// +1: hash 0 is probe/store's own "slot never
				// written" sentinel (see probe's doc comment) —
				// avoided here so every store is a genuine write,
				// not a no-op against the empty-slot convention.
				hash := uint64((seed*opsPerGoroutine+i)%distinctHashes) + 1
				if i%2 == 0 {
					s.store(hash, ttEntry{depth: i % 8, score: i})
				} else {
					s.probe(hash)
				}
			}
		}(g)
	}
	wg.Wait()
}

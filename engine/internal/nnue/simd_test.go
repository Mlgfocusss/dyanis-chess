package nnue

import (
	"math/rand"
	"testing"
)

// The AVX2 loops must agree with the plain ones bit for bit, including
// lengths that are not a multiple of 16 and accumulator values outside
// [0, QA] on both sides. Skipped in builds without them (run the tests
// with GOEXPERIMENT=simd to cover them).
func TestSIMDMatchesScalar(t *testing.T) {
	if !haveSIMD {
		t.Skip("no SIMD in this build (GOEXPERIMENT=simd, amd64 with AVX2)")
	}
	rng := rand.New(rand.NewSource(1))
	vec := func(n, lo, hi int) []int16 {
		s := make([]int16, n)
		for i := range s {
			s[i] = int16(lo + rng.Intn(hi-lo+1))
		}
		return s
	}
	for _, n := range []int{1, 15, 16, 17, 64, 255, 256, 1024, 1040, 2048} {
		for trial := 0; trial < 20; trial++ {
			v := vec(n, -600, 600)
			w := vec(n, -MaxOutputWeight, MaxOutputWeight)
			for _, qa := range []int32{1, 181, 255} {
				if a, b := creluDot(v, w, qa), creluDotSIMD(v, w, qa); a != b {
					t.Fatalf("crelu n=%d qa=%d: scalar %d, simd %d", n, qa, a, b)
				}
				if a, b := screluDot(v, w, qa), screluDotSIMD(v, w, qa); a != b {
					t.Fatalf("screlu n=%d qa=%d: scalar %d, simd %d", n, qa, a, b)
				}
			}

			s, a, b := vec(n, -30000, 30000), vec(n, -30000, 30000), vec(n, -30000, 30000)
			want := make([]int16, n)
			for i := range want {
				want[i] = s[i] + a[i] - b[i] // wraps, like the vector adds
			}
			got := make([]int16, n)
			addSubRowsSIMD(got, s, a, b)
			add := append([]int16(nil), s...)
			addRowSIMD(add, a)
			subRowSIMD(add, b)
			for i := range want {
				if got[i] != want[i] || add[i] != want[i] {
					t.Fatalf("rows n=%d i=%d: want %d, addSub %d, add+sub %d", n, i, want[i], got[i], add[i])
				}
			}
		}
	}
}

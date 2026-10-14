//go:build goexperiment.simd && amd64

// AVX2 versions of the hot loops, built only with GOEXPERIMENT=simd (Go's
// experimental simd/archsimd package) on amd64 and used only when the CPU
// has AVX2; everything else runs the plain loops in accumulator.go. Same
// results to the bit: int16 adds wrap the same way, and the dot products
// are exact (see MaxOutputWeight and the overflow notes below).
//
//	$env:GOEXPERIMENT = "simd"; go build -o dyanis.exe ./cmd/cli

package nnue

import "simd/archsimd"

var haveSIMD = archsimd.X86.AVX2()

func lanes(s []int16, i int) *[16]int16 { return (*[16]int16)(s[i : i+16]) }

func addRowSIMD(v, w []int16) {
	w = w[:len(v)]
	n := len(v) &^ 15
	for i := 0; i < n; i += 16 {
		archsimd.LoadInt16x16(lanes(v, i)).Add(archsimd.LoadInt16x16(lanes(w, i))).Store(lanes(v, i))
	}
	for i := n; i < len(v); i++ {
		v[i] += w[i]
	}
}

func subRowSIMD(v, w []int16) {
	w = w[:len(v)]
	n := len(v) &^ 15
	for i := 0; i < n; i += 16 {
		archsimd.LoadInt16x16(lanes(v, i)).Sub(archsimd.LoadInt16x16(lanes(w, i))).Store(lanes(v, i))
	}
	for i := n; i < len(v); i++ {
		v[i] -= w[i]
	}
}

func addSubRowsSIMD(d, s, a, b []int16) {
	s, a, b = s[:len(d)], a[:len(d)], b[:len(d)]
	n := len(d) &^ 15
	for i := 0; i < n; i += 16 {
		x := archsimd.LoadInt16x16(lanes(s, i))
		x = x.Add(archsimd.LoadInt16x16(lanes(a, i))).Sub(archsimd.LoadInt16x16(lanes(b, i)))
		x.Store(lanes(d, i))
	}
	for i := n; i < len(d); i++ {
		d[i] = s[i] + a[i] - b[i]
	}
}

func hsum(x archsimd.Int32x8) int64 {
	var a [8]int32
	x.Store(&a)
	var s int64
	for _, v := range a {
		s += int64(v)
	}
	return s
}

// creluDotSIMD: each int32 lane gains at most 2·255·127 per step, so it
// cannot overflow for any hidden size Parse accepts.
func creluDotSIMD(v, w []int16, qa int32) int64 {
	w = w[:len(v)]
	zero, top := archsimd.BroadcastInt16x16(0), archsimd.BroadcastInt16x16(int16(qa))
	acc := archsimd.BroadcastInt32x8(0)
	n := len(v) &^ 15
	for i := 0; i < n; i += 16 {
		c := archsimd.LoadInt16x16(lanes(v, i)).Max(zero).Min(top)
		acc = acc.Add(c.DotProductPairs(archsimd.LoadInt16x16(lanes(w, i))))
	}
	total := hsum(acc)
	for i := n; i < len(v); i++ {
		total += int64(clampQA(v[i], qa) * int32(w[i]))
	}
	return total
}

// screluDotSIMD computes Σ c²·w as Σ (c·w)·c: c·w fits int16 exactly
// (|c| ≤ 255, |w| ≤ MaxOutputWeight), and DotProductPairs multiplies it by
// c again into int32. A lane gains at most 2·255²·127 ≈ 1.7·10⁷ per step,
// so the lanes are flushed into int64 every 64 steps.
func screluDotSIMD(v, w []int16, qa int32) int64 {
	w = w[:len(v)]
	zero, top := archsimd.BroadcastInt16x16(0), archsimd.BroadcastInt16x16(int16(qa))
	var total int64
	n := len(v) &^ 15
	for start := 0; start < n; start += 64 * 16 {
		end := min(start+64*16, n)
		acc := archsimd.BroadcastInt32x8(0)
		for i := start; i < end; i += 16 {
			c := archsimd.LoadInt16x16(lanes(v, i)).Max(zero).Min(top)
			acc = acc.Add(c.Mul(archsimd.LoadInt16x16(lanes(w, i))).DotProductPairs(c))
		}
		total += hsum(acc)
	}
	for i := n; i < len(v); i++ {
		c := clampQA(v[i], qa)
		total += int64(c * c * int32(w[i]))
	}
	return total
}

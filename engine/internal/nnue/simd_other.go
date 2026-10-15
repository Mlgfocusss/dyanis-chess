//go:build !(goexperiment.simd && amd64)

package nnue

// Without GOEXPERIMENT=simd on amd64 there are no vector versions (see
// simd_amd64.go); haveSIMD is a constant false, so the compiler drops the
// calls below and the plain loops in accumulator.go run.
const haveSIMD = false

func addRowSIMD(v, w []int16)                    { panic("nnue: no SIMD in this build") }
func subRowSIMD(v, w []int16)                    { panic("nnue: no SIMD in this build") }
func addSubRowsSIMD(d, s, a, b []int16)          { panic("nnue: no SIMD in this build") }
func creluDotSIMD(v, w []int16, qa int32) int64  { panic("nnue: no SIMD in this build") }
func screluDotSIMD(v, w []int16, qa int32) int64 { panic("nnue: no SIMD in this build") }

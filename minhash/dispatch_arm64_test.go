package minhash

import (
	"fmt"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
)

// TestEqCountNEONOracle checks the NEON kernel directly against the
// portable loop across sizes and match densities (multiples of four
// lanes, per the kernel contract).
func TestEqCountNEONOracle(t *testing.T) {
	rng := hashutil.SplitMix64(43)
	for _, n := range []int{4, 8, 16, 128, 4096} {
		for _, matchEvery := range []int{1, 2, 3, 7} {
			t.Run(fmt.Sprintf("n=%d/every=%d", n, matchEvery), func(t *testing.T) {
				a := make([]uint64, n)
				b := make([]uint64, n)
				for i := range a {
					a[i] = rng.Next()
					if i%matchEvery == 0 {
						b[i] = a[i]
					} else {
						b[i] = rng.Next()
					}
				}
				want := eqCountGeneric(a, b)
				if got := eqCountNEON(a, b); got != want {
					t.Errorf("eqCountNEON = %d, want %d", got, want)
				}
			})
		}
	}
}

// TestEqCountNEONZeroGuard exercises the defensive zero-count guard: a
// zero-length call must return 0 rather than enter the loop.
func TestEqCountNEONZeroGuard(t *testing.T) {
	if got := eqCountNEON(nil, nil); got != 0 {
		t.Errorf("eqCountNEON(nil, nil) = %d, want 0", got)
	}
}

// BenchmarkEqCountKernels compares the kernels at k=128 (the go/no-go
// comparison for the NEON kernel; names match the amd64 benchmark for
// cross-machine tables).
func BenchmarkEqCountKernels(b *testing.B) {
	rng := hashutil.SplitMix64(44)
	x := make([]uint64, 128)
	y := make([]uint64, 128)
	for i := range x {
		x[i] = rng.Next()
		y[i] = rng.Next()
	}
	copy(y[:64], x[:64])
	var sink int
	b.Run("scalar", func(b *testing.B) {
		b.SetBytes(128 * 8)
		for b.Loop() {
			sink += eqCountGeneric(x, y)
		}
	})
	b.Run("neon", func(b *testing.B) {
		b.SetBytes(128 * 8)
		for b.Loop() {
			sink += eqCountNEON(x, y)
		}
	})
	_ = sink
}

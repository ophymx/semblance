package minhash

import (
	"fmt"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
)

// TestEqCount checks the dispatched kernel against the obvious loop across
// vector-group boundaries and match densities.
func TestEqCount(t *testing.T) {
	rng := hashutil.SplitMix64(41)
	for _, n := range []int{1, 3, 4, 5, 15, 16, 17, 128, 129} {
		for _, matchEvery := range []int{1, 2, 7} {
			t.Run(fmt.Sprintf("n=%d/every=%d", n, matchEvery), func(t *testing.T) {
				a := make([]uint64, n)
				b := make([]uint64, n)
				want := 0
				for i := range a {
					a[i] = rng.Next()
					if i%matchEvery == 0 {
						b[i] = a[i]
						want++
					} else {
						b[i] = rng.Next()
					}
				}
				if got := eqCount(a, b); got != want {
					t.Errorf("eqCount = %d, want %d", got, want)
				}
				if got := eqCountGeneric(a, b); got != want {
					t.Errorf("eqCountGeneric = %d, want %d", got, want)
				}
			})
		}
	}
}

func BenchmarkJaccard(b *testing.B) {
	rng := hashutil.SplitMix64(42)
	x := make(Signature, 128)
	y := make(Signature, 128)
	for i := range x {
		x[i] = rng.Next()
		y[i] = rng.Next()
	}
	copy(y[:64], x[:64])
	var sink float64
	for b.Loop() {
		sink += Jaccard(x, y)
	}
	_ = sink
}

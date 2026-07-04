package minhash

import (
	"math"
	"slices"
	"testing"

	"github.com/ophymx/semblance/internal/cpuinfo"
	"github.com/ophymx/semblance/internal/hashutil"
)

// TestDispatchBothPaths runs the kernel oracle comparisons with the AVX2
// dispatch forced off and (where the CPU allows) on, so both branches are
// covered regardless of the machine CI happens to run on.
func TestDispatchBothPaths(t *testing.T) {
	orig := useAVX2
	defer func() { useAVX2 = orig }()

	rng := hashutil.SplitMix64(51)
	fill := func(n int) []uint64 {
		out := make([]uint64, n)
		for i := range out {
			out[i] = rng.Next()
		}
		return out
	}
	const k = 128
	a, b, block := fill(k), fill(k), fill(255)
	want := make([]uint64, k)
	for i := range want {
		want[i] = math.MaxUint64
	}
	sketchBlockNaive(want, a, b, block)

	for _, on := range []bool{false, true} {
		if on && !cpuinfo.HasAVX2 {
			t.Log("CPU lacks AVX2; skipping the on-path")
			continue
		}
		useAVX2 = on
		got := make([]uint64, k)
		for i := range got {
			got[i] = math.MaxUint64
		}
		sketchBlock(got, a, b, block)
		if !slices.Equal(got, want) {
			t.Errorf("useAVX2=%v: sketchBlock disagrees with oracle", on)
		}
		if eq := eqCount(want, got); eq != len(want) {
			t.Errorf("useAVX2=%v: eqCount(x, x) = %d, want %d", on, eq, len(want))
		}
	}
}

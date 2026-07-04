package simhash

import (
	"testing"

	"github.com/ophymx/semblance/internal/cpuinfo"
	"github.com/ophymx/semblance/internal/hashutil"
)

// TestDispatchBothPaths runs the pospopcnt oracle comparison with the AVX2
// dispatch forced off and (where the CPU allows) on.
func TestDispatchBothPaths(t *testing.T) {
	orig := useAVX2
	defer func() { useAVX2 = orig }()

	rng := hashutil.SplitMix64(52)
	block := make([]uint64, 255)
	for i := range block {
		block[i] = rng.Next()
	}
	var want [64]int32
	pospopcntNaive(&want, block)

	for _, on := range []bool{false, true} {
		if on && !cpuinfo.HasAVX2 {
			t.Log("CPU lacks AVX2; skipping the on-path")
			continue
		}
		useAVX2 = on
		var got [64]int32
		pospopcnt(&got, block)
		if got != want {
			t.Errorf("useAVX2=%v: pospopcnt disagrees with oracle", on)
		}
	}
}

package simhash

import (
	"fmt"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
)

// pospopcntNaive is the oracle: the obvious per-bit count.
func pospopcntNaive(cnt *[64]int32, block []uint64) {
	for _, h := range block {
		for i := range cnt {
			if h>>i&1 == 1 {
				cnt[i]++
			}
		}
	}
}

// TestPospopcnt checks the dispatched kernel (AVX2/NEON where built,
// generic elsewhere) and the generic kernel against the naive oracle,
// covering tails around the 4-word vector groups and the block cap.
func TestPospopcnt(t *testing.T) {
	rng := hashutil.SplitMix64(21)
	for _, n := range []int{0, 1, 3, 4, 5, 15, 16, 17, 63, 64, 100, 251, 252, 255} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			block := make([]uint64, n)
			for i := range block {
				block[i] = rng.Next()
			}
			var want, gotDispatch, gotGeneric [64]int32
			pospopcntNaive(&want, block)
			pospopcnt(&gotDispatch, block)
			pospopcntGeneric(&gotGeneric, block)
			if gotDispatch != want {
				t.Error("dispatched pospopcnt disagrees with naive oracle")
			}
			if gotGeneric != want {
				t.Error("pospopcntGeneric disagrees with naive oracle")
			}
		})
	}

	// Degenerate patterns: all-ones saturates every position's tally to
	// the CSA capacity; all-zeros exercises the early exit.
	for _, pattern := range []uint64{0, ^uint64(0)} {
		block := make([]uint64, pospopcntBlock)
		for i := range block {
			block[i] = pattern
		}
		var want, got [64]int32
		pospopcntNaive(&want, block)
		pospopcnt(&got, block)
		if got != want {
			t.Errorf("pospopcnt disagrees with oracle on pattern %#x", pattern)
		}
	}
}

func BenchmarkPospopcnt(b *testing.B) {
	rng := hashutil.SplitMix64(22)
	block := make([]uint64, 252)
	for i := range block {
		block[i] = rng.Next()
	}
	b.Run("dispatched", func(b *testing.B) {
		var cnt [64]int32
		b.SetBytes(int64(8 * len(block)))
		for b.Loop() {
			pospopcnt(&cnt, block)
		}
	})
	b.Run("generic", func(b *testing.B) {
		var cnt [64]int32
		b.SetBytes(int64(8 * len(block)))
		for b.Loop() {
			pospopcntGeneric(&cnt, block)
		}
	})
	b.Run("naive", func(b *testing.B) {
		var cnt [64]int32
		b.SetBytes(int64(8 * len(block)))
		for b.Loop() {
			pospopcntNaive(&cnt, block)
		}
	})
}

// BenchmarkPospopcntParallel measures aggregate kernel throughput with all
// GOMAXPROCS workers saturated — run with -cpu to compare scaling.
func BenchmarkPospopcntParallel(b *testing.B) {
	rng := hashutil.SplitMix64(23)
	block := make([]uint64, 252)
	for i := range block {
		block[i] = rng.Next()
	}
	run := func(b *testing.B, kernel func(cnt *[64]int32, block []uint64)) {
		b.SetBytes(int64(8 * len(block)))
		b.RunParallel(func(pb *testing.PB) {
			var cnt [64]int32
			for pb.Next() {
				kernel(&cnt, block)
			}
		})
	}
	b.Run("dispatched", func(b *testing.B) { run(b, pospopcnt) })
	b.Run("generic", func(b *testing.B) { run(b, pospopcntGeneric) })
}

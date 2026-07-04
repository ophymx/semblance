package minhash

import (
	"fmt"
	"math"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
)

// sketchBlockNaive is the oracle: the direct per-element per-permutation
// update.
func sketchBlockNaive(dst, a, b []uint64, block []uint64) {
	for _, x := range block {
		for i := range dst {
			if v := a[i]*x + b[i]; v < dst[i] {
				dst[i] = v
			}
		}
	}
}

// TestSketchBlock checks the dispatched kernel (AVX2 where built) and the
// generic kernel against the naive oracle, covering odd block lengths,
// non-multiple-of-4 k, sub-threshold blocks, and continuation across
// multiple blocks.
func TestSketchBlock(t *testing.T) {
	rng := hashutil.SplitMix64(31)
	fill := func(n int) []uint64 {
		out := make([]uint64, n)
		for i := range out {
			out[i] = rng.Next()
		}
		return out
	}
	for _, k := range []int{4, 6, 8, 128} {
		for _, n := range []int{1, 2, 3, 7, 8, 9, 255, 256} {
			t.Run(fmt.Sprintf("k=%d/n=%d", k, n), func(t *testing.T) {
				a, b, block := fill(k), fill(k), fill(n)
				want := make([]uint64, k)
				gotDispatch := make([]uint64, k)
				gotGeneric := make([]uint64, k)
				for i := range want {
					want[i] = math.MaxUint64
					gotDispatch[i] = math.MaxUint64
					gotGeneric[i] = math.MaxUint64
				}
				sketchBlockNaive(want, a, b, block)
				sketchBlock(gotDispatch, a, b, block)
				sketchBlockGeneric(gotGeneric, a, b, block)
				for i := range want {
					if gotDispatch[i] != want[i] {
						t.Fatalf("dispatched kernel slot %d = %#x, want %#x", i, gotDispatch[i], want[i])
					}
					if gotGeneric[i] != want[i] {
						t.Fatalf("generic kernel slot %d = %#x, want %#x", i, gotGeneric[i], want[i])
					}
				}

				// Continuation: a second block must fold into existing minima.
				block2 := fill(n)
				sketchBlockNaive(want, a, b, block2)
				sketchBlock(gotDispatch, a, b, block2)
				for i := range want {
					if gotDispatch[i] != want[i] {
						t.Fatalf("continuation slot %d = %#x, want %#x", i, gotDispatch[i], want[i])
					}
				}
			})
		}
	}
}

func BenchmarkSketchBlock(b *testing.B) {
	rng := hashutil.SplitMix64(32)
	fill := func(n int) []uint64 {
		out := make([]uint64, n)
		for i := range out {
			out[i] = rng.Next()
		}
		return out
	}
	const k = 128
	pa, pb, block := fill(k), fill(k), fill(sketchBlockSize)
	dst := make([]uint64, k)
	run := func(b *testing.B, kernel func(dst, a, b, block []uint64)) {
		b.SetBytes(int64(8 * len(block)))
		for b.Loop() {
			kernel(dst, pa, pb, block)
		}
	}
	b.Run("dispatched", func(b *testing.B) { run(b, sketchBlock) })
	b.Run("generic", func(b *testing.B) { run(b, sketchBlockGeneric) })
}

// BenchmarkSketchBlockParallel measures aggregate kernel throughput with
// all GOMAXPROCS workers saturated (shared read-only parameters,
// per-goroutine minima) — run with -cpu to compare scaling.
func BenchmarkSketchBlockParallel(b *testing.B) {
	rng := hashutil.SplitMix64(33)
	fill := func(n int) []uint64 {
		out := make([]uint64, n)
		for i := range out {
			out[i] = rng.Next()
		}
		return out
	}
	const k = 128
	pa, pbv, block := fill(k), fill(k), fill(sketchBlockSize)
	run := func(b *testing.B, kernel func(dst, a, b, block []uint64)) {
		b.SetBytes(int64(8 * len(block)))
		b.RunParallel(func(pb *testing.PB) {
			dst := make([]uint64, k)
			for pb.Next() {
				kernel(dst, pa, pbv, block)
			}
		})
	}
	b.Run("dispatched", func(b *testing.B) { run(b, sketchBlock) })
	b.Run("generic", func(b *testing.B) { run(b, sketchBlockGeneric) })
}

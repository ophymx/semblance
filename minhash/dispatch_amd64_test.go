package minhash

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/ophymx/semblance/internal/cpuinfo"
	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/shingle"
)

// TestDispatchBothPaths runs the kernel oracle comparisons with each
// dispatch path forced on in turn — scalar, AVX2, and AVX-512 — so every
// branch is covered regardless of the machine CI happens to run on.
func TestDispatchBothPaths(t *testing.T) {
	origAVX2, origAVX512 := useAVX2, useAVX512
	defer func() { useAVX2, useAVX512 = origAVX2, origAVX512 }()

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

	paths := []struct {
		name    string
		avx2    bool
		avx512  bool
		require bool // CPU capability the path needs
	}{
		{"scalar", false, false, true},
		{"avx2", true, false, cpuinfo.HasAVX2},
		{"avx512", false, true, cpuinfo.HasAVX512},
	}
	for _, p := range paths {
		if !p.require {
			t.Logf("CPU lacks the feature for %s; skipping", p.name)
			continue
		}
		useAVX2, useAVX512 = p.avx2, p.avx512
		got := make([]uint64, k)
		for i := range got {
			got[i] = math.MaxUint64
		}
		sketchBlock(got, a, b, block)
		if !slices.Equal(got, want) {
			t.Errorf("%s: sketchBlock disagrees with oracle", p.name)
		}
		if eq := eqCount(want, got); eq != len(want) {
			t.Errorf("%s: eqCount(x, x) = %d, want %d", p.name, eq, len(want))
		}
	}
}

// TestKernelZeroGuards exercises the defensive zero-count guards in the
// vector kernels: a zero-length destination (or input) must return
// immediately instead of decrementing a zero counter into a ~2^64-iteration
// out-of-bounds loop. The dispatch wrappers never pass zero lengths — the
// callers guarantee k>=1 — so these call the kernels directly.
func TestKernelZeroGuards(t *testing.T) {
	block := []uint64{1, 2, 3, 4, 5, 6, 7, 8}
	if cpuinfo.HasAVX2 {
		sketchBlockAVX2(nil, nil, nil, block)
		if got := eqCountAVX2(nil, nil); got != 0 {
			t.Errorf("eqCountAVX2(nil, nil) = %d, want 0", got)
		}
	}
	if cpuinfo.HasAVX512 {
		sketchBlockAVX512(nil, nil, nil, block)
		if got := eqCountAVX512(nil, nil); got != 0 {
			t.Errorf("eqCountAVX512(nil, nil) = %d, want 0", got)
		}
	}
}

// BenchmarkSketchBlockKernels measures the scalar, AVX2, and AVX-512
// permutation kernels head-to-head on one 256-word block at k=128, so the
// per-ISA speedup is a direct read rather than an inference from the
// runtime-dispatched path.
func BenchmarkSketchBlockKernels(b *testing.B) {
	rng := hashutil.SplitMix64(34)
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
	b.Run("scalar", func(b *testing.B) { run(b, sketchBlockGeneric) })
	if cpuinfo.HasAVX2 {
		b.Run("avx2", func(b *testing.B) { run(b, sketchBlockAVX2) })
	}
	if cpuinfo.HasAVX512 {
		b.Run("avx512", func(b *testing.B) { run(b, sketchBlockAVX512) })
	}
}

// TestEqCountPaths checks the scalar, AVX2, and AVX-512 equality-count
// paths against the obvious loop across match densities, so every branch is
// exercised regardless of which one the runtime dispatch prefers.
func TestEqCountPaths(t *testing.T) {
	origAVX2, origAVX512 := useAVX2, useAVX512
	defer func() { useAVX2, useAVX512 = origAVX2, origAVX512 }()

	rng := hashutil.SplitMix64(43)
	paths := []struct {
		name         string
		avx2, avx512 bool
		require      bool
	}{
		{"scalar", false, false, true},
		{"avx2", true, false, cpuinfo.HasAVX2},
		{"avx512", false, true, cpuinfo.HasAVX512},
	}
	for _, p := range paths {
		if !p.require {
			continue
		}
		useAVX2, useAVX512 = p.avx2, p.avx512
		for _, n := range []int{1, 7, 8, 9, 16, 17, 128} {
			for _, every := range []int{1, 3} {
				a := make([]uint64, n)
				b := make([]uint64, n)
				want := 0
				for i := range a {
					a[i] = rng.Next()
					if i%every == 0 {
						b[i] = a[i]
						want++
					} else {
						b[i] = rng.Next()
					}
				}
				if got := eqCount(a, b); got != want {
					t.Errorf("%s n=%d every=%d: eqCount = %d, want %d", p.name, n, every, got, want)
				}
			}
		}
	}
}

// BenchmarkEqCountKernels measures the scalar, AVX2, and AVX-512 equality
// counters head-to-head at k=128 by forcing each dispatch branch in turn.
func BenchmarkEqCountKernels(b *testing.B) {
	origAVX2, origAVX512 := useAVX2, useAVX512
	defer func() { useAVX2, useAVX512 = origAVX2, origAVX512 }()

	rng := hashutil.SplitMix64(44)
	const k = 128
	x := make([]uint64, k)
	y := make([]uint64, k)
	for i := range x {
		x[i] = rng.Next()
		y[i] = rng.Next()
	}
	copy(y[:k/2], x[:k/2])
	run := func(b *testing.B, avx2, avx512 bool) {
		useAVX2, useAVX512 = avx2, avx512
		var sink int
		b.SetBytes(int64(8 * k))
		for b.Loop() {
			sink += eqCount(x, y)
		}
		_ = sink
	}
	b.Run("scalar", func(b *testing.B) { run(b, false, false) })
	if cpuinfo.HasAVX2 {
		b.Run("avx2", func(b *testing.B) { run(b, true, false) })
	}
	if cpuinfo.HasAVX512 {
		b.Run("avx512", func(b *testing.B) { run(b, false, true) })
	}
}

// BenchmarkSketchIntoPaths measures the full 100 KB Char and Words sketch
// pipelines end-to-end under each dispatch path, so the kernel-level
// speedups can be read against the Amdahl-limited whole-pipeline gain.
func BenchmarkSketchIntoPaths(b *testing.B) {
	origAVX2, origAVX512 := useAVX2, useAVX512
	defer func() { useAVX2, useAVX512 = origAVX2, origAVX512 }()

	vocab := []string{
		"the", "quick", "brown", "fox", "jumps", "over", "lazy", "dog",
		"minhash", "signature", "jaccard", "similarity", "estimate", "shingle",
		"deterministic", "heuristic", "sketch", "banding", "candidate", "index",
	}
	var sb strings.Builder
	rng := hashutil.SplitMix64(1)
	for sb.Len() < 100<<10 {
		sb.WriteString(vocab[rng.Next()%uint64(len(vocab))])
		sb.WriteByte(' ')
	}
	doc := sb.String()

	m := New(128, 0)
	dst := make(Signature, 128)
	bench := func(b *testing.B, avx2, avx512 bool, sketch func()) {
		useAVX2, useAVX512 = avx2, avx512
		b.SetBytes(int64(len(doc)))
		for b.Loop() {
			sketch()
		}
	}
	char := func() { m.SketchInto(dst, shingle.Char(doc, 8)) }
	words := func() { m.SketchInto(dst, shingle.Words(doc, 3)) }

	paths := []struct {
		name         string
		avx2, avx512 bool
		require      bool
	}{
		{"scalar", false, false, true},
		{"avx2", true, false, cpuinfo.HasAVX2},
		{"avx512", false, true, cpuinfo.HasAVX512},
	}
	for _, p := range paths {
		if !p.require {
			continue
		}
		b.Run("char/"+p.name, func(b *testing.B) { bench(b, p.avx2, p.avx512, char) })
		b.Run("words/"+p.name, func(b *testing.B) { bench(b, p.avx2, p.avx512, words) })
	}
}

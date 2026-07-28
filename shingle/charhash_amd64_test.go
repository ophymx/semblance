package shingle

import (
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/ophymx/semblance/internal/cpuinfo"
	"github.com/ophymx/semblance/internal/hashutil"
)

// TestCharHash8Kernels checks the lane-parallel window-hash kernels
// against xxhash.Sum64String on every window, across block counts and
// byte patterns (random, runs, all-zero) — both must be bit-identical.
func TestCharHash8Kernels(t *testing.T) {
	kernels := []struct {
		name    string
		fn      func(dst []uint64, text string)
		require bool
	}{
		{"avx512", charHash8AVX512, cpuinfo.HasAVX512},
		{"avx2", charHash8AVX2, cpuinfo.HasAVX2},
	}
	rng := hashutil.SplitMix64(61)
	patterns := map[string]func(i int) byte{
		"random": func(int) byte { return byte(rng.Next()) },
		"runs":   func(i int) byte { return byte(i / 7) },
		"zero":   func(int) byte { return 0 },
	}
	for _, k := range kernels {
		if !k.require {
			t.Logf("CPU lacks the feature for %s; skipping", k.name)
			continue
		}
		for name, gen := range patterns {
			for _, n := range []int{8, 16, 64, 256, 4096} {
				text := make([]byte, n+8)
				for i := range text {
					text[i] = gen(i)
				}
				s := string(text)
				dst := make([]uint64, n)
				k.fn(dst, s)
				for i, got := range dst {
					if want := xxhash.Sum64String(s[i : i+8]); got != want {
						t.Fatalf("%s/%s n=%d window %d: got %#x, want %#x", k.name, name, n, i, got, want)
					}
				}
			}
		}
	}
}

// TestCharDispatchBothPaths pins that Char(k=8) yields the identical hash
// sequence with each dispatch path forced in turn — scalar, AVX2, and
// AVX-512 — across lengths that exercise the block loop, the scalar tail,
// and the too-short bailout, and that stopping the iteration early works
// on every path.
func TestCharDispatchBothPaths(t *testing.T) {
	origAVX2, origAVX512 := useAVX2, useAVX512
	defer func() { useAVX2, useAVX512 = origAVX2, origAVX512 }()

	paths := []struct {
		name         string
		avx2, avx512 bool
		require      bool
	}{
		{"avx2", true, false, cpuinfo.HasAVX2},
		{"avx512", false, true, cpuinfo.HasAVX512},
	}
	rng := hashutil.SplitMix64(63)
	for _, n := range []int{0, 7, 8, 15, 16, 17, 263, 264, 265, 4096} {
		text := make([]byte, n)
		for i := range text {
			text[i] = byte(rng.Next())
		}
		s := string(text)

		useAVX2, useAVX512 = false, false
		var want []uint64
		for h := range Char(s, 8) {
			want = append(want, h)
		}

		for _, p := range paths {
			if !p.require {
				continue
			}
			useAVX2, useAVX512 = p.avx2, p.avx512
			var got []uint64
			for h := range Char(s, 8) {
				got = append(got, h)
			}
			if len(got) != len(want) {
				t.Fatalf("%s n=%d: got %d hashes, want %d", p.name, n, len(got), len(want))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("%s n=%d hash %d: got %#x, want %#x", p.name, n, i, got[i], want[i])
				}
			}

			// Early stop: taking m hashes yields the same prefix.
			if len(want) > 2 {
				m := len(want) / 2
				var prefix []uint64
				for h := range Char(s, 8) {
					prefix = append(prefix, h)
					if len(prefix) == m {
						break
					}
				}
				for i := range prefix {
					if prefix[i] != want[i] {
						t.Fatalf("%s n=%d early-stop hash %d: got %#x, want %#x", p.name, n, i, prefix[i], want[i])
					}
				}
			}
		}
	}
}

// BenchmarkCharHash8 is the go/no-go comparison for the kernel: the scalar
// per-window xxhash loop versus the AVX-512 lane-parallel kernel on the
// same 4 KB of text.
func BenchmarkCharHash8(b *testing.B) {
	rng := hashutil.SplitMix64(62)
	text := make([]byte, 4096+8)
	for i := range text {
		text[i] = byte(rng.Next())
	}
	s := string(text)
	const n = 4096
	dst := make([]uint64, n)
	b.Run("scalar", func(b *testing.B) {
		b.SetBytes(n)
		for b.Loop() {
			for i := 0; i < n; i++ {
				dst[i] = xxhash.Sum64String(s[i : i+8])
			}
		}
	})
	if cpuinfo.HasAVX2 {
		b.Run("avx2", func(b *testing.B) {
			b.SetBytes(n)
			for b.Loop() {
				charHash8AVX2(dst, s)
			}
		})
	}
	if cpuinfo.HasAVX512 {
		b.Run("avx512", func(b *testing.B) {
			b.SetBytes(n)
			for b.Loop() {
				charHash8AVX512(dst, s)
			}
		})
	}
}

package shingle

import (
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/ophymx/semblance/internal/cpuinfo"
	"github.com/ophymx/semblance/internal/hashutil"
)

// TestCharHash8AVX512 checks the lane-parallel window-hash kernel against
// xxhash.Sum64String on every window, across block counts and byte
// patterns (random, runs, all-zero) — the kernel must be bit-identical.
func TestCharHash8AVX512(t *testing.T) {
	if !cpuinfo.HasAVX512 {
		t.Skip("CPU lacks AVX-512")
	}
	rng := hashutil.SplitMix64(61)
	patterns := map[string]func(i int) byte{
		"random": func(int) byte { return byte(rng.Next()) },
		"runs":   func(i int) byte { return byte(i / 7) },
		"zero":   func(int) byte { return 0 },
	}
	for name, gen := range patterns {
		for _, n := range []int{8, 16, 64, 256, 4096} {
			text := make([]byte, n+8)
			for i := range text {
				text[i] = gen(i)
			}
			s := string(text)
			dst := make([]uint64, n)
			charHash8AVX512(dst, s)
			for i, got := range dst {
				if want := xxhash.Sum64String(s[i : i+8]); got != want {
					t.Fatalf("%s n=%d window %d: got %#x, want %#x", name, n, i, got, want)
				}
			}
		}
	}
}

// TestCharDispatchBothPaths pins that Char(k=8) yields the identical hash
// sequence with the vector path forced off and on, across lengths that
// exercise the block loop, the scalar tail, and the too-short bailout —
// and that stopping the iteration early works on both paths.
func TestCharDispatchBothPaths(t *testing.T) {
	orig := useAVX512
	defer func() { useAVX512 = orig }()

	rng := hashutil.SplitMix64(63)
	for _, n := range []int{0, 7, 8, 15, 16, 17, 263, 264, 265, 4096} {
		text := make([]byte, n)
		for i := range text {
			text[i] = byte(rng.Next())
		}
		s := string(text)

		useAVX512 = false
		var want []uint64
		for h := range Char(s, 8) {
			want = append(want, h)
		}

		if !cpuinfo.HasAVX512 {
			t.Skip("CPU lacks AVX-512")
		}
		useAVX512 = true
		var got []uint64
		for h := range Char(s, 8) {
			got = append(got, h)
		}
		if len(got) != len(want) {
			t.Fatalf("n=%d: got %d hashes, want %d", n, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("n=%d hash %d: got %#x, want %#x", n, i, got[i], want[i])
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
					t.Fatalf("n=%d early-stop hash %d: got %#x, want %#x", n, i, prefix[i], want[i])
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
	if cpuinfo.HasAVX512 {
		b.Run("avx512", func(b *testing.B) {
			b.SetBytes(n)
			for b.Loop() {
				charHash8AVX512(dst, s)
			}
		})
	}
}

package shingle

import (
	"slices"
	"testing"

	"github.com/ophymx/semblance/internal/cpuinfo"
	"github.com/ophymx/semblance/internal/hashutil"
)

// TestFoldShingles3Kernels checks the fold kernels against the scalar
// chain across block counts — bit-identical required.
func TestFoldShingles3Kernels(t *testing.T) {
	kernels := []struct {
		name    string
		fn      func(dst, tokens []uint64)
		require bool
	}{
		{"avx512", foldShingles3AVX512, cpuinfo.HasAVX512},
		{"avx2", foldShingles3AVX2, cpuinfo.HasAVX2},
	}
	rng := hashutil.SplitMix64(71)
	for _, k := range kernels {
		if !k.require {
			t.Logf("CPU lacks the feature for %s; skipping", k.name)
			continue
		}
		for _, n := range []int{8, 16, 64, 256, 4096} {
			tokens := make([]uint64, n+2)
			for i := range tokens {
				tokens[i] = rng.Next()
			}
			want := make([]uint64, n)
			got := make([]uint64, n)
			foldShingles3Scalar(want, tokens)
			k.fn(got, tokens)
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("%s n=%d shingle %d: got %#x, want %#x", k.name, n, i, got[i], want[i])
				}
			}
		}
	}
}

// genWordsText builds a deterministic multi-token document with roughly
// nTok tokens, including some uppercase and multi-byte tokens so the
// tokenizer's slow paths are exercised too.
func genWordsText(nTok int, seed uint64) string {
	vocab := []string{"alpha", "beta", "Gamma", "δέλτα", "x1", "longertoken", "q"}
	rng := hashutil.SplitMix64(seed)
	var out []byte
	for range nTok {
		out = append(out, vocab[rng.Next()%uint64(len(vocab))]...)
		out = append(out, ' ')
	}
	return string(out)
}

// TestWords3DispatchBothPaths pins that Words(w=3) and WordsBlocks(w=3)
// produce identical output — including WordsBlocks' flush cadence — with
// each dispatch path forced in turn (scalar, AVX2, AVX-512), across token
// counts spanning the drain boundary, plus early-stop behavior on each.
func TestWords3DispatchBothPaths(t *testing.T) {
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
	for _, nTok := range []int{0, 2, 3, 4, 255, 256, 257, 258, 259, 1000} {
		text := genWordsText(nTok, uint64(nTok))

		collect := func() (seq []uint64, blocks []int) {
			for h := range Words(text, 3) {
				seq = append(seq, h)
			}
			block := make([]uint64, 100) // odd size to stress cadence
			var fromBlocks []uint64
			WordsBlocks(text, 3, block, func(hashes []uint64) bool {
				fromBlocks = append(fromBlocks, hashes...)
				blocks = append(blocks, len(hashes))
				return true
			})
			if !slices.Equal(seq, fromBlocks) {
				t.Fatalf("nTok=%d: Words and WordsBlocks disagree", nTok)
			}
			return seq, blocks
		}

		useAVX2, useAVX512 = false, false
		wantSeq, wantBlocks := collect()

		for _, p := range paths {
			if !p.require {
				continue
			}
			useAVX2, useAVX512 = p.avx2, p.avx512
			gotSeq, gotBlocks := collect()

			if !slices.Equal(gotSeq, wantSeq) {
				t.Fatalf("%s nTok=%d: vector path shingle sequence differs", p.name, nTok)
			}
			if !slices.Equal(gotBlocks, wantBlocks) {
				t.Fatalf("%s nTok=%d: flush cadence differs: got %v, want %v", p.name, nTok, gotBlocks, wantBlocks)
			}

			// Early stop on each path: taking half the hashes yields the
			// same prefix, and a false-returning flush stops the scan.
			if len(wantSeq) > 2 {
				m := len(wantSeq) / 2
				var prefix []uint64
				for h := range Words(text, 3) {
					prefix = append(prefix, h)
					if len(prefix) == m {
						break
					}
				}
				if !slices.Equal(prefix, wantSeq[:m]) {
					t.Fatalf("%s nTok=%d: early-stop prefix differs", p.name, nTok)
				}
				calls := 0
				WordsBlocks(text, 3, make([]uint64, 8), func(hashes []uint64) bool {
					calls++
					return false
				})
				if calls != 1 {
					t.Fatalf("%s nTok=%d: flush called %d times after returning false", p.name, nTok, calls)
				}
			}
		}
	}
}

// BenchmarkFoldShingles3 is the go/no-go comparison (1.4x gate): scalar
// batch fold vs the AVX2 and AVX-512 kernels over 4096 shingles.
func BenchmarkFoldShingles3(b *testing.B) {
	rng := hashutil.SplitMix64(72)
	const n = 4096
	tokens := make([]uint64, n+2)
	for i := range tokens {
		tokens[i] = rng.Next()
	}
	dst := make([]uint64, n)
	b.Run("scalar", func(b *testing.B) {
		b.SetBytes(8 * n)
		for b.Loop() {
			foldShingles3Scalar(dst, tokens)
		}
	})
	if cpuinfo.HasAVX2 {
		b.Run("avx2", func(b *testing.B) {
			b.SetBytes(8 * n)
			for b.Loop() {
				foldShingles3AVX2(dst, tokens)
			}
		})
	}
	if cpuinfo.HasAVX512 {
		b.Run("avx512", func(b *testing.B) {
			b.SetBytes(8 * n)
			for b.Loop() {
				foldShingles3AVX512(dst, tokens)
			}
		})
	}
}

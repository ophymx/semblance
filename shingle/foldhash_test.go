package shingle

import (
	"fmt"
	"slices"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
)

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

// ringFold3 is the reference w=3 fold: the per-token-arrival ring fold
// that Words uses for every other w, inlined here so the batched path
// stays pinned to it on every platform.
func ringFold3(text string) []uint64 {
	var out []uint64
	var window [3]uint64
	n := 0
	tokenHashes(text, func(th uint64) bool {
		window[n%3] = th
		n++
		if n < 3 {
			return true
		}
		h := hashutil.MixInit
		for j := 0; j < 3; j++ {
			h = hashutil.Mix(h, window[(n+j)%3])
		}
		out = append(out, h)
		return true
	})
	return out
}

// TestWords3BatchMatchesRingFold pins the batched w=3 path — vector
// kernels on amd64, batch scalar fold elsewhere — to the ring fold,
// bit-identically, across token counts spanning the drain boundary,
// including WordsBlocks content and early-stop behavior on both entry
// points.
func TestWords3BatchMatchesRingFold(t *testing.T) {
	for _, nTok := range []int{0, 1, 2, 3, 4, 100, 255, 256, 257, 258, 259, 513, 1000} {
		t.Run(fmt.Sprintf("nTok=%d", nTok), func(t *testing.T) {
			text := genWordsText(nTok, uint64(nTok))
			want := ringFold3(text)

			var seq []uint64
			for h := range Words(text, 3) {
				seq = append(seq, h)
			}
			if !slices.Equal(seq, want) {
				t.Fatalf("Words(w=3) differs from ring fold: %d vs %d shingles", len(seq), len(want))
			}

			var fromBlocks []uint64
			block := make([]uint64, 100) // odd size to stress cadence
			WordsBlocks(text, 3, block, func(hashes []uint64) bool {
				fromBlocks = append(fromBlocks, hashes...)
				return true
			})
			if !slices.Equal(fromBlocks, want) {
				t.Fatalf("WordsBlocks(w=3) differs from ring fold")
			}

			if len(want) > 2 {
				m := len(want) / 2
				var prefix []uint64
				for h := range Words(text, 3) {
					prefix = append(prefix, h)
					if len(prefix) == m {
						break
					}
				}
				if !slices.Equal(prefix, want[:m]) {
					t.Fatalf("early-stop prefix differs")
				}
				calls := 0
				WordsBlocks(text, 3, make([]uint64, 8), func(hashes []uint64) bool {
					calls++
					return false
				})
				if calls != 1 {
					t.Fatalf("flush called %d times after returning false", calls)
				}
			}
		})
	}
}

// TestWords3NoExtraAllocs pins that the batching state stays on the
// stack: the w=3 path must not allocate more than the ring-fold path
// (one window allocation) it replaced.
func TestWords3NoExtraAllocs(t *testing.T) {
	text := genWordsText(300, 7) // spans one full drain
	drain := func(w int) float64 {
		return testing.AllocsPerRun(100, func() {
			for range Words(text, w) {
				_ = struct{}{}
			}
		})
	}
	if w3, w4 := drain(3), drain(4); w3 > w4 {
		t.Errorf("Words(w=3) allocates more than the ring fold: %v > %v", w3, w4)
	}
}

// BenchmarkWords3 measures the full w=3 pipeline (tokenize + batched
// fold) against the w=4 ring-fold pipeline as an in-context reference.
func BenchmarkWords3(b *testing.B) {
	text := genWordsText(20000, 9)
	for _, w := range []int{3, 4} {
		b.Run(fmt.Sprintf("w=%d", w), func(b *testing.B) {
			b.SetBytes(int64(len(text)))
			for b.Loop() {
				for range Words(text, w) {
					_ = struct{}{}
				}
			}
		})
	}
}

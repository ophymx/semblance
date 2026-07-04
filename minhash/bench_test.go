package minhash_test

import (
	"strings"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/minhash"
	"github.com/ophymx/semblance/shingle"
)

// genDoc builds a deterministic pseudo-random document of roughly n bytes.
func genDoc(n int) string {
	vocab := []string{
		"the", "quick", "brown", "fox", "jumps", "over", "lazy", "dog",
		"minhash", "signature", "jaccard", "similarity", "estimate", "shingle",
		"deterministic", "heuristic", "sketch", "banding", "candidate", "index",
	}
	var sb strings.Builder
	sb.Grow(n + 16)
	rng := hashutil.SplitMix64(1)
	for sb.Len() < n {
		sb.WriteString(vocab[rng.Next()%uint64(len(vocab))])
		sb.WriteByte(' ')
	}
	return sb.String()
}

// TestSketchIntoAllocs pins the per-document allocation constants promised
// by the spec: O(1) small allocations per document (iterator closures and
// reusable buffers), zero allocations per shingle. The constants are exact
// so any regression — including one that scales with document size — fails.
func TestSketchIntoAllocs(t *testing.T) {
	m := minhash.New(128, 0)
	dst := make(minhash.Signature, 128)
	for _, size := range []int{1 << 10, 100 << 10} {
		doc := genDoc(size)
		charAllocs := testing.AllocsPerRun(10, func() {
			m.SketchInto(dst, shingle.Char(doc, 8))
		})
		wordAllocs := testing.AllocsPerRun(10, func() {
			m.SketchInto(dst, shingle.Words(doc, 3))
		})
		// Measured: the shingler's iterator closure, SketchInto's yield
		// closure, and the 2 KB block buffer (heap-escaped because the
		// range-over-func body captures it). Exact so any regression
		// fails, including one that scales with document size. A compiler
		// upgrade changing inlining may legitimately move this constant.
		if charAllocs != 3 {
			t.Errorf("char pipeline at %dB: %v allocs/op, want 3", size, charAllocs)
		}
		if wordAllocs != 3 {
			t.Errorf("word pipeline at %dB: %v allocs/op, want 3", size, wordAllocs)
		}
		t.Logf("%dB doc: char=%v word=%v allocs/op", size, charAllocs, wordAllocs)
	}
}

func benchmarkSketchInto(b *testing.B, doc string, mode string) {
	m := minhash.New(128, 0)
	dst := make(minhash.Signature, 128)
	b.SetBytes(int64(len(doc)))
	b.ReportAllocs()
	for b.Loop() {
		switch mode {
		case "char":
			m.SketchInto(dst, shingle.Char(doc, 8))
		case "words":
			m.SketchInto(dst, shingle.Words(doc, 3))
		}
	}
}

func BenchmarkSketchIntoChar1KB(b *testing.B)    { benchmarkSketchInto(b, genDoc(1<<10), "char") }
func BenchmarkSketchIntoChar100KB(b *testing.B)  { benchmarkSketchInto(b, genDoc(100<<10), "char") }
func BenchmarkSketchIntoWords1KB(b *testing.B)   { benchmarkSketchInto(b, genDoc(1<<10), "words") }
func BenchmarkSketchIntoWords100KB(b *testing.B) { benchmarkSketchInto(b, genDoc(100<<10), "words") }

package simhash_test

import (
	"strings"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/simhash"
)

// genDoc builds a deterministic pseudo-random document of roughly n bytes.
func genDoc(n int) string {
	vocab := []string{
		"the", "quick", "brown", "fox", "jumps", "over", "lazy", "dog",
		"simhash", "fingerprint", "hamming", "distance", "cosine", "shingle",
		"deterministic", "heuristic", "sketch", "feature", "weighted", "bits",
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

// TestSketchTextAllocs pins the per-document allocation constant: iterator
// and adapter closures only, independent of document size.
func TestSketchTextAllocs(t *testing.T) {
	for _, size := range []int{1 << 10, 100 << 10} {
		doc := genDoc(size)
		allocs := testing.AllocsPerRun(10, func() {
			simhash.SketchText(doc, 3)
		})
		if allocs != 3 {
			t.Errorf("SketchText at %dB: %v allocs/op, want 3", size, allocs)
		}
		t.Logf("%dB doc: %v allocs/op", size, allocs)
	}
}

func benchmarkSketchText(b *testing.B, doc string) {
	b.SetBytes(int64(len(doc)))
	b.ReportAllocs()
	for b.Loop() {
		simhash.SketchText(doc, 3)
	}
}

func BenchmarkSketchText1KB(b *testing.B)   { benchmarkSketchText(b, genDoc(1<<10)) }
func BenchmarkSketchText100KB(b *testing.B) { benchmarkSketchText(b, genDoc(100<<10)) }

func BenchmarkDistance(b *testing.B) {
	var sink int
	for b.Loop() {
		sink += simhash.Distance(0xDEADBEEFCAFEF00D, 0x0123456789ABCDEF)
	}
	_ = sink
}

package semblance_test

import (
	"strings"
	"testing"

	semblance "github.com/ophymx/semblance"
	"github.com/ophymx/semblance/minhash"
	"github.com/ophymx/semblance/shingle"
)

func genDoc(n int) string {
	vocab := []string{
		"the", "quick", "brown", "fox", "jumps", "over", "lazy", "dog",
		"minhash", "signature", "jaccard", "similarity", "estimate", "shingle",
		"deterministic", "heuristic", "sketch", "banding", "candidate", "index",
	}
	var sb strings.Builder
	sb.Grow(n + 16)
	seed := uint64(1)
	for sb.Len() < n {
		seed += 0x9E3779B97F4A7C15
		z := seed
		z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
		z = (z ^ (z >> 27)) * 0x94D049BB133111EB
		sb.WriteString(vocab[(z^(z>>31))%uint64(len(vocab))])
		sb.WriteByte(' ')
	}
	return sb.String()
}

// BenchmarkSketchIntoFused measures the Sketcher's fused block path against
// the composable iterator path over the same hasher and document.
func BenchmarkSketchIntoFused(b *testing.B) {
	doc := genDoc(100 << 10)
	sk := semblance.NewSketcher(semblance.Defaults())
	dst := make(minhash.Signature, semblance.Defaults().K)
	b.SetBytes(int64(len(doc)))
	b.ReportAllocs()
	for b.Loop() {
		sk.SketchInto(dst, doc)
	}
}

func BenchmarkSketchIntoIterator(b *testing.B) {
	doc := genDoc(100 << 10)
	m := minhash.New(128, 0)
	dst := make(minhash.Signature, 128)
	b.SetBytes(int64(len(doc)))
	b.ReportAllocs()
	for b.Loop() {
		m.SketchInto(dst, shingle.Words(doc, 3))
	}
}

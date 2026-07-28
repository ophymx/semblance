package winnow_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/winnow"
)

const (
	benchK = 8 // k=8 hits shingle.Char's vectorized window-hash path
	benchW = 6
)

// genDoc builds a deterministic high-entropy document of roughly n bytes:
// space-separated base-36 tokens drawn from a seeded PRNG. High entropy is
// deliberate — real prose has thousands of distinct byte k-grams, so its
// winnowing fingerprints rarely collide; a tiny fixed vocabulary would
// instead make "distinct" documents share most fingerprints and push the
// overlap benchmarks into an unrepresentative near-worst case. Varying the
// seed lets two documents share a planted passage and nothing else.
func genDoc(n int, seed uint64) string {
	var sb strings.Builder
	sb.Grow(n + 16)
	rng := hashutil.SplitMix64(seed)
	for sb.Len() < n {
		sb.WriteString(strconv.FormatUint(rng.Next(), 36))
		sb.WriteByte(' ')
	}
	return sb.String()
}

func drainText(text string) int {
	n := 0
	for range winnow.Text(text, benchK, benchW) {
		n++
	}
	return n
}

func benchmarkText(b *testing.B, doc string) {
	b.SetBytes(int64(len(doc)))
	for b.Loop() {
		drainText(doc)
	}
}

// BenchmarkText measures the winnowing scan itself — byte k-gram hashing
// plus the windowed minimum selection — the cost every winnow operation
// pays first.
func BenchmarkText1KB(b *testing.B)   { benchmarkText(b, genDoc(1<<10, 1)) }
func BenchmarkText100KB(b *testing.B) { benchmarkText(b, genDoc(100<<10, 1)) }

// BenchmarkTextParallel saturates all cores on the scan (shared read-only
// document); run with -cpu to compare scaling.
func BenchmarkTextParallel(b *testing.B) {
	doc := genDoc(100<<10, 1)
	b.SetBytes(int64(len(doc)))
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			drainText(doc)
		}
	})
}

// BenchmarkOverlaps measures pairwise span localization on two 100 KB
// documents that share a planted ~4 KB passage (a realistic near-duplicate
// shape) — the scan of both plus the diagonal grouping.
func BenchmarkOverlaps(b *testing.B) {
	shared := genDoc(4<<10, 99)
	a := genDoc(48<<10, 1) + shared + genDoc(48<<10, 2)
	bb := genDoc(48<<10, 3) + shared + genDoc(48<<10, 4)
	b.SetBytes(int64(len(a) + len(bb)))
	for b.Loop() {
		winnow.Overlaps(a, bb, benchK, benchW)
	}
}

// BenchmarkIndexAdd measures indexing throughput: the scan plus posting
// inserts for one 100 KB document into a fresh index.
func BenchmarkIndexAdd(b *testing.B) {
	doc := genDoc(100<<10, 1)
	b.SetBytes(int64(len(doc)))
	b.ReportAllocs()
	for b.Loop() {
		ix := winnow.NewIndex(benchK, benchW)
		ix.Add("d", doc)
	}
}

// buildIndex returns an index of nDocs documents of docSize bytes, each
// carrying a shared ~512 B passage, plus a query document that shares that
// passage with every indexed document — so the query hits every posting.
func buildIndex(nDocs, docSize int) (*winnow.Index, string) {
	shared := " " + genDoc(512, 99) + " "
	ix := winnow.NewIndex(benchK, benchW)
	for i := 0; i < nDocs; i++ {
		ix.Add(strconv.Itoa(i), genDoc(docSize, uint64(i+1))+shared)
	}
	return ix, genDoc(docSize, 424242) + shared
}

// BenchmarkIndexOverlap measures the bounded per-document scoring query
// against a 100-document index.
func BenchmarkIndexOverlap(b *testing.B) {
	ix, query := buildIndex(100, 2<<10)
	b.SetBytes(int64(len(query)))
	for b.Loop() {
		ix.Overlap(query)
	}
}

// BenchmarkIndexMatches measures the span-localization query (per-document
// aligned passages) against the same index.
func BenchmarkIndexMatches(b *testing.B) {
	ix, query := buildIndex(100, 2<<10)
	b.SetBytes(int64(len(query)))
	for b.Loop() {
		ix.Matches(query)
	}
}

// BenchmarkIndexOverlapParallel saturates all cores on the scoring query
// (shared read-only index and query); run with -cpu to compare scaling.
func BenchmarkIndexOverlapParallel(b *testing.B) {
	ix, query := buildIndex(100, 2<<10)
	b.SetBytes(int64(len(query)))
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ix.Overlap(query)
		}
	})
}

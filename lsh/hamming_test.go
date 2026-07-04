package lsh_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/ophymx/semblance/lsh"
	"github.com/ophymx/semblance/simhash"
)

func TestNewHammingIndexPanics(t *testing.T) {
	for _, d := range []int{0, -1, 4, 64} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("NewHammingIndex(%d) did not panic", d)
				}
			}()
			lsh.NewHammingIndex(d)
		}()
	}
}

func TestMaxDist(t *testing.T) {
	if got := lsh.NewHammingIndex(2).MaxDist(); got != 2 {
		t.Errorf("MaxDist() = %d, want 2", got)
	}
}

// flip returns fp with the given bit positions inverted.
func flip(fp simhash.Fingerprint, bits ...int) simhash.Fingerprint {
	for _, b := range bits {
		fp ^= 1 << b
	}
	return fp
}

func TestHammingQueryWithinDistance(t *testing.T) {
	const base simhash.Fingerprint = 0xDEADBEEFCAFEF00D
	for maxDist := 1; maxDist <= 3; maxDist++ {
		t.Run(fmt.Sprintf("maxDist=%d", maxDist), func(t *testing.T) {
			ix := lsh.NewHammingIndex(maxDist)
			ix.Add("base", base)

			// Spread flipped bits across different 16-bit blocks (worst
			// case for pigeonhole) and also concentrate them in one block.
			spreads := [][]int{
				{},               // distance 0
				{3},              // 1
				{3, 20},          // 2, two blocks
				{3, 20, 40},      // 3, three blocks
				{3, 20, 40, 60},  // 4, four blocks
				{0, 1},           // 2, same block
				{0, 1, 2},        // 3, same block
				{0, 1, 2, 3},     // 4, same block
				{15, 16},         // 2, straddling a block boundary
				{15, 16, 31, 32}, // 4, straddling two boundaries
			}
			for _, bits := range spreads {
				q := flip(base, bits...)
				want := len(bits) <= maxDist
				got := slices.Contains(ix.Query(q), "base")
				if got != want {
					t.Errorf("query at distance %d (bits %v): found=%v, want %v",
						len(bits), bits, got, want)
				}
			}
		})
	}
}

func TestHammingNoFalsePositives(t *testing.T) {
	ix := lsh.NewHammingIndex(3)
	// Same low 16-bit block as the query, but distance 48: a block-table
	// hit that verification must reject.
	ix.Add("far", 0xFFFFFFFFFFFF1234)
	if got := ix.Query(0x0000000000001234); got != nil {
		t.Errorf("Query returned unverified candidate: %v", got)
	}
}

func TestHammingDedupAndOrder(t *testing.T) {
	const base simhash.Fingerprint = 0x0123456789ABCDEF
	ix := lsh.NewHammingIndex(2)
	ix.Add("a", base) // matches on every block
	ix.Add("a", base)
	ix.Add("b", flip(base, 7))      // differs in block 0, first agrees on block 1
	ix.Add("c", flip(base, 50))     // differs in block 2, agrees on block 0
	want := []string{"a", "c", "b"} // first-matching-block order, then insertion
	for range 10 {
		if got := ix.Query(base); !slices.Equal(got, want) {
			t.Fatalf("Query = %v, want %v", got, want)
		}
	}
}

func TestHammingEmptyIndex(t *testing.T) {
	if got := lsh.NewHammingIndex(3).Query(42); got != nil {
		t.Errorf("Query on empty index = %v, want nil", got)
	}
}

func TestHammingEndToEnd(t *testing.T) {
	ix := lsh.NewHammingIndex(3)
	docs := map[string]string{
		"orig":      "the quick brown fox jumps over the lazy dog and runs far away into the deep dark forest tonight",
		"nearcopy":  "the quick brown fox jumps over the lazy dog and runs far away into the deep dark forest today",
		"unrelated": "completely different content about minhash signatures and locality sensitive hashing indexes here",
	}
	for id, text := range docs {
		ix.Add(id, simhash.SketchText(text, 3))
	}
	got := ix.Query(simhash.SketchText(docs["orig"], 3))
	if !slices.Contains(got, "orig") {
		t.Errorf("query missed exact fingerprint: %v", got)
	}
	if slices.Contains(got, "unrelated") {
		t.Errorf("query matched unrelated document: %v", got)
	}
	// nearcopy may or may not fall within distance 3; just verify anything
	// returned is genuinely within the bound.
	for _, id := range got {
		d := simhash.Distance(simhash.SketchText(docs[id], 3), simhash.SketchText(docs["orig"], 3))
		if d > 3 {
			t.Errorf("returned %q at distance %d > 3", id, d)
		}
	}
}

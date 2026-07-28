package winnow

import (
	"slices"
	"strings"
	"testing"
)

// TestOverlapsCap verifies the MaxResults cap bounds the quadratic blowup
// on highly repetitive input while leaving normal inputs complete and
// still-sorted.
func TestOverlapsCap(t *testing.T) {
	// Repetitive: without the cap this is ~0.98*n^2 pairs (n^2 > MaxResults
	// for n>~1035); the cap must hold it at exactly MaxResults.
	rep := strings.Repeat("a", 2000)
	got := Overlaps(rep, rep, 5, 4)
	if len(got) != MaxResults {
		t.Errorf("repetitive Overlaps: len = %d, want cap %d", len(got), MaxResults)
	}
	if !slices.IsSortedFunc(got, func(x, y Shared) int {
		if x.PosA != y.PosA {
			return cmpInt(x.PosA, y.PosA)
		}
		return cmpInt(x.PosB, y.PosB)
	}) {
		t.Error("capped result is not sorted")
	}

	// Normal distinct text: well under the cap, results unaffected.
	a := "the quick brown fox jumps over the lazy dog and runs away fast"
	small := Overlaps(a, a, 5, 4)
	if len(small) == 0 || len(small) >= MaxResults {
		t.Errorf("normal Overlaps: len = %d, want (0, cap)", len(small))
	}
}

// TestMatchesCap verifies the same cap on the index path.
func TestMatchesCap(t *testing.T) {
	ix := NewIndex(5, 4)
	ix.Add("doc", strings.Repeat("a", 2000))
	got := ix.Matches(strings.Repeat("a", 2000))
	if len(got) != MaxResults {
		t.Errorf("repetitive Matches: len = %d, want cap %d", len(got), MaxResults)
	}
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

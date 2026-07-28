package winnow

import (
	"slices"
	"strings"
	"testing"
)

// TestOverlapsCap verifies MaxResults bounds the work (and thus the span
// count) on highly repetitive input whose raw match set would be quadratic,
// while leaving normal inputs complete and still sorted.
func TestOverlapsCap(t *testing.T) {
	// Repetitive: the raw match set is ~n^2 (>MaxResults for n>~1035). The
	// cap must hold the returned spans at or under MaxResults and, more to
	// the point, return promptly rather than materializing the cross product.
	rep := strings.Repeat("a", 2000)
	got := Overlaps(rep, rep, 5, 4)
	if len(got) > MaxResults {
		t.Errorf("repetitive Overlaps: len = %d, exceeds cap %d", len(got), MaxResults)
	}
	if !slices.IsSortedFunc(got, spanOrder) {
		t.Error("capped result is not sorted")
	}

	// Normal distinct text: well under the cap, results complete.
	a := "the quick brown fox jumps over the lazy dog and runs away fast"
	small := Overlaps(a, a, 5, 4)
	if len(small) == 0 || len(small) >= MaxResults {
		t.Errorf("normal Overlaps: len = %d, want (0, cap)", len(small))
	}
}

// TestMatchesCap verifies the same bound on the index path.
func TestMatchesCap(t *testing.T) {
	ix := NewIndex(5, 4)
	ix.Add("doc", strings.Repeat("a", 2000))
	got := ix.Matches(strings.Repeat("a", 2000))
	if len(got) > MaxResults {
		t.Errorf("repetitive Matches: len = %d, exceeds cap %d", len(got), MaxResults)
	}
}

func spanOrder(x, y Span) int {
	if x.PosA != y.PosA {
		return cmpInt(x.PosA, y.PosA)
	}
	return cmpInt(x.PosB, y.PosB)
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

package winnow

import (
	"cmp"
	"iter"
	"slices"
)

// Shared is one winnowing fingerprint common to two texts: the same k-gram
// appears at PosA in the first text and PosB in the second. A run of Shared
// values with a constant PosB-PosA offset localizes a contiguous shared
// passage in both.
type Shared struct {
	Hash uint64
	PosA int
	PosB int
}

// Overlaps returns the winnowing fingerprints two texts share, as
// (PosA, PosB) position pairs sorted by PosA then PosB. It is the pairwise
// form of [Index] — for comparing two known documents directly (a reply
// against its parent, two files for plagiarism) without building an index.
// A fingerprint occurring at several positions in either text yields one
// Shared per (PosA, PosB) combination. Texts sharing a run of w+k-1 or more
// bytes share at least one fingerprint. Returns nil if there are none.
// Panics if k <= 0 or w <= 0.
func Overlaps(a, b string, k, w int) []Shared {
	return overlaps(Text(a, k, w), Text(b, k, w))
}

// OverlapsBytes is [Overlaps] for byte slices, without copying. The slices
// must not be mutated until it returns.
func OverlapsBytes(a, b []byte, k, w int) []Shared {
	return overlaps(TextBytes(a, k, w), TextBytes(b, k, w))
}

func overlaps(fa, fb iter.Seq[Fingerprint]) []Shared {
	posA := make(map[uint64][]int)
	for fp := range fa {
		posA[fp.Hash] = append(posA[fp.Hash], fp.Pos)
	}
	var out []Shared
	for fp := range fb {
		for _, pa := range posA[fp.Hash] {
			out = append(out, Shared{Hash: fp.Hash, PosA: pa, PosB: fp.Pos})
		}
	}
	slices.SortFunc(out, func(x, y Shared) int {
		if x.PosA != y.PosA {
			return cmp.Compare(x.PosA, y.PosA)
		}
		return cmp.Compare(x.PosB, y.PosB)
	})
	return out
}

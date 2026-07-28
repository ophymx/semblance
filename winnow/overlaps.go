package winnow

import (
	"cmp"
	"iter"
	"slices"
)

// Span is a maximal aligned passage two texts share: a run of matching
// fingerprints on one diagonal (constant PosB−PosA offset) collapsed into a
// single region. The shared passage begins at byte PosA in the first text
// and PosB in the second and runs Len bytes, so text_a[PosA:PosA+Len] and
// text_b[PosB:PosB+Len] are the aligned regions. A shared paragraph is one
// Span, not one entry per fingerprint.
//
// Winnowing is a heuristic sketch: the fingerprints bounding the span are
// byte-equal k-grams and the run localizes where the texts overlap, but
// interior bytes are not guaranteed identical — treat Span as a located
// region, not a proof of byte equality. Edges land within about w+k bytes
// of the true passage boundaries.
type Span struct {
	PosA int // byte offset of the shared region in the first text
	PosB int // byte offset of the shared region in the second text
	Len  int // byte length of the shared region
}

// Overlaps returns the aligned passages two texts share, as Spans sorted by
// PosA then PosB. It is the pairwise form of [Index] — for comparing two
// known documents directly (a reply against its parent, a document against
// a boilerplate template) without building an index. A shared run of w+k−1
// or more bytes yields at least one Span. Returns nil if there are none.
//
// A fingerprint occurring at several positions in the first text yields one
// Span per alignment. At most [MaxResults] spans are returned, and the work
// is bounded likewise, so highly repetitive input cannot blow up (see
// [MaxResults]). Panics if k <= 0 or w <= 0.
func Overlaps(a, b string, k, w int) []Span {
	return overlaps(Text(a, k, w), Text(b, k, w), k)
}

// OverlapsBytes is [Overlaps] for byte slices, without copying. The slices
// must not be mutated until it returns.
func OverlapsBytes(a, b []byte, k, w int) []Span {
	return overlaps(TextBytes(a, k, w), TextBytes(b, k, w), k)
}

// overlaps groups the shared fingerprints of two texts into diagonal runs.
// Walking the second text's fingerprints in order, each match to the first
// text lands on a diagonal keyed by its offset (aPos − bPos); consecutive
// query fingerprints on the same diagonal extend the same run. Because
// identical text winnows to identical fingerprints, a contiguous shared
// passage is exactly one such run.
func overlaps(fa, fb iter.Seq[Fingerprint], k int) []Span {
	posA := make(map[uint64][]int) // first-text positions per hash, in order
	for f := range fa {
		posA[f.Hash] = append(posA[f.Hash], f.Pos)
	}

	type run struct{ aStart, bStart, aEnd, lastJ int }
	active := make(map[int]*run) // keyed by diagonal offset aPos − bPos
	var out []Span
	emit := func(r *run) {
		out = append(out, Span{PosA: r.aStart, PosB: r.bStart, Len: r.aEnd + k - r.aStart})
	}

	budget := MaxResults
	j := 0
loop:
	for f := range fb {
		for _, aPos := range posA[f.Hash] {
			if budget <= 0 {
				break loop
			}
			budget--
			off := aPos - f.Pos
			if r, ok := active[off]; ok && r.lastJ == j-1 {
				r.aEnd, r.lastJ = aPos, j // extend the open run
			} else {
				if ok {
					emit(r) // the run on this diagonal ended earlier
				}
				active[off] = &run{aStart: aPos, bStart: f.Pos, aEnd: aPos, lastJ: j}
			}
		}
		j++
	}
	for _, r := range active {
		emit(r)
	}
	slices.SortFunc(out, func(x, y Span) int {
		if x.PosA != y.PosA {
			return cmp.Compare(x.PosA, y.PosA)
		}
		return cmp.Compare(x.PosB, y.PosB)
	})
	return out
}

package winnow_test

import (
	"fmt"

	"github.com/ophymx/semblance/winnow"
)

// Winnowing tells you where documents overlap, not just how much: index
// one document's fingerprints by hash, look up the other's, and the
// matching positions localize the shared passage in both. Fingerprints
// are sparse samples, so the located span is approximate — its edges land
// within about w+k bytes of the true boundaries.
func Example() {
	const k, w = 8, 6 // guarantee: shared runs of w+k-1 = 13+ bytes match
	a := "original reporting: the council voted 7-2 to approve the marina expansion late tuesday."
	b := "REPOST >> the council voted 7-2 to approve the marina expansion (saw this yesterday)"

	posInA := map[uint64]int{}
	for fp := range winnow.Text(a, k, w) {
		posInA[fp.Hash] = fp.Pos
	}

	first, last := -1, -1
	for fp := range winnow.Text(b, k, w) {
		if pa, ok := posInA[fp.Hash]; ok {
			if first < 0 {
				first = pa
			}
			last = pa + k
		}
	}
	fmt.Printf("shared passage in a: %q\n", a[first:last])
	// Output:
	// shared passage in a: "e council voted 7-2 to approve the marina expansion"
}

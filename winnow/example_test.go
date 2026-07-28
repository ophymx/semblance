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

// Index gives fragment-level provenance: which indexed documents a suspect
// overlaps, and where. Here a "new" article is caught reusing a passage
// from an earlier one, with the borrowed span localized in both.
func ExampleIndex() {
	const k, w = 8, 6
	ix := winnow.NewIndex(k, w)
	ix.Add("wire-story", "City council approves the marina expansion project after a seven to two vote late Tuesday.")
	ix.Add("gardening", "This week in the garden: pruning roses and preparing beds for the coming autumn frost.")

	suspect := "BREAKING (via tipster): approves the marina expansion project after a seven to two vote — developing."

	// Which document does it overlap most?
	top := ix.Overlap(suspect)[0]
	fmt.Printf("overlaps %q on %d fingerprints\n", top.ID, top.Shared)

	// Localize the borrowed span within the suspect: Matches returns it as
	// one aligned region, ready to slice.
	m := ix.Matches(suspect)[0]
	fmt.Printf("borrowed: %q\n", suspect[m.QueryPos:m.QueryPos+m.Len])
	// Output:
	// overlaps "wire-story" on 14 fingerprints
	// borrowed: " approves the marina expansion project after a seven to two vo"
}

// Overlaps compares two known documents directly, without an index —
// here localizing the passage a paraphrase borrowed verbatim.
func ExampleOverlaps() {
	a := "the quarterly report shows that revenue climbed twelve percent across every region"
	b := "as we noted last week, revenue climbed twelve percent across every region this year"

	// The shared passage comes back as one aligned span; slice it directly.
	s := winnow.Overlaps(a, b, 8, 6)[0]
	fmt.Printf("%q\n", b[s.PosB:s.PosB+s.Len])
	// Output:
	// "venue climbed twelve percent across every r"
}

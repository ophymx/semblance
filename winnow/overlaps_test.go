package winnow_test

import (
	"cmp"
	"slices"
	"strings"
	"testing"

	"github.com/ophymx/semblance/winnow"
)

const (
	ovK = 8
	ovW = 6
)

func hashSet(text string) map[uint64]bool {
	s := map[uint64]bool{}
	for fp := range winnow.Text(text, ovK, ovW) {
		s[fp.Hash] = true
	}
	return s
}

func TestOverlapsSoundAndComplete(t *testing.T) {
	passage := "the marina expansion vote passed seven to two late on tuesday evening downtown here"
	a := "padding text before the good part. " + passage + ". unique tail alpha."
	b := "an entirely different opening line >> " + passage + " ... distinct ending beta."

	got := winnow.Overlaps(a, b, ovK, ovW)
	if len(got) == 0 {
		t.Fatal("no overlaps for a shared passage")
	}

	// Sound: every pair points at byte-equal k-grams in both texts.
	for _, s := range got {
		if a[s.PosA:s.PosA+ovK] != b[s.PosB:s.PosB+ovK] {
			t.Errorf("pair a=%d b=%d: %q != %q", s.PosA, s.PosB,
				a[s.PosA:s.PosA+ovK], b[s.PosB:s.PosB+ovK])
		}
	}

	// Complete: distinct shared hashes == the fingerprint-hash intersection.
	aSet, bSet := hashSet(a), hashSet(b)
	want := map[uint64]bool{}
	for h := range aSet {
		if bSet[h] {
			want[h] = true
		}
	}
	gotHashes := map[uint64]bool{}
	for _, s := range got {
		gotHashes[s.Hash] = true
	}
	if len(gotHashes) != len(want) {
		t.Errorf("distinct shared hashes = %d, intersection = %d", len(gotHashes), len(want))
	}
	for h := range want {
		if !gotHashes[h] {
			t.Errorf("missing shared hash %#x", h)
		}
	}

	// Sorted by (PosA, PosB) and deterministic.
	if !slices.IsSortedFunc(got, func(x, y winnow.Shared) int {
		if x.PosA != y.PosA {
			return cmp.Compare(x.PosA, y.PosA)
		}
		return cmp.Compare(x.PosB, y.PosB)
	}) {
		t.Error("result not sorted by (PosA, PosB)")
	}
	if !slices.Equal(got, winnow.Overlaps(a, b, ovK, ovW)) {
		t.Error("Overlaps not deterministic across calls")
	}

	// Localizes the passage in b.
	lo, hi := got[0].PosB, got[0].PosB+ovK
	for _, s := range got {
		lo = min(lo, s.PosB)
		hi = max(hi, s.PosB+ovK)
	}
	if !strings.Contains(b[lo:hi], "marina expansion vote") {
		t.Errorf("localized span %q missing passage core", b[lo:hi])
	}
}

func TestOverlapsRepeatedPassage(t *testing.T) {
	phrase := "this exact phrase repeats verbatim in the same document twice over here now"
	a := phrase + " ... some filler between the two copies ... " + phrase
	b := "prefix line >> " + phrase + " << suffix line"

	got := winnow.Overlaps(a, b, ovK, ovW)
	// A fingerprint in b that occurs twice in a must appear as two pairs
	// sharing PosB with distinct PosA.
	byB := map[int][]int{}
	for _, s := range got {
		byB[s.PosB] = append(byB[s.PosB], s.PosA)
	}
	twice := false
	for _, as := range byB {
		if len(as) >= 2 {
			twice = true
		}
	}
	if !twice {
		t.Error("expected a b-fingerprint to match two positions in a (repeated passage)")
	}
}

func TestOverlapsDisjoint(t *testing.T) {
	a := "completely unrelated content about gardening roses and winter frost protection methods"
	b := "an article discussing rowing championships along the north waterfront this weekend event"
	if got := winnow.Overlaps(a, b, ovK, ovW); got != nil {
		t.Errorf("disjoint texts overlapped: %v", got)
	}
}

func TestOverlapsBytesEquivalence(t *testing.T) {
	a := "shared winnowing fingerprints localize passages précisément between the two texts"
	b := "these winnowing fingerprints localize passages précisément across two documents here"
	if !slices.Equal(winnow.Overlaps(a, b, ovK, ovW), winnow.OverlapsBytes([]byte(a), []byte(b), ovK, ovW)) {
		t.Error("OverlapsBytes diverges from Overlaps")
	}
}

func TestOverlapsPanics(t *testing.T) {
	for name, fn := range map[string]func(){
		"k=0": func() { winnow.Overlaps("abc", "abc", 0, 4) },
		"w=0": func() { winnow.Overlaps("abc", "abc", 4, 0) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("expected panic, got none")
				}
			}()
			fn()
		})
	}
}

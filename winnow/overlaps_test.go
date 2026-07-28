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

func TestOverlapsSoundAndComplete(t *testing.T) {
	passage := "the marina expansion vote passed seven to two late on tuesday evening downtown here"
	a := "padding text before the good part. " + passage + ". unique tail alpha."
	b := "an entirely different opening line >> " + passage + " ... distinct ending beta."

	got := winnow.Overlaps(a, b, ovK, ovW)
	if len(got) == 0 {
		t.Fatal("no overlaps for a shared passage")
	}

	// Sound: each span's bounding k-grams are byte-equal in both texts, and
	// the offset PosB-PosA is constant across the aligned region.
	for _, s := range got {
		if a[s.PosA:s.PosA+ovK] != b[s.PosB:s.PosB+ovK] {
			t.Errorf("span a=%d b=%d: start k-grams differ: %q != %q",
				s.PosA, s.PosB, a[s.PosA:s.PosA+ovK], b[s.PosB:s.PosB+ovK])
		}
		ea, eb := s.PosA+s.Len-ovK, s.PosB+s.Len-ovK
		if a[ea:ea+ovK] != b[eb:eb+ovK] {
			t.Errorf("span a=%d b=%d len=%d: end k-grams differ", s.PosA, s.PosB, s.Len)
		}
	}

	// The single shared passage collapses to one span (one diagonal), not
	// one entry per fingerprint.
	if len(got) != 1 {
		t.Errorf("shared passage produced %d spans, want 1", len(got))
	}

	// Sorted by (PosA, PosB) and deterministic.
	if !slices.IsSortedFunc(got, func(x, y winnow.Span) int {
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

	// The span localizes the passage in b.
	s := got[0]
	if !strings.Contains(b[s.PosB:s.PosB+s.Len], "marina expansion vote") {
		t.Errorf("localized span %q missing passage core", b[s.PosB:s.PosB+s.Len])
	}
}

func TestOverlapsRepeatedPassage(t *testing.T) {
	phrase := "this exact phrase repeats verbatim in the same document twice over here now"
	a := phrase + " ... some filler between the two copies ... " + phrase
	b := "prefix line >> " + phrase + " << suffix line"

	got := winnow.Overlaps(a, b, ovK, ovW)
	// The phrase appears once in b and twice in a, so b's single occurrence
	// aligns to both a-copies: two spans sharing the same PosB start (one
	// diagonal each) with distinct PosA.
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
		t.Error("expected a b-span to align to two positions in a (repeated passage)")
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

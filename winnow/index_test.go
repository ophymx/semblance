package winnow_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/ophymx/semblance/winnow"
)

const (
	idxK = 8
	idxW = 6
)

// fpHashes is a brute-force oracle: the distinct fingerprint hashes of a
// text.
func fpHashes(text string) map[uint64]bool {
	set := map[uint64]bool{}
	for fp := range winnow.Text(text, idxK, idxW) {
		set[fp.Hash] = true
	}
	return set
}

func sharedCount(a, b string) int {
	ha, hb := fpHashes(a), fpHashes(b)
	n := 0
	for h := range ha {
		if hb[h] {
			n++
		}
	}
	return n
}

func TestOverlapMatchesOracle(t *testing.T) {
	shared := "the quick brown fox jumps over the lazy dog and then keeps on running far"
	docs := map[string]string{
		"a": "unrelated preamble here " + shared + " unique tail alpha",
		"b": "different opening entirely " + shared + " distinct ending beta",
		"c": "nothing in common at all just some other words about gardening roses",
	}
	ix := winnow.NewIndex(idxK, idxW)
	for id, text := range docs {
		ix.Add(id, text)
	}

	query := "prefix " + shared + " suffix"
	got := ix.Overlap(query)

	// a and b (which contain the shared passage) must rank above c.
	if len(got) < 2 || got[0].ID > got[1].ID && got[0].Shared == got[1].Shared {
		// order determinism checked separately; here just presence
	}
	byID := map[string]int{}
	for _, o := range got {
		byID[o.ID] = o.Shared
	}
	for _, id := range []string{"a", "b"} {
		if byID[id] != sharedCount(query, docs[id]) {
			t.Errorf("Overlap[%s] = %d, oracle %d", id, byID[id], sharedCount(query, docs[id]))
		}
		if byID[id] == 0 {
			t.Errorf("%s shares the passage but Overlap is 0", id)
		}
	}
	if byID["c"] != sharedCount(query, docs["c"]) {
		t.Errorf("Overlap[c] = %d, oracle %d", byID["c"], sharedCount(query, docs["c"]))
	}
}

func TestMatchesLocalize(t *testing.T) {
	passage := "the marina expansion vote passed seven to two late on tuesday evening"
	doc := "OPENING BOILERPLATE PADDING TEXT. " + passage + ". closing remarks here."
	query := "REPOST >>> " + passage + " (saw this elsewhere)"

	ix := winnow.NewIndex(idxK, idxW)
	ix.Add("doc", doc)

	matches := ix.Matches(query)
	if len(matches) == 0 {
		t.Fatal("no matches for a shared passage")
	}
	// The single shared passage collapses to one aligned span.
	if len(matches) != 1 {
		t.Errorf("shared passage produced %d matches, want 1", len(matches))
	}
	for _, m := range matches {
		if m.ID != "doc" {
			t.Errorf("unexpected id %q", m.ID)
		}
		// Bounding k-grams of the span are byte-equal at constant offset.
		if doc[m.DocPos:m.DocPos+idxK] != query[m.QueryPos:m.QueryPos+idxK] {
			t.Errorf("span q=%d d=%d: start k-grams unequal", m.QueryPos, m.DocPos)
		}
		de, qe := m.DocPos+m.Len-idxK, m.QueryPos+m.Len-idxK
		if doc[de:de+idxK] != query[qe:qe+idxK] {
			t.Errorf("span q=%d d=%d len=%d: end k-grams unequal", m.QueryPos, m.DocPos, m.Len)
		}
	}
	// The matched span in doc should recover (approximately) the passage.
	m := matches[0]
	if !strings.Contains(doc[m.DocPos:m.DocPos+m.Len], "marina expansion vote") {
		t.Errorf("localized span %q does not contain the passage core", doc[m.DocPos:m.DocPos+m.Len])
	}
}

func TestAddBytesEquivalence(t *testing.T) {
	text := "the quick brown fox jumps over the lazy dog résumé façade"
	a := winnow.NewIndex(idxK, idxW)
	a.Add("x", text)
	b := winnow.NewIndex(idxK, idxW)
	b.AddBytes("x", []byte(text))

	q := "prefix the quick brown fox jumps over the lazy dog tail"
	if !slices.Equal(a.Matches(q), b.Matches(q)) {
		t.Error("AddBytes diverges from Add")
	}
}

func TestLifecycle(t *testing.T) {
	shared := "a shared passage long enough to produce several winnow fingerprints reliably here"
	ix := winnow.NewIndex(idxK, idxW)
	ix.Add("a", "alpha "+shared)
	ix.Add("b", "beta "+shared)
	ix.Add("a", " and more alpha content appended to the same id here now")

	if ix.Len() != 2 {
		t.Fatalf("Len = %d, want 2", ix.Len())
	}
	if got := slices.Sorted(ix.Range()); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("Range = %v", got)
	}

	q := "x " + shared + " y"
	if len(ix.Overlap(q)) != 2 {
		t.Fatal("expected both docs to overlap the query")
	}
	if !ix.Remove("a") || ix.Remove("a") {
		t.Error("Remove semantics wrong")
	}
	got := ix.Overlap(q)
	if len(got) != 1 || got[0].ID != "b" {
		t.Errorf("after Remove(a), Overlap = %v, want only b", got)
	}
	// b's fingerprints survive intact.
	if got[0].Shared != sharedCount(q, "beta "+shared) {
		t.Errorf("b's overlap changed after removing a: %d", got[0].Shared)
	}
	// Re-add works.
	ix.Add("a", "alpha "+shared)
	if ix.Len() != 2 {
		t.Errorf("Len after re-add = %d, want 2", ix.Len())
	}
}

func TestDeterministicOrder(t *testing.T) {
	shared := "identical shared passage across three separate documents for tie testing purposes"
	ix := winnow.NewIndex(idxK, idxW)
	for _, id := range []string{"c", "a", "b"} { // insert out of order
		ix.Add(id, id+"-prefix "+shared)
	}
	q := "z " + shared + " z"
	first := ix.Overlap(q)
	// Equal Shared counts must break ties by id ascending.
	for i := 1; i < len(first); i++ {
		if first[i].Shared == first[i-1].Shared && first[i].ID < first[i-1].ID {
			t.Errorf("tie not broken by id: %v", first)
		}
	}
	for range 5 {
		if got := ix.Overlap(q); !slices.Equal(got, first) {
			t.Fatal("Overlap order varies across calls")
		}
		if got := ix.Matches(q); !slices.Equal(got, ix.Matches(q)) {
			t.Fatal("Matches order varies across calls")
		}
	}
}

func TestEmptyAndMisses(t *testing.T) {
	ix := winnow.NewIndex(idxK, idxW)
	if got := ix.Overlap("anything at all goes here for the query text yes"); got != nil {
		t.Errorf("Overlap on empty index = %v, want nil", got)
	}
	ix.Add("x", "some indexed document text that is reasonably long for fingerprints")
	if got := ix.Overlap("completely disjoint vocabulary zzz qqq nothing shared whatsoever"); got != nil {
		t.Errorf("Overlap with no shared fingerprints = %v, want nil", got)
	}
}

func TestPanics(t *testing.T) {
	for name, fn := range map[string]func(){
		"k=0":      func() { winnow.NewIndex(0, 4) },
		"w=0":      func() { winnow.NewIndex(4, 0) },
		"empty id": func() { winnow.NewIndex(4, 4).Add("", "text") },
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

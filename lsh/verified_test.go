package lsh_test

import (
	"slices"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/lsh"
	"github.com/ophymx/semblance/minhash"
)

// verifiedCorpus builds signatures sharing known fractions of a base set,
// plus the base signature to query with. bands*rows = 128.
func verifiedCorpus(overlaps map[string]float64) (map[string]minhash.Signature, minhash.Signature) {
	m := minhash.New(128, 0)
	rng := hashutil.SplitMix64(201)
	base := make([]uint64, 1000)
	for i := range base {
		base[i] = rng.Next()
	}
	query := m.Sketch(slices.Values(base))
	sigs := make(map[string]minhash.Signature, len(overlaps))
	for id, ov := range overlaps {
		n := int(ov * float64(len(base)))
		set := append([]uint64{}, base[:n]...)
		for len(set) < len(base) {
			set = append(set, rng.Next())
		}
		sigs[id] = m.Sketch(slices.Values(set))
	}
	return sigs, query
}

func TestVerifiedQueryRankedAndScored(t *testing.T) {
	sigs, query := verifiedCorpus(map[string]float64{
		"dup": 0.98, "near": 0.85, "mid": 0.55, "far": 0.25, "unrelated": 0.0,
	})
	vi := lsh.NewVerifiedIndex(16, 8)
	for id, sig := range sigs {
		vi.Add(id, sig)
	}

	got := vi.Query(query, 0.5)
	// Only >= 0.5, ranked descending.
	for i := 1; i < len(got); i++ {
		if got[i].Similarity > got[i-1].Similarity {
			t.Errorf("results not sorted descending: %v", got)
		}
	}
	for _, n := range got {
		if n.Similarity < 0.5 {
			t.Errorf("%s below threshold in results: %v", n.ID, n.Similarity)
		}
		// Similarity is exactly minhash.Jaccard — not merely close.
		if want := minhash.Jaccard(query, sigs[n.ID]); n.Similarity != want {
			t.Errorf("%s Similarity = %v, Jaccard = %v", n.ID, n.Similarity, want)
		}
	}
	// The closest documents lead; far/unrelated are excluded at 0.5.
	if len(got) < 2 || got[0].ID != "dup" || got[1].ID != "near" {
		t.Errorf("ranking = %v, want dup then near leading", got)
	}
	for _, n := range got {
		if n.ID == "unrelated" || n.ID == "far" {
			t.Errorf("%s should be below the 0.5 threshold", n.ID)
		}
	}
}

// TestVerifiedEquivalenceWithIndex proves VerifiedIndex is exactly Index
// plus verification: Query(sig, 0) returns the same id set as Index.Query,
// and thresholding matches manually verifying those candidates.
func TestVerifiedEquivalenceWithIndex(t *testing.T) {
	sigs, query := verifiedCorpus(map[string]float64{
		"a": 0.9, "b": 0.6, "c": 0.3, "d": 0.05,
	})
	ix := lsh.NewIndex(16, 8)
	vi := lsh.NewVerifiedIndex(16, 8)
	for id, sig := range sigs {
		ix.Add(id, sig)
		vi.Add(id, sig)
	}

	candidateSet := slices.Sorted(slices.Values(ix.Query(query)))
	var verifiedIDs []string
	for _, n := range vi.Query(query, 0) {
		verifiedIDs = append(verifiedIDs, n.ID)
	}
	if !slices.Equal(slices.Sorted(slices.Values(verifiedIDs)), candidateSet) {
		t.Errorf("Query(sig, 0) id set %v != Index candidates %v", verifiedIDs, candidateSet)
	}

	// With a threshold, VerifiedIndex must return exactly the Index
	// candidates whose Jaccard clears it.
	const thr = 0.5
	want := map[string]bool{}
	for _, id := range ix.Query(query) {
		if minhash.Jaccard(query, sigs[id]) >= thr {
			want[id] = true
		}
	}
	got := map[string]bool{}
	for _, n := range vi.Query(query, thr) {
		got[n.ID] = true
	}
	if len(got) != len(want) {
		t.Fatalf("threshold result count %d, want %d", len(got), len(want))
	}
	for id := range want {
		if !got[id] {
			t.Errorf("missing %s from thresholded verified query", id)
		}
	}
}

func TestVerifiedAddClones(t *testing.T) {
	rng := hashutil.SplitMix64(202)
	vi := lsh.NewVerifiedIndex(4, 4)
	sig := randSig(&rng, 16)
	orig := slices.Clone(sig)
	vi.Add("x", sig)

	// Mutating the caller's slice must not affect the stored signature.
	for i := range sig {
		sig[i] = 0
	}
	if !slices.Equal(vi.Signature("x"), orig) {
		t.Error("Add retained the caller's slice instead of cloning")
	}
	hits := vi.Query(orig, 0)
	if len(hits) != 1 || hits[0].ID != "x" || hits[0].Similarity != 1 {
		t.Errorf("Query(orig) = %v, want x at 1.0", hits)
	}
	// Signature returns a copy too.
	got := vi.Signature("x")
	got[0] = 123
	if slices.Equal(vi.Signature("x"), got) {
		t.Error("Signature returned a live reference, not a copy")
	}
}

func TestVerifiedReAddNoStaleEntries(t *testing.T) {
	rng := hashutil.SplitMix64(203)
	vi := lsh.NewVerifiedIndex(16, 8)
	a := randSig(&rng, 128)
	b := randSig(&rng, 128)

	vi.Add("doc", a)
	vi.Add("doc", b) // re-add with a different signature
	if vi.Len() != 1 {
		t.Fatalf("Len after re-add = %d, want 1", vi.Len())
	}
	if !slices.Equal(vi.Signature("doc"), b) {
		t.Error("re-add did not replace the stored signature")
	}
	// Re-add must have dropped a's bucket entries; after removing doc, no
	// query — via a's bands or b's — may surface it.
	vi.Remove("doc")
	for _, n := range vi.Query(a, 0) {
		if n.ID == "doc" {
			t.Error("stale entry in old (a) buckets survived re-add + remove")
		}
	}
	if len(vi.Query(b, 0)) != 0 {
		t.Error("doc still present after remove")
	}
}

func TestVerifiedLifecycle(t *testing.T) {
	rng := hashutil.SplitMix64(204)
	vi := lsh.NewVerifiedIndex(4, 4)
	sig := randSig(&rng, 16)
	vi.Add("a", sig)
	vi.Add("b", randSig(&rng, 16))

	if vi.Len() != 2 {
		t.Fatalf("Len = %d, want 2", vi.Len())
	}
	if got := slices.Sorted(vi.Range()); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("Range = %v", got)
	}
	if vi.Signature("missing") != nil {
		t.Error("Signature of unknown id should be nil")
	}
	if !vi.Remove("a") || vi.Remove("a") {
		t.Error("Remove semantics wrong")
	}
	if hits := vi.Query(sig, 0); slices.ContainsFunc(hits, func(n lsh.Neighbor) bool { return n.ID == "a" }) {
		t.Error("removed id still returned")
	}
	if s := vi.Stats(); s.IDs != 1 {
		t.Errorf("Stats.IDs = %d, want 1", s.IDs)
	}
}

func TestVerifiedThresholdAndEmpty(t *testing.T) {
	sigs, query := verifiedCorpus(map[string]float64{"hi": 0.9, "lo": 0.2})
	vi := lsh.NewVerifiedIndex(16, 8)
	for id, sig := range sigs {
		vi.Add(id, sig)
	}
	// minJaccard 0 returns all candidates scored; a high threshold filters.
	if all := vi.Query(query, 0); len(all) == 0 {
		t.Error("Query(_, 0) returned nothing")
	}
	strict := vi.Query(query, 0.8)
	for _, n := range strict {
		if n.ID != "hi" {
			t.Errorf("unexpected %s above 0.8", n.ID)
		}
	}

	empty := lsh.NewVerifiedIndex(4, 4)
	rng := hashutil.SplitMix64(205)
	if got := empty.Query(randSig(&rng, 16), 0); got != nil {
		t.Errorf("Query on empty index = %v, want nil", got)
	}
}

func TestVerifiedPanics(t *testing.T) {
	rng := hashutil.SplitMix64(206)
	vi := lsh.NewVerifiedIndex(4, 4)
	good := randSig(&rng, 16)
	for name, fn := range map[string]func(){
		"bands=0":        func() { lsh.NewVerifiedIndex(0, 4) },
		"rows=0":         func() { lsh.NewVerifiedIndex(4, 0) },
		"add mismatch":   func() { vi.Add("x", make(minhash.Signature, 15)) },
		"query mismatch": func() { vi.Query(make(minhash.Signature, 15), 0) },
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
	_ = good
}

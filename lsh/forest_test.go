package lsh_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/lsh"
	"github.com/ophymx/semblance/minhash"
)

const (
	forestTrees = 8
	forestDepth = 16
	forestK     = forestTrees * forestDepth
)

// forestCorpus builds signatures of sets sharing a given fraction of
// elements with a base set.
func forestCorpus(t *testing.T, overlaps map[string]float64) (map[string]minhash.Signature, minhash.Signature) {
	t.Helper()
	m := minhash.New(forestK, 0)
	rng := hashutil.SplitMix64(131)
	baseSet := make([]uint64, 1000)
	for i := range baseSet {
		baseSet[i] = rng.Next()
	}
	query := m.Sketch(slices.Values(baseSet))

	sigs := make(map[string]minhash.Signature, len(overlaps))
	for id, ov := range overlaps {
		n := int(ov * float64(len(baseSet)))
		set := append([]uint64{}, baseSet[:n]...)
		for len(set) < len(baseSet) {
			set = append(set, rng.Next())
		}
		sigs[id] = m.Sketch(slices.Values(set))
	}
	return sigs, query
}

func TestForestTopK(t *testing.T) {
	sigs, query := forestCorpus(t, map[string]float64{
		"dup":       0.98,
		"near":      0.85,
		"mid":       0.55,
		"far":       0.25,
		"unrelated": 0.0,
	})
	f := lsh.NewForest(forestTrees, forestDepth)
	for _, id := range []string{"unrelated", "far", "mid", "near", "dup"} {
		f.Add(id, sigs[id])
	}

	got := f.Query(query, 2)
	if !slices.Equal(got, []string{"dup", "near"}) {
		t.Errorf("Query(k=2) = %v, want [dup near]", got)
	}

	// Asking for more returns candidates in similarity-proxy order; the
	// most similar always lead.
	all := f.Query(query, 10)
	if len(all) < 3 || all[0] != "dup" || all[1] != "near" {
		t.Errorf("Query(k=10) = %v, want dup then near leading", all)
	}
	if slices.Contains(all, "unrelated") && all[len(all)-1] != "unrelated" {
		t.Errorf("unrelated ranked above closer candidates: %v", all)
	}
}

func TestForestExactMatchFirst(t *testing.T) {
	rng := hashutil.SplitMix64(132)
	f := lsh.NewForest(4, 4)
	sig := randSig(&rng, 16)
	f.Add("other", randSig(&rng, 16))
	f.Add("exact", sig)
	if got := f.Query(sig, 1); !slices.Equal(got, []string{"exact"}) {
		t.Errorf("Query(k=1) = %v, want [exact]", got)
	}
}

// TestForestDepthOrdering checks against a brute-force oracle: candidates
// must appear in non-increasing order of their maximum shared prefix
// depth across trees, and exactly the ids with depth >= 1 are returned.
func TestForestDepthOrdering(t *testing.T) {
	const trees, depth = 4, 4
	rng := hashutil.SplitMix64(133)
	f := lsh.NewForest(trees, depth)
	query := randSig(&rng, trees*depth)

	maxDepth := map[string]int{}
	for i := range 60 {
		id := fmt.Sprintf("doc%02d", i)
		sig := randSig(&rng, trees*depth)
		// Plant shared prefixes of varying depth.
		tr, d := i%trees, i%(depth+1)
		copy(sig[tr*depth:tr*depth+d], query[tr*depth:tr*depth+d])
		f.Add(id, sig)

		best := 0
		for tr := range trees {
			d := 0
			for d < depth && sig[tr*depth+d] == query[tr*depth+d] {
				d++
			}
			best = max(best, d)
		}
		maxDepth[id] = best
	}

	got := f.Query(query, 1000)
	for i := 1; i < len(got); i++ {
		if maxDepth[got[i]] > maxDepth[got[i-1]] {
			t.Fatalf("candidates out of depth order at %d: %v(%d) after %v(%d)",
				i, got[i], maxDepth[got[i]], got[i-1], maxDepth[got[i-1]])
		}
	}
	want := 0
	for _, d := range maxDepth {
		if d >= 1 {
			want++
		}
	}
	if len(got) != want {
		t.Errorf("returned %d candidates, oracle says %d have depth >= 1", len(got), want)
	}
}

func TestForestLifecycle(t *testing.T) {
	rng := hashutil.SplitMix64(134)
	f := lsh.NewForest(4, 4)
	sig := randSig(&rng, 16)
	f.Add("a", sig)
	f.Add("a", randSig(&rng, 16)) // second Add, same id
	f.Add("b", randSig(&rng, 16))

	if f.Len() != 2 {
		t.Errorf("Len = %d, want 2", f.Len())
	}
	if got := slices.Sorted(f.Range()); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("Range = %v", got)
	}
	if got := f.Query(sig, 5); !slices.Contains(got, "a") {
		t.Errorf("Query = %v, want to contain a", got)
	}
	if !f.Remove("a") || f.Remove("a") {
		t.Error("Remove semantics wrong")
	}
	if got := f.Query(sig, 5); slices.Contains(got, "a") {
		t.Errorf("Query after Remove = %v, still contains a", got)
	}

	// Interleaved add/query keeps the lazy sort correct.
	f.Add("c", sig)
	if got := f.Query(sig, 1); !slices.Equal(got, []string{"c"}) {
		t.Errorf("Query after interleaved Add = %v, want [c]", got)
	}

	// Deterministic across repeated queries.
	first := f.Query(sig, 5)
	for range 5 {
		if got := f.Query(sig, 5); !slices.Equal(got, first) {
			t.Fatal("Query result varies across calls")
		}
	}
}

func TestForestEmptyAndMisses(t *testing.T) {
	rng := hashutil.SplitMix64(135)
	f := lsh.NewForest(4, 4)
	if got := f.Query(randSig(&rng, 16), 3); got != nil {
		t.Errorf("Query on empty forest = %v, want nil", got)
	}
	f.Add("x", randSig(&rng, 16))
	// A query sharing no leading values finds nothing.
	if got := f.Query(randSig(&rng, 16), 3); got != nil {
		t.Errorf("Query with no shared prefixes = %v, want nil", got)
	}
}

func TestForestPanics(t *testing.T) {
	rng := hashutil.SplitMix64(136)
	f := lsh.NewForest(4, 4)
	good := randSig(&rng, 16)
	for name, fn := range map[string]func(){
		"trees=0":      func() { lsh.NewForest(0, 4) },
		"depth=0":      func() { lsh.NewForest(4, 0) },
		"add mismatch": func() { f.Add("x", make(minhash.Signature, 15)) },
		"query mismatch": func() {
			f.Query(make(minhash.Signature, 15), 1)
		},
		"k=0": func() { f.Query(good, 0) },
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

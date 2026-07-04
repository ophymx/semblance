package lsh_test

import (
	"math"
	"slices"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/lsh"
	"github.com/ophymx/semblance/minhash"
)

// randSig returns a signature of n pseudo-random values.
func randSig(rng *hashutil.SplitMix64, n int) minhash.Signature {
	sig := make(minhash.Signature, n)
	for i := range sig {
		sig[i] = rng.Next()
	}
	return sig
}

func TestNewIndexPanics(t *testing.T) {
	tests := []struct{ bands, rows int }{{0, 8}, {16, 0}, {-1, 8}, {16, -1}}
	for _, tt := range tests {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("NewIndex(%d, %d) did not panic", tt.bands, tt.rows)
				}
			}()
			lsh.NewIndex(tt.bands, tt.rows)
		}()
	}
}

func TestLengthMismatchPanics(t *testing.T) {
	ix := lsh.NewIndex(4, 4)
	bad := make(minhash.Signature, 15)
	t.Run("Add", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("Add with wrong signature length did not panic")
			}
		}()
		ix.Add("x", bad)
	})
	t.Run("Query", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("Query with wrong signature length did not panic")
			}
		}()
		ix.Query(bad)
	})
}

func TestThreshold(t *testing.T) {
	got := lsh.NewIndex(16, 8).Threshold()
	want := math.Pow(1.0/16, 1.0/8)
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("Threshold() = %v, want %v", got, want)
	}
	if got < 0.70 || got > 0.72 {
		t.Errorf("default 16x8 threshold = %v, want ~0.71", got)
	}
}

func TestEmptyIndex(t *testing.T) {
	rng := hashutil.SplitMix64(1)
	ix := lsh.NewIndex(4, 4)
	if got := ix.Query(randSig(&rng, 16)); got != nil {
		t.Errorf("Query on empty index = %v, want nil", got)
	}
}

func TestExactMatch(t *testing.T) {
	rng := hashutil.SplitMix64(2)
	ix := lsh.NewIndex(4, 4)
	sig := randSig(&rng, 16)
	ix.Add("doc", sig)
	if got := ix.Query(sig); !slices.Equal(got, []string{"doc"}) {
		t.Errorf("Query(same sig) = %v, want [doc]", got)
	}
}

func TestBandGranularity(t *testing.T) {
	rng := hashutil.SplitMix64(3)
	ix := lsh.NewIndex(4, 4)
	base := randSig(&rng, 16)
	ix.Add("doc", base)

	// Agreeing on one full band (even with every other value different)
	// makes a candidate.
	oneBand := randSig(&rng, 16)
	copy(oneBand[4:8], base[4:8]) // band 1 identical
	if got := ix.Query(oneBand); !slices.Equal(got, []string{"doc"}) {
		t.Errorf("Query(one matching band) = %v, want [doc]", got)
	}

	// Agreeing on rows-1 values in every band is not enough.
	nearMiss := slices.Clone(base)
	for band := 0; band < 4; band++ {
		nearMiss[band*4] = rng.Next()
	}
	if got := ix.Query(nearMiss); got != nil {
		t.Errorf("Query(no complete band) = %v, want nil", got)
	}
}

func TestQueryDedup(t *testing.T) {
	rng := hashutil.SplitMix64(4)
	ix := lsh.NewIndex(4, 4)
	sig := randSig(&rng, 16)
	ix.Add("doc", sig) // matches on all 4 bands, and added twice
	ix.Add("doc", sig)
	if got := ix.Query(sig); !slices.Equal(got, []string{"doc"}) {
		t.Errorf("Query = %v, want [doc] exactly once", got)
	}
}

func TestQueryDeterministicOrder(t *testing.T) {
	rng := hashutil.SplitMix64(5)
	ix := lsh.NewIndex(4, 4)
	sig := randSig(&rng, 16)
	for _, id := range []string{"a", "b", "c"} {
		ix.Add(id, sig)
	}
	want := []string{"a", "b", "c"} // insertion order within the bucket
	for range 10 {
		if got := ix.Query(sig); !slices.Equal(got, want) {
			t.Fatalf("Query order = %v, want %v", got, want)
		}
	}
}

// TestEndToEnd exercises the full minhash → lsh pipeline: a high-Jaccard
// pair must be a candidate, a low-Jaccard pair must not. Deterministic
// because everything is seeded (J=0.9 candidate probability ~0.9999,
// J=0.1 ~1.6e-7 at 16x8).
func TestEndToEnd(t *testing.T) {
	const k = 128
	m := minhash.New(k, 0)
	rng := hashutil.SplitMix64(6)

	elems := func(n int) []uint64 {
		out := make([]uint64, n)
		for i := range out {
			out[i] = rng.Next()
		}
		return out
	}
	sketch := func(set []uint64) minhash.Signature {
		return m.Sketch(slices.Values(set))
	}

	shared := elems(9000) // J = 9000/10000 = 0.9 with 500 unique each
	uniqA, uniqB := elems(500), elems(500)
	similarA := sketch(append(slices.Clone(shared), uniqA...))
	similarB := sketch(append(slices.Clone(shared), uniqB...))
	unrelated := sketch(elems(1000))

	ix := lsh.NewIndex(16, 8)
	ix.Add("similarB", similarB)
	ix.Add("unrelated", unrelated)

	got := ix.Query(similarA)
	if !slices.Contains(got, "similarB") {
		t.Errorf("Query missed high-similarity candidate: %v", got)
	}
	if slices.Contains(got, "unrelated") {
		t.Errorf("Query returned low-similarity candidate: %v", got)
	}
}

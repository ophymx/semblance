package minhash_test

import (
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/minhash"
)

func TestJaccardMany(t *testing.T) {
	rng := hashutil.SplitMix64(81)
	q := minhash.Signature(randSet(&rng, 128))
	candidates := make([]minhash.Signature, 20)
	for i := range candidates {
		c := minhash.Signature(randSet(&rng, 128))
		copy(c[:i*6], q[:i*6]) // varying match densities
		candidates[i] = c
	}

	got := minhash.JaccardMany(nil, q, candidates)
	if len(got) != len(candidates) {
		t.Fatalf("result length %d, want %d", len(got), len(candidates))
	}
	for i, c := range candidates {
		// Bit-identical to the single-pair form, not merely close.
		if want := minhash.Jaccard(q, c); got[i] != want {
			t.Errorf("candidate %d: JaccardMany = %v, Jaccard = %v", i, got[i], want)
		}
	}

	// Reusing dst writes in place and returns it.
	dst := make([]float64, len(candidates))
	if got2 := minhash.JaccardMany(dst, q, candidates); &got2[0] != &dst[0] {
		t.Error("JaccardMany did not fill the provided dst")
	}
	for i := range dst {
		if dst[i] != got[i] {
			t.Errorf("dst reuse: slot %d = %v, want %v", i, dst[i], got[i])
		}
	}

	// No candidates: nil dst stays empty, no panic.
	if out := minhash.JaccardMany(nil, q, nil); len(out) != 0 {
		t.Errorf("no candidates: result length %d, want 0", len(out))
	}
}

func TestJaccardManyPanics(t *testing.T) {
	rng := hashutil.SplitMix64(82)
	q := minhash.Signature(randSet(&rng, 8))
	good := []minhash.Signature{minhash.Signature(randSet(&rng, 8))}
	tests := map[string]func(){
		"empty q":            func() { minhash.JaccardMany(nil, minhash.Signature{}, good) },
		"dst length":         func() { minhash.JaccardMany(make([]float64, 2), q, good) },
		"candidate mismatch": func() { minhash.JaccardMany(nil, q, []minhash.Signature{make(minhash.Signature, 7)}) },
	}
	for name, fn := range tests {
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

func BenchmarkJaccardMany(b *testing.B) {
	rng := hashutil.SplitMix64(83)
	q := minhash.Signature(randSet(&rng, 128))
	candidates := make([]minhash.Signature, 100)
	for i := range candidates {
		candidates[i] = minhash.Signature(randSet(&rng, 128))
	}
	dst := make([]float64, len(candidates))
	b.ReportAllocs()
	for b.Loop() {
		minhash.JaccardMany(dst, q, candidates)
	}
}

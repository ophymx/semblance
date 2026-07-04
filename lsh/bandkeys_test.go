package lsh_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/lsh"
	"github.com/ophymx/semblance/minhash"
)

// TestBandKeysGolden pins the key derivation: BandKeys output is a
// persistable artifact, so a change here breaks stored buckets — a
// major-version event, never a test fix.
func TestBandKeysGolden(t *testing.T) {
	sig := make(minhash.Signature, 8)
	for i := range sig {
		sig[i] = uint64(i + 1)
	}
	keys := lsh.BandKeys(nil, sig, 2, 4)
	want := []string{"7ff1d476798020a7", "fd4888f51ae197db"}
	for i, k := range keys {
		if got := fmt.Sprintf("%016x", k); got != want[i] {
			t.Errorf("band %d key = %s, golden %s — persisted buckets are BROKEN if intentional", i, got, want[i])
		}
	}
}

func TestBandKeysAppend(t *testing.T) {
	rng := hashutil.SplitMix64(101)
	sig := randSig(&rng, 16)

	keys := lsh.BandKeys(nil, sig, 4, 4)
	if len(keys) != 4 {
		t.Fatalf("got %d keys, want 4", len(keys))
	}
	// Appends to dst, preserving the prefix.
	pre := []uint64{7, 8}
	both := lsh.BandKeys(pre, sig, 4, 4)
	if !slices.Equal(both[:2], []uint64{7, 8}) || !slices.Equal(both[2:], keys) {
		t.Error("BandKeys did not append to dst")
	}

	// Bands agreeing on their rows produce equal keys; others differ.
	other := randSig(&rng, 16)
	copy(other[8:12], sig[8:12]) // band 2 identical
	otherKeys := lsh.BandKeys(nil, other, 4, 4)
	if otherKeys[2] != keys[2] {
		t.Error("identical band produced different keys")
	}
	if otherKeys[0] == keys[0] {
		t.Error("differing band produced equal keys")
	}
}

func TestBandKeysPanics(t *testing.T) {
	sig := make(minhash.Signature, 16)
	for name, fn := range map[string]func(){
		"bands=0":  func() { lsh.BandKeys(nil, sig, 0, 4) },
		"rows=0":   func() { lsh.BandKeys(nil, sig, 4, 0) },
		"mismatch": func() { lsh.BandKeys(nil, sig, 4, 3) },
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

// TestExternalIndexEquivalence proves the BYO-store pattern: an index
// built externally from BandKeys returns exactly the candidates of
// lsh.Index for the same data.
func TestExternalIndexEquivalence(t *testing.T) {
	const bands, rows = 16, 8
	rng := hashutil.SplitMix64(102)
	ix := lsh.NewIndex(bands, rows)
	external := make(map[uint64]map[uint64][]string) // band -> key -> ids

	sigs := make([]minhash.Signature, 30)
	for i := range sigs {
		sig := randSig(&rng, bands*rows)
		if i%3 == 1 {
			copy(sig[:64], sigs[i-1][:64]) // share bands with a neighbor
		}
		sigs[i] = sig
		id := fmt.Sprintf("doc%d", i)
		ix.Add(id, sig)
		for band, key := range lsh.BandKeys(nil, sig, bands, rows) {
			b := uint64(band)
			if external[b] == nil {
				external[b] = make(map[uint64][]string)
			}
			external[b][key] = append(external[b][key], id)
		}
	}

	for i, sig := range sigs {
		want := slices.Sorted(slices.Values(ix.Query(sig)))
		var got []string
		seen := map[string]bool{}
		for band, key := range lsh.BandKeys(nil, sig, bands, rows) {
			for _, id := range external[uint64(band)][key] {
				if !seen[id] {
					seen[id] = true
					got = append(got, id)
				}
			}
		}
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Fatalf("query %d: external index %v, lsh.Index %v", i, got, want)
		}
	}
}

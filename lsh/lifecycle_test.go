package lsh_test

import (
	"slices"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/lsh"
	"github.com/ophymx/semblance/simhash"
)

func TestIndexRemove(t *testing.T) {
	rng := hashutil.SplitMix64(61)
	ix := lsh.NewIndex(4, 4)
	sigA, sigB := randSig(&rng, 16), randSig(&rng, 16)
	ix.Add("a", sigA)
	ix.Add("b", sigB)
	ix.Add("both", sigA)
	ix.Add("both", sigB) // one id, two signatures

	if got := ix.Len(); got != 3 {
		t.Fatalf("Len = %d, want 3", got)
	}
	if !ix.Remove("both") {
		t.Fatal("Remove(both) = false, want true")
	}
	if ix.Remove("both") || ix.Remove("never-added") {
		t.Error("Remove of absent id returned true")
	}
	if got := ix.Len(); got != 2 {
		t.Fatalf("Len after remove = %d, want 2", got)
	}

	// "both" is gone from every bucket of both its Adds; others unaffected.
	if got := ix.Query(sigA); !slices.Equal(got, []string{"a"}) {
		t.Errorf("Query(sigA) = %v, want [a]", got)
	}
	if got := ix.Query(sigB); !slices.Equal(got, []string{"b"}) {
		t.Errorf("Query(sigB) = %v, want [b]", got)
	}

	// Removing an id added twice with the same signature clears both.
	ix.Add("dup", sigA)
	ix.Add("dup", sigA)
	ix.Remove("dup")
	if got := ix.Query(sigA); slices.Contains(got, "dup") {
		t.Errorf("Query after duplicate-add remove still returns dup: %v", got)
	}

	// Re-adding after removal works.
	ix.Add("a2", sigA)
	if got := ix.Query(sigA); !slices.Contains(got, "a2") {
		t.Errorf("Query after re-add = %v, want to contain a2", got)
	}
}

func TestIndexRange(t *testing.T) {
	rng := hashutil.SplitMix64(62)
	ix := lsh.NewIndex(4, 4)
	want := []string{"x", "y", "z"}
	for _, id := range want {
		ix.Add(id, randSig(&rng, 16))
	}
	ix.Add("x", randSig(&rng, 16)) // second Add must not duplicate in Range

	got := slices.Sorted(ix.Range())
	if !slices.Equal(got, want) {
		t.Errorf("Range = %v, want %v", got, want)
	}

	// Early break is safe.
	for range ix.Range() {
		break
	}
}

func TestHammingRemove(t *testing.T) {
	const base simhash.Fingerprint = 0xDEADBEEFCAFEF00D
	ix := lsh.NewHammingIndex(2)
	ix.Add("a", base)
	ix.Add("near", flip(base, 3))
	ix.Add("multi", base)
	ix.Add("multi", flip(base, 40)) // one id, two fingerprints

	if got := ix.Len(); got != 3 {
		t.Fatalf("Len = %d, want 3", got)
	}
	if !ix.Remove("multi") {
		t.Fatal("Remove(multi) = false, want true")
	}
	if ix.Remove("multi") {
		t.Error("second Remove returned true")
	}
	if got := ix.Len(); got != 2 {
		t.Fatalf("Len after remove = %d, want 2", got)
	}
	got := ix.Query(base)
	if slices.Contains(got, "multi") {
		t.Errorf("Query still returns removed id: %v", got)
	}
	if !slices.Contains(got, "a") || !slices.Contains(got, "near") {
		t.Errorf("Query lost surviving ids: %v", got)
	}
}

func TestHammingRange(t *testing.T) {
	const base simhash.Fingerprint = 0x0123456789ABCDEF
	ix := lsh.NewHammingIndex(1)
	ix.Add("a", base)
	ix.Add("b", flip(base, 7))
	ix.Add("b", flip(base, 9)) // yields once per Add

	seen := map[string][]simhash.Fingerprint{}
	for id, fp := range ix.Range() {
		seen[id] = append(seen[id], fp)
	}
	if len(seen) != 2 || len(seen["a"]) != 1 || len(seen["b"]) != 2 {
		t.Fatalf("Range contents wrong: %v", seen)
	}

	// Range is sufficient to rebuild an equivalent index.
	rebuilt := lsh.NewHammingIndex(1)
	for id, fp := range ix.Range() {
		rebuilt.Add(id, fp)
	}
	if rebuilt.Len() != ix.Len() {
		t.Fatalf("rebuilt Len = %d, want %d", rebuilt.Len(), ix.Len())
	}
	for _, probe := range []simhash.Fingerprint{base, flip(base, 7), flip(base, 9)} {
		a := slices.Sorted(slices.Values(ix.Query(probe)))
		b := slices.Sorted(slices.Values(rebuilt.Query(probe)))
		if !slices.Equal(a, b) {
			t.Errorf("rebuilt index disagrees on %#x: %v vs %v", probe, b, a)
		}
	}
}

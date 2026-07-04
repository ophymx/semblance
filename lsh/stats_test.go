package lsh_test

import (
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/lsh"
	"github.com/ophymx/semblance/simhash"
)

func TestIndexStats(t *testing.T) {
	rng := hashutil.SplitMix64(91)
	ix := lsh.NewIndex(4, 4)

	if got := ix.Stats(); got != (lsh.Stats{}) {
		t.Fatalf("empty index Stats = %+v, want zero", got)
	}

	// Three ids sharing one signature: every band has a single bucket of
	// three; one distinct id adds a size-1 bucket per band.
	shared := randSig(&rng, 16)
	for _, id := range []string{"a", "b", "c"} {
		ix.Add(id, shared)
	}
	ix.Add("lone", randSig(&rng, 16))

	want := lsh.Stats{IDs: 4, Buckets: 8, Entries: 16, MaxBucket: 3}
	if got := ix.Stats(); got != want {
		t.Fatalf("Stats = %+v, want %+v", got, want)
	}

	// Removal shrinks the stats back.
	ix.Remove("b")
	ix.Remove("lone")
	want = lsh.Stats{IDs: 2, Buckets: 4, Entries: 8, MaxBucket: 2}
	if got := ix.Stats(); got != want {
		t.Fatalf("Stats after removals = %+v, want %+v", got, want)
	}
}

func TestHammingStats(t *testing.T) {
	const base simhash.Fingerprint = 0xDEADBEEFCAFEF00D
	ix := lsh.NewHammingIndex(1) // 2 blocks

	if got := ix.Stats(); got != (lsh.Stats{}) {
		t.Fatalf("empty index Stats = %+v, want zero", got)
	}

	ix.Add("a", base)
	ix.Add("b", base)          // same fp: shares both buckets with a
	ix.Add("c", flip(base, 3)) // differs in block 0 (bits 0-31), shares block 1

	want := lsh.Stats{IDs: 3, Buckets: 3, Entries: 6, MaxBucket: 3}
	if got := ix.Stats(); got != want {
		t.Fatalf("Stats = %+v, want %+v", got, want)
	}

	ix.Remove("b")
	want = lsh.Stats{IDs: 2, Buckets: 3, Entries: 4, MaxBucket: 2}
	if got := ix.Stats(); got != want {
		t.Fatalf("Stats after removal = %+v, want %+v", got, want)
	}
}

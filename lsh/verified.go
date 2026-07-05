package lsh

import (
	"cmp"
	"iter"
	"maps"
	"slices"

	"github.com/ophymx/semblance/minhash"
)

// VerifiedIndex is a MinHash banding index that retains each signature, so
// [VerifiedIndex.Query] returns verified, ranked neighbors directly rather
// than the unverified candidate ids of [Index]. It is the tool for the
// common "find near-duplicates and how similar they are" workflow: no
// caller-side signature store, no separate verification step.
//
// It trades memory for that convenience — k*8 bytes per document for the
// stored signatures, versus [Index]'s ids-only footprint. In return it
// needs no per-id band-key list (Remove recomputes keys from the stored
// signature), so it is otherwise leaner than Index. Not safe for concurrent
// use.
type VerifiedIndex struct {
	bands, rows int
	store       bucketStore
	sigs        map[string]minhash.Signature
}

// Neighbor is a verified match returned by [VerifiedIndex.Query].
// Similarity is the estimated Jaccard similarity to the query signature
// (see [minhash.Jaccard] for the error bound).
type Neighbor struct {
	ID         string
	Similarity float64
}

// NewVerifiedIndex returns an empty index for signatures of length
// bands*rows. Panics if bands <= 0 or rows <= 0.
func NewVerifiedIndex(bands, rows int) *VerifiedIndex {
	if bands <= 0 || rows <= 0 {
		panic("lsh: bands and rows must be positive")
	}
	return &VerifiedIndex{
		bands: bands,
		rows:  rows,
		store: make(memStore, bands),
		sigs:  make(map[string]minhash.Signature),
	}
}

// Threshold returns the approximate Jaccard similarity at which the LSH
// candidate probability crosses 1/2: (1/bands)^(1/rows). A minJaccard
// passed to [VerifiedIndex.Query] below this value will silently miss
// pairs the banding never surfaces as candidates — keep the query
// threshold at or above it, or build the index with [Params] for the
// threshold you intend to query at.
func (vi *VerifiedIndex) Threshold() float64 {
	return (&Index{bands: vi.bands, rows: vi.rows}).Threshold()
}

// Add stores id under a copy of sig. The signature is retained (unlike
// [Index.Add]), so the caller may reuse the underlying array — e.g. a
// SketchInto buffer — freely after Add returns. Ids need not be unique; a
// repeated id overwrites its stored signature and re-indexes it, and
// [VerifiedIndex.Remove] drops all of its bucket entries.
// Panics if len(sig) != bands*rows.
func (vi *VerifiedIndex) Add(id string, sig minhash.Signature) {
	keys := BandKeys(nil, sig, vi.bands, vi.rows)
	if prev, ok := vi.sigs[id]; ok {
		// Re-add: drop the old bucket entries before re-indexing, so the
		// id appears once per band as its current signature dictates.
		vi.removeKeys(id, prev)
	}
	for band, key := range keys {
		vi.store.add(band, key, id)
	}
	vi.sigs[id] = slices.Clone(sig)
}

// Query returns every indexed document whose estimated Jaccard similarity
// to sig is at least minJaccard, ranked by Similarity descending (ties by
// id ascending), deterministically. Pass minJaccard 0 to score and rank
// all LSH candidates. Query does not exclude sig's own id if it was added —
// query before adding to find prior near-duplicates. Returns nil if there
// are none. Panics if len(sig) != bands*rows.
func (vi *VerifiedIndex) Query(sig minhash.Signature, minJaccard float64) []Neighbor {
	keys := BandKeys(nil, sig, vi.bands, vi.rows)
	seen := make(map[string]bool)
	var out []Neighbor
	for band, key := range keys {
		for _, id := range vi.store.get(band, key) {
			if seen[id] {
				continue
			}
			seen[id] = true
			if sim := minhash.Jaccard(sig, vi.sigs[id]); sim >= minJaccard {
				out = append(out, Neighbor{ID: id, Similarity: sim})
			}
		}
	}
	slices.SortFunc(out, func(a, b Neighbor) int {
		if a.Similarity != b.Similarity {
			return cmp.Compare(b.Similarity, a.Similarity)
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return out
}

// Remove deletes every entry for id and reports whether it was present.
func (vi *VerifiedIndex) Remove(id string) bool {
	sig, ok := vi.sigs[id]
	if !ok {
		return false
	}
	vi.removeKeys(id, sig)
	delete(vi.sigs, id)
	return true
}

// removeKeys drops id from every bucket its signature maps to.
func (vi *VerifiedIndex) removeKeys(id string, sig minhash.Signature) {
	for band, key := range BandKeys(nil, sig, vi.bands, vi.rows) {
		vi.store.remove(band, key, id)
	}
}

// Len returns the number of distinct ids indexed.
func (vi *VerifiedIndex) Len() int { return len(vi.sigs) }

// Range returns an iterator over the indexed ids, in unspecified order. The
// index must not be modified during iteration. Unlike [Index], a
// VerifiedIndex stores signatures, so it can be enumerated and rebuilt
// without a separate signature store — see [VerifiedIndex.Signature].
func (vi *VerifiedIndex) Range() iter.Seq[string] { return maps.Keys(vi.sigs) }

// Signature returns a copy of the stored signature for id, or nil if id is
// not indexed.
func (vi *VerifiedIndex) Signature(id string) minhash.Signature {
	sig, ok := vi.sigs[id]
	if !ok {
		return nil
	}
	return slices.Clone(sig)
}

// Stats reports bucket occupancy; see [Stats]. Costs a full walk of the
// buckets.
func (vi *VerifiedIndex) Stats() Stats {
	buckets, entries, maxBucket := vi.store.stats()
	return Stats{IDs: len(vi.sigs), Buckets: buckets, Entries: entries, MaxBucket: maxBucket}
}

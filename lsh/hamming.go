package lsh

import (
	"iter"

	"github.com/ophymx/semblance/simhash"
)

// HammingIndex finds stored SimHash fingerprints within a fixed Hamming
// distance of a query, using the block-permutation table approach of Manku,
// Jain & Das Sarma (2007): the 64 bits are split into maxDist+1 blocks, and
// by pigeonhole two fingerprints within maxDist of each other must agree
// exactly on at least one block. Each block gets an exact-match table;
// block hits are verified against the stored fingerprint, so — unlike
// [Index] — Query returns actual matches, not candidates.
//
// Memory is one table entry per block per fingerprint: maxDist+1 entries
// per Add. Not safe for concurrent use.
type HammingIndex struct {
	maxDist int
	blocks  []block // len maxDist+1
	// fps records every fingerprint added under each id, so Remove can
	// find the id's table entries and Range can enumerate contents.
	fps map[string][]simhash.Fingerprint
}

type block struct {
	shift, width uint
	table        map[uint64][]hammingEntry
}

type hammingEntry struct {
	fp simhash.Fingerprint
	id string
}

// NewHammingIndex returns an empty index answering queries at Hamming
// distance <= maxDist. v0 supports maxDist in [1, 3]; larger distances need
// combinatorially more tables and are a post-v0 extension. Panics outside
// that range.
func NewHammingIndex(maxDist int) *HammingIndex {
	if maxDist < 1 || maxDist > 3 {
		panic("lsh: maxDist must be in [1, 3]")
	}
	n := maxDist + 1
	blocks := make([]block, n)
	shift := uint(0)
	for i := range blocks {
		width := uint(64 / n)
		if i < 64%n {
			width++
		}
		blocks[i] = block{shift: shift, width: width, table: make(map[uint64][]hammingEntry)}
		shift += width
	}
	return &HammingIndex{
		maxDist: maxDist,
		blocks:  blocks,
		fps:     make(map[string][]simhash.Fingerprint),
	}
}

// MaxDist returns the distance bound the index answers queries at.
func (ix *HammingIndex) MaxDist() int { return ix.maxDist }

// Add indexes id under the fingerprint. Ids need not be unique; a query
// matching an id added multiple times still returns it once, and
// [HammingIndex.Remove] removes all of its entries.
func (ix *HammingIndex) Add(id string, fp simhash.Fingerprint) {
	for i := range ix.blocks {
		b := &ix.blocks[i]
		key := b.key(fp)
		b.table[key] = append(b.table[key], hammingEntry{fp: fp, id: id})
	}
	ix.fps[id] = append(ix.fps[id], fp)
}

// Remove deletes every entry for id — from all its Adds — and reports
// whether the id was present.
func (ix *HammingIndex) Remove(id string) bool {
	fps, ok := ix.fps[id]
	if !ok {
		return false
	}
	for _, fp := range fps {
		for i := range ix.blocks {
			b := &ix.blocks[i]
			key := b.key(fp)
			bucket := b.table[key]
			kept := bucket[:0]
			for _, e := range bucket {
				if e.id != id {
					kept = append(kept, e)
				}
			}
			if len(kept) == 0 {
				delete(b.table, key)
			} else {
				b.table[key] = kept
			}
		}
	}
	delete(ix.fps, id)
	return true
}

// Len returns the number of distinct ids currently indexed.
func (ix *HammingIndex) Len() int { return len(ix.fps) }

// Stats reports block-table occupancy; see [Stats]. Costs a full walk of
// the tables — for monitoring cadence, not per-operation use.
func (ix *HammingIndex) Stats() Stats {
	st := Stats{IDs: len(ix.fps)}
	for i := range ix.blocks {
		for _, bucket := range ix.blocks[i].table {
			st.Buckets++
			st.Entries += len(bucket)
			st.MaxBucket = max(st.MaxBucket, len(bucket))
		}
	}
	return st
}

// Range returns an iterator over the (id, fingerprint) pairs in the index
// — ids added multiple times yield once per Add — in unspecified order.
// The index must not be modified during iteration. Unlike [Index], the
// HammingIndex stores its fingerprints, so Range alone suffices to rebuild
// or migrate one.
func (ix *HammingIndex) Range() iter.Seq2[string, simhash.Fingerprint] {
	return func(yield func(string, simhash.Fingerprint) bool) {
		for id, fps := range ix.fps {
			for _, fp := range fps {
				if !yield(id, fp) {
					return
				}
			}
		}
	}
}

// Query returns the ids of fingerprints within maxDist of fp, deduplicated,
// in deterministic order (first matching block, then insertion order).
// Matches are verified: there are no false positives, and pigeonhole over
// the blocks guarantees no false negatives. Returns nil if there are none.
func (ix *HammingIndex) Query(fp simhash.Fingerprint) []string {
	var out []string
	seen := make(map[string]bool)
	for i := range ix.blocks {
		b := &ix.blocks[i]
		for _, e := range b.table[b.key(fp)] {
			if simhash.Distance(e.fp, fp) <= ix.maxDist && !seen[e.id] {
				seen[e.id] = true
				out = append(out, e.id)
			}
		}
	}
	return out
}

func (b *block) key(fp simhash.Fingerprint) uint64 {
	return uint64(fp) >> b.shift & (1<<b.width - 1)
}

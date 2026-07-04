package lsh

import (
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
	return &HammingIndex{maxDist: maxDist, blocks: blocks}
}

// MaxDist returns the distance bound the index answers queries at.
func (ix *HammingIndex) MaxDist() int { return ix.maxDist }

// Add indexes id under the fingerprint. Ids need not be unique; a query
// matching an id added multiple times still returns it once.
func (ix *HammingIndex) Add(id string, fp simhash.Fingerprint) {
	for i := range ix.blocks {
		b := &ix.blocks[i]
		key := b.key(fp)
		b.table[key] = append(b.table[key], hammingEntry{fp: fp, id: id})
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

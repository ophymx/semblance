package lsh

import (
	"cmp"
	"iter"
	"maps"
	"slices"
	"sort"

	"github.com/ophymx/semblance/minhash"
)

// Forest is a MinHash LSH Forest (Bawa, Condie & Ganesan 2005): top-k
// similarity retrieval without a fixed threshold. Where [Index] answers
// "everything probably above ~0.71", a Forest answers "the k most similar,
// whatever their similarity" — the natural shape for retrieval tools.
//
// A signature of length trees*depth is split into trees slices of depth
// values; each tree keeps its slice as a sorted key. Queries descend from
// the longest shared key prefix: the more leading values two signatures
// share in a tree, the higher their estimated Jaccard similarity, so
// candidates found at deeper prefixes rank first.
//
// Results are candidates ordered by a similarity proxy (maximum shared
// prefix depth), not exact ranking — re-rank the returned ids with
// [minhash.JaccardMany] when true order matters. Not safe for concurrent
// use.
type Forest struct {
	trees, depth int
	entries      [][]forestEntry // per tree; sorted when !dirty
	dirty        bool
	adds         map[string]int // id → number of Adds
	seq          int
}

type forestEntry struct {
	key []uint64 // this tree's depth signature values
	seq int
	id  string
}

// NewForest returns an empty forest for signatures of length trees*depth.
// More trees improve recall; more depth sharpens ranking. For k=128
// signatures, trees=8 and depth=16 is a reasonable default. Panics if
// trees <= 0 or depth <= 0.
func NewForest(trees, depth int) *Forest {
	if trees <= 0 || depth <= 0 {
		panic("lsh: trees and depth must be positive")
	}
	return &Forest{
		trees:   trees,
		depth:   depth,
		entries: make([][]forestEntry, trees),
		adds:    make(map[string]int),
	}
}

// Add indexes id under the signature. Ids need not be unique: adding the
// same id again is allowed, a query still returns it once, and
// [Forest.Remove] removes all of its entries. Adds are O(1); the next
// Query pays one sort per tree (lazy indexing, amortized over bulk
// loads). Panics if len(sig) != trees*depth.
func (f *Forest) Add(id string, sig minhash.Signature) {
	f.check(sig)
	for t := range f.entries {
		key := slices.Clone(sig[t*f.depth : (t+1)*f.depth])
		f.entries[t] = append(f.entries[t], forestEntry{key: key, seq: f.seq, id: id})
	}
	f.seq++
	f.adds[id]++
	f.dirty = true
}

// Query returns up to k candidate ids, ordered by decreasing maximum
// shared prefix depth (a similarity proxy; deeper agreement means higher
// estimated Jaccard), deduplicated, deterministically. Ids sharing no
// leading value with the query in any tree are never returned, so fewer
// than k results — or none — is possible. Panics if k <= 0 or
// len(sig) != trees*depth.
func (f *Forest) Query(sig minhash.Signature, k int) []string {
	f.check(sig)
	if k <= 0 {
		panic("lsh: k must be positive")
	}
	f.sortIfDirty()
	var out []string
	seen := make(map[string]bool)
	for d := f.depth; d >= 1 && len(out) < k; d-- {
		for t := 0; t < f.trees && len(out) < k; t++ {
			es := f.entries[t]
			qk := sig[t*f.depth : t*f.depth+d]
			lo := sort.Search(len(es), func(i int) bool { return slices.Compare(es[i].key[:d], qk) >= 0 })
			hi := lo + sort.Search(len(es)-lo, func(i int) bool { return slices.Compare(es[lo+i].key[:d], qk) > 0 })
			for _, e := range es[lo:hi] {
				if !seen[e.id] {
					seen[e.id] = true
					out = append(out, e.id)
					if len(out) == k {
						break
					}
				}
			}
		}
	}
	return out
}

// Remove deletes every entry for id — from all its Adds — and reports
// whether the id was present. Cost is proportional to the forest size.
func (f *Forest) Remove(id string) bool {
	if _, ok := f.adds[id]; !ok {
		return false
	}
	delete(f.adds, id)
	for t := range f.entries {
		kept := f.entries[t][:0]
		for _, e := range f.entries[t] {
			if e.id != id {
				kept = append(kept, e)
			}
		}
		f.entries[t] = kept
	}
	return true
}

// Len returns the number of distinct ids currently indexed.
func (f *Forest) Len() int { return len(f.adds) }

// Range returns an iterator over the distinct ids in the forest, in
// unspecified order. The forest must not be modified during iteration.
// Like [Index], a Forest does not store signatures; rebuilding requires
// the caller's signature store.
func (f *Forest) Range() iter.Seq[string] {
	return maps.Keys(f.adds)
}

func (f *Forest) check(sig minhash.Signature) {
	if len(sig) != f.trees*f.depth {
		panic("lsh: signature length does not match trees*depth")
	}
}

func (f *Forest) sortIfDirty() {
	if !f.dirty {
		return
	}
	for t := range f.entries {
		slices.SortFunc(f.entries[t], func(a, b forestEntry) int {
			if c := slices.Compare(a.key, b.key); c != 0 {
				return c
			}
			return cmp.Compare(a.seq, b.seq)
		})
	}
	f.dirty = false
}

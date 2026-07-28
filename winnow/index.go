package winnow

import (
	"cmp"
	"iter"
	"maps"
	"slices"
)

// Index maps winnowing fingerprints to the documents containing them, for
// fragment-level provenance: given a document, find which indexed
// documents share passages with it and — unlike a MinHash index — exactly
// where. Built for near-duplicate localization, quote tracing, and
// boilerplate detection across a corpus.
//
// All indexed documents must use the same (k, w) as the index. Query
// results are deterministic (no dependence on map iteration order). Not
// safe for concurrent use.
type Index struct {
	k, w     int
	postings map[uint64][]posting
	hashes   map[string][]uint64 // id → fingerprint hashes it contributed
}

type posting struct {
	id  string
	pos int
}

// MaxResults caps the number of position pairs [Index.Matches] and
// [Overlaps] return. Both enumerate the cross product of shared
// fingerprint positions, which is quadratic in the worst case: two highly
// repetitive texts (long runs of one byte or token) yield a fingerprint at
// nearly every position with the same hash, so every query position pairs
// with every document position. The cap bounds the work and the output at
// a generous ceiling — far above any legitimate document pair — turning
// the pathological case into a signal ("overlap saturated the cap") rather
// than an out-of-memory. Results are truncated in scan order (query
// position, then document position), so truncation is deterministic;
// reaching the cap means the two texts overlap at least this much. Use
// [Index.Overlap] for a bounded per-document summary instead.
const MaxResults = 1 << 20

// Match is one fingerprint shared between a query and an indexed document:
// the same k-gram appears at QueryPos in the query and DocPos in document
// ID. Runs of matches with a constant DocPos−QueryPos offset localize a
// contiguous shared passage.
type Match struct {
	ID       string // id of the indexed document
	Hash     uint64 // the shared fingerprint hash
	QueryPos int    // byte offset of the k-gram in the query
	DocPos   int    // byte offset of the k-gram in document ID
}

// Overlap summarizes how much a query shares with one document: Shared is
// the number of distinct fingerprints they have in common.
type Overlap struct {
	ID     string // id of the indexed document
	Shared int    // number of distinct fingerprints shared with the query
}

// NewIndex returns an empty index over byte k-grams with winnowing window
// w — the same parameters every [Index.Add] and query will use. Larger w
// means fewer fingerprints (less memory, coarser localization); the match
// guarantee covers shared runs of w+k−1 bytes. Panics if k <= 0 or w <= 0.
func NewIndex(k, w int) *Index {
	if k <= 0 || w <= 0 {
		panic("winnow: k and w must be positive")
	}
	return &Index{
		k:        k,
		w:        w,
		postings: make(map[uint64][]posting),
		hashes:   make(map[string][]uint64),
	}
}

// Add indexes text under id. Ids need not be unique; a repeated id
// accumulates fingerprints and [Index.Remove] drops all of them. Per-id
// fingerprint hashes are retained to support Remove, roughly doubling the
// per-document memory. Panics if id is empty.
func (ix *Index) Add(id, text string) {
	ix.add(id, Text(text, ix.k, ix.w))
}

// AddBytes is [Index.Add] for a byte slice, without copying. The slice is
// not retained or mutated.
func (ix *Index) AddBytes(id string, b []byte) {
	ix.add(id, TextBytes(b, ix.k, ix.w))
}

func (ix *Index) add(id string, fps iter.Seq[Fingerprint]) {
	if id == "" {
		panic("winnow: id must not be empty")
	}
	hs := ix.hashes[id]
	for fp := range fps {
		ix.postings[fp.Hash] = append(ix.postings[fp.Hash], posting{id: id, pos: fp.Pos})
		hs = append(hs, fp.Hash)
	}
	ix.hashes[id] = hs
}

// Overlap returns, for every indexed document sharing at least one
// fingerprint with text, the count of distinct shared fingerprints,
// ranked by Shared descending then id ascending. It answers "which
// documents does this overlap, and how much" — threshold Shared, or divide
// by the query's fingerprint count for a containment-style fraction.
func (ix *Index) Overlap(text string) []Overlap {
	counts := make(map[string]int)
	qseen := make(map[uint64]bool)
	for fp := range Text(text, ix.k, ix.w) {
		if qseen[fp.Hash] {
			continue
		}
		qseen[fp.Hash] = true
		var idSeen map[string]bool
		for _, p := range ix.postings[fp.Hash] {
			if idSeen[p.id] {
				continue
			}
			if idSeen == nil {
				idSeen = make(map[string]bool)
			}
			idSeen[p.id] = true
			counts[p.id]++
		}
	}
	if len(counts) == 0 {
		return nil
	}
	out := make([]Overlap, 0, len(counts))
	for id, n := range counts {
		out = append(out, Overlap{ID: id, Shared: n})
	}
	slices.SortFunc(out, func(a, b Overlap) int {
		if a.Shared != b.Shared {
			return cmp.Compare(b.Shared, a.Shared)
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return out
}

// Matches returns every fingerprint text shares with an indexed document,
// as (query position, document position) pairs — the raw material for
// localizing shared passages. Results are sorted by document id, then
// query position, then document position. A query fingerprint appearing in
// several documents (or several times in one) yields one Match each. At
// most [MaxResults] pairs are returned (see there); reaching the cap means
// the overlap is at least that large.
func (ix *Index) Matches(text string) []Match {
	var out []Match
outer:
	for fp := range Text(text, ix.k, ix.w) {
		for _, p := range ix.postings[fp.Hash] {
			if len(out) == MaxResults {
				break outer
			}
			out = append(out, Match{ID: p.id, Hash: fp.Hash, QueryPos: fp.Pos, DocPos: p.pos})
		}
	}
	slices.SortFunc(out, func(a, b Match) int {
		if a.ID != b.ID {
			return cmp.Compare(a.ID, b.ID)
		}
		if a.QueryPos != b.QueryPos {
			return cmp.Compare(a.QueryPos, b.QueryPos)
		}
		return cmp.Compare(a.DocPos, b.DocPos)
	})
	return out
}

// Remove deletes every fingerprint for id and reports whether it was
// present. Cost is proportional to id's fingerprint count plus the sizes
// of the touched posting lists.
func (ix *Index) Remove(id string) bool {
	hs, ok := ix.hashes[id]
	if !ok {
		return false
	}
	delete(ix.hashes, id)
	seen := make(map[uint64]bool, len(hs))
	for _, h := range hs {
		if seen[h] {
			continue
		}
		seen[h] = true
		bucket := ix.postings[h]
		kept := bucket[:0]
		for _, p := range bucket {
			if p.id != id {
				kept = append(kept, p)
			}
		}
		if len(kept) == 0 {
			delete(ix.postings, h)
		} else {
			ix.postings[h] = kept
		}
	}
	return true
}

// Len returns the number of distinct ids indexed.
func (ix *Index) Len() int { return len(ix.hashes) }

// Range returns an iterator over the indexed ids, in unspecified order.
// The index must not be modified during iteration.
func (ix *Index) Range() iter.Seq[string] { return maps.Keys(ix.hashes) }

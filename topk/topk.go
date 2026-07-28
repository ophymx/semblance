// Package topk implements the SpaceSaving frequent-items sketch (Metwally,
// Agrawal & El Abbadi 2005): track the heaviest items of a stream in fixed
// space, with per-item error bounds.
//
// A sketch with capacity c observing n total occurrences guarantees:
//
//   - every item whose true count exceeds n/c is present;
//   - for every reported item, Count is an upper bound on the true count
//     and Count-Err a lower bound (Err <= n/c).
//
// An entry whose lower bound clears a threshold is a guaranteed heavy
// hitter, not a candidate. Typical use: flood and burst detection — feed
// subject lines, source ids, shingle hashes, or SimHash fingerprints and
// read back the most frequent with bounds.
//
// Unlike the other sketches in this module, the resulting counters depend
// on stream order (an inherent SpaceSaving property — order decides which
// light items occupy counters); the guarantees above hold for any order,
// and identical streams produce identical sketches on every platform.
// Items are held in the sketch, so there is no generic serialization;
// persist Top output instead. Not safe for concurrent use.
package topk

import "slices"

// Entry is a reported item: its true count lies in [Count-Err, Count].
type Entry[T comparable] struct {
	Item  T
	Count uint64 // estimated count, an upper bound on the true count
	Err   uint64 // maximum overestimate: true count >= Count-Err
}

// Sketch is a SpaceSaving summary with a fixed number of counters. The
// zero value is not usable; use [New].
type Sketch[T comparable] struct {
	cap  int
	heap []counter[T] // min-heap by (count, seq)
	pos  map[T]int    // item → heap index
	n    uint64       // total observations
	seq  int          // adoption tiebreaker
}

type counter[T comparable] struct {
	item  T
	count uint64
	err   uint64
	seq   int
}

// New returns a sketch tracking up to capacity items. Larger capacity
// tightens the error bound (Err <= n/capacity). Panics if capacity <= 0.
func New[T comparable](capacity int) *Sketch[T] {
	if capacity <= 0 {
		panic("topk: capacity must be positive")
	}
	return &Sketch[T]{cap: capacity, pos: make(map[T]int, capacity)}
}

// Add observes one occurrence of item.
func (s *Sketch[T]) Add(item T) { s.AddN(item, 1) }

// AddN observes n occurrences of item at once (n = 0 is a no-op). Useful
// for weighted streams, e.g. bytes per source.
func (s *Sketch[T]) AddN(item T, n uint64) {
	if n == 0 {
		return
	}
	s.n += n
	if i, ok := s.pos[item]; ok {
		s.heap[i].count += n
		s.siftDown(i)
		return
	}
	if len(s.heap) < s.cap {
		s.heap = append(s.heap, counter[T]{item: item, count: n, seq: s.seq})
		s.pos[item] = len(s.heap) - 1
		s.seq++
		s.siftUp(len(s.heap) - 1)
		return
	}
	// Evict the minimum counter: the newcomer inherits its count as
	// error, keeping Count an upper bound.
	min := &s.heap[0]
	delete(s.pos, min.item)
	s.pos[item] = 0
	min.item, min.err, min.count = item, min.count, min.count+n
	min.seq = s.seq
	s.seq++
	s.siftDown(0)
}

// Count returns the upper-bound count and error for item: the true count
// lies in [count-err, count]. For an item not currently tracked, both are
// the sketch's minimum counter value (the most it could have occurred
// unseen), or zero while the sketch is below capacity.
func (s *Sketch[T]) Count(item T) (count, err uint64) {
	if i, ok := s.pos[item]; ok {
		return s.heap[i].count, s.heap[i].err
	}
	m := s.minCount()
	return m, m
}

func (s *Sketch[T]) minCount() uint64 {
	if len(s.heap) < s.cap {
		return 0
	}
	return s.heap[0].count
}

// Top returns up to k entries by descending Count (ties by adoption
// order), deterministically. Pass k >= Len for all tracked items.
// Panics if k <= 0.
func (s *Sketch[T]) Top(k int) []Entry[T] {
	if k <= 0 {
		panic("topk: k must be positive")
	}
	sorted := slices.Clone(s.heap)
	slices.SortFunc(sorted, func(a, b counter[T]) int {
		if a.count != b.count {
			if a.count > b.count {
				return -1
			}
			return 1
		}
		return a.seq - b.seq
	})
	k = min(k, len(sorted))
	out := make([]Entry[T], k)
	for i, c := range sorted[:k] {
		out[i] = Entry[T]{Item: c.item, Count: c.count, Err: c.err}
	}
	return out
}

// N returns the total number of observations.
func (s *Sketch[T]) N() uint64 { return s.n }

// Len returns the number of items currently tracked (at most capacity).
func (s *Sketch[T]) Len() int { return len(s.heap) }

// Merge folds other into s, summarizing the concatenation of both
// streams; other is unchanged, and s keeps its own capacity. Bounds are
// preserved conservatively: an item absent from one sketch is charged
// that sketch's minimum counter as both count and error, so Count remains
// an upper bound and Count-Err a lower bound; Err bounds loosen to about
// the sum of the two sketches' n/capacity.
func (s *Sketch[T]) Merge(other *Sketch[T]) {
	minS, minO := s.minCount(), other.minCount()
	merged := make(map[T]Entry[T], len(s.heap)+len(other.heap))
	for _, c := range s.heap {
		if j, ok := other.pos[c.item]; ok {
			oc := other.heap[j]
			merged[c.item] = Entry[T]{Item: c.item, Count: c.count + oc.count, Err: c.err + oc.err}
		} else {
			merged[c.item] = Entry[T]{Item: c.item, Count: c.count + minO, Err: c.err + minO}
		}
	}
	for _, c := range other.heap {
		if _, ok := s.pos[c.item]; !ok {
			merged[c.item] = Entry[T]{Item: c.item, Count: c.count + minS, Err: c.err + minS}
		}
	}

	// Rebuild, keeping the heaviest s.cap items. Deterministic order:
	// existing entries by current adoption order, then other's.
	order := make([]T, 0, len(merged))
	for _, c := range s.heap {
		order = append(order, c.item)
	}
	for _, c := range other.heap {
		if _, dup := s.pos[c.item]; !dup {
			order = append(order, c.item)
		}
	}
	entries := make([]Entry[T], 0, len(order))
	for _, item := range order {
		entries = append(entries, merged[item])
	}
	slices.SortStableFunc(entries, func(a, b Entry[T]) int {
		switch {
		case a.Count > b.Count:
			return -1
		case a.Count < b.Count:
			return 1
		}
		return 0
	})

	s.n += other.n
	s.heap = s.heap[:0]
	clear(s.pos)
	s.seq = 0
	for _, e := range entries[:min(s.cap, len(entries))] {
		s.heap = append(s.heap, counter[T]{item: e.Item, count: e.Count, err: e.Err, seq: s.seq})
		s.pos[e.Item] = len(s.heap) - 1
		s.seq++
		s.siftUp(len(s.heap) - 1)
	}
}

// Reset empties the sketch for reuse.
func (s *Sketch[T]) Reset() {
	s.heap = s.heap[:0]
	clear(s.pos)
	s.n = 0
	s.seq = 0
}

// less orders the min-heap by (count, adoption seq): the evicted counter
// is always the smallest and, among equals, the oldest — deterministic.
func (s *Sketch[T]) less(i, j int) bool {
	a, b := &s.heap[i], &s.heap[j]
	if a.count != b.count {
		return a.count < b.count
	}
	return a.seq < b.seq
}

func (s *Sketch[T]) swap(i, j int) {
	s.heap[i], s.heap[j] = s.heap[j], s.heap[i]
	s.pos[s.heap[i].item] = i
	s.pos[s.heap[j].item] = j
}

func (s *Sketch[T]) siftUp(i int) {
	for i > 0 {
		parent := (i - 1) / 2
		if !s.less(i, parent) {
			return
		}
		s.swap(i, parent)
		i = parent
	}
}

func (s *Sketch[T]) siftDown(i int) {
	for {
		least := i
		if l := 2*i + 1; l < len(s.heap) && s.less(l, least) {
			least = l
		}
		if r := 2*i + 2; r < len(s.heap) && s.less(r, least) {
			least = r
		}
		if least == i {
			return
		}
		s.swap(i, least)
		i = least
	}
}

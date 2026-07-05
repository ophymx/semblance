// Package sample provides deterministic reservoir sampling (Vitter's
// Algorithm R): a uniform random sample of up to k items from a stream of
// unknown length, in fixed space.
//
// After n items, every item has probability k/n of being in the sample
// (uniform up to a 2^-64 range-reduction bias). Sampling is deterministic
// given the seed and the stream order — the same stream and seed produce
// the same sample on every platform — and, like package topk, depends on
// stream order by nature. The reservoir is valid at any point: Sample may
// be read mid-stream and adding may continue afterward.
//
// Typical use: pick k representative documents from a cluster or group
// for triage, reproducibly.
package sample

import (
	"math/bits"
	"slices"

	"github.com/ophymx/semblance/internal/hashutil"
)

// Reservoir holds a uniform sample of up to k items. The zero value is
// not usable; use [New]. Not safe for concurrent use.
type Reservoir[T any] struct {
	rng   hashutil.SplitMix64
	seed  uint64
	items []T
	k     int
	n     uint64
}

// New returns an empty reservoir sampling up to k items, driven by seed.
// Panics if k <= 0.
func New[T any](k int, seed uint64) *Reservoir[T] {
	if k <= 0 {
		panic("sample: k must be positive")
	}
	return &Reservoir[T]{rng: hashutil.SplitMix64(seed), seed: seed, k: k}
}

// Add observes one item.
func (r *Reservoir[T]) Add(item T) {
	r.n++
	if len(r.items) < r.k {
		r.items = append(r.items, item)
		return
	}
	// Replace a random slot with probability k/n: draw j uniform in
	// [0, n) via Lemire range reduction; j < k selects the slot.
	j, _ := bits.Mul64(r.rng.Next(), r.n)
	if j < uint64(r.k) {
		r.items[j] = item
	}
}

// Sample returns a copy of the current sample: min(k, n) items in
// deterministic (reservoir slot) order.
func (r *Reservoir[T]) Sample() []T {
	return slices.Clone(r.items)
}

// N returns the number of items observed.
func (r *Reservoir[T]) N() uint64 { return r.n }

// Len returns the current sample size, min(k, n).
func (r *Reservoir[T]) Len() int { return len(r.items) }

// Reset empties the reservoir and restarts its random stream from the
// original seed, so identical input streams resample identically.
func (r *Reservoir[T]) Reset() {
	r.items = r.items[:0]
	r.n = 0
	r.rng = hashutil.SplitMix64(r.seed)
}

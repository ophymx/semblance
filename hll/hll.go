// Package hll implements HyperLogLog cardinality sketches (Flajolet, Fusy,
// Gandouet & Meunier 2007): estimate the number of distinct 64-bit
// elements in a stream using 2^p one-byte registers.
//
// Standard error is about 1.04/sqrt(2^p) — p=14 (16 KiB) gives ~0.8%.
// Sketches are mergeable ([Sketch.Merge] computes the sketch of the union
// exactly, like minhash.Union), order-independent, duplicate-insensitive,
// and deterministic: the same multiset of elements produces the same
// registers on every platform.
//
// This is the classic dense estimator with 64-bit hashing (linear counting
// for the small range; no large-range correction is needed at 64 bits) —
// not HLL++: there is no sparse mode and no empirical bias tables, so
// expect the estimator's mild bias hump (~1–2%) near n ≈ 2.5·2^p.
// Elements are scrambled internally with a frozen mixer, so arbitrary
// identifiers work, not only pre-hashed values; feeding the library's
// shingle streams counts distinct shingles.
package hll

import (
	"iter"
	"math"
	"math/bits"

	"github.com/ophymx/semblance/internal/hashutil"
)

const (
	minP = 4
	maxP = 18
)

// Sketch is a dense HyperLogLog with 2^p registers. The zero value is not
// usable; use [New].
type Sketch struct {
	p    uint8
	regs []uint8
}

// New returns an empty sketch with 2^p registers (2^p bytes). Standard
// error of estimates is about 1.04/sqrt(2^p). Panics if p is outside
// [4, 18].
func New(p int) *Sketch {
	if p < minP || p > maxP {
		panic("hll: p must be in [4, 18]")
	}
	return &Sketch{p: uint8(p), regs: make([]uint8, 1<<p)}
}

// P returns the sketch's precision parameter.
func (s *Sketch) P() int { return int(s.p) }

// Add observes an element. Duplicates never change the sketch.
func (s *Sketch) Add(x uint64) {
	x = hashutil.Mix64(x)
	idx := x >> (64 - s.p)
	rho := uint8(bits.LeadingZeros64(x<<s.p) + 1)
	if maxRho := 64 - s.p + 1; rho > maxRho {
		rho = maxRho
	}
	if rho > s.regs[idx] {
		s.regs[idx] = rho
	}
}

// AddSeq observes every element of a sequence, e.g. a shingle stream.
func (s *Sketch) AddSeq(elems iter.Seq[uint64]) {
	for x := range elems {
		s.Add(x)
	}
}

// Estimate returns the estimated number of distinct elements observed.
func (s *Sketch) Estimate() float64 {
	m := float64(len(s.regs))
	sum := 0.0
	zeros := 0
	for _, r := range s.regs {
		sum += math.Ldexp(1, -int(r))
		if r == 0 {
			zeros++
		}
	}
	if est := alpha(len(s.regs)) * m * m / sum; est > 2.5*m || zeros == 0 {
		return est
	}
	return m * math.Log(m/float64(zeros)) // linear counting, small range
}

// Merge folds other into s, making s the sketch of the union of both
// element streams — exact, not an estimate: merging equals having added
// both streams to one sketch. other is unchanged. Panics if the
// precisions differ.
func (s *Sketch) Merge(other *Sketch) {
	if s.p != other.p {
		panic("hll: precision mismatch")
	}
	for i, r := range other.regs {
		if r > s.regs[i] {
			s.regs[i] = r
		}
	}
}

// Reset empties the sketch for reuse.
func (s *Sketch) Reset() {
	clear(s.regs)
}

func alpha(m int) float64 {
	switch m {
	case 16:
		return 0.673
	case 32:
		return 0.697
	case 64:
		return 0.709
	}
	return 0.7213 / (1 + 1.079/float64(m))
}

// Package minhash implements MinHash signatures (Broder 1997) for
// estimating the Jaccard similarity of sets from small fixed-size sketches.
//
// A [MinHasher] applies k independent permutations of the 64-bit hash space
// and keeps, per permutation, the minimum permuted value seen — yielding a
// [Signature] of k words. For two sets with Jaccard similarity J, each
// signature slot matches with probability J, so [Jaccard] estimates J with
// standard error about sqrt(J(1-J)/k) <= 1/(2*sqrt(k)); k=128 gives a
// worst-case standard error of about 0.044. Sketching costs O(n*k) for n
// input hashes; comparison costs O(k).
//
// Signatures are only comparable when produced with equal k and equal seed.
// A length mismatch is detected ([Jaccard] panics); a seed mismatch is NOT
// detectable in memory — comparing signatures from different seeds silently
// yields garbage. Persist the parameters alongside stored signatures.
//
// Everything is deterministic: same input, k, and seed produce the same
// signature on every platform, in every process.
package minhash

import (
	"iter"
	"math"

	"github.com/ophymx/semblance/internal/hashutil"
)

// Signature is a MinHash sketch: k minimum permuted values. Signatures with
// no elements ever sketched into them consist entirely of math.MaxUint64
// (the "empty-set signature"); note Jaccard of two empty-set signatures is
// therefore 1.
type Signature []uint64

// MinHasher sketches streams of element hashes into signatures of length k.
// It applies k permutations of the form a*x + b (mod 2^64) with odd a — each
// a bijection of the 64-bit space — with parameters derived from the seed
// via a SplitMix64 stream. The permutation family is frozen: it will not
// change within a major version.
//
// A MinHasher is immutable after New and safe for concurrent use, provided
// concurrent SketchInto calls use distinct dst buffers.
type MinHasher struct {
	seed uint64
	a, b []uint64
}

// New returns a MinHasher producing signatures of length k from the given
// seed. Panics if k <= 0 or k > 65535 (the serialization format stores k
// in 16 bits; practical k is a few hundred at most).
func New(k int, seed uint64) *MinHasher {
	if k <= 0 || k > math.MaxUint16 {
		panic("minhash: k must be in [1, 65535]")
	}
	rng := hashutil.SplitMix64(seed)
	a := make([]uint64, k)
	b := make([]uint64, k)
	for i := range a {
		a[i] = rng.Next() | 1 // odd => a*x+b is a bijection mod 2^64
		b[i] = rng.Next()
	}
	return &MinHasher{seed: seed, a: a, b: b}
}

// K returns the signature length.
func (m *MinHasher) K() int { return len(m.a) }

// Seed returns the seed the MinHasher was created with.
func (m *MinHasher) Seed() uint64 { return m.seed }

// Sketch consumes the element hashes and returns a new signature.
func (m *MinHasher) Sketch(hashes iter.Seq[uint64]) Signature {
	dst := make(Signature, len(m.a))
	m.SketchInto(dst, hashes)
	return dst
}

// SketchInto consumes the element hashes into dst, overwriting it. It is
// the low-allocation path: reusing dst across documents avoids the
// per-document signature allocation of [MinHasher.Sketch]. Panics if
// len(dst) != k.
func (m *MinHasher) SketchInto(dst Signature, hashes iter.Seq[uint64]) {
	m.Reset(dst)
	// Element hashes are buffered into blocks so the kernel can run over
	// contiguous data (see sketchBlock).
	var buf [sketchBlockSize]uint64
	n := 0
	for x := range hashes {
		buf[n] = x
		n++
		if n == sketchBlockSize {
			m.Update(dst, buf[:])
			n = 0
		}
	}
	if n > 0 {
		m.Update(dst, buf[:n])
	}
}

// Reset initializes dst to the empty-set signature (all math.MaxUint64),
// ready for [MinHasher.Update]. Panics if len(dst) != k.
func (m *MinHasher) Reset(dst Signature) {
	if len(dst) != len(m.a) {
		panic("minhash: dst length does not match k")
	}
	for i := range dst {
		dst[i] = math.MaxUint64
	}
}

// Update folds a batch of element hashes into dst, which must have been
// initialized by [MinHasher.Reset] (or hold a previous sketch to merge
// into: updating is the same element-wise minimum that [Union] computes).
// Reset + any partitioning of a document's hashes into Update calls
// produces the identical signature to [MinHasher.Sketch]. Panics if
// len(dst) != k.
func (m *MinHasher) Update(dst Signature, hashes []uint64) {
	if len(dst) != len(m.a) {
		panic("minhash: dst length does not match k")
	}
	if len(hashes) == 0 {
		return
	}
	sketchBlock(dst, m.a, m.b, hashes)
}

// Jaccard estimates the Jaccard similarity of the sets a and b were
// sketched from, as the fraction of matching slots. Standard error is about
// sqrt(J(1-J)/len(a)). Panics if the signatures differ in length or are
// empty. Signatures sketched with different seeds compare as garbage; this
// cannot be detected here.
func Jaccard(a, b Signature) float64 {
	if len(a) != len(b) {
		panic("minhash: signature length mismatch")
	}
	if len(a) == 0 {
		panic("minhash: empty signature")
	}
	return float64(eqCount(a, b)) / float64(len(a))
}

// eqCountGeneric counts positions where a and b match; the portable kernel
// behind [Jaccard].
func eqCountGeneric(a, b []uint64) int {
	b = b[:len(a)]
	eq := 0
	for i := range a {
		if a[i] == b[i] {
			eq++
		}
	}
	return eq
}

// Union writes into dst the signature of the union of the sets a and b were
// sketched from: the element-wise minimum. This is exact, not an estimate:
// Union(dst, sketch(A), sketch(B)) equals sketch(A ∪ B). dst may alias a or
// b. Panics if the lengths differ.
func Union(dst, a, b Signature) {
	if len(dst) != len(a) || len(a) != len(b) {
		panic("minhash: signature length mismatch")
	}
	for i := range a {
		dst[i] = min(a[i], b[i])
	}
}

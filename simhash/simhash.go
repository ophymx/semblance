// Package simhash implements 64-bit SimHash fingerprints (Charikar 2002)
// for estimating the cosine similarity of weighted feature sets from a
// single machine word.
//
// For two feature vectors at angle θ, each fingerprint bit differs with
// probability θ/π, so the expected Hamming distance is 64·θ/π with standard
// deviation sqrt(64·p·(1-p)) <= 4 bits (p = θ/π). Cosine similarity can be
// estimated back from a distance d as cos(π·d/64). Sketching costs O(64·n)
// for n features; comparison is a single XOR and popcount.
//
// The bit rule is frozen: bit i of the fingerprint is 1 iff the weighted
// sum of feature-hash bits i is strictly positive (ties round to 0).
// Changing it would break stored fingerprints and is a major-version event.
//
// Everything is deterministic: same features and weights produce the same
// fingerprint on every platform, in every process. [SketchText] shares the
// unicode-tables determinism caveat of the shingle package.
package simhash

import (
	"iter"
	"math/bits"

	"github.com/ophymx/semblance/shingle"
)

// Fingerprint is a 64-bit SimHash. The zero Fingerprint is what sketching
// zero features (or features whose weights cancel exactly) produces; note
// Distance between two zero fingerprints is 0, i.e. empty inputs compare as
// identical.
type Fingerprint uint64

// Sketch computes the fingerprint of a weighted feature stream. Each
// (feature hash, weight) pair adds weight to a per-bit counter where the
// hash has a 1 bit and subtracts it where it has a 0 bit; fingerprint bits
// are the counter signs. The same feature emitted n times with weight 1 is
// identical to emitting it once with weight n. Zero-weight features are
// no-ops; negative weights subtract (a feature may cancel itself out).
func Sketch(weighted iter.Seq2[uint64, int]) Fingerprint {
	var counts [64]int
	for h, w := range weighted {
		for i := range counts {
			if h>>i&1 == 1 {
				counts[i] += w
			} else {
				counts[i] -= w
			}
		}
	}
	var fp Fingerprint
	for i, c := range counts {
		if c > 0 {
			fp |= 1 << i
		}
	}
	return fp
}

// SketchText fingerprints text using its overlapping w-word shingles as
// features, each with weight 1 (repeated shingles accumulate naturally).
// See [shingle.Words] for tokenization. Text with fewer than w tokens
// produces the zero Fingerprint. Panics if w <= 0.
func SketchText(text string, w int) Fingerprint {
	shingles := shingle.Words(text, w) // validate w eagerly
	return Sketch(func(yield func(uint64, int) bool) {
		for h := range shingles {
			if !yield(h, 1) {
				return
			}
		}
	})
}

// Distance returns the Hamming distance between two fingerprints: the
// number of differing bits, in [0, 64].
func Distance(a, b Fingerprint) int {
	return bits.OnesCount64(uint64(a ^ b))
}

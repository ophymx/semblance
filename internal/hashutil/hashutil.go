// Package hashutil provides the seeded parameter generation and hash mixing
// shared by the semblance packages.
//
// Everything in this package is frozen: signatures produced by semblance are
// meant to be stored and compared across processes and library versions, so
// changing any constant or formula here breaks stored signatures and is a
// major-version change.
package hashutil

import "math/bits"

// SplitMix64 is a deterministic 64-bit PRNG (Steele, Lea & Flood 2014) used
// to derive permutation parameters from a seed. The zero value is a valid
// stream seeded with 0.
type SplitMix64 uint64

// Next advances the stream and returns the next 64-bit value.
func (s *SplitMix64) Next() uint64 {
	*s += 0x9E3779B97F4A7C15
	z := uint64(*s)
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// MixInit is the initial accumulator value for Mix chains.
const MixInit uint64 = 0xC2B2AE3D27D4EB4F

// Mix folds hash h into accumulator acc and returns the new accumulator.
// Chains started from MixInit combine a sequence of hashes into one; the
// rotation makes the fold order-sensitive, so permuted sequences produce
// different results.
func Mix(acc, h uint64) uint64 {
	acc = bits.RotateLeft64(acc, 31)
	acc ^= h
	return acc * 0x9E3779B97F4A7C15
}

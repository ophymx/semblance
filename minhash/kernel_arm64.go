package minhash

// The NEON kernel processes four permutations per iteration (two 2-lane
// vector pairs), permutation-block-major like the AVX2 kernel. NEON has no
// 64-bit lane multiply: the low 64 bits of a[i]*x are synthesized from
// widening 32-bit multiplies (umull/umlal for the cross terms, shifted
// into place), and the unsigned 64-bit min uses cmhi + bit. NEON is
// baseline on arm64, so no build gate is needed.

// sketchBlockNEON requires len(dst) == len(a) == len(b),
// len(dst) % 4 == 0, len(block) % 2 == 0, and len(block) > 0.
//
//go:noescape
func sketchBlockNEON(dst, a, b, block []uint64)

func sketchBlock(dst, a, b []uint64, block []uint64) {
	if n := len(block) &^ 1; len(dst)%4 == 0 && n >= 8 {
		sketchBlockNEON(dst, a, b, block[:n])
		block = block[n:]
	}
	sketchBlockGeneric(dst, a, b, block)
}
